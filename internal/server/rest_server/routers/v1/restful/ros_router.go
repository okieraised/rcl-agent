package restful

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/okieraised/monitoring-agent/internal/api_response"
	"github.com/okieraised/monitoring-agent/internal/cerrors"
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
	routes.GET("/nodes", r.listNodes)
	routes.GET("/services", r.listServiceNamesAndTypesByNode)
}

func (r *ROSRouter) listTopics(ctx *gin.Context) {
	rootCtx, span := r.tracer.Start(ctx, ctx.Request.URL.Path, trace.WithAttributes(attribute.KeyValue{
		Key:   constants.APIFieldRequestID,
		Value: attribute.StringValue(ctx.GetString(constants.APIFieldRequestID)),
	}))
	defer span.End()

	resp := api_response.New[any](ctx)
	lg := r.logger.With(
		zap.String(constants.APIFieldRequestID, ctx.GetString(constants.APIFieldRequestID)),
	)
	lg.Info("Received new ROS topic list request")

	// Handler
	_, cSpan := r.tracer.Start(rootCtx, "handler")
	result, appErr := r.svc.ListTopics(ctx, &restful.ListTopicsInput{TracerCtx: rootCtx, Tracer: r.tracer})
	if appErr != nil {
		cSpan.End()
		lg.Error(appErr.Error())
		resp.Populate(appErr.Code, appErr.Message, nil, nil, nil)
		ctx.JSON(http.StatusInternalServerError, resp)
		return
	}
	cSpan.End()

	lg.Info("Completed ROS topic list request")
	resp.Populate(result.Code, result.Message, result.Data, nil, result.Count)
	ctx.JSON(http.StatusOK, resp)
	return
}

func (r *ROSRouter) listNodes(ctx *gin.Context) {
	rootCtx, span := r.tracer.Start(ctx, ctx.Request.URL.Path, trace.WithAttributes(attribute.KeyValue{
		Key:   constants.APIFieldRequestID,
		Value: attribute.StringValue(ctx.GetString(constants.APIFieldRequestID)),
	}))
	defer span.End()

	resp := api_response.New[any](ctx)
	lg := r.logger.With(
		zap.String(constants.APIFieldRequestID, ctx.GetString(constants.APIFieldRequestID)),
	)
	lg.Info("Received new ROS node list request")

	// Handler
	_, cSpan := r.tracer.Start(rootCtx, "handler")
	result, appErr := r.svc.ListNodes(ctx, &restful.ListNodesInput{TracerCtx: rootCtx, Tracer: r.tracer})
	if appErr != nil {
		cSpan.End()
		lg.Error(appErr.Error())
		resp.Populate(appErr.Code, appErr.Message, nil, nil, nil)
		ctx.JSON(http.StatusInternalServerError, resp)
		return
	}
	cSpan.End()

	lg.Info("Completed ROS node list request")
	resp.Populate(result.Code, result.Message, result.Data, nil, result.Count)
	ctx.JSON(http.StatusOK, resp)
	return
}

type GetServiceNamesAndTypesByNodeRequest struct {
	Node      *string `form:"node" validate:"required"`
	Namespace *string `form:"namespace" validate:"required"`
}

func (req *GetServiceNamesAndTypesByNodeRequest) validate() error {
	if req.Node == nil {
		return errors.New("node name is required")
	}
	if req.Namespace == nil {
		return errors.New("namespace is required")
	}
	return nil
}

func (req *GetServiceNamesAndTypesByNodeRequest) ToGetServiceNamesAndTypesByNodeInput(tracerCtx context.Context, tracer trace.Tracer) *restful.GetServiceNamesAndTypesByNodeInput {
	return &restful.GetServiceNamesAndTypesByNodeInput{
		TracerCtx: tracerCtx,
		Tracer:    tracer,
		NodeName:  req.Node,
		Namespace: req.Namespace,
	}
}

func (r *ROSRouter) listServiceNamesAndTypesByNode(ctx *gin.Context) {
	rootCtx, span := r.tracer.Start(ctx, ctx.Request.URL.Path, trace.WithAttributes(attribute.KeyValue{
		Key:   constants.APIFieldRequestID,
		Value: attribute.StringValue(ctx.GetString(constants.APIFieldRequestID)),
	}))
	defer span.End()

	resp := api_response.New[any](ctx)
	lg := r.logger.With(
		zap.String(constants.APIFieldRequestID, ctx.GetString(constants.APIFieldRequestID)),
	)
	lg.Info("Received new ROS service list request")

	// binding
	var req GetServiceNamesAndTypesByNodeRequest
	err := ctx.ShouldBindQuery(&req)
	if err != nil {
		lg.Error(err.Error())
		resp.Populate(cerrors.ErrGenericBadRequest.Code, cerrors.ErrGenericBadRequest.Message, nil, nil, nil)
		ctx.JSON(http.StatusBadRequest, resp)
		return
	}
	err = req.validate()
	if err != nil {
		lg.Error(err.Error())
		resp.Populate(cerrors.ErrGenericBadRequest.Code, cerrors.ErrGenericBadRequest.Message, nil, nil, nil)
	}

	// Handler
	_, cSpan := r.tracer.Start(rootCtx, "handler")
	result, appErr := r.svc.GetServiceNamesAndTypesByNode(
		ctx,
		req.ToGetServiceNamesAndTypesByNodeInput(rootCtx, r.tracer),
	)
	if appErr != nil {
		cSpan.End()
		lg.Error(appErr.Error())
		resp.Populate(appErr.Code, appErr.Message, nil, nil, nil)
		ctx.JSON(http.StatusInternalServerError, resp)
		return
	}
	cSpan.End()

	lg.Info("Completed ROS service list request")
	resp.Populate(result.Code, result.Message, result.Data, nil, result.Count)
	ctx.JSON(http.StatusOK, resp)
	return
}
