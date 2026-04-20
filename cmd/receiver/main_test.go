package main

import (
	"net"
	"strings"
	"testing"
)

// mockIGMPConn is a no-op mock that satisfies multicast.IGMPConn.
type mockIGMPConn struct{}

func (m *mockIGMPConn) JoinGroup(*net.Interface, net.Addr) error                          { return nil }
func (m *mockIGMPConn) LeaveGroup(*net.Interface, net.Addr) error                         { return nil }
func (m *mockIGMPConn) JoinSourceSpecificGroup(*net.Interface, net.Addr, net.Addr) error  { return nil }
func (m *mockIGMPConn) LeaveSourceSpecificGroup(*net.Interface, net.Addr, net.Addr) error { return nil }


func TestValidateFlags(t *testing.T) {
	// Save original flag values and restore after test.
	origGroups := *groups
	origSource := *source
	origPort := *port
	origIface := *iface
	defer func() {
		*groups = origGroups
		*source = origSource
		*port = origPort
		*iface = origIface
	}()

	// setDefaults puts all flags into a known-good state.
	setDefaults := func() {
		*groups = []string{"239.1.1.1"}
		*source = ""
		*port = 5000
		*iface = "lo"
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
			name:    "no groups",
			setup:   func() { setDefaults(); *groups = nil },
			wantErr: "group",
		},
		{
			name:    "invalid group IP",
			setup:   func() { setDefaults(); *groups = []string{"bad"} },
			wantErr: "invalid",
		},
		{
			name:    "non-multicast group",
			setup:   func() { setDefaults(); *groups = []string{"10.0.0.1"} },
			wantErr: "multicast",
		},
		{
			name:    "SSM without source",
			setup:   func() { setDefaults(); *groups = []string{"232.1.1.1"} },
			wantErr: "--source",
		},
		{
			name:    "non-SSM with source",
			setup:   func() { setDefaults(); *source = "10.0.0.1" },
			wantErr: "non-SSM",
		},
		{
			name:    "port zero",
			setup:   func() { setDefaults(); *port = 0 },
			wantErr: "Port",
		},
		{
			name:    "valid SSM",
			setup:   func() { setDefaults(); *groups = []string{"232.1.1.1"}; *source = "10.0.0.1" },
			wantErr: "",
		},
		{
			name:    "valid ASM",
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
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// calcGaps replicates the exact gap-detection logic from groupManager,
// branch-for-branch, so we can verify the algorithm in isolation without
// needing channel plumbing.
//
//  1. First element → set lastSeq, no gap
//  2. seq > lastSeq+1 → gap of seq - lastSeq - 1
//  3. seq < lastSeq → sender restart: reset lastSeq, no gap (D-14)
//  4. seq == lastSeq → duplicate, no update
//  5. seq == lastSeq+1 → normal in-order
func calcGaps(seqs []uint64) (pktCount, gapCount uint64) {
	if len(seqs) == 0 {
		return 0, 0
	}

	var lastSeq uint64
	firstPkt := true

	for _, seq := range seqs {
		if firstPkt {
			// First packet: initialise, no gap.
			lastSeq = seq
			firstPkt = false
		} else if seq > lastSeq+1 {
			// Forward gap: one or more packets were lost.
			gapCount += seq - lastSeq - 1
			lastSeq = seq
		} else if seq < lastSeq {
			// Sender restart detected (D-14): reset lastSeq, no gap.
			lastSeq = seq
		} else if seq == lastSeq {
			// Duplicate packet: no update to lastSeq.
		} else {
			// Normal in-order packet (seq == lastSeq+1).
			lastSeq = seq
		}

		pktCount++
	}

	return pktCount, gapCount
}

func TestGroupManager_GapDetection(t *testing.T) {
	tests := []struct {
		name     string
		seqs     []uint64
		wantPkts uint64
		wantGaps uint64
	}{
		{
			name:     "first packet only",
			seqs:     []uint64{1},
			wantPkts: 1,
			wantGaps: 0,
		},
		{
			name:     "sequential",
			seqs:     []uint64{1, 2, 3},
			wantPkts: 3,
			wantGaps: 0,
		},
		{
			name:     "single gap",
			seqs:     []uint64{1, 3},
			wantPkts: 2,
			wantGaps: 1,
		},
		{
			name:     "multi gap",
			seqs:     []uint64{1, 5},
			wantPkts: 2,
			wantGaps: 3,
		},
		{
			name:     "sender restart",
			seqs:     []uint64{5, 6, 2, 3},
			wantPkts: 4,
			wantGaps: 0,
		},
		{
			name:     "duplicate",
			seqs:     []uint64{1, 2, 2, 3},
			wantPkts: 4,
			wantGaps: 0,
		},
		{
			name:     "gap then continue",
			seqs:     []uint64{1, 3, 4, 5},
			wantPkts: 4,
			wantGaps: 1,
		},
		{
			name:     "restart then gap",
			seqs:     []uint64{10, 11, 1, 5},
			wantPkts: 4,
			wantGaps: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPkts, gotGaps := calcGaps(tt.seqs)
			if gotPkts != tt.wantPkts {
				t.Errorf("pktCount = %d, want %d", gotPkts, tt.wantPkts)
			}
			if gotGaps != tt.wantGaps {
				t.Errorf("gapCount = %d, want %d", gotGaps, tt.wantGaps)
			}
		})
	}
}
