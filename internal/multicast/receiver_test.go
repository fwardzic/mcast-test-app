package multicast

import (
	"errors"
	"net"
	"sync"
	"testing"
)

type mockCall struct {
	iface string
	group string
}

type mockSSMCall struct {
	iface  string
	group  string
	source string
}

type mockIGMPConn struct {
	mu         sync.Mutex
	joinCalls  []mockCall
	leaveCalls []mockCall
	joinSSM    []mockSSMCall
	leaveSSM   []mockSSMCall
	err        error
}

func (m *mockIGMPConn) JoinGroup(ifi *net.Interface, group net.Addr) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.joinCalls = append(m.joinCalls, mockCall{iface: ifi.Name, group: group.String()})
	return m.err
}

func (m *mockIGMPConn) LeaveGroup(ifi *net.Interface, group net.Addr) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.leaveCalls = append(m.leaveCalls, mockCall{iface: ifi.Name, group: group.String()})
	return m.err
}

func (m *mockIGMPConn) JoinSourceSpecificGroup(ifi *net.Interface, group, source net.Addr) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.joinSSM = append(m.joinSSM, mockSSMCall{iface: ifi.Name, group: group.String(), source: source.String()})
	return m.err
}

func (m *mockIGMPConn) LeaveSourceSpecificGroup(ifi *net.Interface, group, source net.Addr) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.leaveSSM = append(m.leaveSSM, mockSSMCall{iface: ifi.Name, group: group.String(), source: source.String()})
	return m.err
}

var fakeIface = &net.Interface{Index: 1, Name: "eth0"}

func TestJoinASM(t *testing.T) {
	tests := []struct {
		name    string
		connErr error
		wantErr bool
		wantMsg string
	}{
		{"valid group", nil, false, ""},
		{"conn error", errors.New("join failed"), true, "JoinASM"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := &mockIGMPConn{err: tc.connErr}
			err := JoinASM(m, fakeIface, net.ParseIP("239.1.1.1"))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.wantMsg != "" && !containsStr(err.Error(), tc.wantMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.wantMsg)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(m.joinCalls) != 1 {
					t.Errorf("expected 1 call, got %d", len(m.joinCalls))
				}
			}
		})
	}
}

func TestLeaveASM(t *testing.T) {
	tests := []struct {
		name    string
		connErr error
		wantErr bool
		wantMsg string
	}{
		{"valid group", nil, false, ""},
		{"conn error", errors.New("leave failed"), true, "LeaveASM"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := &mockIGMPConn{err: tc.connErr}
			err := LeaveASM(m, fakeIface, net.ParseIP("239.1.1.1"))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.wantMsg != "" && !containsStr(err.Error(), tc.wantMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.wantMsg)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(m.leaveCalls) != 1 {
					t.Errorf("expected 1 call, got %d", len(m.leaveCalls))
				}
			}
		})
	}
}

func TestJoinSSM(t *testing.T) {
	tests := []struct {
		name    string
		connErr error
		wantErr bool
		wantMsg string
	}{
		{"valid group+source", nil, false, ""},
		{"conn error", errors.New("ssm join failed"), true, "JoinSSM"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := &mockIGMPConn{err: tc.connErr}
			err := JoinSSM(m, fakeIface, net.ParseIP("232.1.1.1"), net.ParseIP("10.0.0.1"))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.wantMsg != "" && !containsStr(err.Error(), tc.wantMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.wantMsg)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(m.joinSSM) != 1 {
					t.Errorf("expected 1 call, got %d", len(m.joinSSM))
				}
			}
		})
	}
}

func TestLeaveSSM(t *testing.T) {
	tests := []struct {
		name    string
		connErr error
		wantErr bool
		wantMsg string
	}{
		{"valid group+source", nil, false, ""},
		{"conn error", errors.New("ssm leave failed"), true, "LeaveSSM"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := &mockIGMPConn{err: tc.connErr}
			err := LeaveSSM(m, fakeIface, net.ParseIP("232.1.1.1"), net.ParseIP("10.0.0.1"))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.wantMsg != "" && !containsStr(err.Error(), tc.wantMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.wantMsg)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(m.leaveSSM) != 1 {
					t.Errorf("expected 1 call, got %d", len(m.leaveSSM))
				}
			}
		})
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
