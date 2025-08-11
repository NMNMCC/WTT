package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"wtt/common"

	"github.com/gorilla/websocket"
)

func TestServer(t *testing.T) {
	connM := &sync.Map{}
	upgrader := websocket.Upgrader{}

	// Create a test server using the real HandleConnection logic
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" || parts[1] != "secret" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("Failed to upgrade: %v", err)
			return
		}
		// Use the actual exported handler
		go HandleConnection(conn, connM)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Test unauthorized connection
	t.Run("Unauthorized", func(t *testing.T) {
		header := http.Header{"Authorization": {"Bearer wrong-token"}}
		_, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
		if err == nil {
			t.Fatal("Expected an error for unauthorized connection, got nil")
		}
		if resp == nil {
			t.Fatalf("Expected a response for unauthorized connection, got nil")
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("Expected status code %d, got %d", http.StatusUnauthorized, resp.StatusCode)
		}
	})

	// Test message forwarding
	t.Run("Forwarding", func(t *testing.T) {
		header := http.Header{"Authorization": {"Bearer secret"}}

		// Client A
		connA, _, err := websocket.DefaultDialer.Dial(wsURL, header)
		if err != nil {
			t.Fatalf("Client A failed to connect: %v", err)
		}
		defer connA.Close()

		// Client B
		connB, _, err := websocket.DefaultDialer.Dial(wsURL, header)
		if err != nil {
			t.Fatalf("Client B failed to connect: %v", err)
		}
		defer connB.Close()

		// Register Client A
		regA := common.TypedMessage[any]{SenderID: "clientA"}
		if err := connA.WriteJSON(regA); err != nil {
			t.Fatalf("Client A failed to register: %v", err)
		}

		// Register Client B
		regB := common.TypedMessage[any]{SenderID: "clientB"}
		if err := connB.WriteJSON(regB); err != nil {
			t.Fatalf("Client B failed to register: %v", err)
		}

		// Wait for registrations to be processed
		time.Sleep(100 * time.Millisecond)

		// A -> B
		msgBody := "hello from A"
		msg := common.TypedMessage[string]{
			SenderID: "clientA",
			TargetID: "clientB",
			Type:     "test",
			Payload:  msgBody,
		}
		err = connA.WriteJSON(msg)
		if err != nil {
			t.Fatalf("Client A failed to send message: %v", err)
		}

		// B receives
		connB.SetReadDeadline(time.Now().Add(2 * time.Second))
		var receivedMsg common.Message
		err = connB.ReadJSON(&receivedMsg)
		if err != nil {
			t.Fatalf("Client B failed to receive message: %v", err)
		}

		var receivedPayload string
		if err := json.Unmarshal(receivedMsg.Payload, &receivedPayload); err != nil {
			t.Fatalf("Failed to unmarshal received payload: %v", err)
		}

		if receivedPayload != msgBody {
			t.Fatalf("Message mismatch: got '%s', want '%s'", receivedPayload, msgBody)
		}
	})
}
