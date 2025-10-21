package services

import (
	"context"
	"io"

	"fmt"

	"github.com/okieraised/monitoring-agent/internal/infrastructure/log"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/ros_node"
	sensor_msgs_msg "github.com/okieraised/monitoring-agent/internal/ros_msgs/sensor_msgs/msg"
	std_msgs_msg "github.com/okieraised/monitoring-agent/internal/ros_msgs/std_msgs/msg"
	"github.com/okieraised/monitoring-agent/internal/server/grpc_server/routes"
	"github.com/okieraised/rclgo/humble"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type ROSInterfaceService struct {
	routes.UnimplementedROSInterfaceServer
}

func (s *ROSInterfaceService) TopicList(ctx context.Context, in *routes.ROSTopicListRequest) (*routes.ROSTopicListResponse, error) {
	log.Default().Info("request new topic list request")
	topicMapper, err := ros_node.Node().GetTopicNamesAndTypes(true)
	if err != nil {
		wErr := errors.Wrap(err, "failed to get topic names and types")
		log.Default().Error(wErr.Error())
		return &routes.ROSTopicListResponse{}, status.Errorf(codes.Internal, wErr.Error())
	}

	data := make([]*routes.TopicMessageTypes, 0, len(topicMapper))
	for topic, msgTypes := range topicMapper {
		data = append(data, &routes.TopicMessageTypes{
			Topic: topic,
			Types: msgTypes,
		})
	}

	return &routes.ROSTopicListResponse{
		Data: data,
	}, nil
}

func (s *ROSInterfaceService) StreamCompressedImage(in *routes.SubscribeRequest, stream grpc.ServerStreamingServer[routes.CompressedImageResponse]) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	log.Default().Info(fmt.Sprintf("received new compressed image stream request for topic [%s]", in.GetTopicName()))
	topicMapper, err := ros_node.Node().GetTopicNamesAndTypes(true)
	if err != nil {
		wErr := errors.Wrap(err, "failed to get topic names and types")
		log.Default().Error(wErr.Error())
		return status.Errorf(codes.Internal, wErr.Error())
	}

	if _, ok := topicMapper[in.TopicName]; !ok {
		wErr := fmt.Errorf("topic [%s] is not found", in.TopicName)
		return status.Errorf(codes.NotFound, wErr.Error())
	}

	sub, err := sensor_msgs_msg.NewCompressedImageSubscription(
		ros_node.Node(),
		in.TopicName,
		nil,
		func(msg *sensor_msgs_msg.CompressedImage, info *humble.MessageInfo, err error) {
			if err != nil {
				log.Default().Error(fmt.Sprintf("failed to subscribe: %v", err))
				return
			}
			if ctx.Err() != nil {
				return
			}

			err = stream.Send(&routes.CompressedImageResponse{Format: msg.Format, Data: msg.Data})
			if err != nil {
				log.Default().Error(fmt.Sprintf("failed to send response: %v", err))
				cancel()
				return
			}
		},
	)
	if err != nil {
		wErr := errors.Wrapf(err, "failed to subscribe to topic [%s]", in.TopicName)
		return status.Errorf(codes.Internal, wErr.Error())
	}
	defer func(sub *sensor_msgs_msg.CompressedImageSubscription) {
		cErr := sub.Close()
		if cErr != nil && err == nil {
			log.Default().Error(fmt.Sprintf("failed to close subscriber: %v", err))
			err = status.Errorf(codes.Internal, cErr.Error())
		}
	}(sub)

	ws, err := humble.NewWaitSet()
	if err != nil {
		wErr := errors.Wrapf(err, "failed to waitset to topic [%s]", in.TopicName)
		return status.Errorf(codes.Internal, wErr.Error())
	}
	defer func(ws *humble.WaitSet) {
		cErr := ws.Close()
		if cErr != nil && err == nil {
			log.Default().Error(fmt.Sprintf("failed to close waitset: %v", cErr))
			err = status.Errorf(codes.Internal, cErr.Error())
		}
	}(ws)
	ws.AddSubscriptions(sub.Subscription)
	runErr := ws.Run(ctx)
	switch {
	case runErr == nil:
		log.Default().Info(fmt.Sprintf("closing compressed image subscriber to topic [%s]", in.TopicName))
		return nil
	case errors.Is(runErr, context.Canceled):
		log.Default().Info("context is canceled")
		return nil
	default:
		return status.Errorf(codes.Internal, runErr.Error())
	}
}

func (s *ROSInterfaceService) StreamJointStates(in *routes.SubscribeRequest, stream grpc.ServerStreamingServer[routes.JointStateResponse]) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	log.Default().Info(fmt.Sprintf("received new joint states streaming request for topic [%s]", in.GetTopicName()))
	topicMapper, err := ros_node.Node().GetTopicNamesAndTypes(true)
	if err != nil {
		wErr := errors.Wrap(err, "failed to get topic names and types")
		log.Default().Error(wErr.Error())
		return status.Errorf(codes.Internal, wErr.Error())
	}

	if _, ok := topicMapper[in.TopicName]; !ok {
		wErr := fmt.Errorf("topic [%s] is not found", in.TopicName)
		return status.Errorf(codes.NotFound, wErr.Error())
	}

	sub, err := sensor_msgs_msg.NewJointStateSubscription(
		ros_node.Node(),
		in.TopicName,
		nil,
		func(msg *sensor_msgs_msg.JointState, info *humble.MessageInfo, err error) {
			if err != nil {
				log.Default().Error(fmt.Sprintf("failed to subscribe: %v", err))
				return
			}
			if ctx.Err() != nil {
				return
			}

			header := &routes.Header{
				Stamp: &routes.Time{
					Sec:     msg.Header.Stamp.Sec,
					Nanosec: msg.Header.Stamp.Nanosec,
				},
				FrameId: msg.Header.FrameId,
			}

			err = stream.Send(&routes.JointStateResponse{
				Header:   header,
				Name:     msg.Name,
				Position: msg.Position,
				Velocity: msg.Velocity,
				Effort:   msg.Effort,
			})
			if err != nil {
				log.Default().Error(fmt.Sprintf("failed to send response: %v", err))
				cancel()
				return
			}
		},
	)
	if err != nil {
		wErr := errors.Wrapf(err, "failed to subscribe to topic [%s]", in.TopicName)
		return status.Errorf(codes.Internal, wErr.Error())
	}
	defer func(sub *sensor_msgs_msg.JointStateSubscription) {
		cErr := sub.Close()
		if cErr != nil && err == nil {
			log.Default().Error(fmt.Sprintf("failed to close subscriber: %v", err))
			err = status.Errorf(codes.Internal, cErr.Error())
		}
	}(sub)

	ws, err := humble.NewWaitSet()
	if err != nil {
		wErr := errors.Wrapf(err, "failed to waitset to topic [%s]", in.TopicName)
		return status.Errorf(codes.Internal, wErr.Error())
	}
	defer func(ws *humble.WaitSet) {
		cErr := ws.Close()
		if cErr != nil && err == nil {
			log.Default().Error(fmt.Sprintf("failed to close waitset: %v", cErr))
			err = status.Errorf(codes.Internal, cErr.Error())
		}
	}(ws)
	ws.AddSubscriptions(sub.Subscription)
	runErr := ws.Run(ctx)
	switch {
	case runErr == nil:
		log.Default().Info(fmt.Sprintf("closing joint states subscriber to topic [%s]", in.TopicName))
		return nil
	case errors.Is(runErr, context.Canceled):
		log.Default().Info("context is canceled")
		return nil
	default:
		return status.Errorf(codes.Internal, runErr.Error())
	}
}

func (s *ROSInterfaceService) UploadJointCommands(stream grpc.ClientStreamingServer[routes.JointCommandRequest, routes.UploadResponse]) error {
	log.Default().Info("Received new client joint commands streaming request")
	if p, ok := peer.FromContext(stream.Context()); ok {
		log.Default().Info(fmt.Sprintf("Client connected from peer [%s]", p.Addr.String()))
	}

	var count uint64
	pub, err := std_msgs_msg.NewFloat64MultiArrayPublisher(ros_node.Node(), "hello", nil)
	if err != nil {
		wErr := errors.Wrapf(err, "failed to create multi array publisher")
		return status.Errorf(codes.Internal, wErr.Error())
	}
	defer func(pub *std_msgs_msg.Float64MultiArrayPublisher) {
		cErr := pub.Close()
		if cErr != nil && err == nil {
			err = status.Errorf(codes.Internal, cErr.Error())
		}
	}(pub)

	for {
		if err = stream.Context().Err(); err != nil {
			switch {
			case errors.Is(err, context.Canceled):
				return nil
			default:
				return status.Errorf(codes.Canceled, err.Error())
			}
		}

		msg, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&routes.UploadResponse{ReceivedCount: count})
		}
		if err != nil {
			wErr := errors.Wrapf(err, "failed to receive message")
			return status.Errorf(codes.Internal, wErr.Error())
		}

		dim := make([]std_msgs_msg.MultiArrayDimension, 0, len(msg.Payload.Layout.Dim))
		for _, arrDim := range msg.Payload.Layout.Dim {
			dim = append(dim, std_msgs_msg.MultiArrayDimension{
				Label:  arrDim.Label,
				Size:   arrDim.Size,
				Stride: arrDim.Stride,
			})
		}

		rosMsg := &std_msgs_msg.Float64MultiArray{
			Layout: std_msgs_msg.MultiArrayLayout{
				Dim:        dim,
				DataOffset: msg.Payload.Layout.DataOffset,
			},
			Data: msg.Payload.Data,
		}

		err = pub.Publish(rosMsg)
		if err != nil {
			wErr := errors.Wrapf(err, "failed to publish joint command message")
			return status.Errorf(codes.Internal, wErr.Error())
		}
		count++
	}
}
