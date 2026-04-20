// receiver.go provides IGMP join/leave helpers for multicast group management.
package multicast

import (
	"fmt"
	"net"
)

// IGMPConn is the interface for IGMP multicast group membership operations.
// *ipv4.PacketConn from golang.org/x/net/ipv4 satisfies this interface.
type IGMPConn interface {
	JoinGroup(ifi *net.Interface, group net.Addr) error
	LeaveGroup(ifi *net.Interface, group net.Addr) error
	JoinSourceSpecificGroup(ifi *net.Interface, group, source net.Addr) error
	LeaveSourceSpecificGroup(ifi *net.Interface, group, source net.Addr) error
}

// JoinASM sends an IGMP membership report for the given ASM multicast group
// on the specified interface.
func JoinASM(conn IGMPConn, ifi *net.Interface, group net.IP) error {
	if err := conn.JoinGroup(ifi, &net.UDPAddr{IP: group}); err != nil {
		return fmt.Errorf("multicast: JoinASM %s: %w", group, err)
	}
	return nil
}

// LeaveASM sends an IGMP leave message for the given ASM multicast group
// on the specified interface.
func LeaveASM(conn IGMPConn, ifi *net.Interface, group net.IP) error {
	if err := conn.LeaveGroup(ifi, &net.UDPAddr{IP: group}); err != nil {
		return fmt.Errorf("multicast: LeaveASM %s: %w", group, err)
	}
	return nil
}

// JoinSSM sends an IGMPv3 source-specific membership report for the given
// (group, source) pair on the specified interface.
func JoinSSM(conn IGMPConn, ifi *net.Interface, group, source net.IP) error {
	if err := conn.JoinSourceSpecificGroup(ifi, &net.UDPAddr{IP: group}, &net.UDPAddr{IP: source}); err != nil {
		return fmt.Errorf("multicast: JoinSSM %s: %w", group, err)
	}
	return nil
}

// LeaveSSM sends an IGMPv3 source-specific leave for the given (group, source)
// pair on the specified interface.
func LeaveSSM(conn IGMPConn, ifi *net.Interface, group, source net.IP) error {
	if err := conn.LeaveSourceSpecificGroup(ifi, &net.UDPAddr{IP: group}, &net.UDPAddr{IP: source}); err != nil {
		return fmt.Errorf("multicast: LeaveSSM %s: %w", group, err)
	}
	return nil
}
