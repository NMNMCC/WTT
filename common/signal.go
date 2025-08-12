// Package common contains shared data structures and utility functions for the WTT application.
package common

import (
	"github.com/pion/webrtc/v4"
)

// RTCMessageType defines the type of a WebRTC signaling message.
type RTCMessageType string

// Constants for the different types of WebRTC signaling messages.
const (
	RTCRegister  RTCMessageType = "register"  // Message to register a host with the signaling server.
	RTCOffer     RTCMessageType = "offer"     // Message containing an SDP offer.
	RTCAnswer    RTCMessageType = "answer"    // Message containing an SDP answer.
	RTCCandidate RTCMessageType = "candidate" // Message containing an ICE candidate.
)

// RTCRegisterPayload is the payload for a register message.
type RTCRegisterPayload struct {
	ID string `json:"id"` // The ID of the host to register.
}

// RTCOfferPayload is the payload for an offer message.
type RTCOfferPayload struct {
	SDP webrtc.SessionDescription `json:"sdp"` // The SDP offer.
}

// RTCAnswerPayload is the payload for an answer message.
type RTCAnswerPayload struct {
	SDP webrtc.SessionDescription `json:"sdp"` // The SDP answer.
}

// RTCCandidatePayload is the payload for a candidate message.
type RTCCandidatePayload struct {
	Candidate webrtc.ICECandidateInit `json:"candidate"` // The ICE candidate.
}

// RTCMessage is a generic container for a WebRTC signaling message.
// It includes the message type and a generic payload.
type RTCMessage[P RTCPayload] struct {
	Type    RTCMessageType `json:"type"`    // The type of the message.
	Payload P              `json:"payload"` // The message payload.
}

// RTCPayload is a constraint that defines the possible types for the payload of an RTCMessage.
type RTCPayload interface {
	RTCRegisterPayload | RTCOfferPayload | RTCAnswerPayload | RTCCandidatePayload | any
}
