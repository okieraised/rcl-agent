package restful

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/okieraised/monitoring-agent/internal/api_response"
	"github.com/okieraised/monitoring-agent/internal/constants"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/log"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/tracer_client"
	"github.com/okieraised/monitoring-agent/internal/server/rest_server/services/v1/restful"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type HealthcheckRouter struct {
	svc    restful.IHealthcheckService
	logger *log.Logger
	tracer trace.Tracer
}

func NewHealthcheckRouter(svc restful.IHealthcheckService) *HealthcheckRouter {
	logger := log.MustNewECSLogger()
	return &HealthcheckRouter{
		svc:    svc,
		logger: logger,
		tracer: tracer_client.Tracer("healthcheck"),
	}
}

func (r *HealthcheckRouter) Routes(engine *gin.RouterGroup) {
	routes := engine.Group("/health")
	routes.GET("", r.healthcheck)
}

func (r *HealthcheckRouter) healthcheck(ctx *gin.Context) {
	rootCtx, span := r.tracer.Start(ctx, ctx.Request.URL.Path, trace.WithAttributes(attribute.KeyValue{
		Key:   constants.APIFieldRequestID,
		Value: attribute.StringValue(ctx.GetString(constants.APIFieldRequestID)),
	}))
	defer span.End()

	resp := api_response.New[any](ctx)
	lg := r.logger.With(
		zap.String(constants.APIFieldRequestID, ctx.GetString(constants.APIFieldRequestID)),
	)
	lg.Info("Received new healthcheck request")

	_, cSpan := r.tracer.Start(rootCtx, "handler")
	result, appErr := r.svc.Healthcheck(ctx, &restful.HealthcheckInput{
		TracerCtx: rootCtx,
		Tracer:    r.tracer,
	})
	if appErr != nil {
		cSpan.End()
		lg.Error(appErr.Error())
		resp.Populate(appErr.Code, appErr.Message, nil, nil, nil)
		ctx.JSON(http.StatusInternalServerError, resp)
		return
	}
	cSpan.End()

	lg.Info("Healthcheck completed")

	resp.Populate(result.Code, result.Message, result.Data, nil, nil)
	ctx.JSON(http.StatusOK, resp)
	return
}
