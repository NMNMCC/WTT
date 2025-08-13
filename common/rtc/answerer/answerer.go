package answerer

import (
	"wtt/common/rtc"

	"github.com/pion/webrtc/v4"
)

var A_CreatePeerConnection = rtc.CreatePeerConnection

var B_SetOfferAsRemoteDescription = rtc.SetRemoteDescription

func C_CreateAnswer(pc *webrtc.PeerConnection, cfg webrtc.AnswerOptions) (*webrtc.SessionDescription, error) {
	answer, err := pc.CreateAnswer(&cfg)
	if err != nil {
		return nil, err
	}
	return &answer, nil
}

var D_SetAnswerAsLocalDescription = rtc.SetLocalDescription
