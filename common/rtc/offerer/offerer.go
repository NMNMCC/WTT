package offerer

import (
	wr "wtt/common/rtc"

	"github.com/pion/webrtc/v4"
)

var A_CreatePeerConnection = wr.CreatePeerConnection

var B_CreateDataChannel = wr.CreateDataChannel

func C_CreateOffer(pc *webrtc.PeerConnection, cfg webrtc.OfferOptions) (*webrtc.SessionDescription, error) {
	offer, err := pc.CreateOffer(&cfg)
	if err != nil {
		return nil, err
	}
	return &offer, nil
}

var D_SetOfferAsLocalDescription = wr.SetLocalDescription
