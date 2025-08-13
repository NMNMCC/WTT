package host

import (
	"context"
	"log/slog"

	"wtt/common"
	"wtt/common/rtc"
	"wtt/common/rtc/answerer"

	"github.com/go-resty/resty/v2"
	"github.com/pion/webrtc/v4"
)

func Run(ctx context.Context, id, signalingAddr, localAddr string, protocol common.NetProtocol) <-chan error {
	slog.Info("host running")

	ec := make(chan error)

	pcCfg := webrtc.Configuration{}
	slog.Debug("creating peer connection")
	pc, err := answerer.A_CreatePeerConnection(pcCfg)
	if err != nil {
		slog.Error("create peer connection error", "err", err)
		ec <- err
		return ec
	}
	defer pc.Close()

	dcCh := make(chan *webrtc.DataChannel, 1)
	pc.OnDataChannel(func(d *webrtc.DataChannel) {
		slog.Info("data channel created", "label", d.Label())
		dcCh <- d
	})

	slog.Info("connecting to websocket server", "addr", signalingAddr)

	hc := resty.New().SetBaseURL(signalingAddr)
	if err := rtc.RegisterHost(hc, id); err != nil {
		slog.Error("register host error", "err", err)
		ec <- err
		return ec
	}

	slog.Info("waiting for offer")
	offer, err := rtc.ReceiveSignal(hc, common.RTCOfferType)
	if err != nil {
		slog.Error("receive offer error", "err", err)
		ec <- err
		return ec
	}

	slog.Info("setting remote description")
	if err := answerer.B_SetOfferAsRemoteDescription(pc, offer); err != nil {
		slog.Error("set remote description error", "err", err)
		ec <- err
		return ec
	}

	ansOpt := webrtc.AnswerOptions{}
	slog.Debug("creating answer")
	ans, err := answerer.C_CreateAnswer(pc, ansOpt)
	if err != nil {
		slog.Error("create answer error", "err", err)
		ec <- err
		return ec
	}
	slog.Info("setting local description")
	if err := answerer.D_SetAnswerAsLocalDescription(pc, *ans); err != nil {
		slog.Error("set local description error", "err", err)
		ec <- err
		return ec
	}

	<-webrtc.GatheringCompletePromise(pc)
	ld := pc.LocalDescription()
	if ld == nil {
		slog.Error("local description is nil after gathering")
		ec <- webrtc.ErrConnectionClosed
		return ec
	}

	ansM := common.RTCAnswer{
		HostID:             id,
		SessionDescription: *ld,
	}
	slog.Info("sending answer")
	if err := rtc.SendSignal(hc, common.RTCAnswerType, ansM); err != nil {
		slog.Error("send answer error", "err", err)
		ec <- err
		return ec
	}

	slog.Info("waiting for data channel")
	var dc *webrtc.DataChannel
	select {
	case dc = <-dcCh:
		// ok
	case <-ctx.Done():
		ec <- ctx.Err()
		return ec
	}
	defer dc.Close()

	dcOpen := make(chan struct{}, 1)
	dc.OnOpen(func() { dcOpen <- struct{}{} })
	slog.Info("waiting for data channel to open")
	select {
	case <-dcOpen:
		// ok
	case <-ctx.Done():
		ec <- ctx.Err()
		return ec
	}

	slog.Info("start bridging", "protocol", protocol, "local", localAddr)
	return common.Merge(ec, common.Bridge(protocol, localAddr, dc))
}
