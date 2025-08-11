package common

import (
	"encoding/json"
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestProtocolConstants(t *testing.T) {
	tests := []struct {
		protocol Protocol
		expected string
	}{
		{TCP, "tcp"},
		{UDP, "udp"},
	}

	for _, tt := range tests {
		if string(tt.protocol) != tt.expected {
			t.Errorf("Protocol %v should equal %s, got %s", tt.protocol, tt.expected, string(tt.protocol))
		}
	}
}

func TestTunnelStruct(t *testing.T) {
	tunnel := Tunnel{
		Initiator:  "client-123",
		Respondent: "host-456",
	}

	if tunnel.Initiator != "client-123" {
		t.Errorf("Expected Initiator to be 'client-123', got %s", tunnel.Initiator)
	}
	if tunnel.Respondent != "host-456" {
		t.Errorf("Expected Respondent to be 'host-456', got %s", tunnel.Respondent)
	}
}

func TestMessageTypes(t *testing.T) {
	tests := []struct {
		msgType  MessageType
		expected string
	}{
		{Register, "register"},
		{Offer, "offer"},
		{Answer, "answer"},
		{Candidate, "candidate"},
	}

	for _, tt := range tests {
		if string(tt.msgType) != tt.expected {
			t.Errorf("MessageType %v should equal %s, got %s", tt.msgType, tt.expected, string(tt.msgType))
		}
	}
}

func TestOfferPayload(t *testing.T) {
	sdp := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  "v=0\r\no=- 123 456 IN IP4 127.0.0.1\r\n",
	}

	payload := OfferPayload{SDP: sdp}

	if payload.SDP.Type != webrtc.SDPTypeOffer {
		t.Errorf("Expected SDP type to be offer, got %v", payload.SDP.Type)
	}
	if payload.SDP.SDP != sdp.SDP {
		t.Errorf("Expected SDP content to match, got %s", payload.SDP.SDP)
	}
}

func TestAnswerPayload(t *testing.T) {
	sdp := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  "v=0\r\no=- 789 012 IN IP4 127.0.0.1\r\n",
	}

	payload := AnswerPayload{SDP: sdp}

	if payload.SDP.Type != webrtc.SDPTypeAnswer {
		t.Errorf("Expected SDP type to be answer, got %v", payload.SDP.Type)
	}
	if payload.SDP.SDP != sdp.SDP {
		t.Errorf("Expected SDP content to match, got %s", payload.SDP.SDP)
	}
}

func TestCandidatePayload(t *testing.T) {
	candidate := webrtc.ICECandidateInit{
		Candidate: "candidate:1 1 UDP 2113667326 192.168.1.100 54400 typ host",
	}

	payload := CandidatePayload{Candidate: candidate}

	if payload.Candidate.Candidate != candidate.Candidate {
		t.Errorf("Expected candidate to match, got %s", payload.Candidate.Candidate)
	}
}

func TestMessage(t *testing.T) {
	offerPayload := OfferPayload{
		SDP: webrtc.SessionDescription{
			Type: webrtc.SDPTypeOffer,
			SDP:  "v=0\r\no=- 123 456 IN IP4 127.0.0.1\r\n",
		},
	}

	msg := Message[OfferPayload]{
		Type:     Offer,
		Payload:  offerPayload,
		TargetID: "host-123",
		SenderID: "client-456",
	}

	if msg.Type != Offer {
		t.Errorf("Expected message type to be offer, got %v", msg.Type)
	}
	if msg.TargetID != "host-123" {
		t.Errorf("Expected target ID to be 'host-123', got %s", msg.TargetID)
	}
	if msg.SenderID != "client-456" {
		t.Errorf("Expected sender ID to be 'client-456', got %s", msg.SenderID)
	}
	if msg.Payload.SDP.Type != webrtc.SDPTypeOffer {
		t.Errorf("Expected payload SDP type to be offer, got %v", msg.Payload.SDP.Type)
	}
}

func TestReUnmarshal(t *testing.T) {
	// Test successful unmarshaling
	original := map[string]interface{}{
		"type":      "offer",
		"target_id": "host-123",
		"sender_id": "client-456",
		"payload": map[string]interface{}{
			"sdp": map[string]interface{}{
				"type": "offer",
				"sdp":  "v=0\r\no=- 123 456 IN IP4 127.0.0.1\r\n",
			},
		},
	}

	result, err := ReUnmarshal[Message[OfferPayload]](original)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.Type != "offer" {
		t.Errorf("Expected type to be 'offer', got %s", result.Type)
	}
	if result.TargetID != "host-123" {
		t.Errorf("Expected target ID to be 'host-123', got %s", result.TargetID)
	}

	// Test with invalid data that can't be marshaled
	invalidData := make(chan int) // channels can't be marshaled to JSON
	_, err = ReUnmarshal[Message[OfferPayload]](invalidData)
	if err == nil {
		t.Error("Expected error when marshaling invalid data, got nil")
	}
}

func TestMessageJSONSerialization(t *testing.T) {
	// Test that Message can be properly serialized and deserialized
	msg := Message[OfferPayload]{
		Type: Offer,
		Payload: OfferPayload{
			SDP: webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  "v=0\r\no=- 123 456 IN IP4 127.0.0.1\r\n",
			},
		},
		TargetID: "host-123",
		SenderID: "client-456",
	}

	// Serialize to JSON
	jsonData, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal message: %v", err)
	}

	// Deserialize from JSON
	var deserialized Message[OfferPayload]
	err = json.Unmarshal(jsonData, &deserialized)
	if err != nil {
		t.Fatalf("Failed to unmarshal message: %v", err)
	}

	// Verify the deserialized data matches the original
	if deserialized.Type != msg.Type {
		t.Errorf("Expected type %v, got %v", msg.Type, deserialized.Type)
	}
	if deserialized.TargetID != msg.TargetID {
		t.Errorf("Expected target ID %s, got %s", msg.TargetID, deserialized.TargetID)
	}
	if deserialized.SenderID != msg.SenderID {
		t.Errorf("Expected sender ID %s, got %s", msg.SenderID, deserialized.SenderID)
	}
	if deserialized.Payload.SDP.Type != msg.Payload.SDP.Type {
		t.Errorf("Expected SDP type %v, got %v", msg.Payload.SDP.Type, deserialized.Payload.SDP.Type)
	}
}