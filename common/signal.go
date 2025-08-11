package common

import (
	"encoding/json"
	"fmt"

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

type Message[P Payload] struct {
	Type     MessageType `json:"type"`
	Payload  P           `json:"payload"`
	TargetID string      `json:"target_id"`
	SenderID string      `json:"sender_id"`
}

type Payload interface {
	OfferPayload | AnswerPayload | CandidatePayload | any
}

func ReUnmarshal[T any](j any) (T, error) {
	var payload T
	b, err := json.Marshal(j)
	if err != nil {
		return payload, fmt.Errorf("failed to marshal json: %w", err)
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		return payload, fmt.Errorf("failed to unmarshal json into %T: %w", payload, err)
	}
	return payload, nil
}
