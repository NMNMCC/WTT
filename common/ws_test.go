package common

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestWebSocketConn(t *testing.T) {
	var upgrader = websocket.Upgrader{}

	t.Run("with token", func(t *testing.T) {
		token := "test-token"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			expectedHeader := "Bearer " + token
			if authHeader != expectedHeader {
				t.Errorf("expected Authorization header %q, got %q", expectedHeader, authHeader)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			_, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("Failed to upgrade connection: %v", err)
			}
		}))
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
		conn, err := WebSocketConn(wsURL, token)
		if err != nil {
			t.Fatalf("WebSocketConn failed: %v", err)
		}
		conn.Close()
	})

	t.Run("without token", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				t.Errorf("expected empty Authorization header, got %q", authHeader)
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}
			_, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("Failed to upgrade connection: %v", err)
			}
		}))
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
		conn, err := WebSocketConn(wsURL, "")
		if err != nil {
			t.Fatalf("WebSocketConn failed: %v", err)
		}
		conn.Close()
	})

	t.Run("invalid address", func(t *testing.T) {
		_, err := WebSocketConn("ws://invalid-address", "")
		if err == nil {
			t.Fatal("expected error for invalid address, got nil")
		}
	})
}
