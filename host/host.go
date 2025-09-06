package host

import (
	"context"
	"log/slog"
	"net"

	"wtt/common"
	"wtt/common/rtc"
	"wtt/common/rtc/answerer"

	"github.com/go-resty/resty/v2"
	"github.com/pion/webrtc/v4"
)

func Run(ctx context.Context, id, signalingAddr, localAddr string, protocol common.NetProtocol) <-chan error {
	slog.Info("starting host", "id", id, "server", signalingAddr)
	ec := make(chan error)

	go func() {
		for {
			select {
			case <-ctx.Done():
				slog.Info("host context cancelled")
				ec <- ctx.Err()
				return
			default:
			}

			slog.Info("waiting for new peer connection")

			pcCfg := webrtc.Configuration{}
			pc, err := answerer.A_CreatePeerConnection(pcCfg)
			if err != nil {
				slog.Error("failed to create peer connection", "err", err)
				ec <- err
				return
			}

			dcC := make(chan *webrtc.DataChannel, 1)
			pc.OnDataChannel(func(dc *webrtc.DataChannel) {
				slog.Debug("data channel created", "label", dc.Label())
				dcC <- dc
			})

			hc := resty.New().SetBaseURL(signalingAddr)
			if err := rtc.RegisterHost(hc, id); err != nil {
				slog.Error("failed to register host", "err", err)
				ec <- err
				return
			}

			slog.Debug("waiting for offer")
			offer, err := rtc.ReceiveRTCEvent(hc, common.RTCOfferType, id)
			if err != nil {
				slog.Error("failed to receive offer", "err", err)
				ec <- err
				return
			}
			slog.Debug("received offer", "id", id)

			slog.Debug("setting remote description")
			if err := answerer.B_SetOfferAsRemoteDescription(pc, *offer); err != nil {
				slog.Error("failed to set remote description", "err", err)
				ec <- err
				return
			}

			answerO := webrtc.AnswerOptions{}
			slog.Debug("creating answer")
			answer, err := answerer.C_CreateAnswer(pc, answerO)
			if err != nil {
				slog.Error("failed to create answer", "err", err)
				ec <- err
				return
			}
			slog.Debug("setting local description")
			if err := answerer.D_SetAnswerAsLocalDescription(pc, *answer); err != nil {
				slog.Error("failed to set local description", "err", err)
				ec <- err
				return
			}

			<-webrtc.GatheringCompletePromise(pc)
			ld := pc.LocalDescription()
			if ld == nil {
				err := webrtc.ErrConnectionClosed
				slog.Error("local description is nil", "err", err)
				ec <- err
				return
			}

			slog.Debug("sending answer")
			if err := rtc.SendRTCEvent(hc, common.RTCAnswerType, id, *ld); err != nil {
				slog.Error("failed to send answer", "err", err)
				ec <- err
				return
			}

			slog.Debug("waiting for data channel")
			select {
			case dc := <-dcC:
				opened := make(chan struct{})
				dc.OnOpen(func() {
					slog.Debug("data channel opened")
					opened <- struct{}{}
				})

				slog.Debug("waiting for data channel to open")
				select {
				case <-opened:
					slog.Info("start bridging", "protocol", protocol, "local", localAddr)

					var bridgeErrCh <-chan error
					switch protocol {
					case common.TCP:
						conn, err := net.Dial("tcp", localAddr)
						if err != nil {
							slog.Error("failed to dial local tcp service", "err", err)
							pc.Close() // Close the current peer connection
							continue   // And try to get a new one
						}
						bridgeErrCh = common.BridgeStream(dc, conn)
					case common.UDP:
						conn, err := net.ListenPacket("udp", localAddr)
						if err != nil {
							slog.Error("failed to listen on local udp", "err", err)
							pc.Close()
							continue
						}
						bridgeErrCh = common.BridgePacket(dc, conn)
					}

					if err := <-bridgeErrCh; err != nil {
						slog.Error("bridge finished with error", "err", err)
					} else {
						slog.Debug("bridge finished cleanly")
					}

				case <-ctx.Done():
					slog.Info("host context cancelled during bridge")
					ec <- ctx.Err()
				}
			case <-ctx.Done():
				slog.Info("host context cancelled waiting for dc")
				ec <- ctx.Err()
			}

			if err := pc.Close(); err != nil {
				slog.Error("failed to close peer connection", "err", err)
			}
		}
	}()

	return ec
}
