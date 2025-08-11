package main

import (
	"testing"
	"wtt/cmd"
)

func TestMainCommandStructure(t *testing.T) {
	// Test that main package exports expected commands
	if cmd.Server.Name != "server" {
		t.Errorf("Expected server command name to be 'server', got '%s'", cmd.Server.Name)
	}

	if cmd.Client.Name != "client" {
		t.Errorf("Expected client command name to be 'client', got '%s'", cmd.Client.Name)
	}

	if cmd.Host.Name != "host" {
		t.Errorf("Expected host command name to be 'host', got '%s'", cmd.Host.Name)
	}
}

func TestMainFunction(t *testing.T) {
	// Test that main function exists and can be imported
	// We can't easily test the actual execution without setting up OS args
	// but we can test that it compiles and the structure is correct
	
	// The main function is in main.go and should be available
	// This test mainly verifies the package structure
	if cmd.Server.Action == nil {
		t.Error("Expected server command to have an action")
	}
	
	if cmd.Client.Action == nil {
		t.Error("Expected client command to have an action")
	}
	
	if cmd.Host.Action == nil {
		t.Error("Expected host command to have an action")
	}
}