package client

import (
	"testing"
	"wtt/common"
)

func TestClientConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      ClientConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config",
			config: ClientConfig{
				ID:        "client-123",
				HostID:    "host-456",
				SigAddr:   "ws://localhost:8080",
				LocalAddr: "localhost:9090",
				Protocol:  common.TCP,
				STUNAddrs: []string{"stun:stun.l.google.com:19302"},
				Token:     "test-token",
				Timeout:   10,
			},
			expectError: false,
		},
		{
			name: "empty host ID",
			config: ClientConfig{
				ID:        "client-123",
				HostID:    "",
				SigAddr:   "ws://localhost:8080",
				LocalAddr: "localhost:9090",
				Protocol:  common.TCP,
				Timeout:   10,
			},
			expectError: true,
			errorMsg:    "client mode requires a target host ID",
		},
		{
			name: "empty signaling address",
			config: ClientConfig{
				ID:        "client-123",
				HostID:    "host-456",
				SigAddr:   "",
				LocalAddr: "localhost:9090",
				Protocol:  common.TCP,
				Timeout:   10,
			},
			expectError: true,
			errorMsg:    "client mode requires a signaling server address",
		},
		{
			name: "empty local address",
			config: ClientConfig{
				ID:        "client-123",
				HostID:    "host-456",
				SigAddr:   "ws://localhost:8080",
				LocalAddr: "",
				Protocol:  common.TCP,
				Timeout:   10,
			},
			expectError: true,
			errorMsg:    "client mode requires a local address to listen on",
		},
		{
			name: "invalid protocol",
			config: ClientConfig{
				ID:        "client-123",
				HostID:    "host-456",
				SigAddr:   "ws://localhost:8080",
				LocalAddr: "localhost:9090",
				Protocol:  common.Protocol("invalid"),
				Timeout:   10,
			},
			expectError: true,
			errorMsg:    "unsupported protocol: invalid",
		},
		{
			name: "zero timeout",
			config: ClientConfig{
				ID:        "client-123",
				HostID:    "host-456",
				SigAddr:   "ws://localhost:8080",
				LocalAddr: "localhost:9090",
				Protocol:  common.TCP,
				Timeout:   0,
			},
			expectError: true,
			errorMsg:    "timeout must be positive, got: 0",
		},
		{
			name: "negative timeout",
			config: ClientConfig{
				ID:        "client-123",
				HostID:    "host-456",
				SigAddr:   "ws://localhost:8080",
				LocalAddr: "localhost:9090",
				Protocol:  common.TCP,
				Timeout:   -5,
			},
			expectError: true,
			errorMsg:    "timeout must be positive, got: -5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if err.Error() != tt.errorMsg {
					t.Errorf("Expected error '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestClientConfigDefaults(t *testing.T) {
	// Test that the client config struct properly handles various scenarios
	cfg := ClientConfig{
		ID:       "test-client",
		HostID:   "test-host",
		SigAddr:  "ws://localhost:8080",
		LocalAddr: "localhost:9090",
		Protocol: common.TCP,
		Timeout:  10,
	}

	// Test that validation passes with minimal required fields
	err := cfg.Validate()
	if err != nil {
		t.Errorf("Expected validation to pass with minimal config, got error: %v", err)
	}

	// Test that optional fields can be empty
	cfg.STUNAddrs = nil
	err = cfg.Validate()
	if err != nil {
		t.Errorf("Expected validation to pass with nil STUNAddrs, got error: %v", err)
	}

	cfg.Token = ""
	err = cfg.Validate()
	if err != nil {
		t.Errorf("Expected validation to pass with empty token, got error: %v", err)
	}

	cfg.ID = ""
	err = cfg.Validate()
	if err != nil {
		t.Errorf("Expected validation to pass with empty ID, got error: %v", err)
	}
}

func TestClientProtocolValidation(t *testing.T) {
	baseConfig := ClientConfig{
		ID:        "client-123",
		HostID:    "host-456",
		SigAddr:   "ws://localhost:8080",
		LocalAddr: "localhost:9090",
		Timeout:   10,
	}

	validProtocols := []common.Protocol{common.TCP, common.UDP}
	for _, protocol := range validProtocols {
		t.Run(string(protocol), func(t *testing.T) {
			cfg := baseConfig
			cfg.Protocol = protocol
			err := cfg.Validate()
			if err != nil {
				t.Errorf("Expected protocol %s to be valid, got error: %v", protocol, err)
			}
		})
	}

	invalidProtocols := []string{"http", "ftp", "ssh", "", "INVALID"}
	for _, protocol := range invalidProtocols {
		t.Run("invalid_"+protocol, func(t *testing.T) {
			cfg := baseConfig
			cfg.Protocol = common.Protocol(protocol)
			err := cfg.Validate()
			if err == nil {
				t.Errorf("Expected protocol %s to be invalid, but validation passed", protocol)
			}
		})
	}
}

// Note: Testing the actual Run, emit, and tunnel functions would require
// setting up real WebRTC connections and signaling servers, which is
// complex for unit tests. Those would be better suited for integration tests.
// However, we can test the validation logic and basic structure.

func TestRunWithInvalidConfig(t *testing.T) {
	// Test that Run returns early with invalid config
	invalidConfigs := []ClientConfig{
		{}, // Empty config
		{
			HostID: "host-123",
			// Missing other required fields
		},
		{
			HostID:    "host-123",
			SigAddr:   "ws://localhost:8080",
			LocalAddr: "localhost:9090",
			Protocol:  common.Protocol("invalid"),
			Timeout:   10,
		},
	}

	for i, cfg := range invalidConfigs {
		t.Run(string(rune('A'+i)), func(t *testing.T) {
			// Run should return early due to validation failure
			// We can't easily test the exact error output since Run logs with glog
			// and returns void, but we can verify it doesn't panic
			Run(cfg)
		})
	}
}

func TestClientConfigStructure(t *testing.T) {
	// Test that ClientConfig has all expected fields with correct types
	cfg := ClientConfig{}

	// Test field types by assignment
	cfg.ID = "test"
	cfg.HostID = "test"
	cfg.SigAddr = "test"
	cfg.LocalAddr = "test"
	cfg.Protocol = common.TCP
	cfg.STUNAddrs = []string{"test"}
	cfg.Token = "test"
	cfg.Timeout = 10

	// Verify fields are accessible
	if cfg.ID != "test" {
		t.Error("ID field not working correctly")
	}
	if cfg.HostID != "test" {
		t.Error("HostID field not working correctly")
	}
	if cfg.SigAddr != "test" {
		t.Error("SigAddr field not working correctly")
	}
	if cfg.LocalAddr != "test" {
		t.Error("LocalAddr field not working correctly")
	}
	if cfg.Protocol != common.TCP {
		t.Error("Protocol field not working correctly")
	}
	if len(cfg.STUNAddrs) != 1 || cfg.STUNAddrs[0] != "test" {
		t.Error("STUNAddrs field not working correctly")
	}
	if cfg.Token != "test" {
		t.Error("Token field not working correctly")
	}
	if cfg.Timeout != 10 {
		t.Error("Timeout field not working correctly")
	}
}