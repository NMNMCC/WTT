package host

import (
	"context"
	"encoding/json"

	"wtt/common"
	"wtt/common/rtc"
	"wtt/common/rtc/answerer"

	"github.com/pion/webrtc/v4"
)

// Run 启动主机侧流程。
// signalingAddr: 信令服务器地址（ws/wss）。
// localAddr: 本地转发地址。
// protocol: 传输协议（tcp/udp）。
func Run(ctx context.Context, signalingAddr, localAddr string, protocol common.Protocol) error {
	pcCfg := webrtc.Configuration{}
	pc, err := answerer.A_CreatePeerConnection(pcCfg)
	if err != nil {
		return err
	}
	defer pc.Close()

	// Wait for remote-created DataChannel
	dcCh := make(chan *webrtc.DataChannel, 1)
	pc.OnDataChannel(func(d *webrtc.DataChannel) {
		dcCh <- d
	})

	wsc, err := common.ConnectToWebSocketServer(ctx, signalingAddr)
	if err != nil {
		return err
	}
	defer wsc.Close()

	var ofM common.RTCMessage[common.RTCOfferPayload]
	if err := wsc.ReadJSON(&ofM); err != nil {
		return err
	}

	if err := answerer.B_SetOfferAsRemoteDescription(pc, ofM.Payload.SDP); err != nil {
		return err
	}

	ansOpt := webrtc.AnswerOptions{}
	ans, err := answerer.C_CreateAnswer(pc, ansOpt)
	if err != nil {
		return err
	}
	if err := answerer.D_SetAnswerAsLocalDescription(pc, *ans); err != nil {
		return err
	}

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

	var dc *webrtc.DataChannel
	select {
	case dc = <-dcCh:
		// ok
	case <-ctx.Done():
		return ctx.Err()
	}
	defer dc.Close()

	return common.Bridge(protocol, localAddr, dc)
}
