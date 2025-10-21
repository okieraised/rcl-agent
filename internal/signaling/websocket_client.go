package signaling

import (
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

type WSClient struct {
	ID      uuid.UUID
	Conn    *websocket.Conn
	send    chan common.SignalingMessage
	onMsgFn MessageHandler
	hub     *WebsocketHub
}

// NewWebsocketClient creates a new websocket client
func NewWebsocketClient(id uuid.UUID, conn *websocket.Conn, hub *WebsocketHub) *WSClient {
	return &WSClient{
		ID:   id,
		Conn: conn,
		send: make(chan common.SignalingMessage, 16),
		hub:  hub,
	}
}

func (c *WSClient) SetMessageHandler(fn MessageHandler) {
	c.onMsgFn = fn
}

func (c *WSClient) Read() {
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

func (c *WSClient) Write() {
	go c.PingLoop()

	for {
		select {
		case message, ok := <-c.send:
			if err := c.Conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				log.Default().Info(errors.Wrap(err, "failed to set write deadline").Error())
			}

			if !ok {
				// Channel closed -> send close frame
				if err := c.Conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
					log.Default().Info(errors.Wrap(err, "failed to send close message").Error())
				}
				return
			}

			if err := c.Conn.WriteJSON(message); err != nil {
				log.Default().Info(errors.Wrap(err, "failed to send message").Error())
				return
			}
		}
	}
}

// PingLoop handles periodic WebSocket pings to keep the connection alive.
func (c *WSClient) PingLoop() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.Conn.Close()
	}()

	for {
		select {
		case <-ticker.C:
			if err := c.Conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				log.Default().Info(errors.Wrap(err, "failed to set write deadline").Error())
			}
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Default().Info(errors.Wrap(err, "failed to send ping").Error())
				return
			}
		}
	}
}

func (c *WSClient) Close() {
	close(c.send)
}
