package typ

import (
	"encoding/json"

	"github.com/pion/webrtc/v4"
)

type PeerType string

const (
	PeerTypeService  PeerType = "service"
	PeerTypeConsumer PeerType = "consumer"
)

type ID struct {
	ServiceID  string
	ConsumerID string
}

type Envelope struct {
	ID
	Type    EnvelopeType
	Payload json.RawMessage
}

type EnvelopeType string

const (
	EnvelopeTypeRTCOffer     EnvelopeType = "rtc_offer"
	EnvelopeTypeRTCAnswerOK  EnvelopeType = "rtc_answer_ok"
	EnvelopeTypeRTCAnswerNO  EnvelopeType = "rtc_answer_no"
	EnvelopeTypeICECandidate EnvelopeType = "ice_candidate"
)

type RTCOffer struct {
	SessionDescription webrtc.SessionDescription
}

type RTCAnswerOK struct {
	SessionDescription webrtc.SessionDescription
}

type RTCAnswerNO struct {
	Reason string
}

type ICECandidate struct {
	Candidate webrtc.ICECandidate
}
