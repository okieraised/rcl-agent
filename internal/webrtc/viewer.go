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
	id         uuid.UUID
	ctx        context.Context
	cancel     context.CancelFunc
	cMqtt      mqtt.Client
	cWebsocket *signaling.WebsocketClient
	hub        *signaling.WebsocketHub
	sem        map[string]struct{}
	frameCh    map[string]chan []byte
	enableCh   map[string]chan bool
	closeCh    map[string]chan struct{}
	errCh      map[string]chan error
	wg         sync.WaitGroup
	mu         sync.RWMutex
	closeOnce  sync.Once
}

func NewWebRTCDaemon(ctx context.Context) (*WebRTCDaemon, error) {
	log.Default().Info("Starting WebRTC streaming daemon")

	cCtx, cancel := context.WithCancel(ctx)
	webRTCViewer := &WebRTCDaemon{
		id:       uuid.New(),
		ctx:      cCtx,
		cancel:   cancel,
		sem:      make(map[string]struct{}),
		frameCh:  make(map[string]chan []byte),
		enableCh: make(map[string]chan bool),
		closeCh:  make(map[string]chan struct{}),
		errCh:    make(map[string]chan error),
	}

	if viper.GetBool(config.AgentEnableMQTT) {
		webRTCViewer.cMqtt = mqtt_client.Client()
	} else {
		webRTCViewer.hub = signaling.GetWebsocketHubInstance()
	}

	return webRTCViewer, nil
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
	err := payload.ValidateWebRTC()
	if err != nil {
		log.Default().Info(errors.Wrap(err, "failed to validate webrtc payload").Error())
		return
	}
	log.Default().Debug(fmt.Sprintf("WebRTC signaling message received: %v", payload))
	switch payload.Payload.Type {
	case constants.MsgTypeWebRTCInit:
		w.mu.Lock()
		defer w.mu.Unlock()
		if _, ok := w.sem[payload.Payload.TopicName]; ok {
			log.Default().Info(fmt.Sprintf("There is an existing subscriber on topic [%s]", payload.Payload.TopicName))
			return
		}
		w.sem[payload.Payload.TopicName] = struct{}{}
		w.frameCh[payload.Payload.TopicName] = make(chan []byte, 64)
		w.enableCh[payload.Payload.TopicName] = make(chan bool, 1)
		w.closeCh[payload.Payload.TopicName] = make(chan struct{})
		w.errCh[payload.Payload.TopicName] = make(chan error, 1)
		w.topicSubscriptionHandler(payload)
	case constants.MsgTypeOffer:
		w.sdpExchangeHandler(payload)
	case constants.MsgTypeAnswer:
	default:
		wErr := fmt.Errorf("invalid payload type: %s", payload.Payload.Type)
		log.Default().Error(wErr.Error())
		return
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

		g, runCtx := errgroup.WithContext(ctx)

		iceServers := []webrtc.ICEServer{
			{
				URLs: []string{
					"stun:stun.l.google.com:19302",
				},
			},
		}
		pc, err := webrtc.NewPeerConnection(webrtc.Configuration{ICEServers: iceServers})
		if err != nil {
			wErr := errors.Wrapf(err, "failed to create peer connection")
			log.Default().Error(wErr.Error())
			w.errCh[offerPayload.Payload.TopicName] <- wErr
			return
		}
		defer func() {
			cErr := pc.Close()
			if cErr != nil && err == nil {
				log.Default().Error(fmt.Sprintf("peer connection close: %v", cErr))
				w.errCh[offerPayload.Payload.TopicName] <- cErr
			}
		}()

		iceConnectedCtx, iceConnectedCtxCancel := context.WithCancel(runCtx)
		defer iceConnectedCtxCancel()

		pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
			log.Default().Info(fmt.Sprintf("ICE Connection State changed: %s", state.String()))
			if state == webrtc.ICEConnectionStateConnected || state == webrtc.ICEConnectionStateCompleted {
				iceConnectedCtxCancel()
			}
		})

		// Signal when the peer closes/disconnects, so we can exit cleanly.
		peerClosed := make(chan struct{})
		var closePeerClosed sync.Once

		pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
			log.Default().Info(fmt.Sprintf("Peer Connection State changed: %s", s.String()))
			if s == webrtc.PeerConnectionStateFailed ||
				s == webrtc.PeerConnectionStateDisconnected ||
				s == webrtc.PeerConnectionStateClosed {
				closePeerClosed.Do(func() { close(peerClosed) })
			}
		})

		// Create a video track and add to peer
		sessionID := fmt.Sprintf(
			"video_%s_%s",
			strings.ReplaceAll(offerPayload.Payload.TransactionID.String(), "-", "_"),
			strings.ReplaceAll(offerPayload.Payload.TopicName, "/", "_"),
		)
		videoTrack, err := webrtc.NewTrackLocalStaticSample(
			webrtc.RTPCodecCapability{
				MimeType: webrtc.MimeTypeH264,
			},
			sessionID,
			viper.GetString(config.AgentID),
		)
		if err != nil {
			wErr := errors.Wrapf(err, "failed to create track")
			log.Default().Error(wErr.Error())
			w.errCh[offerPayload.Payload.TopicName] <- wErr
			return
		}

		rtpSender, err := pc.AddTrack(videoTrack)
		if err != nil {
			wErr := errors.Wrapf(err, "failed to add track")
			log.Default().Error(wErr.Error())
			w.errCh[offerPayload.Payload.TopicName] <- wErr
			return
		}

		// Read incoming RTCP packets
		// Before these packets are returned, they are processed by interceptors. For things
		// like NACK this needs to be called.
		g.Go(func() error {
			rtcpBuf := make([]byte, 1500)
			for {
				if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
					return nil
				}
			}
		})

		g.Go(func() error {
			enc, out, encErr := h264_encoder.NewH264Encoder(runCtx, nil)
			if encErr != nil {
				return fmt.Errorf("failed to create new h264 encoder: %w", encErr)
			}
			defer func() {
				cErr := enc.Close()
				if cErr != nil {
					log.Default().Error(fmt.Sprintf("failed to close encoder: %v", cErr))
					if encErr == nil {
						encErr = cErr
					}
				}
			}()

			h264, hErr := h264reader.NewReader(out)
			if hErr != nil {
				return fmt.Errorf("failed to create h264 reader: %w", hErr)
			}

			subCtx, subCancel := context.WithCancel(runCtx)
			defer subCancel()
			subGCtx, subCtx := errgroup.WithContext(subCtx)

			// Feed JPEG frames from ROS into the encoder stdin.
			subGCtx.Go(func() error {
				for {
					select {
					case <-subCtx.Done():
						return subCtx.Err()
					case frame, ok := <-w.frameCh[offerPayload.Payload.TopicName]:
						if !ok {
							return nil
						}
						wErr := enc.WriteFrame(frame)
						if wErr != nil && !errors.Is(wErr, io.EOF) {
							return fmt.Errorf("enc.WriteFrame: %w", wErr)
						}
					}
				}
			})

			select {
			case <-iceConnectedCtx.Done():
				// ok
			case <-runCtx.Done():
				_ = subGCtx.Wait()
				return runCtx.Err()
			}

			for {
				select {
				case <-runCtx.Done():
					_ = subGCtx.Wait()
					return runCtx.Err()
				default:
					nal, nErr := h264.NextNAL()
					if nErr != nil {
						if errors.Is(nErr, io.EOF) {
							log.Default().Info("H264 stream ended")
							_ = subGCtx.Wait()
							return nil
						}
						_ = subGCtx.Wait()
						return errors.Wrapf(nErr, "H264 stream ended")
					}

					if wErr := videoTrack.WriteSample(media.Sample{
						Data:     nal.Data,
						Duration: 33 * time.Millisecond,
					}); wErr != nil {
						_ = subGCtx.Wait()
						return errors.Wrap(wErr, "failed to write sample")
					}
				}
			}
		})

		offer, err := w.decodeSessionDescription(offerPayload.Payload.SDP)
		if err != nil {
			wErr := errors.Wrapf(err, "failed to decode session description")
			log.Default().Error(wErr.Error())
			w.errCh[offerPayload.Payload.TopicName] <- wErr
			return
		}

		if err = pc.SetRemoteDescription(offer); err != nil {
			wErr := errors.Wrapf(err, "failed to set remote description")
			log.Default().Error(wErr.Error())
			w.errCh[offerPayload.Payload.TopicName] <- wErr
			return
		}

		answer, err := pc.CreateAnswer(nil)
		if err != nil {
			wErr := errors.Wrapf(err, "failed to create answer")
			log.Default().Error(wErr.Error())
			w.errCh[offerPayload.Payload.TopicName] <- wErr
			return
		}

		gatherComplete := webrtc.GatheringCompletePromise(pc)

		if err = pc.SetLocalDescription(answer); err != nil {
			wErr := errors.Wrapf(err, "failed to set local description")
			log.Default().Error(wErr.Error())
			w.errCh[offerPayload.Payload.TopicName] <- wErr
			return
		}

		<-gatherComplete

		w.enableCh[offerPayload.Payload.TopicName] <- true

		// Encode the session description, then publish it back to mqtt
		sdp, err := w.encodeSessionDescription(pc.LocalDescription())
		if err != nil {
			wErr := errors.Wrapf(err, "failed to encode session description")
			log.Default().Error(wErr.Error())
			w.errCh[offerPayload.Payload.TopicName] <- wErr
			return
		}

		// Send the answer back to client
		sdpPayload := common.SignalingMessage{
			Header: offerPayload.Header,
			Payload: common.SignalingBody{
				TransactionID: offerPayload.Payload.TransactionID,
				Type:          constants.MsgTypeAnswer,
				TopicName:     offerPayload.Payload.TopicName,
				Session:       sessionID,
				SDP:           sdp,
			},
		}

		if viper.GetBool(config.AgentEnableMQTT) {
			bSDPPayload, err := json.Marshal(sdpPayload)
			if err != nil {
				wErr := errors.Wrapf(err, "failed to encode session description")
				log.Default().Error(wErr.Error())
				w.errCh[offerPayload.Payload.TopicName] <- wErr
				return
			}
			token := w.cMqtt.Publish(viper.GetString(config.MqttWebRTCAnswerTopic), qosExactlyOnce, false, bSDPPayload)
			if token.WaitTimeout(5*time.Second) && token.Error() != nil {
				w.errCh[offerPayload.Payload.TopicName] <- errors.Wrap(token.Error(), "failed to publish answer through mqtt")
				return
			}
		} else {
			err = w.cWebsocket.WriteJSON(sdpPayload)
			if err != nil {
				w.errCh[offerPayload.Payload.TopicName] <- errors.Wrap(err, "failed to send answer through websocket")
				return
			}
		}

		done := make(chan error, 1)
		go func() {
			done <- g.Wait()
		}()

		select {
		case err = <-done:
			w.errCh[offerPayload.Payload.TopicName] <- err
			return

		case <-peerClosed:
			// Remote hung up: cascade cancel, close PC to unblock RTCP reader, then wait.
			cancel()
			_ = pc.Close()
			w.errCh[offerPayload.Payload.TopicName] <- <-done
			return

		case <-ctx.Done():
			// Ctrl-C or parent canceled: cascade cancel, close PC, then wait.
			cancel()
			_ = pc.Close()

			// Grace period
			shutdownTimer := time.NewTimer(1 * time.Second)
			defer shutdownTimer.Stop()

			select {
			case dErr := <-done:
				w.errCh[offerPayload.Payload.TopicName] <- dErr
				return
			case <-shutdownTimer.C:
				w.errCh[offerPayload.Payload.TopicName] <- fmt.Errorf("shutdown timed out")
				return
			}
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

func (w *WebRTCDaemon) topicSubscriptionHandler(payload common.SignalingMessage) {
	w.wg.Add(1)
	go func() {
		ctx, cancel := context.WithCancel(w.ctx)

		defer func() {
			cancel()
			w.wg.Done()
			w.mu.Lock()
			delete(w.sem, payload.Payload.TopicName)
			delete(w.frameCh, payload.Payload.TopicName)
			delete(w.enableCh, payload.Payload.TopicName)
			delete(w.closeCh, payload.Payload.TopicName)
			w.mu.Unlock()
		}()
		defer recoverFn()

		go func() {
			err := newCameraSubscriber(
				ctx,
				ros_node.Node(),
				payload.Payload.TopicName,
				w.frameCh[payload.Payload.TopicName],
				w.enableCh[payload.Payload.TopicName],
			)
			if err != nil && !errors.Is(err, context.Canceled) {
				wErr := errors.Wrap(err, "error creating new camera subscriber")
				log.Default().Error(wErr.Error())
				w.errCh[payload.Payload.TopicName] <- wErr
			}
		}()

		for {
			select {
			case <-ctx.Done():
				log.Default().Info(fmt.Sprintf("Shutting down subscriber node for topic [%s]", payload.Payload.TopicName))
				return
			case <-w.closeCh[payload.Payload.TopicName]:
				log.Default().Info(fmt.Sprintf("Closing subscriber node for topic [%s]", payload.Payload.TopicName))
				return
			case err := <-w.errCh[payload.Payload.TopicName]:
				if !errors.Is(err, context.Canceled) {
					log.Default().Error(err.Error())
				}
				return
			}
		}
	}()
}

func (w *WebRTCDaemon) Close() {
	w.closeOnce.Do(func() {
		w.cancel()
		w.wg.Wait()
		for _, ch := range w.frameCh {
			close(ch)
		}
		for _, ch := range w.enableCh {
			close(ch)
		}
		for _, ch := range w.closeCh {
			close(ch)
		}
		for _, ch := range w.errCh {
			close(ch)

		}
	})
}
