package restful

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/okieraised/monitoring-agent/internal/api_response"
	"github.com/okieraised/monitoring-agent/internal/cerrors"
	"github.com/okieraised/monitoring-agent/internal/constants"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/log"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/ros_node"
	"github.com/okieraised/monitoring-agent/internal/utilities"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type IROSService interface {
	ListTopics(ctx *gin.Context, input *ListTopicsInput) (*api_response.BaseOutput, *cerrors.AppError)
	ListNodes(ctx *gin.Context, input *ListNodesInput) (*api_response.BaseOutput, *cerrors.AppError)
	GetServiceNamesAndTypesByNode(ctx *gin.Context, input *GetServiceNamesAndTypesByNodeInput) (*api_response.BaseOutput, *cerrors.AppError)
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
	Topic string   `json:"topic"`
	Types []string `json:"types"`
}

func (svc *ROSService) ListTopics(ctx *gin.Context, input *ListTopicsInput) (*api_response.BaseOutput, *cerrors.AppError) {
	rootCtx, span := input.Tracer.Start(input.TracerCtx, "list-topics")
	defer span.End()

	resp := &api_response.BaseOutput{}
	lg := svc.logger.With(
		zap.String(constants.APIFieldRequestID, ctx.GetString(constants.APIFieldRequestID)),
	)

	_, cSpan := input.Tracer.Start(rootCtx, "get-ros-topic")
	rosNode := ros_node.Node()
	topicMapper, err := rosNode.GetTopicNamesAndTypes(true)
	if err != nil {
		cSpan.End()
		wErr := errors.Wrap(err, "failed to get topic names and types")
		lg.Error(wErr.Error())
		return nil, cerrors.ErrGenericInternalServer
	}
	cSpan.End()

	respData := make([]ListTopicsOutput, 0, len(topicMapper))
	for topic, types := range topicMapper {
		respData = append(respData, ListTopicsOutput{
			Topic: topic,
			Types: types,
		})
	}

	resp.Code = cerrors.OK.Code
	resp.Message = cerrors.OK.Message
	resp.Data = respData
	resp.Count = len(respData)
	return resp, nil
}

type ListNodesInput struct {
	TracerCtx context.Context
	Tracer    trace.Tracer
}

type ListNodesOutput struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (svc *ROSService) ListNodes(ctx *gin.Context, input *ListNodesInput) (*api_response.BaseOutput, *cerrors.AppError) {
	rootCtx, span := input.Tracer.Start(input.TracerCtx, "list-nodes")
	defer span.End()

	resp := &api_response.BaseOutput{}
	lg := svc.logger.With(
		zap.String(constants.APIFieldRequestID, ctx.GetString(constants.APIFieldRequestID)),
	)

	_, cSpan := input.Tracer.Start(rootCtx, "get-ros-nodes")
	rosNode := ros_node.Node()
	nodes, namespaces, err := rosNode.GetNodeNames()
	if err != nil {
		cSpan.End()
		wErr := errors.Wrap(err, "failed to get node names and namespaces")
		lg.Error(wErr.Error())
		return nil, cerrors.ErrGenericInternalServer
	}
	cSpan.End()

	respData := make([]ListNodesOutput, 0, len(nodes))
	for idx, node := range nodes {
		respData = append(respData, ListNodesOutput{
			Name:      node,
			Namespace: namespaces[idx],
		})
	}

	resp.Code = cerrors.OK.Code
	resp.Message = cerrors.OK.Message
	resp.Data = respData
	resp.Count = len(respData)
	return resp, nil
}

type GetServiceNamesAndTypesByNodeInput struct {
	TracerCtx context.Context
	Tracer    trace.Tracer
	NodeName  *string
	Namespace *string
}

type GetServiceNamesAndTypesByNodeOutput struct {
	Name  string   `json:"name"`
	Types []string `json:"types"`
}

func (svc *ROSService) GetServiceNamesAndTypesByNode(ctx *gin.Context, input *GetServiceNamesAndTypesByNodeInput) (*api_response.BaseOutput, *cerrors.AppError) {
	rootCtx, span := input.Tracer.Start(input.TracerCtx, "list-services")
	defer span.End()

	resp := &api_response.BaseOutput{}
	lg := svc.logger.With(
		zap.String(constants.APIFieldRequestID, ctx.GetString(constants.APIFieldRequestID)),
	)

	_, cSpan := input.Tracer.Start(rootCtx, "get-ros-nodes")
	rosNode := ros_node.Node()
	services, err := rosNode.GetActionServerNamesAndTypesByNode(
		utilities.Deref(input.NodeName),
		utilities.Deref(input.Namespace),
	)
	if err != nil {
		cSpan.End()
		wErr := errors.Wrap(err, "failed to get service names and types")
		lg.Error(wErr.Error())
		return nil, cerrors.ErrGenericInternalServer
	}
	cSpan.End()

	respData := make([]GetServiceNamesAndTypesByNodeOutput, 0, len(services))
	for srv, types := range services {
		respData = append(respData, GetServiceNamesAndTypesByNodeOutput{
			Name:  srv,
			Types: types,
		})
	}

	resp.Code = cerrors.OK.Code
	resp.Message = cerrors.OK.Message
	resp.Data = respData
	resp.Count = len(respData)
	return resp, nil
}
