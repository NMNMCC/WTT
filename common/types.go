package common

// Protocol defines the transport protocol for the tunnel.
type Protocol string

const (
	TCP Protocol = "tcp"
	UDP Protocol = "udp"
)
