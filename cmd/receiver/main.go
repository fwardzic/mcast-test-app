// Package main implements the multicast receiver binary.
//
// It reads CLI flags, opens a multicast socket, joins the configured groups,
// and runs a receive loop that decodes incoming packets and tracks per-group
// sequence gaps. A clean shutdown happens when the process receives SIGINT
// (Ctrl-C) or SIGTERM: the watcher goroutine closes the socket to unblock
// the read loop, groupManager leaves all groups, and everything drains cleanly.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	flag "github.com/spf13/pflag"
	"golang.org/x/net/ipv4"

	"github.com/fwardzic/mcast-test-app/internal/config"
	"github.com/fwardzic/mcast-test-app/internal/multicast"
	"github.com/fwardzic/mcast-test-app/internal/packet"
)

// CLI flags. Receiver needs fewer flags than the sender: no symbols, TTL,
// rate, or loopback — it just listens and reports what it sees.
var (
	groups = flag.StringArrayP("group", "g", nil, "Multicast group address (repeatable)")
	source = flag.StringP("source", "s", "", "Source IP for SSM groups (232.0.0.0/8 only)")
	port   = flag.IntP("port", "p", config.DefaultPort, "UDP destination port")
	iface  = flag.StringP("iface", "i", "", "Network interface name (required)")
)

// ReceivedPacket bundles a decoded packet with IP-layer metadata from the
// ipv4.ControlMessage. This decouples groupManager from the ipv4 package.
type ReceivedPacket struct {
	Pkt *packet.Packet // decoded JSON payload
	TTL int            // IP TTL from cm.TTL
	Src net.IP         // unicast sender IP from cm.Src
	Dst net.IP         // multicast group IP from cm.Dst
}

// GroupStats tracks per-group sequence and loss counters.
type GroupStats struct {
	PktCount uint64
	GapCount uint64
	LastSeq  uint64
	FirstPkt bool // true until the first packet is received; prevents false gap on init
}

func main() {
	// Use structured logging to stderr so stdout stays clean for piping.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	flag.Parse()

	if err := validateFlags(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}

	// signal.NotifyContext is preferred over raw signal.Notify because it
	// returns a context we can pass directly into goroutines. When a signal
	// arrives, the context is cancelled automatically.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	rc, err := multicast.NewReceiverConn(*port)
	if err != nil {
		slog.Error("failed to create receiver socket", "err", err)
		os.Exit(1)
	}
	defer rc.Close()

	ifi, err := net.InterfaceByName(*iface)
	if err != nil {
		slog.Error("failed to find interface", "iface", *iface, "err", err)
		os.Exit(1)
	}

	// Join all groups before starting the receive loop so we don't miss
	// packets that arrive between socket creation and join.
	specs := buildSpecs()
	for _, spec := range specs {
		ip := net.ParseIP(spec.Group)
		if config.IsSSMAddress(ip) && spec.SourceIP != "" {
			if err := multicast.JoinSSM(rc.PC(), ifi, ip, net.ParseIP(spec.SourceIP)); err != nil {
				slog.Error("SSM join failed", "group", spec.Group, "source", spec.SourceIP, "err", err)
				os.Exit(1)
			}
		} else {
			if err := multicast.JoinASM(rc.PC(), ifi, ip); err != nil {
				slog.Error("ASM join failed", "group", spec.Group, "err", err)
				os.Exit(1)
			}
		}
	}

	slog.Info("receiver starting", "groups", *groups, "port", *port, "iface", *iface)

	packetCh := make(chan ReceivedPacket, 64)

	var wg sync.WaitGroup

	// Watcher goroutine: unblocks pc.ReadFrom on shutdown by closing the
	// underlying connection. No WaitGroup needed — it has no cleanup.
	go func() {
		<-ctx.Done()
		rc.Close()
	}()

	wg.Add(1)
	go receiveLoop(ctx, rc.PC(), packetCh, &wg)

	wg.Add(1)
	go groupManager(packetCh, specs, rc.PC(), ifi, &wg)

	wg.Wait()
	slog.Info("receiver: shutdown complete")
}

// validateFlags checks that all required flags are present and values are sane.
// It runs before any socket operations so we fail fast with a clear message.
func validateFlags() error {
	if *iface == "" {
		return fmt.Errorf("--iface is required")
	}

	if len(*groups) == 0 {
		return fmt.Errorf("at least one --group flag is required")
	}

	for i, grp := range *groups {
		ip := net.ParseIP(grp)
		if ip == nil {
			return fmt.Errorf("group[%d]: invalid address %q", i, grp)
		}
		if !ip.IsMulticast() {
			return fmt.Errorf("group[%d]: %q is not a multicast address", i, grp)
		}
		isSSM := config.IsSSMAddress(ip)
		if isSSM && *source == "" {
			return fmt.Errorf("group[%d]: SSM group %s requires --source flag", i, grp)
		}
		if !isSSM && *source != "" {
			return fmt.Errorf("group[%d]: non-SSM group %s cannot use --source; use a 232.0.0.0/8 address for SSM", i, grp)
		}
	}

	spec := config.GroupSpec{Port: *port}
	return spec.Validate()
}

// buildSpecs converts CLI flags into a slice of GroupSpec values for use by
// the join loop and groupManager.
func buildSpecs() []config.GroupSpec {
	specs := make([]config.GroupSpec, len(*groups))
	for i, grp := range *groups {
		specs[i] = config.GroupSpec{
			Group:  grp,
			Port:   *port,
			SourceIP: *source,
		}
	}
	return specs
}

// receiveLoop reads packets from the multicast socket and sends decoded
// results to packetCh until the context is cancelled (socket closed by watcher).
//
// The blocking pc.ReadFrom call is unblocked by the watcher goroutine closing
// the underlying connection. We distinguish intentional shutdown (ctx.Err()
// != nil) from unexpected errors by checking the context state.
func receiveLoop(ctx context.Context, pc *ipv4.PacketConn, packetCh chan<- ReceivedPacket, wg *sync.WaitGroup) {
	defer wg.Done()
	defer close(packetCh) // signals groupManager to drain and exit

	buf := make([]byte, 65536) // max UDP payload; allocated once outside the loop

	for {
		n, cm, _, err := pc.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				return // clean shutdown — conn was closed by watcher goroutine
			}
			slog.Error("read error", "err", err)
			return
		}

		p, err := packet.Decode(buf[:n])
		if err != nil {
			slog.Warn("malformed packet", "err", err)
			continue // log and skip; don't crash the loop
		}

		rp := ReceivedPacket{Pkt: p}
		if cm != nil {
			rp.TTL = cm.TTL
			rp.Src = cm.Src
			rp.Dst = cm.Dst
		}
		packetCh <- rp
	}
}

// groupManager processes decoded packets from packetCh, tracking per-group
// sequence numbers and gap counts. When the channel closes (receiveLoop exited),
// it leaves all multicast groups before returning.
//
// Gap detection: if we see sequence N followed by sequence N+k (k > 1), we know
// k-1 packets were lost. The first packet for each group initialises LastSeq
// without counting a gap. Sender restarts (sequence goes backward) reset LastSeq
// without counting gaps — the sender legitimately restarted.
func groupManager(
	packetCh <-chan ReceivedPacket,
	specs []config.GroupSpec,
	conn multicast.IGMPConn,
	ifi *net.Interface,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	// Initialise stats for each joined group.
	stats := make(map[string]*GroupStats)
	for _, spec := range specs {
		stats[spec.Group] = &GroupStats{FirstPkt: true}
	}

	// range over channel: blocks until receiveLoop closes packetCh.
	for rp := range packetCh {
		pkt := rp.Pkt
		gs, ok := stats[rp.Dst.String()]
		if !ok {
			// Packet for a group we didn't join — shouldn't happen, log and skip.
			slog.Warn("unexpected group", "dst", rp.Dst.String())
			continue
		}

		// Gap detection logic.
		if gs.FirstPkt {
			// First packet for this group: initialise, no gap.
			gs.LastSeq = pkt.Sequence
			gs.FirstPkt = false
		} else if pkt.Sequence > gs.LastSeq+1 {
			// Forward gap: one or more packets were lost.
			gs.GapCount += pkt.Sequence - gs.LastSeq - 1
			gs.LastSeq = pkt.Sequence
		} else if pkt.Sequence < gs.LastSeq {
			// Sequence went backward — sender restart detected (D-14).
			// Reset LastSeq without counting a gap.
			slog.Warn("sender restart detected",
				"group", rp.Dst.String(),
				"lastSeq", gs.LastSeq,
				"newSeq", pkt.Sequence,
			)
			gs.LastSeq = pkt.Sequence
		} else if pkt.Sequence == gs.LastSeq {
			// Duplicate packet — log warning, don't update LastSeq.
			slog.Warn("duplicate packet",
				"group", rp.Dst.String(),
				"seq", pkt.Sequence,
			)
		} else {
			// Normal in-order packet (seq == lastSeq+1).
			gs.LastSeq = pkt.Sequence
		}

		gs.PktCount++

		slog.Info("recv",
			"group", rp.Dst.String(),
			"seq", pkt.Sequence,
			"symbol", pkt.Symbol,
			"src", rp.Src.String(),
			"ttl", rp.TTL,
			"pkts", gs.PktCount,
			"gaps", gs.GapCount,
		)
	}

	// packetCh closed → receiveLoop has exited → safe to leave groups.
	for _, spec := range specs {
		ip := net.ParseIP(spec.Group)
		slog.Info("leaving group", "group", spec.Group)
		if config.IsSSMAddress(ip) && spec.SourceIP != "" {
			if err := multicast.LeaveSSM(conn, ifi, ip, net.ParseIP(spec.SourceIP)); err != nil {
				slog.Error("SSM leave failed", "group", spec.Group, "err", err)
			}
		} else {
			if err := multicast.LeaveASM(conn, ifi, ip); err != nil {
				slog.Error("ASM leave failed", "group", spec.Group, "err", err)
			}
		}
	}
}
