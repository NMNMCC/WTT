package common

type NetProtocol string

const (
	TCP NetProtocol = "tcp"
	UDP NetProtocol = "udp"
)

type RTCEventType string

const (
	RTCRegisterType RTCEventType = "register"
	RTCOfferType    RTCEventType = "offer"
	RTCAnswerType   RTCEventType = "answer"
)
