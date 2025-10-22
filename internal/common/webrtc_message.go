package common

import (
	"time"

	"github.com/google/uuid"
	"github.com/okieraised/monitoring-agent/internal/constants"
	"github.com/pkg/errors"
)

type SignalingMessage struct {
	Header  Header        `json:"header"`
	Payload SignalingBody `json:"payload"`
}

// Header follows VDA5050-like metadata.
type Header struct {
	HeaderID     int64     `json:"headerId"`          // monotonic increasing
	Version      string    `json:"version"`           // message version, e.g. "1.0.0"
	Manufacturer string    `json:"manufacturer"`      // who created the message
	SerialNumber string    `json:"serialNumber"`      // robot or agent serial
	AgentID      string    `json:"agentId,omitempty"` // unique ID of the agent
	RobotID      string    `json:"robotId,omitempty"` // unique ID of the robot
	Timestamp    time.Time `json:"timestamp"`         // ISO 8601 timestamp
	MessageType  string    `json:"messageType"`       // "Signaling" / "Negotiation" / "Status"
}

// SignalingBody contains the actual SDP / ICE data and negotiation details.
type SignalingBody struct {
	TransactionID uuid.UUID             `json:"transactionId"`  // unique negotiation session
	Type          constants.MessageType `json:"type"`           // Offer, Answer, ICE
	TopicName     string                `json:"topicName"`      // ROS2 topic name
	Session       string                `json:"session"`        // optional session name / topic
	SDP           string                `json:"sdp,omitempty"`  // base64-encoded SDP
	Meta          map[string]any        `json:"meta,omitempty"` // optional extra fields (network, transport info)
}

func (req *SignalingMessage) ValidateWebRTC() error {
	if req.Payload.TransactionID == uuid.Nil {
		return errors.New("invalid/missing transaction id")
	}

	if req.Header.MessageType != constants.MsgHeaderTypeWebRTC {
		return errors.Errorf("invalid message type: %s", req.Header.MessageType)
	}

	if req.Payload.Type == constants.MsgTypeWebRTCInit || req.Payload.Type == constants.MsgTypeOffer {
		if req.Payload.TopicName == "" {
			return errors.New("ros topic name is required")
		}
	}

	if req.Payload.Type == constants.MsgTypeOffer && req.Payload.SDP == "" {
		return errors.New("peer offer's session description is required")
	}

	if req.Payload.Type == constants.MsgTypeAnswer && req.Payload.SDP == "" {
		return errors.New("peer answer's session description is required")
	}

	return nil
}
