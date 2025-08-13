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
	ec := make(chan error)

	go func() {
		pcCfg := webrtc.Configuration{}
		pc, err := offerer.A_CreatePeerConnection(pcCfg)
		if err != nil {
			ec <- err
			return
		}
		defer pc.Close()

		id, err := uuid.NewRandom()
		if err != nil {
			ec <- err
			return
		}
		dc, err := offerer.B_CreateDataChannel(pc, id.String())
		if err != nil {
			ec <- err
			return
		}
		defer dc.Close()

		dcOpen := make(chan struct{}, 1)
		dc.OnOpen(func() { dcOpen <- struct{}{} })

		ofCfg := webrtc.OfferOptions{}
		of, err := offerer.C_CreateOffer(pc, ofCfg)
		if err != nil {
			ec <- err
			return
		}

		slog.Info("setting local description")
		if err := offerer.D_SetOfferAsLocalDescription(pc, *of); err != nil {
			ec <- err
			return
		}

		<-webrtc.GatheringCompletePromise(pc)
		ld := pc.LocalDescription()
		if ld == nil {
			ec <- webrtc.ErrConnectionClosed
			return
		}

		hc := resty.New().SetBaseURL(serverAddr)

		slog.Info("sending offer", "offer", ld)
		if err := rtc.SendRTCEvent(hc, common.RTCOfferType, hostID, *ld); err != nil {
			ec <- err
			return
		}

		slog.Info("waiting for answer")
		answer, err := rtc.ReceiveRTCEvent(hc, common.RTCAnswerType, hostID)
		if err != nil {
			ec <- err
			return
		}
		slog.Info("setting remote description")
		if err := offerer.E_SetAnswerAsRemoteDescription(pc, *answer); err != nil {
			ec <- err
			return
		}

		slog.Info("waiting for data channel to open")
		select {
		case <-dcOpen:
			slog.Info("start bridging", "protocol", protocol, "local", localAddr)
			common.Output(ec, common.Bridge(protocol, localAddr, dc))
			return
		case <-ctx.Done():
			ec <- ctx.Err()
			return
		}
	}()

	return ec
}
