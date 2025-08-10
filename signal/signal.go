package signal

import "github.com/pion/webrtc/v3"

// Message is the structure used for all signaling communication.
type Message struct {
	Type      string      `json:"type"`
	Payload   interface{} `json:"payload"`
	TargetID  string      `json:"target_id,omitempty"`
	SenderID  string      `json:"sender_id,omitempty"`
}

// OfferPayload is the payload for an offer message.
type OfferPayload struct {
	SDP webrtc.SessionDescription `json:"sdp"`
}

// AnswerPayload is the payload for an answer message.
type AnswerPayload struct {
	SDP webrtc.SessionDescription `json:"sdp"`
}

// CandidatePayload is the payload for an ICE candidate message.
type CandidatePayload struct {
	Candidate webrtc.ICECandidateInit `json:"candidate"`
}

// Constants for message types
const (
	MessageTypeRegisterHost = "register-host"
	MessageTypeConnectClient = "connect-client"
	MessageTypeOffer        = "offer"
	MessageTypeAnswer       = "answer"
	MessageTypeCandidate    = "candidate"
	MessageTypeError        = "error"
)
