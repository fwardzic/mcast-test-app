package multicast

import (
	"fmt"
	"net"
	"sync"

	"golang.org/x/net/ipv4"
)

// ReceiverConn is a multicast-aware UDP socket for receiving packets.
// It mirrors SenderConn but is configured for receiving: it binds to a
// specific port and enables control-message flags so each ReadFrom returns
// the TTL, source IP, and destination IP of the incoming packet.
type ReceiverConn struct {
	pc       *ipv4.PacketConn
	conn     net.PacketConn
	closeOnce sync.Once
	closeErr  error
}

// NewReceiverConn creates a multicast receiver socket bound to the given port.
//
// The socket listens on all interfaces (0.0.0.0) because the actual interface
// selection happens later via IGMP group joins (JoinGroup / JoinSourceSpecificGroup).
//
// SetControlMessage tells the kernel to populate a control message on each
// received packet with:
//   - FlagTTL: the IP TTL value (useful for debugging hop counts)
//   - FlagSrc: the source IP address
//   - FlagDst: the destination (multicast group) IP address
func NewReceiverConn(port int) (*ReceiverConn, error) {
	// Bind to 0.0.0.0 on the specified port so we can receive multicast
	// packets destined to any group on this port.
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: port})
	if err != nil {
		return nil, fmt.Errorf("multicast: NewReceiverConn port %d: %w", port, err)
	}

	// Wrap with ipv4.PacketConn to access multicast socket options and
	// per-packet control messages (TTL, src, dst).
	pc := ipv4.NewPacketConn(conn)

	// Enable control-message fields we need for display and debugging.
	if err := pc.SetControlMessage(ipv4.FlagTTL|ipv4.FlagSrc|ipv4.FlagDst, true); err != nil {
		conn.Close()
		return nil, fmt.Errorf("multicast: NewReceiverConn port %d: %w", port, err)
	}

	return &ReceiverConn{pc: pc, conn: conn}, nil
}

// Close shuts down the receiver socket. We close the underlying net.PacketConn
// which owns the file descriptor — the ipv4.PacketConn is just a wrapper.
func (r *ReceiverConn) Close() error {
	r.closeOnce.Do(func() {
		r.closeErr = r.conn.Close()
	})
	return r.closeErr
}

// PC returns the underlying ipv4.PacketConn for IGMP membership management
// and reading packets with control messages.
func (r *ReceiverConn) PC() *ipv4.PacketConn { return r.pc }
