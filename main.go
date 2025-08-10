package main

import (
	"flag"

	"github.com/golang/glog"
	"github.com/spf13/pflag"
	"webrtc-tunnel/cmd"
)

func main() {
	// glog flags are registered in init(), so they will be included here.
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	// Defer flushing all logs before the program exits.
	defer glog.Flush()

	cmd.Execute()
}
