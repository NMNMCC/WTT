package common

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
)

func TestWebSocketConn(t *testing.T) {
	// Create a test WebSocket server
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Upgrade to WebSocket
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("Failed to upgrade connection: %v", err)
			return
		}
		defer conn.Close()

		// Echo any received messages for testing
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				break
			}
			err = conn.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				break
			}
		}
	}))
	defer server.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + server.URL[4:] // Replace "http" with "ws"

	t.Run("successful connection with token", func(t *testing.T) {
		conn, err := WebSocketConn(wsURL, "test-token")
		if err != nil {
			t.Fatalf("Expected successful connection, got error: %v", err)
		}
		defer conn.Close()

		// Test that we can send and receive a message
		testMessage := "hello"
		err = conn.WriteMessage(websocket.TextMessage, []byte(testMessage))
		if err != nil {
			t.Fatalf("Failed to send message: %v", err)
		}

		_, receivedMessage, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("Failed to read message: %v", err)
		}

		if string(receivedMessage) != testMessage {
			t.Errorf("Expected to receive %s, got %s", testMessage, string(receivedMessage))
		}
	})

	t.Run("connection without token", func(t *testing.T) {
		conn, err := WebSocketConn(wsURL, "")
		if err == nil {
			conn.Close()
			t.Error("Expected connection to fail without token, but it succeeded")
		}
	})

	t.Run("connection with wrong token", func(t *testing.T) {
		conn, err := WebSocketConn(wsURL, "wrong-token")
		if err == nil {
			conn.Close()
			t.Error("Expected connection to fail with wrong token, but it succeeded")
		}
	})

	t.Run("connection to invalid URL", func(t *testing.T) {
		_, err := WebSocketConn("ws://invalid-host:99999", "test-token")
		if err == nil {
			t.Error("Expected connection to fail with invalid URL, but it succeeded")
		}
	})
}

func TestWebSocketConnHeaderFormat(t *testing.T) {
	// Test that the Authorization header is formatted correctly
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		
		// Check that the header has the correct Bearer format
		if auth != "Bearer my-test-token" {
			t.Errorf("Expected Authorization header 'Bearer my-test-token', got '%s'", auth)
			http.Error(w, "Bad Authorization", http.StatusBadRequest)
			return
		}

		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.Close()
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	conn, err := WebSocketConn(wsURL, "my-test-token")
	if err != nil {
		t.Fatalf("Connection failed: %v", err)
	}
	conn.Close()
}

func TestWebSocketConnEmptyToken(t *testing.T) {
	// Test that empty token doesn't add Authorization header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		
		// Empty token should result in no Authorization header
		if auth != "" {
			t.Errorf("Expected no Authorization header with empty token, got '%s'", auth)
		}

		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.Close()
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	conn, err := WebSocketConn(wsURL, "")
	if err != nil {
		t.Fatalf("Connection failed: %v", err)
	}
	conn.Close()
}