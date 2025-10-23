package services

import (
	"context"

	"github.com/okieraised/monitoring-agent/internal/infrastructure/log"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/ros_node"
	vr_common_msgs_srv "github.com/okieraised/monitoring-agent/internal/ros_msgs/vr_common_msgs/srv"
	"github.com/okieraised/monitoring-agent/internal/server/grpc_server/routes"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type RobotActionService struct {
	routes.UnimplementedRobotActionInterfaceServer
}

func (s *RobotActionService) SetAction(ctx context.Context, in *routes.SetActionRequest) (*routes.SetActionResponse, error) {

	req := vr_common_msgs_srv.SetRobotAction_Request{
		ActionId:  in.ActionId,
		Duration:  in.DurationMs,
		Overwrite: in.Overwrite,
	}

	client, err := vr_common_msgs_srv.NewSetRobotActionClient(ros_node.Node(), "/set_robot_action", nil)
	if err != nil {
		wErr := errors.Wrap(err, "failed to create set action client")
		log.Default().Error(wErr.Error())
		return &routes.SetActionResponse{}, status.Errorf(codes.Internal, wErr.Error())
	}

	resp, _, err := client.Send(ctx, &req)
	if err != nil {
		wErr := errors.Wrap(err, "failed to send set action request")
		log.Default().Error(wErr.Error())
		return &routes.SetActionResponse{}, status.Errorf(codes.Internal, wErr.Error())
	}

	return &routes.SetActionResponse{
		Error:           routes.ActionStatus(resp.ErrorCode),
		CurrentActionId: resp.CurrentActionId,
		RemainingMs:     resp.RemainingMs,
	}, nil
}

func (s *RobotActionService) SetMode(ctx context.Context, in *routes.SetModeRequest) (*routes.SetModeResponse, error) {

	req := vr_common_msgs_srv.SetRobotActionMode_Request{Mode: in.Mode}

	client, err := vr_common_msgs_srv.NewSetRobotActionModeClient(ros_node.Node(), "/set_robot_action", nil)
	if err != nil {
		wErr := errors.Wrap(err, "failed to create set action mode client")
		log.Default().Error(wErr.Error())
		return &routes.SetModeResponse{}, status.Errorf(codes.Internal, wErr.Error())
	}

	resp, _, err := client.Send(ctx, &req)
	if err != nil {
		wErr := errors.Wrap(err, "failed to send set action mode request")
		log.Default().Error(wErr.Error())
		return &routes.SetModeResponse{}, status.Errorf(codes.Internal, wErr.Error())
	}

	return &routes.SetModeResponse{
		Success: resp.Success,
		Log:     resp.Log,
	}, nil
}

func (s *RobotActionService) PlaybackAction(ctx context.Context, in *routes.ActionPlaybackRequest) (*routes.ActionPlaybackResponse, error) {
	req := vr_common_msgs_srv.ActionPlaybackCommand_Request{
		ActionId:   in.ActionId,
		DurationMs: in.DurationMs,
		DelayMs:    in.DelayMs,
	}

	client, err := vr_common_msgs_srv.NewActionPlaybackCommandClient(ros_node.Node(), "/set_robot_action", nil)
	if err != nil {
		wErr := errors.Wrap(err, "failed to create playback action client")
		log.Default().Error(wErr.Error())
		return &routes.ActionPlaybackResponse{}, status.Errorf(codes.Internal, wErr.Error())
	}

	resp, _, err := client.Send(ctx, &req)
	if err != nil {
		wErr := errors.Wrap(err, "failed to send playback action request")
		log.Default().Error(wErr.Error())
		return &routes.ActionPlaybackResponse{}, status.Errorf(codes.Internal, wErr.Error())
	}

	return &routes.ActionPlaybackResponse{
		Success:  resp.Success,
		ActionFb: resp.ActionFb,
	}, nil
}
