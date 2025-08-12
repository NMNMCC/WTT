// Package answerer provides a step-by-step guide for setting up the answering side of a WebRTC connection.
// The functions are prefixed with letters (A, B, C, ...) to indicate the required order of execution.
package answerer

import (
	wr "wtt/common/rtc"

	"github.com/pion/webrtc/v4"
)

// A_CreatePeerConnection creates a new WebRTC peer connection.
// It is the first step in the WebRTC setup process for the answerer.
var A_CreatePeerConnection = wr.CreatePeerConnection

// B_SetOfferAsRemoteDescription sets the offer received from the signaling server as the remote description.
// This is the second step for the answerer.
var B_SetOfferAsRemoteDescription = wr.SetRemoteDescription

// C_CreateAnswer creates an SDP answer to the received offer.
// This answer must be sent back to the offering peer through the signaling channel.
func C_CreateAnswer(pc *webrtc.PeerConnection, cfg webrtc.AnswerOptions) (*webrtc.SessionDescription, error) {
	answer, err := pc.CreateAnswer(&cfg)
	if err != nil {
		return nil, err
	}
	return &answer, nil
}

// D_SetAnswerAsLocalDescription sets the generated answer as the local description.
// This is the final step for the answerer.
var D_SetAnswerAsLocalDescription = wr.SetLocalDescription
