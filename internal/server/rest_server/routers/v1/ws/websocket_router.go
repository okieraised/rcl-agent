package ws

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/okieraised/monitoring-agent/internal/api_response"
	"github.com/okieraised/monitoring-agent/internal/constants"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/log"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/tracer_client"
	"github.com/okieraised/monitoring-agent/internal/server/rest_server/services/v1/ws"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type WebsocketRouter struct {
	svc    ws.IWebsocketService
	logger *log.Logger
	tracer trace.Tracer
}

func NewWebsocketRouter(svc ws.IWebsocketService) *WebsocketRouter {
	logger := log.MustNewECSLogger()
	return &WebsocketRouter{
		svc:    svc,
		logger: logger,
		tracer: tracer_client.Tracer("websocket_router"),
	}
}

func (r *WebsocketRouter) Routes(engine *gin.RouterGroup) {
	routes := engine.Group("")
	routes.GET("", r.exchange)
}

func (r *WebsocketRouter) exchange(ctx *gin.Context) {
	rootCtx, span := r.tracer.Start(ctx, ctx.Request.URL.Path, trace.WithAttributes(attribute.KeyValue{
		Key:   constants.APIFieldRequestID,
		Value: attribute.StringValue(ctx.GetString(constants.APIFieldRequestID)),
	}))
	defer span.End()

	resp := api_response.New[any](ctx)
	r.logger.With(
		zap.String(constants.APIFieldRequestID, ctx.GetString(constants.APIFieldRequestID)),
	).Debug("Received new websocket handshake for exchanging messages")

	// Handler
	_, cSpan := r.tracer.Start(rootCtx, "handler")
	_, err := r.svc.Exchange(ctx, rootCtx, r.tracer)
	if err != nil {
		cSpan.End()
		r.logger.Error(err.Error())
		resp.Populate(err.Code, err.Message, nil, nil, nil)
		ctx.JSON(http.StatusInternalServerError, resp)
		return
	}
	cSpan.End()
}
