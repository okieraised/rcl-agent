package ws

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/okieraised/monitoring-agent/internal/api_response"
	"github.com/okieraised/monitoring-agent/internal/cerrors"
	"github.com/okieraised/monitoring-agent/internal/constants"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/log"
	"github.com/okieraised/monitoring-agent/internal/signaling"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type IWebsocketService interface {
	Exchange(ctx *gin.Context, tracerCtx context.Context, tracer trace.Tracer) (*api_response.BaseOutput, *cerrors.AppError)
}

type WebsocketService struct {
	hub      *signaling.WebsocketHub
	logger   *log.Logger
	upgrader websocket.Upgrader
}

func NewWebsocketService(options ...func(*WebsocketService)) *WebsocketService {
	var upgrader = websocket.Upgrader{
		HandshakeTimeout: 5 * time.Second,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	svc := &WebsocketService{}
	for _, opt := range options {
		opt(svc)
	}
	logger := log.MustNewECSLogger()
	svc.upgrader = upgrader
	svc.logger = logger

	return svc
}

func WithWebsocketHub(hub *signaling.WebsocketHub) func(*WebsocketService) {
	return func(c *WebsocketService) {
		c.hub = hub
	}
}

func (svc *WebsocketService) Exchange(
	ctx *gin.Context,
	tracerCtx context.Context,
	tracer trace.Tracer,
) (*api_response.BaseOutput, *cerrors.AppError) {
	rootCtx, span := tracer.Start(tracerCtx, "exchange-sdp")
	defer span.End()

	resp := &api_response.BaseOutput{}
	svc.logger.With(
		zap.String(constants.APIFieldRequestID, ctx.GetString(constants.APIFieldRequestID)),
	)

	_, cSpan := tracer.Start(rootCtx, "upgrade-connection")
	connID := uuid.New()
	svc.logger.Info(fmt.Sprintf("New client connection established with ID: %s", connID.String()))
	conn, err := svc.upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		svc.logger.Error(err.Error())
		cSpan.End()
	}
	cSpan.End()

	client := signaling.NewWebsocketClient(connID, conn, svc.hub)

	svc.hub.GetRegister() <- client
	go client.Write()
	go client.Read()

	resp.Code = cerrors.OK.Code
	resp.Message = cerrors.OK.Message
	return resp, nil
}
