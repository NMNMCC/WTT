// Package host implements the host-side logic of the WTT application.
// The host acts as the "answerer" in the WebRTC signaling process.
package host

import (
	"context"
	"encoding/json"

	"wtt/common"
	"wtt/common/rtc"
	"wtt/common/rtc/answerer"

	"github.com/pion/webrtc/v4"
)

// Run starts the host process.
// It connects to the signaling server, waits for an offer from a client,
// establishes a WebRTC connection, and then bridges a local port to the client.
func Run(ctx context.Context, signalingAddr, localAddr string, protocol common.Protocol) error {
	pcCfg := webrtc.Configuration{}
	pc, err := answerer.A_CreatePeerConnection(pcCfg)
	if err != nil {
		return err
	}
	defer pc.Close()

	// DataChannel is created by the remote peer (client), so we wait for it.
	dcCh := make(chan *webrtc.DataChannel, 1)
	pc.OnDataChannel(func(d *webrtc.DataChannel) {
		dcCh <- d
	})

	wsc, err := common.ConnectToWebSocketServer(ctx, signalingAddr)
	if err != nil {
		return err
	}
	defer wsc.Close()

	// FIXME: The host should first register with the signaling server using its ID.
	// Currently, it just waits for any offer, which is not secure or specific.

	// Wait for the offer from the client via the signaling server.
	var ofM common.RTCMessage[common.RTCOfferPayload]
	if err := wsc.ReadJSON(&ofM); err != nil {
		return err
	}

	// Set the received offer as the remote description.
	if err := answerer.B_SetOfferAsRemoteDescription(pc, ofM.Payload.SDP); err != nil {
		return err
	}

	// Create an answer to the offer.
	ansOpt := webrtc.AnswerOptions{}
	ans, err := answerer.C_CreateAnswer(pc, ansOpt)
	if err != nil {
		return err
	}

	// Set the created answer as the local description.
	if err := answerer.D_SetAnswerAsLocalDescription(pc, *ans); err != nil {
		return err
	}

	// Send the answer back to the client via the signaling server.
	ansM, err := json.Marshal(common.RTCMessage[common.RTCAnswerPayload]{
		Type: common.RTCAnswer,
		Payload: common.RTCAnswerPayload{
			SDP: *ans,
		},
	})
	if err != nil {
		return err
	}
	if err := rtc.SendSignal(wsc, ansM); err != nil {
		return err
	}

	// Wait for the DataChannel to be established.
	var dc *webrtc.DataChannel
	select {
	case dc = <-dcCh:
		// DataChannel is ready.
	case <-ctx.Done():
		return ctx.Err()
	}
	defer dc.Close()

	// Bridge the local connection with the WebRTC DataChannel.
	return common.Bridge(protocol, localAddr, dc)
}
