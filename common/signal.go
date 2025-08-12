package common

import (
	"github.com/pion/webrtc/v4"
)

type RTCMessageType string

const (
	RTCRegister  RTCMessageType = "register"
	RTCOffer     RTCMessageType = "offer"
	RTCAnswer    RTCMessageType = "answer"
	RTCCandidate RTCMessageType = "candidate"
)

type RTCRegisterPayload struct {
	ID string `json:"id"`
}

type RTCOfferPayload struct {
	SDP webrtc.SessionDescription `json:"sdp"`
}

type RTCAnswerPayload struct {
	SDP webrtc.SessionDescription `json:"sdp"`
}

type RTCCandidatePayload struct {
	Candidate webrtc.ICECandidateInit `json:"candidate"`
}

type RTCMessage[P RTCPayload] struct {
	Type    RTCMessageType `json:"type"`
	Payload P              `json:"payload"`
}

type RTCPayload interface {
	RTCRegisterPayload | RTCOfferPayload | RTCAnswerPayload | RTCCandidatePayload | any
}
