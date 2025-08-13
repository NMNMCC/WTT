package common

import "github.com/pion/webrtc/v4"

type NetProtocol string

const (
	TCP NetProtocol = "tcp"
	UDP NetProtocol = "udp"
)

type RTCEventType string

const (
	RTCRegisterType  RTCEventType = "register"
	RTCOfferType     RTCEventType = "offer"
	RTCAnswerType    RTCEventType = "answer"
	RTCCandidateType RTCEventType = "candidate"
)

type RTCRegister struct {
	HostID string `json:"host_id"`
}

type RTCOffer struct {
	HostID             string                    `json:"host_id"`
	SessionDescription webrtc.SessionDescription `json:"sdp"`
}

type RTCAnswer struct {
	HostID             string                    `json:"host_id"`
	SessionDescription webrtc.SessionDescription `json:"sdp"`
}

type RTCCandidate struct {
	HostID       string                  `json:"host_id"`
	ICECandidate webrtc.ICECandidateInit `json:"candidate"`
}

type IDMessage[P any] struct {
	MessageID string `json:"message_id"`
	Payload   P      `json:"payload"`
}

type RTCEventPayload interface {
	RTCRegister | RTCOffer | RTCAnswer | RTCCandidate | struct{}
}
