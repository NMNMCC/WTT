package host

import (
	"context"
	"encoding/json"
	"log/slog"

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
	slog.Info("host running")
	pcCfg := webrtc.Configuration{}
	slog.Debug("creating peer connection")
	pc, err := answerer.A_CreatePeerConnection(pcCfg)
	if err != nil {
		slog.Error("create peer connection error", "err", err)
		return err
	}
	defer pc.Close()

	// Wait for remote-created DataChannel
	dcCh := make(chan *webrtc.DataChannel, 1)
	pc.OnDataChannel(func(d *webrtc.DataChannel) {
		slog.Info("data channel created", "label", d.Label())
		dcCh <- d
	})

	slog.Info("connecting to websocket server", "addr", signalingAddr)
	wsc, err := common.ConnectToWebSocketServer(ctx, signalingAddr)
	if err != nil {
		slog.Error("connect to websocket server error", "err", err)
		return err
	}
	defer wsc.Close()

	slog.Info("waiting for offer")
	var ofM common.RTCMessage[common.RTCOfferPayload]
	if err := wsc.ReadJSON(&ofM); err != nil {
		slog.Error("read offer error", "err", err)
		return err
	}

	slog.Info("setting remote description")
	if err := answerer.B_SetOfferAsRemoteDescription(pc, ofM.Payload.SDP); err != nil {
		slog.Error("set remote description error", "err", err)
		return err
	}

	ansOpt := webrtc.AnswerOptions{}
	slog.Debug("creating answer")
	ans, err := answerer.C_CreateAnswer(pc, ansOpt)
	if err != nil {
		slog.Error("create answer error", "err", err)
		return err
	}
	slog.Info("setting local description")
	if err := answerer.D_SetAnswerAsLocalDescription(pc, *ans); err != nil {
		slog.Error("set local description error", "err", err)
		return err
	}

	ansM, err := json.Marshal(common.RTCMessage[common.RTCAnswerPayload]{
		Type: common.RTCAnswer,
		Payload: common.RTCAnswerPayload{
			SDP: *ans,
		},
	})
	if err != nil {
		slog.Error("marshal answer error", "err", err)
		return err
	}
	slog.Info("sending answer")
	if err := rtc.SendSignal(wsc, ansM); err != nil {
		slog.Error("send answer error", "err", err)
		return err
	}

	slog.Info("waiting for data channel")
	var dc *webrtc.DataChannel
	select {
	case dc = <-dcCh:
		// ok
	case <-ctx.Done():
		return ctx.Err()
	}
	defer dc.Close()

	slog.Info("start bridging", "protocol", protocol, "local", localAddr)
	return common.Bridge(protocol, localAddr, dc)
}
