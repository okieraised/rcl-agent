package routers

import (
	"github.com/okieraised/monitoring-agent/internal/server/rest_server/services/v1/restful"
	"github.com/okieraised/monitoring-agent/internal/server/rest_server/services/v1/ws"
)

type V1Rest struct {
	healthcheck *restful.HealthcheckService
	webrtc      *restful.WebRTCService
}

func NewV1RestState() *V1Rest {
	return &V1Rest{}
}

func (svc *V1Rest) SetWebRTCService(webrtc *restful.WebRTCService) {
	svc.webrtc = webrtc
}

func (svc *V1Rest) GetWebRTCService() *restful.WebRTCService {
	return svc.webrtc
}

func (svc *V1Rest) SetHealthcheckService(healthcheck *restful.HealthcheckService) {
	svc.healthcheck = healthcheck
}

func (svc *V1Rest) GetHealthcheckService() *restful.HealthcheckService {
	return svc.healthcheck
}

type Websocket struct {
	websocket *ws.WebsocketService
}

func NewWebsocketState() *Websocket {
	return &Websocket{}
}

func (svc *Websocket) SetWebsocketService(webrtc *ws.WebsocketService) {
	svc.websocket = webrtc
}

func (svc *Websocket) GetWebsocketService() *ws.WebsocketService {
	return svc.websocket
}

type AppState struct {
	v1Rest    *V1Rest
	websocket *Websocket
}

func NewAppState() *AppState {
	return &AppState{}
}

func (svc *AppState) SetV1RestState(v1Rest *V1Rest) {
	svc.v1Rest = v1Rest
}

func (svc *AppState) GetV1RestState() *V1Rest {
	return svc.v1Rest
}

func (svc *AppState) GetWebsocketState() *Websocket {
	return svc.websocket
}

func (svc *AppState) SetWebsocketState(ws *Websocket) {
	svc.websocket = ws
}
