package common

import (
	"encoding/json"

	"github.com/pion/webrtc/v4"
)

type MessageType string

const (
	Register  MessageType = "register"
	Offer     MessageType = "offer"
	Answer    MessageType = "answer"
	Candidate MessageType = "candidate"
)

type OfferPayload struct {
	SDP webrtc.SessionDescription `json:"sdp"`
}

type AnswerPayload struct {
	SDP webrtc.SessionDescription `json:"sdp"`
}

type CandidatePayload struct {
	Candidate webrtc.ICECandidateInit `json:"candidate"`
}

// Message is used for decoding incoming messages, where the payload type is unknown.
type Message struct {
	Type     MessageType     `json:"type"`
	Payload  json.RawMessage `json:"payload"`
	TargetID string          `json:"target_id"`
	SenderID string          `json:"sender_id"`
}

// TypedMessage is used for encoding outgoing messages, where the payload type is known.
type TypedMessage[P any] struct {
	Type     MessageType `json:"type"`
	Payload  P           `json:"payload"`
	TargetID string      `json:"target_id"`
	SenderID string      `json:"sender_id"`
}
