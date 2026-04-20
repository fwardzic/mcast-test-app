package main

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

// mockWriter records how many times WriteTo was called.
type mockWriter struct {
	mu    sync.Mutex
	count int
}

func (m *mockWriter) WriteTo(b []byte, dst net.Addr) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.count++
	return len(b), nil
}

func (m *mockWriter) Close() error { return nil }

func (m *mockWriter) getCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.count
}

func TestValidateFlags(t *testing.T) {
	// Save original flag values and restore after test.
	origGroup := *group
	origPort := *port
	origIface := *iface
	origTTL := *ttl
	origRate := *rate
	defer func() {
		*group = origGroup
		*port = origPort
		*iface = origIface
		*ttl = origTTL
		*rate = origRate
	}()

	// setDefaults puts all flags into a known-good state.
	setDefaults := func() {
		*group = "239.1.1.1"
		*port = 5000
		*iface = "lo"
		*ttl = 2
		*rate = 1
	}

	tests := []struct {
		name    string
		setup   func()
		wantErr string // substring expected in error; empty means nil error
	}{
		{
			name:    "missing iface",
			setup:   func() { setDefaults(); *iface = "" },
			wantErr: "iface",
		},
		{
			name:    "invalid group IP",
			setup:   func() { setDefaults(); *group = "notanip" },
			wantErr: "invalid",
		},
		{
			name:    "non-multicast group",
			setup:   func() { setDefaults(); *group = "10.0.0.1" },
			wantErr: "multicast",
		},
		{
			name:    "port zero",
			setup:   func() { setDefaults(); *port = 0 },
			wantErr: "Port",
		},
		{
			name:    "rate zero",
			setup:   func() { setDefaults(); *rate = 0 },
			wantErr: "rate",
		},
		{
			name:    "valid flags",
			setup:   setDefaults,
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			err := validateFlags()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// contains checks if s contains substr (avoids importing strings for one use).
func contains(s, substr string) bool {
	return len(substr) <= len(s) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSendLoop_CancelledContext(t *testing.T) {
	// An already-cancelled context should cause sendLoop to return immediately
	// without sending any packets.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mw := &mockWriter{}
	dst := &net.UDPAddr{IP: net.ParseIP("239.1.1.1"), Port: 5000}

	done := make(chan struct{})
	go func() {
		sendLoop(ctx, mw, dst, "239.1.1.1", 100)
		close(done)
	}()

	select {
	case <-done:
		// Good — returned promptly.
	case <-time.After(2 * time.Second):
		t.Fatal("sendLoop did not return after context cancellation")
	}

	if c := mw.getCount(); c != 0 {
		t.Fatalf("expected 0 packets sent, got %d", c)
	}
}

func TestSendLoop_SendsPackets(t *testing.T) {
	// With rate=100 (10ms interval) and 150ms timeout, we expect roughly
	// 10-15 packets. We allow a wide range to avoid flaky CI failures.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	mw := &mockWriter{}
	dst := &net.UDPAddr{IP: net.ParseIP("239.1.1.1"), Port: 5000}

	sendLoop(ctx, mw, dst, "239.1.1.1", 100)

	c := mw.getCount()
	if c < 5 || c > 25 {
		t.Fatalf("expected 5-25 packets sent in 150ms at 100pps, got %d", c)
	}
}
