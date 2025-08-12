package client

import (
	"context"
	"encoding/json"
	"log/slog"
	"wtt/common"
	"wtt/common/rtc"
	"wtt/common/rtc/offerer"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

// Run 启动客户端流程。
// serverAddr: 信令服务器地址（ws/wss）。
// hostID: 目标主机 ID。
// localAddr: 本地转发地址。
// protocol: 传输协议（tcp/udp）。
func Run(ctx context.Context, serverAddr, hostID, localAddr string, protocol common.Protocol) error {
	slog.Info("client running")
	pcCfg := webrtc.Configuration{}
	slog.Debug("creating peer connection")
	pc, err := offerer.A_CreatePeerConnection(pcCfg)
	if err != nil {
		slog.Error("create peer connection error", "err", err)
		return err
	}
	defer pc.Close()

	id, err := uuid.NewRandom()
	if err != nil {
		slog.Error("generate uuid error", "err", err)
		return err
	}
	slog.Debug("creating data channel")
	dc, err := offerer.B_CreateDataChannel(pc, id.String())
	if err != nil {
		slog.Error("create data channel error", "err", err)
		return err
	}
	defer dc.Close()

	ofCfg := webrtc.OfferOptions{}
	slog.Debug("creating offer")
	of, err := offerer.C_CreateOffer(pc, ofCfg)
	if err != nil {
		slog.Error("create offer error", "err", err)
		return err
	}
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
		slog.Error("marshal offer error", "err", err)
		return err
	}
	slog.Info("connecting to websocket server", "addr", serverAddr)
	wsc, err := common.ConnectToWebSocketServer(ctx, serverAddr)
	if err != nil {
		slog.Error("connect to websocket server error", "err", err)
		return err
	}
	defer wsc.Close()
	slog.Info("sending offer")
	rtc.SendSignal(wsc, ofM)

	slog.Info("waiting for answer")
	var ansM common.RTCMessage[common.RTCAnswerPayload]
	if err := wsc.ReadJSON(&ansM); err != nil {
		slog.Error("read answer error", "err", err)
		return err
	}
	slog.Info("setting remote description")
	offerer.D_SetOfferAsLocalDescription(pc, ansM.Payload.SDP)

	slog.Info("start bridging", "protocol", protocol, "local", localAddr)
	return common.Bridge(protocol, localAddr, dc)
}
