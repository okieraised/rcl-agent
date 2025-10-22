package restful

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/okieraised/monitoring-agent/internal/api_response"
	"github.com/okieraised/monitoring-agent/internal/cerrors"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/log"
	"go.opentelemetry.io/otel/trace"
)

type IROSService interface {
	ListTopics(ctx *gin.Context, input *ListTopicsInput) (*api_response.BaseOutput, *cerrors.AppError)
}

type ROSService struct {
	logger *log.Logger
}

func NewROSService(options ...func(*ROSService)) *ROSService {
	svc := &ROSService{}
	for _, opt := range options {
		opt(svc)
	}
	logger := log.MustNewECSLogger()
	svc.logger = logger
	return svc
}

type ListTopicsInput struct {
	TracerCtx context.Context
	Tracer    trace.Tracer
}

type ListTopicsOutput struct {
}

func (svc *ROSService) ListTopics(ctx *gin.Context, input *HealthcheckInput) (*api_response.BaseOutput, *cerrors.AppError) {
	return &api_response.BaseOutput{}, nil
}
