package common

import (
	"time"

	"github.com/google/uuid"
)

type SignalingMessage struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type WebRTCSDPExchangeMsg struct {
	HeaderID  int       `json:"headerId"`
	Timestamp time.Time `json:"timestamp"`
	AgentID   string    `json:"agentId"`
}

type MQTTWebRTCSDPOfferMsg struct {
	HeaderID  int               `json:"headerId"`
	Timestamp time.Time         `json:"timestamp"`
	AgentID   string            `json:"agentId"`
	State     WebRTCSDPOfferMsg `json:"state"`
}

type WebRTCSDPOfferMsg struct {
	ID       uuid.UUID `json:"id"`
	Type     string    `json:"type"`
	ROSTopic string    `json:"ros_topic"`
	Offer    string    `json:"offer"`
}

type MQTTWebRTCSDPAnswerMsg struct {
	HeaderID  int                `json:"headerId"`
	Timestamp time.Time          `json:"timestamp"`
	AgentID   string             `json:"agentId"`
	RobotID   string             `json:"robotId"`
	State     WebRTCSDPAnswerMsg `json:"state"`
}

type WebRTCSDPAnswerMsg struct {
	ID       uuid.UUID `json:"id"`
	ROSTopic string    `json:"ros_topic"`
	Answer   string    `json:"answer"`
}
