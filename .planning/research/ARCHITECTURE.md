# Architecture Research: Go Multicast Sender/Receiver Tool

*Researched: 2026-04-20*

---

## 1. Binary Separation Strategy

**Recommendation: Two separate `main` packages under `cmd/`**

```
cmd/
  sender/main.go
  receiver/main.go
```

**Why separate binaries over subcommands:**
- Sender and receiver have completely different runtime profiles. The sender never reads from a socket; the receiver never writes to one. A single binary with subcommands (`mcast sender` / `mcast receiver`) adds a dispatch layer with zero benefit.
- Each binary can be statically compiled and dropped independently into a container — simpler `Dockerfile` / Kubernetes manifest per role.
- Beginner-friendly: opening `cmd/sender/main.go` gives a complete, linear narrative of what the sender does without branching on a mode flag.
- Separate binaries do not preclude shared packages — they simply import them.

---

## 2. Package Layout

**Recommendation: `cmd/` + `internal/`** (small but structured)

```
mcast-test-app/
├── cmd/
│   ├── sender/
│   │   └── main.go          ← wires everything together, owns flag parsing
│   └── receiver/
│       └── main.go          ← wires everything together, owns flag parsing
├── internal/
│   ├── packet/              ← shared wire format (encode/decode)
│   ├── display/             ← ANSI terminal engine
│   ├── multicast/           ← UDP socket helpers, group join/leave
│   └── config/              ← shared flag/config structs
├── k8s/                     ← Kubernetes manifests
├── go.mod
└── go.sum
```

**Why `internal/` over flat:**
- `internal/` prevents external import (nobody else will accidentally depend on your wire format).
- Each sub-package has a single clear responsibility, making it easier to explain to a Go beginner ("this package only knows about how bytes are structured on the wire").
- A flat layout (`packet.go`, `display.go` in root) works for very small projects but collapses quickly; the four domains here (packet, display, multicast, config) map naturally to four packages.

**Why not go deeper:**
- No `pkg/` layer — that's a pattern for libraries exported to others, which this isn't.
- No `internal/sender/` or `internal/receiver/` sub-trees — the separation lives at `cmd/`, not inside `internal/`.

---

## 3. Goroutine Architecture

### Sender

```
main goroutine
  └── flag parse → build config → open UDP socket
        └── spawn: stdin command loop  (goroutine)
        └── spawn: send ticker loop    (goroutine)
        └── wait: shutdown signal      (main blocks on context cancel)
```

| Goroutine | Role | Communication |
|-----------|------|---------------|
| `sendLoop` | Sends one packet per tick per group; increments sequence | receives `stopCh` / context cancel |
| `stdinLoop` | Reads interactive commands (join/leave group) | sends `command` structs on a channel to `sendLoop` or a group manager |
| `main` | Signal handler; blocks until context cancelled | owns `cancel()` |

### Receiver

```
main goroutine
  └── flag parse → build config → join groups → open UDP socket
        └── spawn: receive loop      (goroutine)
        └── spawn: display loop      (goroutine)
        └── spawn: stdin command loop (goroutine)
        └── wait: shutdown signal    (main blocks on context cancel)
```

| Goroutine | Role | Communication |
|-----------|------|---------------|
| `receiveLoop` | `ReadFromUDP` in a tight loop; parses raw bytes into `Packet` structs | sends `Packet` on `packetCh chan Packet` (buffered) |
| `displayLoop` | Consumes `packetCh`; maintains line buffer; redraws scroll region | owns terminal state |
| `stdinLoop` | Reads lines from stdin; parses join/leave commands | sends `Command` on `cmdCh chan Command` to a `groupManager` |
| `groupManager` | Serialises group membership changes (join/leave syscalls) | receives `Command`, calls `multicast.Join/Leave` |
| `main` | Signal handler | owns context + `cancel()` |

**Key design principle:** `receiveLoop` and `displayLoop` are deliberately decoupled by a channel. The UDP read path must never block on terminal I/O.

**Channel sizing:** `packetCh` should be buffered (~64 slots). If the display loop falls behind, packets are dropped at the channel (with a counter), not at the socket. This keeps the receive loop hot.

---

## 4. Shared Code Between Sender and Receiver

### `internal/packet` — Wire Format

Both binaries must agree on the byte layout. This package owns:

```go
type Packet struct {
    Sequence  uint64
    Group     net.IP
    Source    net.IP
    Timestamp time.Time
    Payload   string   // short ticker-style string, e.g. "AAPL +0.42%"
}

func Encode(p Packet) ([]byte, error)
func Decode(b []byte) (Packet, error)
```

Implementation: JSON or a simple length-prefixed binary. JSON is recommended for a beginner project — human-readable with `tcpdump`, no custom parser needed, tiny payloads.

### `internal/config` — Shared Config Structs

```go
type GroupSpec struct {
    Group  net.IP
    Source net.IP   // nil = ASM, set = SSM
    Port   int
}
```

Both `cmd/sender/main.go` and `cmd/receiver/main.go` parse flags into `[]GroupSpec`. Parsing lives in `main`; the struct lives in `config`.

### `internal/display` — ANSI Engine

Only the receiver uses this at runtime, but defining it as a package means:
- It can be unit-tested without a real terminal.
- The sender could later gain a minimal status line without copy-paste.

### `internal/multicast` — Socket Helpers

Wraps `golang.org/x/net/ipv4` and `net` stdlib to provide:

```go
func NewSenderConn(iface string, port int) (*net.UDPConn, error)
func NewReceiverConn(iface string, port int) (*net.UDPConn, error)
func JoinASM(conn *net.UDPConn, iface *net.Interface, group net.IP) error
func LeaveASM(conn *net.UDPConn, iface *net.Interface, group net.IP) error
func JoinSSM(conn *net.UDPConn, iface *net.Interface, group, source net.IP) error
func LeaveSSM(conn *net.UDPConn, iface *net.Interface, group, source net.IP) error
```

Isolating syscall details here means `main.go` never touches `ipv4.PacketConn` directly.

---

## 5. ANSI Display Engine Design

### Fixed-Height Scroll Region

The display shows the last N lines (N ≈ 20) of received packets inside a fixed vertical region without scrolling the whole terminal. This uses ANSI scroll-region escape codes.

**Core escape sequences needed:**
```
ESC[<top>;<bottom>r   — set scroll region
ESC[<row>;<col>H      — move cursor to row, col
ESC[2K                — erase entire line
ESC[s / ESC[u         — save / restore cursor position
ESC[?25l / ESC[?25h   — hide / show cursor
```

### Render Loop Design

```
displayLoop:
  maintain: []string lines   (ring buffer, cap = displayHeight)
  on receive Packet from packetCh:
    format Packet → one display line string
    append to ring buffer (drop oldest if full)
    call render(lines)

render(lines):
  move cursor to top of scroll region
  for each line: print, clear to end, move down
  move cursor below scroll region (status/command line)
```

**Why a ring buffer instead of re-drawing from scratch each packet:**
- Cheap: only one write syscall sequence per packet.
- Correct: if no packet arrives, the display doesn't flicker or blank.

### Status Line

Reserve 2 lines below the scroll region:
- Line N+1: joined groups summary (updated on join/leave)
- Line N+2: command prompt (`> `) for stdin input

### Initialisation / Teardown

```
Init():
  hide cursor
  set scroll region [1, displayHeight]
  clear scroll region
  draw header line (static, above scroll region)

Teardown():
  reset scroll region (ESC[r)
  show cursor
  move cursor to bottom
```

This ensures a clean terminal after Ctrl-C.

---

## 6. Multiple Group Membership

**Data structure:** `map[string]GroupSpec` keyed by `"group:source"` (source = `""` for ASM).

**Operations are serialised** through the `groupManager` goroutine (or a `sync.Mutex` around the map). Never join/leave from two goroutines concurrently — the kernel IGMP state and the map must stay consistent.

**At startup:** parse `--group` flags → call `JoinASM/JoinSSM` for each → populate map.

**Interactive join/leave:**
```
> join 239.1.1.1
> join 239.1.1.2 source 10.0.0.5   ← SSM
> leave 239.1.1.1
```

The stdin parser produces a `Command{Op, GroupSpec}` sent to `groupManager`. The groupManager updates the map and calls the appropriate join/leave helper.

**Receive loop:** a single `UDPConn` receives from all joined groups on the same port (standard multicast socket behaviour). The destination IP in the IP header (read via `ReadMsgUDP` or `ipv4.PacketConn.ReadFrom`) identifies which group the packet was destined for.

---

## 7. Signal Handling and Graceful Shutdown

```go
// In main():
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

go func() {
    <-sigCh
    cancel()
}()

// All goroutines receive ctx; select on ctx.Done() to exit.
```

**Shutdown sequence (receiver):**
1. `cancel()` fires → all goroutines begin exit.
2. `receiveLoop` exits → closes `packetCh`.
3. `displayLoop` drains `packetCh` then exits (range over closed channel).
4. `groupManager` leaves all groups (deferred `Leave` calls or explicit drain of `cmdCh`).
5. `display.Teardown()` resets terminal.
6. `main` returns.

**Why not `sync.WaitGroup` everywhere:** A `WaitGroup` is appropriate for fan-out workers of equal priority. Here the shutdown order matters (display must outlive receive, teardown must be last), so a sequenced context + channel close is clearer.

---

## 8. Component Boundaries and Data Flow

```
┌─────────────────────────────────────────────────────────┐
│ cmd/sender/main.go                                      │
│  ┌──────────┐   []GroupSpec   ┌────────────────────┐   │
│  │ flag/    │ ─────────────→  │ multicast.Sender   │   │
│  │ config   │                 │ (UDP write loop)   │   │
│  └──────────┘                 └────────────────────┘   │
│  ┌──────────┐  Command chan   ┌────────────────────┐   │
│  │ stdin    │ ─────────────→  │ groupManager       │   │
│  │ loop     │                 │ (add/remove groups)│   │
│  └──────────┘                 └────────────────────┘   │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│ cmd/receiver/main.go                                    │
│                                                         │
│  [UDP socket] ──raw bytes──→ receiveLoop                │
│                                   │ packet.Decode()     │
│                                   ↓                     │
│                            packetCh (buffered)          │
│                                   │                     │
│                                   ↓                     │
│                            displayLoop                  │
│                            (ring buffer + ANSI render)  │
│                                                         │
│  [stdin] ──text──→ stdinLoop ──Command──→ groupManager  │
│                                               │         │
│                                               ↓         │
│                                    multicast.Join/Leave │
│                                    (updates socket + map│
└─────────────────────────────────────────────────────────┘

Shared packages (no goroutines, pure functions / types):
  internal/packet   ←── used by both sender (Encode) and receiver (Decode)
  internal/config   ←── used by both for GroupSpec
  internal/display  ←── used by receiver displayLoop
  internal/multicast ←── used by both for socket setup; receiver for join/leave
```

**Data flow summary:**

| Flow | Direction | Mechanism |
|------|-----------|-----------|
| Raw UDP bytes → structured Packet | receiveLoop → displayLoop | buffered channel |
| Interactive commands → group state | stdinLoop → groupManager | unbuffered command channel |
| Packet → terminal lines | displayLoop → stdout | ANSI write |
| Config → all components | main → goroutines at spawn | function args / closure |
| Shutdown signal → all goroutines | OS → main → all | context cancellation |

---

## 9. Suggested Build Order

Dependencies flow upward; build lower layers first.

```
Phase 1 — Foundation (no goroutines yet)
  internal/config      ← no deps
  internal/packet      ← no deps (just encoding)
  internal/multicast   ← depends on net, x/net/ipv4

Phase 2 — Sender binary (simpler: no display, no receive)
  cmd/sender/main.go   ← config + packet + multicast
  Validates: packet encoding, UDP send, multicast group targeting

Phase 3 — Display engine (isolated, testable)
  internal/display     ← no deps beyond os/fmt
  Test: mock lines → verify ANSI escape output

Phase 4 — Receiver binary
  cmd/receiver/main.go ← config + packet + multicast + display
  Validates: full receive → decode → display pipeline

Phase 5 — Interactive commands (both binaries)
  stdinLoop + groupManager in both cmd/sender and cmd/receiver
  Validates: dynamic join/leave, runtime group changes

Phase 6 — Kubernetes manifests
  k8s/sender.yaml, k8s/receiver.yaml
  Validates: end-to-end in-cluster multicast
```

**Build order rationale:**
- The sender is built before the receiver because it has no display dependency — it's the simpler binary and exercises the shared packet/multicast foundation first.
- The display engine is built as a standalone package with mock input before it's wired into the receiver goroutine architecture — this keeps the ANSI complexity debuggable in isolation.
- Interactive commands come last: they layer on top of a working static system, so bugs in join/leave don't obscure earlier integration issues.

---

## 10. Key Architectural Decisions

| Decision | Rationale |
|----------|-----------|
| Two separate binaries | Different runtime profiles; simpler per-binary narrative for a Go learner |
| `cmd/` + `internal/` layout | Clear separation without over-engineering; standard Go convention |
| Channel between receiveLoop and displayLoop | Decouples I/O latency from display latency; prevents socket blocking on terminal writes |
| `groupManager` goroutine serialises join/leave | Kernel multicast state + Go map must not be mutated concurrently |
| JSON wire format | Human-readable in `tcpdump -A`; no custom parser; appropriate for learning |
| ANSI scroll region (no ncurses) | Keeps code in stdlib + simple escape strings; no cgo dependency; matches project constraints |
| context.Context for shutdown | Idiomatic Go; propagates cleanly through goroutine tree; enables ordered teardown |
| Static compilation (CGO_DISABLED=1) | Required for nicolaka/netshoot scratch-layer container; `x/net/ipv4` is pure Go |
