package host

import (
	"testing"
	"wtt/common"
)

func TestHostConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      HostConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config",
			config: HostConfig{
				ID:        "host-123",
				SigAddr:   "ws://localhost:8080",
				LocalAddr: "localhost:9090",
				Protocol:  common.TCP,
				STUNAddrs: []string{"stun:stun.l.google.com:19302"},
				Token:     "test-token",
			},
			expectError: false,
		},
		{
			name: "empty host ID",
			config: HostConfig{
				ID:        "",
				SigAddr:   "ws://localhost:8080",
				LocalAddr: "localhost:9090",
				Protocol:  common.TCP,
			},
			expectError: true,
			errorMsg:    "host ID is required",
		},
		{
			name: "empty signaling address",
			config: HostConfig{
				ID:        "host-123",
				SigAddr:   "",
				LocalAddr: "localhost:9090",
				Protocol:  common.TCP,
			},
			expectError: true,
			errorMsg:    "signaling server address is required",
		},
		{
			name: "empty local address",
			config: HostConfig{
				ID:       "host-123",
				SigAddr:  "ws://localhost:8080",
				LocalAddr: "",
				Protocol: common.TCP,
			},
			expectError: true,
			errorMsg:    "local address is required",
		},
		{
			name: "invalid protocol",
			config: HostConfig{
				ID:        "host-123",
				SigAddr:   "ws://localhost:8080",
				LocalAddr: "localhost:9090",
				Protocol:  common.Protocol("invalid"),
			},
			expectError: true,
			errorMsg:    "protocol must be 'tcp' or 'udp', got: invalid",
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

func TestHostConfigDefaults(t *testing.T) {
	// Test that the host config struct properly handles various scenarios
	cfg := HostConfig{
		ID:        "test-host",
		SigAddr:   "ws://localhost:8080",
		LocalAddr: "localhost:9090",
		Protocol:  common.TCP,
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
}

func TestHostProtocolValidation(t *testing.T) {
	baseConfig := HostConfig{
		ID:        "host-123",
		SigAddr:   "ws://localhost:8080",
		LocalAddr: "localhost:9090",
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

func TestRunWithInvalidConfig(t *testing.T) {
	// Test that Run returns error with invalid config
	invalidCfg := HostConfig{
		ID:       "", // Invalid - empty
		SigAddr:  "ws://localhost:8080",
		LocalAddr: "localhost:9090",
		Protocol: common.TCP,
	}
	
	err := Run(invalidCfg)
	if err == nil {
		t.Error("Expected Run to return error with invalid config, got nil")
	}
	
	expectedMsg := "invalid host configuration: host ID is required"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestHostConfigStructure(t *testing.T) {
	// Test that HostConfig has all expected fields with correct types
	cfg := HostConfig{}

	// Test field types by assignment
	cfg.ID = "test"
	cfg.SigAddr = "test"
	cfg.LocalAddr = "test"
	cfg.Protocol = common.TCP
	cfg.STUNAddrs = []string{"test"}
	cfg.Token = "test"

	// Verify fields are accessible
	if cfg.ID != "test" {
		t.Error("ID field not working correctly")
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
}

// Note: Testing the actual Run, wait, answer, and other WebRTC-related functions 
// would require setting up real WebRTC connections and signaling servers, which is
// complex for unit tests. Those would be better suited for integration tests.
// However, we can test the validation logic and basic structure.

func TestCreatePCFunction(t *testing.T) {
	// Test createPC with empty STUN servers
	pc, err := createPC([]string{})
	if err != nil {
		t.Errorf("Expected createPC to succeed with empty STUN servers, got error: %v", err)
	}
	if pc == nil {
		t.Error("Expected createPC to return non-nil PeerConnection")
	}
	if pc != nil {
		pc.Close() // Clean up
	}

	// Test createPC with STUN servers
	stunServers := []string{"stun:stun.l.google.com:19302"}
	pc, err = createPC(stunServers)
	if err != nil {
		t.Errorf("Expected createPC to succeed with STUN servers, got error: %v", err)
	}
	if pc == nil {
		t.Error("Expected createPC to return non-nil PeerConnection")
	}
	if pc != nil {
		pc.Close() // Clean up
	}
}

func TestEnsureLocalFunction(t *testing.T) {
	// Test ensureLocal with a valid PeerConnection
	pc, err := createPC([]string{})
	if err != nil {
		t.Fatalf("Failed to create PeerConnection for test: %v", err)
	}
	defer pc.Close()

	// This should fail because we haven't set a remote description
	err = ensureLocal(pc)
	if err == nil {
		t.Error("Expected ensureLocal to fail without remote description, but it succeeded")
	}
}

// Mock test for addICECandidate function
func TestAddICECandidateValidation(t *testing.T) {
	// Test with wrong message type
	wrongMsg := common.Message[common.CandidatePayload]{
		Type: common.Offer, // Wrong type
		Payload: common.CandidatePayload{},
	}

	pc, err := createPC([]string{})
	if err != nil {
		t.Fatalf("Failed to create PeerConnection for test: %v", err)
	}
	defer pc.Close()

	err = addICECandidate(pc, wrongMsg)
	if err == nil {
		t.Error("Expected addICECandidate to fail with wrong message type, but it succeeded")
	}

	expectedError := "expected candidate message, got offer"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}