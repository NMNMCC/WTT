package client

import (
	"context"
	"fmt"
	"log/slog"
	"net"
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
		slog.Info("starting client", "server", serverAddr, "hostID", hostID)
		pcCfg := webrtc.Configuration{}
		pc, err := offerer.A_CreatePeerConnection(pcCfg)
		if err != nil {
			slog.Error("failed to create peer connection", "err", err)
			ec <- err
			return
		}
		defer pc.Close()

		id, err := uuid.NewRandom()
		if err != nil {
			slog.Error("failed to create random id", "err", err)
			ec <- err
			return
		}
		dc, err := offerer.B_CreateDataChannel(pc, id.String())
		if err != nil {
			slog.Error("failed to create data channel", "err", err)
			ec <- err
			return
		}
		defer dc.Close()

		dcOpen := make(chan struct{}, 1)
		dc.OnOpen(func() {
			slog.Debug("data channel opened")
			dcOpen <- struct{}{}
		})

		ofCfg := webrtc.OfferOptions{}
		of, err := offerer.C_CreateOffer(pc, ofCfg)
		if err != nil {
			slog.Error("failed to create offer", "err", err)
			ec <- err
			return
		}

		slog.Debug("setting local description")
		if err := offerer.D_SetOfferAsLocalDescription(pc, *of); err != nil {
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

		hc := resty.New().SetBaseURL(serverAddr)

		slog.Debug("sending offer")
		if err := rtc.SendRTCEvent(hc, common.RTCOfferType, hostID, *ld); err != nil {
			slog.Error("failed to send offer", "err", err)
			ec <- err
			return
		}

		slog.Debug("waiting for answer")
		answer, err := rtc.ReceiveRTCEvent(hc, common.RTCAnswerType, hostID)
		if err != nil {
			slog.Error("failed to receive answer", "err", err)
			ec <- err
			return
		}
		slog.Debug("setting remote description")
		if err := offerer.E_SetAnswerAsRemoteDescription(pc, *answer); err != nil {
			slog.Error("failed to set remote description", "err", err)
			ec <- err
			return
		}

		slog.Debug("waiting for data channel to open")
		select {
		case <-dcOpen:
			slog.Info("start bridging", "protocol", protocol, "local", localAddr)

			switch protocol {
			case common.TCP:
				l, err := net.Listen("tcp", localAddr)
				if err != nil {
					err = fmt.Errorf("client failed to listen on local port: %w", err)
					slog.Error("tcp listen error", "err", err)
					ec <- err
					return
				}
				defer l.Close()

				slog.Info("client listening for local connections", "addr", l.Addr())

				// Accept one connection
				conn, err := l.Accept()
				if err != nil {
					// if context is cancelled, this is expected
					if ctx.Err() == nil {
						err = fmt.Errorf("client failed to accept connection: %w", err)
						slog.Error("tcp accept error", "err", err)
						ec <- err
					}
					return
				}

				bridgeErrCh := common.BridgeStream(dc, conn)
				if err := <-bridgeErrCh; err != nil {
					slog.Error("bridge finished with error", "err", err)
					ec <- err
				} else {
					slog.Debug("bridge finished cleanly")
					ec <- nil
				}

			case common.UDP:
				// UDP logic for the client is more complex as it doesn't have a clear "accept" model.
				// For now, we'll assume the same ListenPacket logic as the host is sufficient,
				// though a real-world scenario might need more sophisticated handling.
				conn, err := net.ListenPacket("udp", localAddr)
				if err != nil {
					err = fmt.Errorf("client failed to listen on local udp: %w", err)
					slog.Error("udp listen error", "err", err)
					ec <- err
					return
				}
				bridgeErrCh := common.BridgePacket(dc, conn)
				if err := <-bridgeErrCh; err != nil {
					slog.Error("udp bridge finished with error", "err", err)
					ec <- err
				} else {
					ec <- nil
				}
			}
			return
		case <-ctx.Done():
			slog.Info("client context cancelled")
			ec <- ctx.Err()
			return
		}
	}()

	return ec
}
