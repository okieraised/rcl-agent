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

type ROSRouter struct {
	svc    restful.IROSService
	logger *log.Logger
	tracer trace.Tracer
}

func NewROSRouter(svc restful.IROSService) *ROSRouter {
	logger := log.MustNewECSLogger()
	return &ROSRouter{
		svc:    svc,
		logger: logger,
		tracer: tracer_client.Tracer("ros_http"),
	}
}

func (r *ROSRouter) Routes(engine *gin.RouterGroup) {
	routes := engine.Group("/ros")
	routes.GET("/topics", r.listTopics)
}

func (r *ROSRouter) listTopics(ctx *gin.Context) {
	rootCtx, span := r.tracer.Start(ctx, ctx.Request.URL.Path, trace.WithAttributes(attribute.KeyValue{
		Key:   constants.APIFieldRequestID,
		Value: attribute.StringValue(ctx.GetString(constants.APIFieldRequestID)),
	}))
	defer span.End()

	resp := api_response.New[any](ctx)
	r.logger.With(
		zap.String(constants.APIFieldRequestID, ctx.GetString(constants.APIFieldRequestID)),
	).Info("Received new ROS topic list request")

	// Handler
	_, cSpan := r.tracer.Start(rootCtx, "handler")
	result, appErr := r.svc.ListTopics(ctx, &restful.ListTopicsInput{TracerCtx: rootCtx, Tracer: r.tracer})
	if appErr != nil {
		cSpan.End()
		r.logger.Error(appErr.Error())
		resp.Populate(appErr.Code, appErr.Message, nil, nil, nil)
		ctx.JSON(http.StatusInternalServerError, resp)
		return
	}
	cSpan.End()

	resp.Populate(result.Code, result.Message, result.Data, nil, nil)
	ctx.JSON(http.StatusOK, resp)
	return
}
