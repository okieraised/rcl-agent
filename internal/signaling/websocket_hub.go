package signaling

import (
	"context"
	"fmt"

	"github.com/okieraised/monitoring-agent/internal/common"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/log"
	"go.opentelemetry.io/otel/trace"
)

type WebsocketHub struct {
	tracer     trace.Tracer                 // Tracing client
	clients    map[*WSClient]bool           // Registered clients.
	broadcast  chan common.SignalingMessage // Inbound messages from the clients.
	register   chan *WSClient               // Register requests from the clients.
	unregister chan *WSClient               // Unregistered clients.
}

func (h *WebsocketHub) GetClients() map[*WSClient]bool {
	return h.clients
}

func (h *WebsocketHub) GetBroadcast() chan common.SignalingMessage {
	return h.broadcast
}

func (h *WebsocketHub) GetRegister() chan *WSClient {
	return h.register
}

func (h *WebsocketHub) GetUnregister() chan *WSClient {
	return h.unregister
}

var webRTCHub *WebsocketHub

func GetWebsocketHubInstance() *WebsocketHub {
	if webRTCHub == nil {
		panic("WebsocketHub is not initialized")
	}
	return webRTCHub
}

func NewWebsocketHub() *WebsocketHub {
	webRTCHub = &WebsocketHub{
		clients:    make(map[*WSClient]bool),
		broadcast:  make(chan common.SignalingMessage),
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
	}

	return webRTCHub
}

func (h *WebsocketHub) Run(ctx context.Context) {
	log.Default().Info("Starting to listen for new clients and messages")
	go func() {
		for {
			select {
			case client := <-h.register:
				h.RegisterNewClient(client)
			case client := <-h.unregister:
				h.RemoveClient(client)
			case message := <-h.broadcast:
				h.HandleMessage(message)
			case <-ctx.Done():
				log.Default().Info("Shutting down WebRTC websocket hub")
				return
			}
		}
	}()
}

func (h *WebsocketHub) RegisterNewClient(client *WSClient) {
	if _, ok := h.clients[client]; !ok {
		log.Default().Debug(fmt.Sprintf("Registering new client with id [%s]", client.ID.String()))
		h.clients[client] = true
	} else {
		log.Default().Debug(fmt.Sprintf("Client with id [%s] already registered", client.ID.String()))
	}
	log.Default().Debug(fmt.Sprintf("There are [%d] clients connected", len(h.clients)))
}

func (h *WebsocketHub) RemoveClient(client *WSClient) {
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)
		log.Default().Debug(fmt.Sprintf("Client with id [%s] disconnected", client.ID.String()))
	}
}

func (h *WebsocketHub) HandleMessage(message common.SignalingMessage) {
	log.Default().Debug(fmt.Sprintf("Received message [%v]", message))

	log.Default().Debug("Publishing message to all connected clients")
	for client := range h.clients {
		select {
		case client.send <- message:
		default:
			close(client.send)
			delete(h.clients, client)
		}
	}
}
