package common

// Protocol defines the transport protocol for the tunnel.
type Protocol string

const (
	TCP Protocol = "tcp"
	UDP Protocol = "udp"
)

// Destination holds the address and port for a network endpoint.
type Destination struct {
	Addr string
	Port string
}

// Portal represents one end of the tunnel with its protocol and destination.
type Portal struct {
	Protocol    Protocol
	Destination Destination
}

// Tunnel defines the entire tunnel configuration.
type Tunnel struct {
	Initiator  Portal
	Bootstrap  Destination
	Respondent Portal
}
