package client

import (
	"context"
	"log/slog"
	"wtt/common"
	"wtt/common/rtc"
	"wtt/common/rtc/offerer"

	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

func Run(ctx context.Context, serverAddr, hostID, localAddr string, protocol common.NetProtocol) <-chan error {
	slog.Info("client running")

	ec := make(chan error)

	pcCfg := webrtc.Configuration{}
	pc, err := offerer.A_CreatePeerConnection(pcCfg)
	if err != nil {
		ec <- err
	}
	defer pc.Close()

	id, err := uuid.NewRandom()
	if err != nil {
		ec <- err
	}
	dc, err := offerer.B_CreateDataChannel(pc, id.String())
	if err != nil {
		ec <- err
	}
	defer dc.Close()

	dcOpen := make(chan struct{}, 1)
	dc.OnOpen(func() { dcOpen <- struct{}{} })

	ofCfg := webrtc.OfferOptions{}
	of, err := offerer.C_CreateOffer(pc, ofCfg)
	if err != nil {
		ec <- err
	}

	slog.Info("setting local description")
	if err := offerer.D_SetOfferAsLocalDescription(pc, *of); err != nil {
		ec <- err
	}

	<-webrtc.GatheringCompletePromise(pc)
	ld := pc.LocalDescription()
	if ld == nil {
		ec <- webrtc.ErrConnectionClosed
		return ec
	}

	offerM := common.RTCOffer{
		HostID:             hostID,
		SessionDescription: *ld,
	}
	hc := resty.New()

	slog.Info("sending offer")
	rtc.SendSignal(hc, common.RTCOfferType, offerM)

	slog.Info("waiting for answer")
	answer, err := rtc.ReceiveSignal(hc, common.RTCAnswerType)
	if err != nil {
		ec <- err
	}
	slog.Info("setting remote description")
	if err := offerer.E_SetAnswerAsRemoteDescription(pc, answer); err != nil {
		ec <- err
	}

	slog.Info("waiting for data channel to open")
	select {
	case <-dcOpen:
		// ok
	case <-ctx.Done():
		ec <- ctx.Err()
	}

	slog.Info("start bridging", "protocol", protocol, "local", localAddr)
	return common.Merge(ec, common.Bridge(protocol, localAddr, dc))
}
