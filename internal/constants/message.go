package constants

type MessageType string

const (
	MsgTypeOffer      MessageType = "Offer"
	MsgTypeAnswer     MessageType = "Answer"
	MsgTypeWebRTCInit MessageType = "WebRTCInit"
)

type MessageHeaderType string

const (
	MsgHeaderTypeWebRTC = "webrtc"
)
