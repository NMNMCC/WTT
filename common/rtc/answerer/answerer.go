package answerer

import (
	wr "wtt/common/rtc"

	"github.com/pion/webrtc/v4"
)

var A_CreatePeerConnection = wr.CreatePeerConnection

var B_SetOfferAsRemoteDescription = wr.SetRemoteDescription

func C_CreateAnswer(pc *webrtc.PeerConnection, cfg webrtc.AnswerOptions) (*webrtc.SessionDescription, error) {
	answer, err := pc.CreateAnswer(&cfg)
	if err != nil {
		return nil, err
	}
	return &answer, nil
}

var D_SetAnswerAsLocalDescription = wr.SetLocalDescription
