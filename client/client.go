// Package client implements the client-side logic of the WTT application.
// The client acts as the "offerer" in the WebRTC signaling process.
package client

import (
	"context"
	"encoding/json"
	"wtt/common"
	"wtt/common/rtc"
	"wtt/common/rtc/offerer"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

// Run starts the client process.
// It creates a WebRTC connection to a host via a signaling server,
// and then bridges a local port to the host.
func Run(ctx context.Context, serverAddr, hostID, localAddr string, protocol common.Protocol) error {
	// Create a new WebRTC peer connection.
	pcCfg := webrtc.Configuration{}
	pc, err := offerer.A_CreatePeerConnection(pcCfg)
	if err != nil {
		return err
	}
	defer pc.Close()

	// Create a new DataChannel. The label is a random UUID.
	id, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	dc, err := offerer.B_CreateDataChannel(pc, id.String())
	if err != nil {
		return err
	}
	defer dc.Close()

	// Create an SDP offer.
	ofCfg := webrtc.OfferOptions{}
	of, err := offerer.C_CreateOffer(pc, ofCfg)
	if err != nil {
		return err
	}

	// Create the offer message to be sent to the signaling server.
	// This message is wrapped in a forward container to be routed to the specified host.
	ofM, err := json.Marshal(
		common.WebSocketForwardMessageContainer[common.RTCMessage[common.RTCOfferPayload]]{
			Type:       common.MessageTypeForward,
			ReceiverID: hostID,
			Content: common.RTCMessage[common.RTCOfferPayload]{
				Type: common.RTCOffer,
				Payload: common.RTCOfferPayload{
					SDP: *of,
				},
			},
		})
	if err != nil {
		return err
	}

	// Connect to the signaling server and send the offer.
	wsc, err := common.ConnectToWebSocketServer(ctx, serverAddr)
	if err != nil {
		return err
	}
	defer wsc.Close()
	if err := rtc.SendSignal(wsc, ofM); err != nil {
		return err
	}

	// Wait for the answer from the host via the signaling server.
	var ansM common.RTCMessage[common.RTCAnswerPayload]
	if err := wsc.ReadJSON(&ansM); err != nil {
		return err
	}

	// Set the received answer as the remote description.
	if err := offerer.D_SetOfferAsLocalDescription(pc, ansM.Payload.SDP); err != nil {
		return err
	}

	// Bridge the local connection with the WebRTC DataChannel.
	return common.Bridge(protocol, localAddr, dc)
}
