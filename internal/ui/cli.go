package ui

var Cli struct {
	Service struct {
		ID       string `help:"unique service id" required:""`
		Endpoint string `help:"service endpoint" required:""`
		Type     string `help:"service type" enum:"tcp,udp,unix" required:""`
		Output   string `help:"service output" required:""`
	} `cmd:"service"`

	Consumer struct {
		ID        string `help:"unique consumer id" required:""`
		Endpoint  string `help:"consumer endpoint" required:""`
		Type      string `help:"consumer type" enum:"tcp,udp,unix" required:""`
		Input     string `help:"consumer input" required:""`
		ServiceID string `help:"service id to connect to" required:""`
	} `cmd:"consumer"`

	Server struct {
		Endpoint string `help:"server endpoint" required:""`
	} `cmd:"server"`
}
