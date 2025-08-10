package server

import (
	"testing"
)

// mockClosableConnection is a mock implementation of the closableConnection interface for testing.
type mockClosableConnection struct {
	isClosed bool
}

func (m *mockClosableConnection) Close() error {
	m.isClosed = true
	return nil
}

func (m *mockClosableConnection) WriteJSON(v interface{}) error {
	// Not needed for these tests
	return nil
}

func (m *mockClosableConnection) ReadJSON(v interface{}) error {
	// Not needed for these tests
	return nil
}


func TestPeerManager_AddAndGet(t *testing.T) {
	pm := newPeerManager()
	hostID := "host1"
	mockConn := &mockClosableConnection{}

	err := pm.addHost(hostID, mockConn)
	if err != nil {
		t.Fatalf("Expected no error when adding a new host, but got %v", err)
	}

	retrievedConn := pm.getHost(hostID)
	if retrievedConn != mockConn {
		t.Errorf("Expected to get the mock connection for host '%s', but got a different value", hostID)
	}
}

func TestPeerManager_AddDuplicate(t *testing.T) {
	pm := newPeerManager()
	hostID := "host-duplicate"
	mockConn1 := &mockClosableConnection{}
	mockConn2 := &mockClosableConnection{}

	// Add the first time
	err1 := pm.addHost(hostID, mockConn1)
	if err1 != nil {
		t.Fatalf("Failed to add host the first time: %v", err1)
	}

	// Add the second time
	err2 := pm.addHost(hostID, mockConn2)
	if err2 == nil {
		t.Fatal("Expected an error when adding a duplicate host, but got nil")
	}
}

func TestPeerManager_Remove(t *testing.T) {
	pm := newPeerManager()
	hostID := "host-to-remove"
	mockConn := &mockClosableConnection{}

	// Add the host
	if err := pm.addHost(hostID, mockConn); err != nil {
		t.Fatalf("Failed to add host before removal test: %v", err)
	}

	// Verify it's there
	if pm.getHost(hostID) == nil {
		t.Fatal("Host was not added correctly before removal test")
	}

	// Remove the host
	pm.removeHost(hostID)

	// Verify it's gone
	if pm.getHost(hostID) != nil {
		t.Error("Expected host to be removed, but it's still present")
	}

	// Verify Close was called
	if !mockConn.isClosed {
		t.Error("Expected Close() to be called on the connection, but it wasn't")
	}
}

func TestPeerManager_RemoveNonExistent(t *testing.T) {
	pm := newPeerManager()
	pm.removeHost("does-not-exist")
}
