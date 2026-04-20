//go:build integration

// Integration tests for SenderConn. These require a real network interface
// and are skipped unless you pass -tags integration to go test.
package multicast

import (
	"net"
	"runtime"
	"testing"
)

// TestNewSenderConn_Loopback creates a real multicast sender on the loopback
// interface and sends a single packet to verify the socket works end-to-end.
func TestNewSenderConn_Loopback(t *testing.T) {
	// Pick the loopback interface name based on OS.
	// Linux uses "lo", macOS (Darwin) uses "lo0".
	loIface := "lo"
	if runtime.GOOS == "darwin" {
		loIface = "lo0"
	}

	sc, err := NewSenderConn(loIface, 2, true)
	if err != nil {
		t.Fatalf("NewSenderConn(%q, 2, true) error: %v", loIface, err)
	}

	// Send a test packet to a multicast address.
	dst := &net.UDPAddr{IP: net.ParseIP("239.1.1.1"), Port: 5000}
	n, err := sc.WriteTo([]byte("test"), dst)
	if err != nil {
		t.Fatalf("WriteTo error: %v", err)
	}
	if n <= 0 {
		t.Fatalf("WriteTo returned %d bytes, want > 0", n)
	}

	if err := sc.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
}

// TestNewSenderConn_BadInterface verifies that NewSenderConn returns a clear
// error when the requested network interface does not exist.
func TestNewSenderConn_BadInterface(t *testing.T) {
	_, err := NewSenderConn("doesnotexist99", 1, false)
	if err == nil {
		t.Fatal("expected error for non-existent interface, got nil")
	}

	// The error message should mention the bad interface name so the user
	// can spot their typo quickly.
	if got := err.Error(); !contains(got, "doesnotexist99") {
		t.Errorf("error %q does not mention interface name", got)
	}
}

// contains is a tiny helper to avoid importing strings just for one check.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
