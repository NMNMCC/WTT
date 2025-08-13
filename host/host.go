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

	go func() {
		for {
			pcCfg := webrtc.Configuration{}
			slog.Debug("creating peer connection")
			pc, err := answerer.A_CreatePeerConnection(pcCfg)
			if err != nil {
				slog.Error("create peer connection error", "err", err)
				ec <- err
				return
			}
			defer pc.Close()

			dcC := make(chan *webrtc.DataChannel, 1)
			pc.OnDataChannel(func(dc *webrtc.DataChannel) {
				slog.Info("data channel created", "label", dc.Label())
				dcC <- dc
			})

			hc := resty.New().SetBaseURL(signalingAddr)
			if err := rtc.RegisterHost(hc, id); err != nil {
				slog.Error("register host error", "err", err)
				ec <- err
				return
			}

			slog.Info("waiting for offer")
			offer, err := rtc.ReceiveRTCEvent(hc, common.RTCOfferType, id)
			if err != nil {
				slog.Error("receive offer error", "err", err)
				ec <- err
				return
			}

			slog.Info("setting remote description")
			if err := answerer.B_SetOfferAsRemoteDescription(pc, offer); err != nil {
				slog.Error("set remote description error", "err", err)
				ec <- err
				return
			}

			answerO := webrtc.AnswerOptions{}
			slog.Debug("creating answer")
			answer, err := answerer.C_CreateAnswer(pc, answerO)
			if err != nil {
				slog.Error("create answer error", "err", err)
				ec <- err
				return
			}
			slog.Info("setting local description")
			if err := answerer.D_SetAnswerAsLocalDescription(pc, *answer); err != nil {
				slog.Error("set local description error", "err", err)
				ec <- err
				return
			}

			<-webrtc.GatheringCompletePromise(pc)
			ld := pc.LocalDescription()
			if ld == nil {
				slog.Error("local description is nil after gathering")
				ec <- webrtc.ErrConnectionClosed
				return
			}

			answerM := common.RTCAnswer{
				HostID:             id,
				SessionDescription: *ld,
			}
			slog.Info("sending answer")
			if err := rtc.SendRTCEvent(hc, common.RTCAnswerType, id, answerM); err != nil {
				slog.Error("send answer error", "err", err)
				ec <- err
				return
			}

			slog.Info("waiting for data channel")
			select {
			case dc := <-dcC:
				opened := make(chan struct{})
				dc.OnOpen(func() { opened <- struct{}{} })

				slog.Info("waiting for data channel to open")
				select {
				case <-opened:
					defer dc.Close()
					slog.Info("start bridging", "protocol", protocol, "local", localAddr)

					go common.Output(ec, common.Bridge(protocol, localAddr, dc))
				case <-ctx.Done():
					ec <- ctx.Err()
				}
			case <-ctx.Done():
				ec <- ctx.Err()
			}

		}
	}()

	return ec
}
