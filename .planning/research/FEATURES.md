# Feature Research: Multicast Test Tools & Financial Ticker Simulators

> **Research type:** Greenfield features survey — what exists in the space, what matters, what to skip.
> **Purpose:** Inform requirements definition for `mcast-test-app` (Go, Kubernetes, beginner-friendly).
> **Date:** 2026-04-20

---

## Reference Tools Surveyed

| Tool | Type | Notes |
|------|------|-------|
| `iperf3` | Active network tester | UDP multicast mode, rate/bandwidth control |
| `mcast` / `mctest` (various Linux utils) | Simple multicast send/recv | TTL, group, port CLI flags |
| `msend` / `mreceive` (UNI tools) | Classic multicast test | ASM only, minimal display |
| `smcroute` test tooling | Multicast routing test | Focus on routing table validation |
| `VLC` multicast streaming | Media multicast | Overkill — media protocol |
| Financial tick simulators (SolarFlare/STAC) | Latency bench | PCIe/RDMA focused, not network topology |
| Wireshark IGMP dissector | Passive observer | Complement, not replacement |
| `omping` | Multicast ping | Latency/reachability only, no data stream |

---

## Feature Taxonomy

### TABLE STAKES
> Without these, the tool cannot perform its basic function. Non-negotiable.

#### Sender — Table Stakes

| Feature | Description | Complexity | Dependencies |
|---------|-------------|------------|--------------|
| **Send UDP to multicast group** | Open a UDP socket, set multicast destination IP:port, send datagrams | Low | Network interface must be specified |
| **Sequence numbering** | Monotonically increasing counter embedded in payload; receiver detects gaps | Low | Payload format |
| **Configurable multicast group via flag** | `-group 239.1.1.1` CLI flag | Low | None |
| **Configurable port via flag** | `-port 5000` | Low | None |
| **Configurable interface via flag** | `-iface eth0` — essential in multi-homed containers | Low | OS must expose named interfaces |
| **Set multicast TTL** | `IP_MULTICAST_TTL` socket option; default 1 (link-local) vs higher for routed | Low | Raw socket or `golang.org/x/net` |
| **Send rate control** | `-rate 10` msgs/sec using a `time.Ticker`; prevents flooding | Low | None |
| **Graceful shutdown** | `SIGINT`/`SIGTERM` handling; flush, close socket cleanly | Low | None |

#### Receiver — Table Stakes

| Feature | Description | Complexity | Dependencies |
|---------|-------------|------------|--------------|
| **Join multicast group (ASM)** | `IP_ADD_MEMBERSHIP` socket option with group + interface | Low | `golang.org/x/net/ipv4` |
| **Receive UDP datagrams from group** | Read loop, parse payload | Low | Group join |
| **Sequence gap detection** | Track last-seen seq; log gap when `current - last > 1` | Low | Sequence numbering in payload |
| **Packet counter** | Total received, gaps seen, displayed in header line | Low | None |
| **Configurable group/port/interface via flags** | Mirror of sender flags | Low | None |
| **Display received data** | Print payload content to terminal | Low | None |
| **Graceful shutdown** | Leave group cleanly on exit | Low | Group join |

#### IGMP — Table Stakes

| Feature | Description | Complexity | Dependencies |
|---------|-------------|------------|--------------|
| **IGMPv2 ASM group join** | Default OS behavior on most Linux kernels; `IP_ADD_MEMBERSHIP` | Low | Kernel handles IGMP; app just sets socket option |
| **IGMPv3 ASM group join** | Kernel sends IGMPv3 Report with EXCLUDE{} (equivalent to v2 join) when using `IP_ADD_MEMBERSHIP` on v3-capable kernel | None (automatic) | Kernel IGMPv3 support |
| **IGMPv3 SSM group join** | `IP_ADD_SOURCE_MEMBERSHIP` or `MCAST_JOIN_SOURCE_GROUP`; restricts to specific source IP | Medium | `golang.org/x/net/ipv4`; kernel must support SSM |

#### Container/Deployment — Table Stakes

| Feature | Description | Complexity | Dependencies |
|---------|-------------|------------|--------------|
| **Static binary compilation** | `CGO_ENABLED=0 GOOS=linux go build`; no shared lib deps for netshoot | Low | Pure Go dependencies only |
| **Kubernetes manifests** | Deployment/Pod YAML for sender and receiver | Low | Container image available |

---

### DIFFERENTIATORS
> These elevate the tool above bare-minimum and serve the project's specific goals (learning, visibility, Kubernetes testing). Include selectively based on complexity budget.

#### Sender — Differentiators

| Feature | Description | Complexity | Value for This Project | Dependencies |
|---------|-------------|------------|----------------------|--------------|
| **Multi-group send** | Send to multiple groups simultaneously (goroutine per group) | Medium | High — simulates multiple ticker symbols | Per-group goroutines + rate control |
| **Named "ticker symbols"** | Embed a short symbol string (e.g. `AAPL`, `GOOG`) per group; purely cosmetic | Low | High — makes it feel like a ticker feed | Payload format |
| **Payload timestamp** | Embed `time.Now().UnixNano()` in payload for latency estimation | Low | Medium — enables rough one-way delay display | Payload format; clock sync not guaranteed |
| **Variable payload size flag** | `-size 64` pads payload to N bytes | Low | Low — not needed for functional testing | None |
| **Burst mode** | Send N packets then pause | Medium | Low | Rate control |
| **Runtime rate change** | Adjust send rate via stdin command | Medium | Low | Interactive control |

#### Receiver — Differentiators

| Feature | Description | Complexity | Value for This Project | Dependencies |
|---------|-------------|------------|----------------------|--------------|
| **Fixed-height scrolling terminal display** | ANSI escape codes to define a scroll region; last N lines scroll inside it; header stays pinned | Medium | **High** — core UX goal of this project | ANSI terminal |
| **IP header summary display** | Show `src:port → dst:port TTL=X` parsed from each datagram's IP/UDP headers | Medium | **High** — makes multicast routing visible | `golang.org/x/net/ipv4` raw recv or `syscall` |
| **Per-group stats line** | One summary line per joined group: pkts, gaps, rate | Medium | High — multi-group clarity | Multi-group tracking |
| **Runtime group join/leave** | Read commands from stdin while running (`join 239.1.1.2`, `leave 239.1.1.1`) | Medium | **High** — interactive IGMP demo | Group membership management goroutine |
| **SSM source display** | Show which source IP packets are accepted from when in SSM mode | Low | High — makes SSM visible | SSM join |
| **Gap/loss log** | When gap detected, print `[GAP] expected 42, got 50 (8 lost)` | Low | High | Sequence tracking |
| **Elapsed time display** | Show how long receiver has been running | Low | Medium | None |
| **Per-second rate display** | Packets/sec received, computed over sliding window | Medium | Medium | Rate tracking goroutine |

#### IGMP — Differentiators

| Feature | Description | Complexity | Value for This Project | Dependencies |
|---------|-------------|------------|----------------------|--------------|
| **Explicit IGMPv2 mode** | Force kernel to use v2 via `/proc/sys/net/ipv4/conf/<iface>/force_igmp_version=2` | Medium | High — side-by-side v2 vs v3 demo | Requires `NET_ADMIN` cap in container |
| **Explicit IGMPv3 mode** | Default on modern kernels; confirm via `/proc` or `ip maddr` | Low | High — testing SSM requires v3 | None beyond SSM join |
| **SSM source-specific join** | `MCAST_JOIN_SOURCE_GROUP` with source IP flag; kernel sends IGMPv3 INCLUDE report | Medium | **High** — SSM is a key test scenario | `golang.org/x/net/ipv4`; IGMPv3 router support |
| **Multiple simultaneous group memberships** | Join several groups on same interface | Medium | High — realistic ticker scenario | Goroutine per group or single socket with multiple memberships |

#### Interactive Control — Differentiators

| Feature | Description | Complexity | Value for This Project | Dependencies |
|---------|-------------|------------|----------------------|--------------|
| **stdin command loop** | Read lines from os.Stdin while display runs; parse `join`, `leave`, `quit` | Medium | **High** — live IGMP membership changes | Terminal raw mode or line mode |
| **Command help display** | Print available commands in pinned header area | Low | High | Display system |
| **Start/stop sending** (sender) | `stop` / `start` commands to pause emission | Low | Medium | Rate control goroutine |

#### Observability — Differentiators

| Feature | Description | Complexity | Value for This Project | Dependencies |
|---------|-------------|------------|----------------------|--------------|
| **Prometheus metrics endpoint** | `/metrics` HTTP server exposing packet counters | High | Low — overkill for a learning tool | `prometheus/client_golang` |
| **Structured log output** | JSON log lines to stderr | Low | Low | None |
| **Verbose flag** | `-v` prints full packet hex dump | Low | Medium — useful for debugging | None |

#### Container/Deployment — Differentiators

| Feature | Description | Complexity | Value for This Project | Dependencies |
|---------|-------------|------------|----------------------|--------------|
| **Dockerfile** | Multi-stage build; scratch or distroless final image | Low | Medium — netshoot already has tools; Dockerfile is for custom deploys | Static binary |
| **Kubernetes ConfigMap for groups** | Mount group list as config file | Medium | Low — CLI flags are simpler | K8s knowledge |
| **`NET_ADMIN` capability in manifests** | Required for IGMP version forcing and some SSM operations | Low | High — needed for full demo | K8s security context |

---

### ANTI-FEATURES
> Deliberately out of scope. Build these and you've made the project worse.

| Feature | Why NOT to Build It | Risk if Built |
|---------|-------------------|---------------|
| **Realistic FIX/ITCH/FAST protocol** | Complex encoding; defeats the "Go beginner can read this" goal | Code becomes unreadable; debugging becomes about protocol, not networking |
| **Web UI / REST API** | Requires HTTP server, HTML, CORS; explodes scope | Distraction from multicast learning |
| **Latency benchmarking / histograms** | Requires clock sync (PTP/GPS); meaningless in most K8s environments | False precision; misleads users |
| **IPv6 / MLD** | Different socket API surface; doubles complexity for no added learning value in v1 | Out of scope per PROJECT.md |
| **Docker Compose** | Not the target environment; distracts from K8s-specific multicast behavior | Out of scope per PROJECT.md |
| **ncurses / TUI library** | Adds a non-stdlib dependency; ANSI escape codes achieve the same result simply | Unnecessary complexity for a learning tool |
| **Prometheus / OpenTelemetry** | Heavy dependencies; not useful for interactive demo | Bloats binary; obscures simple counter logic |
| **Replay from pcap** | Complex parsing; not needed for functional testing | Scope creep |
| **Traffic shaping / packet loss injection** | tc/netem exists for this; reimplementing in userspace is wrong layer | Reinventing existing kernel tools |
| **Multicast routing daemon (PIM/DVMRP)** | That's smcroute/pimd territory | Completely wrong scope |

---

## Feature Dependency Graph

```
Static binary compilation
  └── All container/K8s deployment

Interface flag (-iface)
  └── All socket operations (send and receive)
  └── IGMP group join

ASM group join (IP_ADD_MEMBERSHIP)
  └── Sequence gap detection
  └── Per-group stats
  └── Runtime join/leave
  └── Multi-group receive

SSM group join (MCAST_JOIN_SOURCE_GROUP)
  └── IGMPv3 (automatic when SSM requested)
  └── Source IP display

Sequence numbering in payload
  └── Gap detection
  └── Loss counter

Rate control (time.Ticker)
  └── Multi-group send (one ticker per group)
  └── Per-second rate display

Fixed-height terminal display
  └── IP header summary display
  └── Per-group stats line
  └── Command help display

stdin command loop
  └── Runtime join/leave
  └── Start/stop (sender)
```

---

## Complexity Budget Summary

| Category | Table Stakes Effort | Differentiator Effort | Notes |
|----------|--------------------|-----------------------|-------|
| Sender core | Low | Low-Medium | Multi-group adds goroutine management |
| Receiver core | Low | Medium | Gap detection and stats are straightforward |
| IGMP / socket options | Low-Medium | Medium | SSM path needs `golang.org/x/net/ipv4` |
| Terminal display | — | Medium | ANSI scrolling region is the trickiest piece |
| Interactive control | — | Medium | Stdin loop + display concurrency need care |
| K8s manifests | Low | Low | Straightforward YAML |

**Total complexity:** Medium overall. The hardest single piece is the concurrent terminal display + stdin command loop without a TUI library. Everything else is low-to-medium Go socket programming.

---

## Recommendations for This Project

### Must Build (Table Stakes)
All table stakes rows above. Without them the tool doesn't demonstrate multicast at all.

### Should Build (High-value Differentiators)
- Fixed-height scrolling terminal display — it's the signature UX feature
- IP header summary (src/dst/TTL) per packet — makes routing visible
- SSM join with source IP flag — key IGMPv3 demo scenario
- Runtime join/leave via stdin — makes IGMP membership changes interactive
- Multi-group receive with per-group stats — simulates ticker feed realism
- Named ticker symbols per group — cosmetic but makes demo compelling
- `NET_ADMIN` in K8s manifest — required for full IGMP version control

### Could Build (Low-value Differentiators — defer)
- Payload timestamp / one-way delay display (clock sync unreliable in K8s)
- Verbose hex dump flag
- Structured JSON logging

### Do Not Build (Anti-features)
Everything in the Anti-features table above. The project's core value is clarity and learnability, not completeness.
