package webrtc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/okieraised/monitoring-agent/internal/common"
	"github.com/okieraised/monitoring-agent/internal/config"
	"github.com/okieraised/monitoring-agent/internal/constants"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/h264_encoder"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/log"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/mqtt_client"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/ros_node"
	"github.com/okieraised/monitoring-agent/internal/signaling"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/h264reader"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
)

const (
	qosExactlyOnce byte = 2
)

func recoverFn() {
	if rec := recover(); rec != nil {
		log.Default().Info(fmt.Sprintf("recovered from panic: %v: %s", rec, debug.Stack()))
	}
}

func getHTTPPort() int {
	port := viper.GetInt(config.AgentHTTPPort)
	if port <= 0 {
		return constants.AgentDefaultHTTPPort
	}
	return port
}

type WebRTCDaemon struct {
	id          uuid.UUID
	ctx         context.Context
	cancel      context.CancelFunc
	closeOnce   sync.Once
	wg          sync.WaitGroup
	mu          sync.RWMutex
	cMqtt       mqtt.Client
	cWebsocket  *signaling.WebsocketClient
	hub         *signaling.WebsocketHub
	broadcaster *topicBroadcaster
}

// NewWebRTCDaemon starts a webrtc daemon with MQTT or WebSocket
func NewWebRTCDaemon(parent context.Context) (*WebRTCDaemon, error) {
	log.Default().Info("Starting WebRTC streaming daemon")

	ctx, cancel := context.WithCancel(parent)

	d := &WebRTCDaemon{
		id:          uuid.New(),
		ctx:         ctx,
		cancel:      cancel,
		broadcaster: newTopicBroadcaster(),
	}

	if viper.GetBool(config.AgentEnableMQTT) {
		d.cMqtt = mqtt_client.Client()
	} else {
		d.hub = signaling.GetWebsocketHubInstance()
	}

	return d, nil
}

func (w *WebRTCDaemon) Start() error {
	ctx, cancel := context.WithCancel(w.ctx)
	defer cancel()
	defer recoverFn()

	// If using mqtt
	if viper.GetBool(config.AgentEnableMQTT) {
		token := w.cMqtt.Subscribe(viper.GetString(config.MqttWebRTCOfferTopic), qosExactlyOnce, w.mqttHandler)
		if token.WaitTimeout(5*time.Second) && token.Error() != nil {
			return token.Error()
		}
	} else { // Revert to websocket
		var conn *websocket.Conn
		var err error
		for {
			dialer := websocket.Dialer{}
			conn, _, err = dialer.Dial(fmt.Sprintf("ws://localhost:%d/ws", getHTTPPort()), nil)
			if err != nil {
				wErr := errors.Wrapf(err, "failed to connect to websocket hub")
				log.Default().Info(wErr.Error())
				time.Sleep(1 * time.Second)
				continue
			} else {
				log.Default().Info("Successfully connected WebRTC daemon to websocket hub")
				break
			}
		}

		client := signaling.NewWebsocketClient(w.id, conn, w.hub)
		client.SetMessageHandler(w.websocketHandler)
		w.hub.GetRegister() <- client
		go client.Read()
		w.cWebsocket = client
	}

	for {
		select {
		case <-ctx.Done():
			log.Default().Info("Stopping WebRTC streaming daemon")
			return nil
		}
	}
}

func (w *WebRTCDaemon) messageHandler(payload common.SignalingMessage) {
	if err := payload.ValidateWebRTC(); err != nil {
		log.Default().Error(err.Error())
		return
	}

	switch payload.Payload.Type {
	case constants.MsgTypeWebRTCInit:
		log.Default().Info("WebRTCInit received (no-op; subscriber starts on offer)")
	case constants.MsgTypeOffer:
		w.sdpExchangeHandler(payload)
	case constants.MsgTypeAnswer:
	default:
		log.Default().Error(fmt.Sprintf("invalid payload type: %s", payload.Payload.Type))
	}
}

// websocketHandler handles the websocket signaling messages
func (w *WebRTCDaemon) websocketHandler(payload common.SignalingMessage) {
	w.messageHandler(payload)
}

// mqttHandler handles the mqtt signaling messages
func (w *WebRTCDaemon) mqttHandler(client mqtt.Client, msg mqtt.Message) {
	var payload common.SignalingMessage
	err := json.Unmarshal(msg.Payload(), &payload)
	if err != nil {
		wErr := errors.Wrapf(err, "failed to unmarshal payload")
		log.Default().Error(wErr.Error())
		return
	}
	w.messageHandler(payload)
}

func (w *WebRTCDaemon) sdpExchangeHandler(offerPayload common.SignalingMessage) {
	go func() {
		ctx, cancel := context.WithCancel(w.ctx)
		defer cancel()
		defer recoverFn()
		topic := offerPayload.Payload.TopicName

		// Each viewer gets its own channel
		frameCh := make(chan []byte, 64)
		w.broadcaster.AddViewer(ctx, ros_node.Node(), topic, frameCh)
		defer func() {
			w.broadcaster.RemoveViewer(topic, frameCh)
			close(frameCh)
		}()

		// Create the PeerConnection
		iceServers := []webrtc.ICEServer{
			{
				URLs: []string{
					"stun:stun.l.google.com:19302",
				},
			},
		}

		pc, err := webrtc.NewPeerConnection(webrtc.Configuration{ICEServers: iceServers})
		if err != nil {
			log.Default().Error(errors.Wrap(err, "failed to create PeerConnection").Error())
			return
		}
		defer func() {
			if cerr := pc.Close(); cerr != nil {
				log.Default().Error(errors.Wrap(cerr, "failed to close PeerConnection").Error())
			}
		}()

		// ICE and state handlers
		iceConnectedCtx, iceConnectedCtxCancel := context.WithCancel(ctx)
		defer iceConnectedCtxCancel()

		pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
			log.Default().Info(fmt.Sprintf("ICE connection state: %s", state.String()))
			if state == webrtc.ICEConnectionStateConnected ||
				state == webrtc.ICEConnectionStateCompleted {
				iceConnectedCtxCancel()
			}
		})

		peerClosed := make(chan struct{})
		var closePeerClosed sync.Once
		pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
			log.Default().Info(fmt.Sprintf("Peer connection state: %s", s.String()))
			if s == webrtc.PeerConnectionStateFailed ||
				s == webrtc.PeerConnectionStateDisconnected ||
				s == webrtc.PeerConnectionStateClosed {
				closePeerClosed.Do(func() { close(peerClosed) })
			}
		})

		// Create track and add to peer
		sessionID := fmt.Sprintf(
			"video_%s_%s",
			strings.ReplaceAll(offerPayload.Payload.TransactionID.String(), "-", "_"),
			strings.ReplaceAll(topic, "/", "_"),
		)

		videoTrack, err := webrtc.NewTrackLocalStaticSample(
			webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
			sessionID,
			viper.GetString(config.AgentID),
		)
		if err != nil {
			log.Default().Error(errors.Wrap(err, "failed to create track").Error())
			return
		}

		rtpSender, err := pc.AddTrack(videoTrack)
		if err != nil {
			log.Default().Error(errors.Wrap(err, "failed to add track").Error())
			return
		}

		// RTCP reader (required for NACK/PLI feedback)
		go func() {
			buf := make([]byte, 1500)
			for {
				if _, _, rtcpErr := rtpSender.Read(buf); rtcpErr != nil {
					return
				}
			}
		}()

		// Encoding and streaming pipeline
		g, runCtx := errgroup.WithContext(ctx)

		g.Go(func() error {
			enc, out, encErr := h264_encoder.NewH264Encoder(runCtx, nil)
			if encErr != nil {
				return fmt.Errorf("failed to create H264 encoder: %w", encErr)
			}
			defer func() {
				if cErr := enc.Close(); cErr != nil {
					log.Default().Error(fmt.Sprintf("failed to close encoder: %v", cErr))
				}
			}()

			h264, hErr := h264reader.NewReader(out)
			if hErr != nil {
				return fmt.Errorf("failed to create H264 reader: %w", hErr)
			}

			// Feed JPEG/PNG frames into the encoder
			subCtx, subCancel := context.WithCancel(runCtx)
			defer subCancel()
			subG, subCtx := errgroup.WithContext(subCtx)

			subG.Go(func() error {
				for {
					select {
					case <-subCtx.Done():
						return subCtx.Err()
					case frame, ok := <-frameCh:
						if !ok {
							return nil
						}
						if err := enc.WriteFrame(frame); err != nil && !errors.Is(err, io.EOF) {
							return fmt.Errorf("failed to write frame: %w", err)
						}
					}
				}
			})

			// Wait for ICE connected
			select {
			case <-iceConnectedCtx.Done():
			case <-runCtx.Done():
				_ = subG.Wait()
				return runCtx.Err()
			}

			// Push encoded NAL units to WebRTC track
			for {
				select {
				case <-runCtx.Done():
					_ = subG.Wait()
					return runCtx.Err()
				default:
					nal, nErr := h264.NextNAL()
					if nErr != nil {
						if errors.Is(nErr, io.EOF) {
							log.Default().Info("H264 stream ended")
							_ = subG.Wait()
							return nil
						}
						_ = subG.Wait()
						return errors.Wrap(nErr, "failed to read H264 NAL")
					}

					if wErr := videoTrack.WriteSample(media.Sample{
						Data:     nal.Data,
						Duration: 33 * time.Millisecond,
					}); wErr != nil {
						_ = subG.Wait()
						return errors.Wrap(wErr, "failed to write WebRTC sample")
					}
				}
			}
		})

		// Decode remote SDP offer
		offer, err := w.decodeSessionDescription(offerPayload.Payload.SDP)
		if err != nil {
			log.Default().Error(errors.Wrap(err, "failed to decode session description").Error())
			return
		}
		if err := pc.SetRemoteDescription(offer); err != nil {
			log.Default().Error(errors.Wrap(err, "failed to set remote description").Error())
			return
		}

		answer, err := pc.CreateAnswer(nil)
		if err != nil {
			log.Default().Error(errors.Wrap(err, "failed to create answer").Error())
			return
		}

		gatherComplete := webrtc.GatheringCompletePromise(pc)
		err = pc.SetLocalDescription(answer)
		if err != nil {
			log.Default().Error(errors.Wrap(err, "failed to set local description").Error())
			return
		}
		<-gatherComplete

		// Send answer (WebSocket or MQTT)
		sdp, err := w.encodeSessionDescription(pc.LocalDescription())
		if err != nil {
			log.Default().Error(errors.Wrap(err, "failed to encode session description").Error())
			return
		}

		answerMsg := common.SignalingMessage{
			Header: offerPayload.Header,
			Payload: common.SignalingBody{
				TransactionID: offerPayload.Payload.TransactionID,
				Type:          constants.MsgTypeAnswer,
				TopicName:     topic,
				Session:       sessionID,
				SDP:           sdp,
			},
		}

		if viper.GetBool(config.AgentEnableMQTT) {
			b, _ := json.Marshal(answerMsg)
			token := w.cMqtt.Publish(viper.GetString(config.MqttWebRTCAnswerTopic), qosExactlyOnce, false, b)
			if token.WaitTimeout(5*time.Second) && token.Error() != nil {
				log.Default().Error(errors.Wrap(token.Error(), "failed to publish answer").Error())
				return
			}
		} else {
			if err := w.cWebsocket.WriteJSON(answerMsg); err != nil {
				log.Default().Error(errors.Wrap(err, "failed to send answer via websocket").Error())
				return
			}
		}

		// Wait for completion or shutdown
		done := make(chan error, 1)
		go func() { done <- g.Wait() }()

		select {
		case err := <-done:
			if err != nil {
				log.Default().Error(errors.Wrap(err, "pipeline error").Error())
			}
		case <-peerClosed:
			cancel()
			log.Default().Info(fmt.Sprintf("Peer closed for topic [%s]", topic))
		case <-ctx.Done():
			cancel()
			log.Default().Info(fmt.Sprintf("Context canceled for topic [%s]", topic))
		}
	}()
}

func (w *WebRTCDaemon) decodeSessionDescription(sdp string) (webrtc.SessionDescription, error) {
	offer := webrtc.SessionDescription{}

	b, err := base64.StdEncoding.DecodeString(sdp)
	if err != nil {
		return offer, err
	}
	if err = json.Unmarshal(b, &offer); err != nil {
		return offer, err
	}
	return offer, nil
}

func (w *WebRTCDaemon) encodeSessionDescription(sdp *webrtc.SessionDescription) (string, error) {
	b, err := json.Marshal(sdp)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal session description")
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func (w *WebRTCDaemon) Close() {
	w.closeOnce.Do(func() {
		w.cancel()
		w.wg.Wait()
	})
}
