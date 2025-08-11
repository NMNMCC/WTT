package test

import (
	"wtt/host"
)

// ExampleCompile ensures the package compiles with the new API without executing anything at build time.
func ExampleCompile() {
	_ = host.Run(host.HostConfig{})
}
