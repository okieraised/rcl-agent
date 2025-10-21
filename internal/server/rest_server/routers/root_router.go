package routers

import (
	"github.com/gin-gonic/gin"
	"github.com/okieraised/monitoring-agent/internal/server/rest_server/routers/v1/restful"
	"github.com/okieraised/monitoring-agent/internal/server/rest_server/routers/v1/ws"
)

type RootRouter struct {
	appState *AppState
}

func NewRootRouter(appState *AppState) *RootRouter {
	return &RootRouter{
		appState: appState,
	}
}

func (rr *RootRouter) InitRouters(engine *gin.Engine) {
	// http
	rootAPIRouter := engine.Group("/api")
	v1Router := rootAPIRouter.Group("/v1")
	{
		webRTCRouter := restful.NewWebRTCRouter(rr.appState.GetV1RestState().GetWebRTCService())
		webRTCRouter.Routes(v1Router)

		healthcheckRouter := restful.NewHealthcheckRouter(rr.appState.GetV1RestState().GetHealthcheckService())
		healthcheckRouter.Routes(v1Router)
	}

	// websocket
	{
		rootWSRouter := engine.Group("/ws")
		websocketRouter := ws.NewWebsocketRouter(rr.appState.GetWebsocketState().GetWebsocketService())
		websocketRouter.Routes(rootWSRouter)
	}
}
