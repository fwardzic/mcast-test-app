// Package multicast provides helpers for creating and managing multicast sockets.
// It wraps Go's standard net package with golang.org/x/net/ipv4 to expose
// multicast-specific socket options (TTL, interface, loopback) that the stdlib
// does not provide on its own.
package multicast

import (
	"fmt"
	"net"

	"golang.org/x/net/ipv4"
)

// PacketWriter is the interface used by the send loop to write packets.
// It exists so tests can substitute a mock without touching a real socket.
type PacketWriter interface {
	WriteTo(b []byte, dst net.Addr) (int, error)
	Close() error
}

// SenderConn is a multicast-aware UDP socket for sending packets.
// It wraps two layers:
//   - conn: the standard library net.PacketConn (owns the OS file descriptor)
//   - pc:   an ipv4.PacketConn that adds multicast socket options on top
//
// Both refer to the same underlying OS socket. We keep both because the stdlib
// conn is what we close (it owns the fd), while the ipv4 wrapper is how we set
// multicast TTL, interface, and loopback — options the stdlib doesn't expose.
type SenderConn struct {
	pc   *ipv4.PacketConn
	conn net.PacketConn
}

// Compile-time check: SenderConn must satisfy PacketWriter.
var _ PacketWriter = (*SenderConn)(nil)

// NewSenderConn creates a multicast sender socket bound to the given interface.
//
// Parameters:
//   - ifaceName: OS network interface name (e.g. "eth0"). Must exist and be up.
//   - ttl:       multicast TTL — how many router hops the packet may cross (1 = LAN only).
//   - loopback:  whether sent packets are looped back to the sending host.
//     NOTE: macOS defaults loopback to true; Linux defaults to true as well,
//     but behaviour can vary by kernel config. We set it explicitly to be safe.
func NewSenderConn(ifaceName string, ttl int, loopback bool) (*SenderConn, error) {
	// Bind to all interfaces on an ephemeral port — the OS picks a source port.
	// We only send; we never need to receive on this socket.
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("multicast: ListenPacket: %w", err)
	}

	// Wrap the stdlib conn with ipv4.PacketConn to unlock multicast options.
	pc := ipv4.NewPacketConn(conn)

	// Look up the interface by name. This is the most common point of failure
	// (typo in flag, interface not up), so the error message includes the name.
	ifi, err := net.InterfaceByName(ifaceName)
	if err != nil {
		conn.Close() // clean up the socket we already opened
		return nil, fmt.Errorf("multicast: interface %q not found: %w", ifaceName, err)
	}

	// Tell the kernel to send multicast packets out this specific interface
	// instead of whatever the routing table would normally choose.
	if err := pc.SetMulticastInterface(ifi); err != nil {
		conn.Close()
		return nil, fmt.Errorf("multicast: SetMulticastInterface: %w", err)
	}

	// Set multicast TTL (only affects packets to 224.0.0.0/4 destinations).
	if err := pc.SetMulticastTTL(ttl); err != nil {
		conn.Close()
		return nil, fmt.Errorf("multicast: SetMulticastTTL(%d): %w", ttl, err)
	}

	// Explicitly control whether the OS delivers our own multicast packets
	// back to us. We set this explicitly because the default varies by OS.
	if err := pc.SetMulticastLoopback(loopback); err != nil {
		conn.Close()
		return nil, fmt.Errorf("multicast: SetMulticastLoopback: %w", err)
	}

	return &SenderConn{pc: pc, conn: conn}, nil
}

// WriteTo sends payload b to the multicast destination dst.
// The nil second argument to pc.WriteTo means "no per-packet control message"
// — we already configured TTL and interface at socket level.
func (s *SenderConn) WriteTo(b []byte, dst net.Addr) (int, error) {
	return s.pc.WriteTo(b, nil, dst)
}

// Close shuts down the sender socket. We close the underlying net.PacketConn
// (which owns the file descriptor), NOT the ipv4.PacketConn — it is just a
// wrapper and has no Close method of its own.
func (s *SenderConn) Close() error {
	return s.conn.Close()
}
