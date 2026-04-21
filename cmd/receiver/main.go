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
	"strings"
	"sync"
	"syscall"
	"time"

	flag "github.com/spf13/pflag"
	"golang.org/x/net/ipv4"
	"golang.org/x/term"

	"github.com/fwardzic/mcast-test-app/internal/config"
	"github.com/fwardzic/mcast-test-app/internal/display"
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

	// Detect whether stdout is a TTY. If so, use the ANSI scrolling display;
	// otherwise fall back to structured log output (e.g. when piped or in k8s).
	isTTY := term.IsTerminal(int(os.Stdout.Fd()))
	var termHeight int
	if isTTY {
		_, termHeight, _ = term.GetSize(int(os.Stdout.Fd()))
	}

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

	if !isTTY || termHeight < 10 {
		if termHeight < 10 && isTTY {
			slog.Warn("terminal too small for ANSI display, falling back to log output", "height", termHeight)
		}
		wg.Add(1)
		go groupManager(packetCh, nil, nil, specs, rc.PC(), ifi, &wg)
	} else {
		scrollRows := termHeight - 1
		display.Init(os.Stdout, scrollRows)
		defer display.Teardown(os.Stdout, termHeight)

		linesCh := make(chan string, 64)
		statsCh := make(chan string, 4)

		wg.Add(1)
		go displayLoop(ctx, linesCh, statsCh, scrollRows, &wg)

		wg.Add(1)
		go groupManager(packetCh, linesCh, statsCh, specs, rc.PC(), ifi, &wg)
	}

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

// displayLoop renders the terminal UI at 10 Hz. It receives pre-formatted
// lines from groupManager via linesCh and stats updates via statsCh.
// It does NOT read from packetCh — groupManager is the sole consumer.
func displayLoop(
	ctx context.Context,
	linesCh <-chan string,
	statsCh <-chan string,
	scrollRows int,
	wg *sync.WaitGroup,
) {
	defer wg.Done()
	ring := display.NewRingBuf(scrollRows)
	status := ""
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)
	for {
		select {
		case line, ok := <-linesCh:
			if !ok {
				return
			}
			ring.Push(line)
		case s, ok := <-statsCh:
			if ok {
				status = s
			}
		case <-ticker.C:
			display.Render(os.Stdout, ring.Ordered(), status, scrollRows)
		case <-sigCh:
			_, h, err := term.GetSize(int(os.Stdout.Fd()))
			if err == nil && h >= 10 {
				scrollRows = h - 1
				ring = display.NewRingBuf(scrollRows)
				display.Init(os.Stdout, scrollRows)
			}
		case <-ctx.Done():
			return
		}
	}
}

// formatLine formats a received packet into a single display line with color.
func formatLine(rp ReceivedPacket, colorCode int) string {
	delta := "?"
	t, err := time.Parse("2006-01-02T15:04:05.000Z07:00", rp.Pkt.Timestamp)
	if err == nil {
		delta = fmt.Sprintf("%d", time.Since(t).Milliseconds())
	}
	line := fmt.Sprintf("%s -> %s  seq=%d  ttl=%d  sym=%s  d=%sms",
		rp.Src.String(), rp.Dst.String(),
		rp.Pkt.Sequence, rp.TTL, rp.Pkt.Symbol, delta)
	return display.Colorize(line, colorCode)
}

// rateWindow tracks packet timestamps over a sliding window to compute rate.
type rateWindow struct {
	times []time.Time
}

func (w *rateWindow) record(t time.Time) {
	w.times = append(w.times, t)
}

func (w *rateWindow) rate() float64 {
	const window = 5 * time.Second
	cutoff := time.Now().Add(-window)
	start := 0
	for start < len(w.times) && w.times[start].Before(cutoff) {
		start++
	}
	w.times = w.times[start:]
	return float64(len(w.times)) / window.Seconds()
}

// buildStatus creates the status line summarising per-group stats and rates.
func buildStatus(specs []config.GroupSpec, stats map[string]*GroupStats, rates map[string]*rateWindow) string {
	parts := make([]string, 0, len(specs))
	for _, spec := range specs {
		gs := stats[spec.Group]
		r := rates[spec.Group].rate()
		parts = append(parts, fmt.Sprintf("%s: %d pkts  %d gaps  %.1f pkt/s",
			spec.Group, gs.PktCount, gs.GapCount, r))
	}
	return strings.Join(parts, " | ")
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
	linesCh chan<- string,
	statsCh chan<- string,
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

	// Color map uses normalised IP keys to match rp.Dst.String() lookups.
	colorMap := make(map[string]int, len(specs))
	for i, spec := range specs {
		key := net.ParseIP(spec.Group).String()
		colorMap[key] = display.GroupColor(i)
	}
	rates := make(map[string]*rateWindow)
	for _, spec := range specs {
		rates[spec.Group] = &rateWindow{}
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

		// Track rate and send formatted line + stats to displayLoop (TTY mode).
		rates[rp.Dst.String()].record(time.Now())
		if linesCh != nil {
			colorCode := colorMap[rp.Dst.String()]
			select {
			case linesCh <- formatLine(rp, colorCode):
			default: // don't block if displayLoop hasn't consumed yet
			}
		}
		if statsCh != nil {
			select {
			case statsCh <- buildStatus(specs, stats, rates):
			default: // don't block if displayLoop hasn't consumed yet
			}
		}

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

	// Close display channels so displayLoop can exit cleanly.
	if linesCh != nil {
		close(linesCh)
	}
	if statsCh != nil {
		close(statsCh)
	}
}