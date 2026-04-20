// Package main implements the multicast sender binary.
//
// It reads CLI flags, opens a multicast socket, and runs a send loop that
// publishes one JSON-encoded packet per tick to the configured multicast group.
// A clean shutdown happens when the process receives SIGINT (Ctrl-C) or SIGTERM.
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
	"time"

	flag "github.com/spf13/pflag"

	"github.com/fwardzic/mcast-test-app/internal/config"
	"github.com/fwardzic/mcast-test-app/internal/multicast"
	"github.com/fwardzic/mcast-test-app/internal/packet"
)

// CLI flags. We define them at package level so both main() and validateFlags()
// can access the same pointers. pflag gives us GNU-style --long and -short flags.
var (
	groups   = flag.StringArrayP("group", "g", nil, "Multicast group address (repeatable)")
	symbols  = flag.StringArrayP("symbol", "S", nil, "Ticker symbol paired by index (repeatable)")
	source   = flag.StringP("source", "s", "", "Source IP for SSM groups (232.0.0.0/8 only)")
	port     = flag.IntP("port", "p", config.DefaultPort, "UDP destination port")
	iface    = flag.StringP("iface", "i", "", "Network interface name (required)")
	ttl      = flag.IntP("ttl", "t", 2, "Multicast TTL (hop limit)")
	rate     = flag.IntP("rate", "r", config.DefaultRate, "Packets per second")
	loopback = flag.BoolP("loopback", "l", false, "Enable multicast loopback")
)

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
	// arrives, the context is cancelled automatically — no manual channel select needed.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sc, err := multicast.NewSenderConn(*iface, *ttl, *loopback)
	if err != nil {
		slog.Error("failed to create sender socket", "err", err)
		os.Exit(1)
	}
	defer sc.Close()

	slog.Info("sender starting",
		"groups", *groups,
		"port", *port,
		"iface", *iface,
		"rate", *rate,
		"ttl", *ttl,
	)

	var wg sync.WaitGroup
	for i, grp := range *groups {
		spec := config.GroupSpec{
			Group:    grp,
			Port:     *port,
			Symbol:   (*symbols)[i],
			SourceIP: *source,
		}
		dst := &net.UDPAddr{IP: net.ParseIP(spec.Group), Port: spec.Port}
		wg.Add(1)
		go func(spec config.GroupSpec, dst *net.UDPAddr) {
			defer wg.Done()
			sendLoop(ctx, sc, dst, spec, *rate)
		}(spec, dst)
	}
	wg.Wait()
	slog.Info("sender: shutdown complete")
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
	if len(*symbols) != len(*groups) {
		return fmt.Errorf("number of --symbol flags (%d) must match --group flags (%d)",
			len(*symbols), len(*groups))
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
	spec := config.GroupSpec{Port: *port, TTL: *ttl}
	if err := spec.Validate(); err != nil {
		return err
	}
	if *rate < 1 {
		return fmt.Errorf("rate must be >= 1")
	}
	return nil
}

// sendLoop sends one packet per tick until the context is cancelled.
//
// How it works: time.NewTicker fires at a fixed interval (1/rate seconds).
// The for-select loop waits for either a tick (send a packet) or context
// cancellation (stop sending). This pattern is the standard Go way to run
// a periodic task with clean shutdown.
func sendLoop(ctx context.Context, w multicast.PacketWriter, dst net.Addr, spec config.GroupSpec, pps int) {
	ticker := time.NewTicker(time.Second / time.Duration(pps))
	defer ticker.Stop()

	// seq starts at 0 and is pre-incremented, so the first packet sent has
	// sequence number 1. This makes gap detection simpler on the receiver side:
	// if the receiver sees seq N followed by seq N+2, exactly one packet was lost.
	var seq uint64

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			seq++

			p := &packet.Packet{
				Sequence:  seq,
				Group:     spec.Group,
				Symbol:    spec.Symbol,
				Timestamp: packet.Now(),
			}

			data, err := packet.Encode(p)
			if err != nil {
				// Encode errors are programming bugs (nil pointer, etc.), not
				// transient failures. We log and continue rather than crashing
				// because losing one packet is better than killing the sender.
				slog.Error("encode failed", "err", err, "symbol", spec.Symbol)
				continue
			}

			// WriteTo errors are typically transient network issues (interface
			// down, buffer full). We log and keep going so the sender recovers
			// automatically when the network comes back.
			if _, err := w.WriteTo(data, dst); err != nil {
				slog.Error("send failed", "err", err, "seq", seq, "group", spec.Group)
				continue
			}

			slog.Info("sent", "seq", seq, "group", spec.Group, "symbol", spec.Symbol)
		}
	}
}
