# Research Summary: Go Multicast Test App

*Synthesized: 2026-04-20. Source files: STACK.md, FEATURES.md, ARCHITECTURE.md, PITFALLS.md*

---

## What We're Building

A Go-based multicast sender/receiver tool that runs in Kubernetes (on `nicolaka/netshoot`) and demonstrates IGMPv2/v3, ASM/SSM group membership, and live packet flow via an interactive ANSI terminal UI. Two statically-compiled binaries: **sender** and **receiver**.

---

## Technology Stack (STACK.md)

**Two external dependencies only — both official Go extended stdlib:**

| Dependency | Version | Role |
|---|---|---|
| `golang.org/x/net` | v0.53.0 | Multicast sockets: `ipv4.NewPacketConn`, `JoinSourceSpecificGroup`, `SetControlMessage` |
| `golang.org/x/term` | v0.42.0 | TTY detection (`IsTerminal`) and size query (`GetSize`) |

**Everything else is stdlib:** `net`, `flag`, `log/slog`, `os/signal`, `sync`, `bufio`, `time`, `context`.

**Build:** Go 1.26.2, `CGO_ENABLED=0`, `-tags netgo -ldflags="-extldflags '-static'"`. Multi-stage Dockerfile: `golang:1.26.2-alpine` builder → `nicolaka/netshoot:v0.15` runtime. Static-linkage is verified inside the builder stage (`file | grep 'statically linked'`).

**Kubernetes:** `NET_RAW` + `NET_ADMIN` capabilities; `stdin: true` + `tty: true` for ANSI UI; `hostNetwork: true` recommended as the first-test mode to bypass CNI multicast support questions. Node selector: `kubernetes.io/os: linux`.

**Do not add:** any TUI library (bubbletea, tview, tcell), CLI framework (cobra, urfave), logging library (logrus, zap), gopacket, or ipv6 packages.

---

## Feature Scope (FEATURES.md)

### Must Build — Table Stakes
- **Sender:** UDP multicast send, sequence numbering, `-group`/`-port`/`-iface`/`-rate`/`-ttl` flags, graceful shutdown.
- **Receiver:** ASM group join, receive loop, gap detection, packet counter, graceful leave on exit.
- **IGMP:** IGMPv2/v3 ASM join (`IP_ADD_MEMBERSHIP`); IGMPv3 SSM join (`IP_ADD_SOURCE_MEMBERSHIP`).
- **Container:** static binary, Kubernetes manifests.

### Should Build — High-value Differentiators
- Fixed-height ANSI scrolling terminal display (~20 lines) — the signature UX feature.
- IP header summary per packet (src IP, dst IP, TTL) via `SetControlMessage` ancillary data.
- SSM join with `--source` flag (`JoinSourceSpecificGroup`).
- Runtime group join/leave via stdin (`join 239.1.1.1`, `leave 239.1.1.1`).
- Multi-group receive with per-group stats (packets, gaps, rate).
- Named ticker symbols per group (e.g. `AAPL`, `GOOG`) — cosmetic but makes demos compelling.
- `NET_ADMIN` in K8s manifests for IGMP version control.

### Defer / Do Not Build
- Payload timestamp / latency display (clock sync unreliable in K8s).
- Prometheus metrics, web UI, REST API, Docker Compose, replay from pcap, multicast routing daemon, FIX/ITCH protocol, IPv6/MLD, ncurses/TUI library, traffic shaping.

---

## Architecture (ARCHITECTURE.md)

### Project Layout
```
mcast-test-app/
├── cmd/sender/main.go       ← flag parse, wires everything, owns signal handling
├── cmd/receiver/main.go     ← flag parse, wires everything, owns signal handling
├── internal/
│   ├── config/              ← shared GroupSpec struct
│   ├── packet/              ← Encode/Decode (JSON wire format)
│   ├── multicast/           ← socket helpers: JoinASM, JoinSSM, LeaveASM, LeaveSSM
│   └── display/             ← ANSI engine: Init, Render, Teardown
├── k8s/
├── go.mod
└── go.sum
```

Two separate binaries — different runtime profiles, simpler per-binary narrative for Go learners.

### Goroutine Architecture

**Sender:** `main` (signal) + `sendLoop` goroutine + `stdinLoop` goroutine. Commands flow via channel to a group manager.

**Receiver (5 goroutines):**
```
receiveLoop  ──packetCh (buffered, 64)──►  displayLoop  ──► os.Stdout (ANSI)
stdinLoop    ──cmdCh──►  groupManager  ──►  multicast.Join/Leave
main         ──context.Cancel──►  all goroutines
```
`receiveLoop` and `displayLoop` are decoupled by a buffered channel so the UDP read path never blocks on terminal I/O. If `displayLoop` falls behind, excess packets are counted and dropped at the channel, not at the socket.

### Wire Format
JSON-encoded `Packet{Sequence, Group, Source, Timestamp, Payload}` — human-readable in `tcpdump -A`, no custom parser.

### ANSI Display Engine
- Scroll region: `ESC[<top>;<bottom>r` pins N lines; cursor positioning for each line.
- Ring buffer of display lines — one write sequence per packet, no flicker.
- Status lines below scroll region: joined groups summary + command prompt.
- `Init()` hides cursor and sets region; `Teardown()` (deferred + signal handler) resets region and shows cursor.
- TTY detection at startup: if not a TTY, fall back to plain `fmt.Println`.

### Build Order
1. `internal/config`, `internal/packet`, `internal/multicast` — no display, validates encoding and sockets.
2. `cmd/sender` — simpler binary, exercises shared foundation first.
3. `internal/display` — isolated, unit-testable with mock lines.
4. `cmd/receiver` — wires all packages together.
5. Interactive stdin commands in both binaries.
6. Kubernetes manifests.

### Shutdown Sequence (Receiver)
`SIGINT/SIGTERM` → `context.Cancel()` → goroutines exit → `packetCh` closes → `displayLoop` drains → `groupManager` leaves all groups → `display.Teardown()` → `main` returns.

---

## Key Pitfalls & Mitigations (PITFALLS.md)

### Critical — Phase 1 (must get right before writing any code)

| Pitfall | Mitigation |
|---|---|
| Bind socket to `0.0.0.0` instead of group address | `net.UDPAddr{IP: groupAddr, Port: port}` — per-group, per-socket |
| Wrong interface for IGMP joins | Call `SetMulticastInterface(iface)` on sender; pass explicit `iface` to `JoinGroup` on receiver. Make `-iface` mandatory. |
| Using `JoinGroup` for SSM (silently falls back to ASM) | Use `JoinSourceSpecificGroup` for SSM. Encapsulate as `joinSSM()` vs `joinASM()`. |
| SSM leave with wrong call | Mirror join/leave: `LeaveSourceSpecificGroup` for SSM. Track ASM/SSM state per group in a struct. |
| SSM group address outside 232.0.0.0/8 | Validate at flag-parse time; reject non-SSM addresses in SSM mode. |
| No IP header data available on plain UDPConn | Use `ipv4.NewPacketConn` + `SetControlMessage(FlagTTL|FlagSrc|FlagDst, true)`. No raw socket needed. |
| `net.ListenPacket` vs `net.ListenUDP` fragility | Use `net.ListenUDP("udp4", addr)` → concrete `*net.UDPConn` → `ipv4.NewPacketConn`. Always check `JoinGroup` error. |
| `IP_MULTICAST_LOOP` on by default (sender receives own packets) | Call `SetMulticastLoopback(false)` on sender. Expose as `--loopback` flag for local testing. |
| Non-static binary breaks container | `CGO_ENABLED=0 -tags netgo -ldflags="-extldflags '-static'"`. Verify with `file | grep statically linked` in CI. |
| Goroutine-unsafe membership map | Confine map mutations to `groupManager` goroutine. Use `sync.RWMutex` for packet ring buffer. |
| ANSI escape codes on non-TTY (kubectl logs garbage) | `term.IsTerminal(fd)` at startup; fall back to plain-text output if not a TTY. |
| Terminal left broken after Ctrl-C | `defer display.Teardown()` + `signal.Notify` for SIGINT/SIGTERM to run cleanup before exit. |
| Blocking stdin freezes display | Run stdin reader in its own goroutine; main goroutine drives render ticker + `select` on channels. |

### Important — Phase 2 (Kubernetes)

| Pitfall | Mitigation |
|---|---|
| CNI doesn't forward multicast | Document CNI requirements (Cilium native routing, Multus+macvlan, SR-IOV). Provide `hostNetwork: true` fallback. Add startup connectivity check. |
| IGMP snooping drops cross-node traffic | Check `bridge/multicast_snooping`; disable for debug. Document Kind cluster issue. |
| `hostNetwork` changes pod IP, breaks SSM source filter | Re-derive source IP from actual interface at startup. Provide `--source-ip` override flag. |
| No multicast routing daemon in cluster | Document as prerequisite: PIM must be pre-configured or use same-node testing. |
| Missing `CAP_NET_RAW`/`CAP_NET_ADMIN` | Explicitly add both in `securityContext.capabilities`. Document why each is needed. |
| Kernel forced to IGMPv2 mode | Check `/proc/sys/net/ipv4/conf/<iface>/force_igmp_version` at startup; warn if nonzero. |

---

## Decision Log

| Decision | Rationale |
|---|---|
| Two separate binaries | Different runtime profiles; clear per-binary narrative for Go learners |
| `internal/` package layout | Standard Go convention; clear single-responsibility packages; no external import leakage |
| JSON wire format | Human-readable in tcpdump; no custom parser; appropriate for a learning project |
| Raw ANSI over TUI library | PROJECT.md explicit requirement; avoids non-stdlib deps and complex event models |
| `flag` over cobra/urfave | No subcommands needed; keeps code beginner-readable |
| `log/slog` over logrus/zap | Stdlib since Go 1.21; zero dependencies |
| Buffered `packetCh` (64 slots) | Decouples UDP read latency from display latency; prevents socket blocking on terminal I/O |
| `context.Context` for shutdown | Idiomatic Go; enables ordered teardown across goroutine tree |
| `groupManager` goroutine | Serialises kernel IGMP state + Go map mutations — prevents concurrent map panics |
| `nicolaka/netshoot:v0.15` runtime | Project mandate; includes tcpdump, ip, tshark for multicast debugging alongside the app |

---

*Research date: 2026-04-20*
