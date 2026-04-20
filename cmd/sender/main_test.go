package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"sync"
	"testing"
	"time"

	"github.com/fwardzic/mcast-test-app/internal/packet"
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

// recordingWriter captures each raw payload written by sendLoop.
type recordingWriter struct {
	mu      sync.Mutex
	packets [][]byte
}

func (r *recordingWriter) WriteTo(b []byte, dst net.Addr) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]byte, len(b))
	copy(cp, b)
	r.packets = append(r.packets, cp)
	return len(b), nil
}

func (r *recordingWriter) Close() error { return nil }

func (r *recordingWriter) getPackets() [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][]byte, len(r.packets))
	copy(out, r.packets)
	return out
}

// TestSendLoop_SequenceNumbersAreMonotonicallyIncreasing verifies SEND-02:
// each packet written by sendLoop has a sequence number exactly one greater
// than the previous packet (i.e. no gaps, no resets).
func TestSendLoop_SequenceNumbersAreMonotonicallyIncreasing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	rw := &recordingWriter{}
	dst := &net.UDPAddr{IP: net.ParseIP("239.1.1.1"), Port: 5000}

	sendLoop(ctx, rw, dst, "239.1.1.1", 100)

	payloads := rw.getPackets()
	if len(payloads) < 2 {
		t.Fatalf("need at least 2 packets to verify monotonic sequence; got %d", len(payloads))
	}

	var prev uint64
	for i, raw := range payloads {
		p, err := packet.Decode(raw)
		if err != nil {
			t.Fatalf("packet[%d]: decode error: %v", i, err)
		}
		if i == 0 {
			if p.Sequence != 1 {
				t.Fatalf("packet[0]: expected sequence 1 (first packet), got %d", p.Sequence)
			}
			prev = p.Sequence
			continue
		}
		if p.Sequence != prev+1 {
			t.Fatalf("packet[%d]: expected sequence %d, got %d (not monotonically increasing by 1)",
				i, prev+1, p.Sequence)
		}
		prev = p.Sequence
	}
}

// TestSendLoop_SignalNotifyContext_ShutdownViaSIGINT verifies SEND-08:
// the signal.NotifyContext path in main() cancels the context on SIGINT,
// which causes sendLoop to exit. We test this end-to-end in-process by
// sending os.Interrupt to the current process and verifying sendLoop exits.
//
// Note: os.Interrupt is delivered as a signal to the whole process. We
// install our own signal.NotifyContext here — mirroring what main() does —
// to drive the context cancel, then verify sendLoop exits promptly.
func TestSendLoop_SignalNotifyContext_ShutdownViaSIGINT(t *testing.T) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	rw := &recordingWriter{}
	dst := &net.UDPAddr{IP: net.ParseIP("239.1.1.1"), Port: 5000}

	done := make(chan struct{})
	go func() {
		sendLoop(ctx, rw, dst, "239.1.1.1", 10) // slow rate so we don't flood
		close(done)
	}()

	// Give the loop a moment to start ticking.
	time.Sleep(20 * time.Millisecond)

	// Send SIGINT to ourselves — this is the exact signal path main() uses.
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}
	if err := p.Signal(os.Interrupt); err != nil {
		t.Fatalf("Signal(SIGINT): %v", err)
	}

	select {
	case <-done:
		// Good — sendLoop exited after signal-driven context cancellation.
	case <-time.After(2 * time.Second):
		t.Fatal("sendLoop did not exit within 2s after SIGINT")
	}
}
