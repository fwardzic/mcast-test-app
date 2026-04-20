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
	group    = flag.StringP("group", "g", config.DefaultGroup, "Multicast group address")
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

	dst := &net.UDPAddr{IP: net.ParseIP(*group), Port: *port}

	slog.Info("sender starting",
		"group", *group,
		"port", *port,
		"iface", *iface,
		"rate", *rate,
		"ttl", *ttl,
	)

	// We use a WaitGroup to ensure the send loop goroutine finishes before
	// main returns. This guarantees the "shutdown complete" log line appears
	// only after all sending has stopped.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sendLoop(ctx, sc, dst, *group, *rate)
	}()

	wg.Wait()
	slog.Info("sender: shutdown complete")
}

// validateFlags checks that all required flags are present and values are sane.
// It runs before any socket operations so we fail fast with a clear message.
func validateFlags() error {
	if *iface == "" {
		return fmt.Errorf("--iface is required")
	}

	ip := net.ParseIP(*group)
	if ip == nil {
		return fmt.Errorf("invalid group address: %q", *group)
	}
	if !ip.IsMulticast() {
		return fmt.Errorf("address %q is not a multicast address (must be 224.0.0.0/4)", *group)
	}

	// Reuse the config package's validation for port and TTL ranges.
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
func sendLoop(ctx context.Context, w multicast.PacketWriter, dst net.Addr, grp string, pps int) {
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
				Group:     grp,
				Timestamp: packet.Now(),
			}

			data, err := packet.Encode(p)
			if err != nil {
				// Encode errors are programming bugs (nil pointer, etc.), not
				// transient failures. We log and continue rather than crashing
				// because losing one packet is better than killing the sender.
				slog.Error("encode failed", "err", err)
				continue
			}

			// WriteTo errors are typically transient network issues (interface
			// down, buffer full). We log and keep going so the sender recovers
			// automatically when the network comes back.
			if _, err := w.WriteTo(data, dst); err != nil {
				slog.Error("send failed", "err", err, "seq", seq)
				continue
			}

			slog.Info("sent", "seq", seq, "group", grp)
		}
	}
}
