package signaling

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/okieraised/monitoring-agent/internal/common"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/log"
	"github.com/pkg/errors"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 16 << 20
)

type MessageHandler func(msg common.SignalingMessage)

type WebsocketClient struct {
	ID      uuid.UUID
	Conn    *websocket.Conn
	send    chan common.SignalingMessage
	onMsgFn MessageHandler
	hub     *WebsocketHub
	writeMu sync.Mutex
	closed  chan struct{}
}

// NewWebsocketClient creates a new websocket client
func NewWebsocketClient(id uuid.UUID, conn *websocket.Conn, hub *WebsocketHub) *WebsocketClient {

	c := &WebsocketClient{
		ID:     id,
		Conn:   conn,
		send:   make(chan common.SignalingMessage, 16),
		hub:    hub,
		closed: make(chan struct{}),
	}

	go c.pingLoop()

	return c
}

func (c *WebsocketClient) SetMessageHandler(fn MessageHandler) {
	c.onMsgFn = fn
}

func (c *WebsocketClient) Send(msg common.SignalingMessage) {
	select {
	case c.send <- msg:
	default:
		log.Default().Info(fmt.Sprintf("client %s's send buffer is full, dropping message", c.ID))
	}
}

func (c *WebsocketClient) Read() {
	defer func() {
		c.hub.unregister <- c
		_ = c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	err := c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	if err != nil {
		wErr := errors.Wrap(err, "failed to set read deadline")
		log.Default().Info(wErr.Error())
	}
	c.Conn.SetPongHandler(func(string) error {
		err = c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		if err != nil {
			wErr := errors.Wrap(err, "failed to set read deadline")
			log.Default().Info(wErr.Error())
		}
		return nil
	})

	for {
		var msg common.SignalingMessage
		err = c.Conn.ReadJSON(&msg)
		if err != nil {
			wErr := errors.Wrap(err, "failed to read message")
			log.Default().Info(wErr.Error())
			break
		}

		if c.onMsgFn != nil {
			c.onMsgFn(msg)
		} else {
			c.hub.broadcast <- msg
		}
	}
}

func (c *WebsocketClient) Write() {

	for {
		select {
		case message, ok := <-c.send:
			if err := c.Conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				log.Default().Info(errors.Wrap(err, "failed to set write deadline").Error())
			}

			if !ok {
				// Channel closed -> send close frame
				if err := c.safeWrite(websocket.CloseMessage, []byte{}); err != nil {
					log.Default().Info(errors.Wrap(err, "failed to send close message").Error())
				}
				return
			}

			if err := c.WriteJSON(message); err != nil {
				log.Default().Info(errors.Wrap(err, "failed to send message").Error())
				return
			}
		}
	}
}

func (c *WebsocketClient) safeWrite(msgType int, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if err := c.Conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		return err
	}
	return c.Conn.WriteMessage(msgType, data)
}

func (c *WebsocketClient) WriteJSON(v any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if err := c.Conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		return err
	}
	return c.Conn.WriteJSON(v)
}

func (c *WebsocketClient) pingLoop() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.Conn.Close()
	}()

	for {
		select {
		case <-ticker.C:
			if err := c.safeWrite(websocket.PingMessage, nil); err != nil {
				log.Default().Error(errors.Wrap(err, fmt.Sprintf("client [%s] ping error", c.ID.String())).Error())
				return
			}
		case <-c.closed:
			return
		}
	}
}

func (c *WebsocketClient) Close() {
	select {
	case <-c.closed:
		return
	default:
		close(c.closed)
		close(c.send)
		_ = c.Conn.Close()
	}
}
