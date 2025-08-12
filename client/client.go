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

// Run 启动客户端流程。
// serverAddr: 信令服务器地址（ws/wss）。
// hostID: 目标主机 ID。
// localAddr: 本地转发地址。
// protocol: 传输协议（tcp/udp）。
func Run(ctx context.Context, serverAddr, hostID, localAddr string, protocol common.Protocol) error {
	pcCfg := webrtc.Configuration{}
	pc, err := offerer.A_CreatePeerConnection(pcCfg)
	if err != nil {
		return err
	}
	defer pc.Close()

	id, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	dc, err := offerer.B_CreateDataChannel(pc, id.String())
	if err != nil {
		return err
	}
	defer dc.Close()

	ofCfg := webrtc.OfferOptions{}
	of, err := offerer.C_CreateOffer(pc, ofCfg)
	if err != nil {
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
		return err
	}
	wsc, err := common.ConnectToWebSocketServer(ctx, serverAddr)
	if err != nil {
		return err
	}
	defer wsc.Close()
	rtc.SendSignal(wsc, ofM)

	var ansM common.RTCMessage[common.RTCAnswerPayload]
	if err := wsc.ReadJSON(&ansM); err != nil {
		return err
	}
	offerer.D_SetOfferAsLocalDescription(pc, ansM.Payload.SDP)

	return common.Bridge(protocol, localAddr, dc)
}
