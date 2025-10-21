package restful

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/okieraised/monitoring-agent/internal/api_response"
	"github.com/okieraised/monitoring-agent/internal/cerrors"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/log"
	"go.opentelemetry.io/otel/trace"
)

type IWebRTCService interface {
	Offer(ctx *gin.Context, input *WebRTCOfferInput) (*api_response.BaseOutput, *cerrors.AppError)
}

type WebRTCService struct {
	logger *log.Logger
}

func NewWebRTCService(options ...func(*WebRTCService)) *WebRTCService {
	svc := &WebRTCService{}
	for _, opt := range options {
		opt(svc)
	}
	logger := log.MustNewECSLogger()
	svc.logger = logger
	return svc
}

type WebRTCOfferInput struct {
	TracerCtx context.Context
	Tracer    trace.Tracer
	Offer     *string
}

type WebRTCOfferOutput struct {
	Answer *string `json:"answer"`
}

func (svc *WebRTCService) Offer(ctx *gin.Context, input *WebRTCOfferInput) (*api_response.BaseOutput, *cerrors.AppError) {

	return &api_response.BaseOutput{}, nil
}
