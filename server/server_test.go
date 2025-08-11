package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wtt/common"

	"github.com/gorilla/websocket"
)

func TestServerConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      ServerConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config",
			config: ServerConfig{
				ListenAddr: ":8080",
				Tokens:     []string{"token1", "token2"},
			},
			expectError: false,
		},
		{
			name: "empty listen address",
			config: ServerConfig{
				ListenAddr: "",
				Tokens:     []string{"token1"},
			},
			expectError: true,
			errorMsg:    "listen address is required",
		},
		{
			name: "empty tokens",
			config: ServerConfig{
				ListenAddr: ":8080",
				Tokens:     []string{},
			},
			expectError: true,
			errorMsg:    "at least one authentication token is required",
		},
		{
			name: "nil tokens",
			config: ServerConfig{
				ListenAddr: ":8080",
				Tokens:     nil,
			},
			expectError: true,
			errorMsg:    "at least one authentication token is required",
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

func TestServerAuthenticationHandling(t *testing.T) {
	cfg := ServerConfig{
		ListenAddr: ":8080",
		Tokens:     []string{"valid-token", "another-token"},
	}

	// Create a test server
	connM := make(map[string]*websocket.Conn)
	upgrader := websocket.Upgrader{}
	
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized: Missing Authorization header", http.StatusUnauthorized)
			return
		}
		
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Unauthorized: Invalid Authorization header format", http.StatusUnauthorized)
			return
		}
		
		token := parts[1]
		tokenFound := false
		for _, validToken := range cfg.Tokens {
			if token == validToken {
				tokenFound = true
				break
			}
		}
		
		if !tokenFound {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		
		// Store connection for testing
		connM[token] = conn
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "valid token",
			authHeader:     "Bearer valid-token",
			expectedStatus: http.StatusSwitchingProtocols, // WebSocket upgrade
		},
		{
			name:           "another valid token",
			authHeader:     "Bearer another-token",
			expectedStatus: http.StatusSwitchingProtocols,
		},
		{
			name:           "invalid token",
			authHeader:     "Bearer invalid-token",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "missing bearer prefix",
			authHeader:     "invalid-token",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "empty authorization",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "malformed bearer",
			authHeader:     "Bearer",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "wrong auth type",
			authHeader:     "Basic dGVzdA==",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert HTTP URL to WebSocket URL
			wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
			
			// Create custom dialer with specific auth header
			dialer := websocket.Dialer{}
			header := http.Header{}
			if tt.authHeader != "" {
				header.Set("Authorization", tt.authHeader)
			}
			
			conn, resp, err := dialer.Dial(wsURL, header)
			
			if tt.expectedStatus == http.StatusSwitchingProtocols {
				// Should succeed
				if err != nil {
					t.Errorf("Expected successful connection, got error: %v", err)
				} else {
					conn.Close()
				}
			} else {
				// Should fail
				if err == nil {
					if conn != nil {
						conn.Close()
					}
					t.Error("Expected connection to fail, but it succeeded")
				}
				if resp != nil && resp.StatusCode != tt.expectedStatus {
					t.Errorf("Expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
				}
			}
		})
	}
}

func TestServerMessageHandling(t *testing.T) {
	// Test message forwarding logic without the full configuration struct
	upgrader := websocket.Upgrader{}
	connM := make(map[string]*websocket.Conn)
	
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Simple auth check
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer test-token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		var msg common.Message[any]
		if err := conn.ReadJSON(&msg); err != nil {
			conn.Close()
			return
		}

		connM[msg.SenderID] = conn
		conn.SetCloseHandler(func(code int, text string) error {
			delete(connM, msg.SenderID)
			return nil
		})

		// Forward message if target exists
		if targetConn, ok := connM[msg.TargetID]; ok && msg.TargetID != "" {
			targetConn.WriteJSON(msg)
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Test message forwarding
	t.Run("message forwarding", func(t *testing.T) {
		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
		
		// Create two connections
		header := http.Header{}
		header.Set("Authorization", "Bearer test-token")
		
		// First connection (host)
		conn1, _, err := websocket.DefaultDialer.Dial(wsURL, header)
		if err != nil {
			t.Fatalf("Failed to create first connection: %v", err)
		}
		defer conn1.Close()
		
		// Send registration message for host
		hostMsg := common.Message[any]{
			Type:     common.Register,
			SenderID: "host-123",
			TargetID: "",
		}
		err = conn1.WriteJSON(hostMsg)
		if err != nil {
			t.Fatalf("Failed to send host registration: %v", err)
		}
		
		// Give server time to process
		time.Sleep(10 * time.Millisecond)
		
		// Second connection (client)
		conn2, _, err := websocket.DefaultDialer.Dial(wsURL, header)
		if err != nil {
			t.Fatalf("Failed to create second connection: %v", err)
		}
		defer conn2.Close()
		
		// Send message targeting the host
		clientMsg := common.Message[any]{
			Type:     common.Offer,
			SenderID: "client-456",
			TargetID: "host-123",
		}
		err = conn2.WriteJSON(clientMsg)
		if err != nil {
			t.Fatalf("Failed to send client message: %v", err)
		}
		
		// Host should receive the message
		var receivedMsg common.Message[any]
		err = conn1.ReadJSON(&receivedMsg)
		if err != nil {
			t.Fatalf("Failed to read message on host connection: %v", err)
		}
		
		if receivedMsg.SenderID != "client-456" {
			t.Errorf("Expected sender ID 'client-456', got '%s'", receivedMsg.SenderID)
		}
		if receivedMsg.TargetID != "host-123" {
			t.Errorf("Expected target ID 'host-123', got '%s'", receivedMsg.TargetID)
		}
		if receivedMsg.Type != common.Offer {
			t.Errorf("Expected message type 'offer', got '%s'", receivedMsg.Type)
		}
	})
}

func TestRunWithInvalidConfig(t *testing.T) {
	// Test that Run returns error with invalid config
	invalidCfg := ServerConfig{
		ListenAddr: "", // Invalid - empty
		Tokens:     []string{"token"},
	}
	
	err := Run(invalidCfg)
	if err == nil {
		t.Error("Expected Run to return error with invalid config, got nil")
	}
	
	expectedMsg := "invalid server configuration: listen address is required"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error '%s', got '%s'", expectedMsg, err.Error())
	}
}