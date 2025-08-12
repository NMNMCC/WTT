// Package offerer provides a step-by-step guide for setting up the offering side of a WebRTC connection.
// The functions are prefixed with letters (A, B, C, ...) to indicate the required order of execution.
package offerer

import (
	wr "wtt/common/rtc"

	"github.com/pion/webrtc/v4"
)

// A_CreatePeerConnection creates a new WebRTC peer connection.
// It is the first step in the WebRTC setup process for the offerer.
var A_CreatePeerConnection = wr.CreatePeerConnection

// B_CreateDataChannel creates a new data channel for communication.
// This is the second step.
var B_CreateDataChannel = wr.CreateDataChannel

// C_CreateOffer creates an SDP offer.
// This offer must be sent to the answering peer through a signaling channel.
func C_CreateOffer(pc *webrtc.PeerConnection, cfg webrtc.OfferOptions) (*webrtc.SessionDescription, error) {
	offer, err := pc.CreateOffer(&cfg)
	if err != nil {
		return nil, err
	}
	return &offer, nil
}

// D_SetOfferAsLocalDescription sets the generated offer as the local description.
// This is the final step for the offerer before waiting for the answerer's response.
var D_SetOfferAsLocalDescription = wr.SetLocalDescription
