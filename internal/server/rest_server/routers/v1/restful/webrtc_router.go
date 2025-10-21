package restful

import (
	"context"
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

type WebRTCRouter struct {
	svc    restful.IWebRTCService
	logger *log.Logger
	tracer trace.Tracer
}

func NewWebRTCRouter(svc restful.IWebRTCService) *WebRTCRouter {
	logger := log.MustNewECSLogger()
	return &WebRTCRouter{
		svc:    svc,
		logger: logger,
		tracer: tracer_client.Tracer("webrtc_http_router"),
	}
}

func (r *WebRTCRouter) Routes(engine *gin.RouterGroup) {
	routes := engine.Group("/webrtc")
	routes.POST("/session", r.createSession)
	routes.POST("/offer", r.offer)
}

func (r *WebRTCRouter) createSession(ctx *gin.Context) {
}

type WebRTCOfferRequest struct {
	Offer *string `json:"offer"`
}

func (req *WebRTCOfferRequest) validate() *cerrors.AppError {
	if req.Offer == nil {
		return cerrors.ErrMissingWebRTCOffer
	}
	return nil
}

func (req *WebRTCOfferRequest) ToWebRTCOfferInput(
	ctx context.Context,
	tracer trace.Tracer,
) *restful.WebRTCOfferInput {
	return &restful.WebRTCOfferInput{
		TracerCtx: ctx,
		Tracer:    tracer,
		Offer:     req.Offer,
	}
}

func (r *WebRTCRouter) offer(ctx *gin.Context) {
	rootCtx, span := r.tracer.Start(ctx, ctx.Request.URL.Path, trace.WithAttributes(attribute.KeyValue{
		Key:   constants.APIFieldRequestID,
		Value: attribute.StringValue(ctx.GetString(constants.APIFieldRequestID)),
	}))
	defer span.End()

	resp := api_response.New[any](ctx)
	r.logger.With(
		zap.String(constants.APIFieldRequestID, ctx.GetString(constants.APIFieldRequestID)),
	).Info("Received new WebRTC offer request")

	// serialization
	_, cSpan := r.tracer.Start(rootCtx, "serialization")
	var req WebRTCOfferRequest
	err := ctx.ShouldBindJSON(&req)
	if err != nil {
		cSpan.End()
		r.logger.Error(err.Error())
		resp.Populate(cerrors.ErrGenericBadRequest.Code, cerrors.ErrGenericBadRequest.Message, nil, nil, nil)
		ctx.JSON(http.StatusBadRequest, resp)
		return
	}
	cSpan.End()

	// Validation
	_, cSpan = r.tracer.Start(rootCtx, "validation")
	appErr := req.validate()
	if appErr != nil {
		cSpan.End()
		r.logger.Error(appErr.Error())
		resp.Populate(appErr.Code, appErr.Message, nil, nil, nil)
		ctx.JSON(http.StatusBadRequest, resp)
		return
	}
	cSpan.End()

	// Handler
	_, cSpan = r.tracer.Start(rootCtx, "handler")
	result, appErr := r.svc.Offer(ctx, req.ToWebRTCOfferInput(rootCtx, r.tracer))
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
