# Requirements: mcast-test-app

**Defined:** 2026-04-20
**Core Value:** Clearly show multicast traffic flowing between sender and receiver with enough detail to verify multicast routing works — while keeping the code simple enough for a Go beginner to understand and learn from.

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases.

### Sender

- [ ] **SEND-01**: Sender can send UDP datagrams to a configurable multicast group address
- [ ] **SEND-02**: Sender embeds monotonically increasing sequence number in each packet
- [ ] **SEND-03**: Sender accepts group, port, interface, TTL, and rate via CLI flags
- [ ] **SEND-04**: Sender supports configurable send rate (messages/sec) via time.Ticker
- [ ] **SEND-05**: Sender can send to multiple multicast groups simultaneously (goroutine per group)
- [ ] **SEND-06**: Sender embeds a named ticker symbol (e.g. AAPL) per group in payload
- [ ] **SEND-07**: Sender embeds timestamp in payload for rough latency display
- [ ] **SEND-08**: Sender handles SIGINT/SIGTERM and shuts down cleanly

### Receiver

- [ ] **RECV-01**: Receiver can join a multicast group (ASM) and receive UDP datagrams
- [ ] **RECV-02**: Receiver detects sequence gaps and logs lost packet count
- [ ] **RECV-03**: Receiver displays packet counter (total received, gaps)
- [ ] **RECV-04**: Receiver accepts group, port, interface via CLI flags
- [ ] **RECV-05**: Receiver displays traffic in a fixed ~20-line ANSI scrolling region (no full-screen scroll)
- [ ] **RECV-06**: Receiver shows IP header summary per packet (src:port -> dst:port, TTL)
- [ ] **RECV-07**: Receiver shows per-group stats line (pkts, gaps, rate)
- [ ] **RECV-08**: Receiver handles SIGINT/SIGTERM, leaves groups, resets terminal cleanly

### IGMP / Multicast

- [ ] **IGMP-01**: App supports IGMPv2 ASM group join/leave
- [ ] **IGMP-02**: App supports IGMPv3 ASM group join/leave
- [ ] **IGMP-03**: App supports IGMPv3 SSM join/leave with source IP filter
- [ ] **IGMP-04**: SSM enforces 232.0.0.0/8 range validation at flag-parse time

### Build / Deploy

- [ ] **BILD-01**: Go binaries compiled statically (CGO_ENABLED=0)
- [ ] **BILD-02**: Cross-compiled for linux/amd64 and linux/arm64
- [ ] **BILD-03**: Multi-stage Dockerfile with nicolaka/netshoot as runtime image
- [ ] **BILD-04**: Kubernetes manifests for sender and receiver pods
- [ ] **BILD-05**: Pod securityContext includes NET_RAW and NET_ADMIN capabilities

### Code Quality

- [ ] **CODE-01**: Beginner-friendly Go with comments explaining "why" not just "what"
- [ ] **CODE-02**: Simple flat-ish package layout (cmd/ + internal/)

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Interactive Control

- **CTRL-01**: Interactive commands while running (join, leave groups dynamically)
- **CTRL-02**: Runtime rate change for sender via stdin command

### Deployment

- **DEPL-01**: hostNetwork mode option in K8s manifests
- **DEPL-02**: Docker Compose alternative for non-K8s environments

### Observability

- **OBSV-01**: Per-second sliding window rate display
- **OBSV-02**: Elapsed time display

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Realistic market protocol (FIX, ITCH) | Too complex, defeats learning purpose |
| Web UI or GUI | Terminal-only tool |
| Latency benchmarking | Functional test tool, not a benchmark |
| IPv6 multicast / MLD | IPv4 IGMP only for v1 |
| ncurses / TUI library | Hides Go fundamentals from learner; raw ANSI keeps it transparent |
| Prometheus metrics | Over-engineering for a test tool |
| Pcap replay | Different use case |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| CODE-01 | 1 — Foundation | 🔲 Not started |
| CODE-02 | 1 — Foundation | 🔲 Not started |
| SEND-01 | 2 — Sender Core | 🔲 Not started |
| SEND-02 | 2 — Sender Core | 🔲 Not started |
| SEND-03 | 2 — Sender Core | 🔲 Not started |
| SEND-04 | 2 — Sender Core | 🔲 Not started |
| SEND-08 | 2 — Sender Core | 🔲 Not started |
| SEND-05 | 3 — Multi-Group & IGMP | 🔲 Not started |
| SEND-06 | 3 — Multi-Group & IGMP | 🔲 Not started |
| SEND-07 | 3 — Multi-Group & IGMP | 🔲 Not started |
| IGMP-01 | 3 — Multi-Group & IGMP | 🔲 Not started |
| IGMP-02 | 3 — Multi-Group & IGMP | 🔲 Not started |
| IGMP-03 | 3 — Multi-Group & IGMP | 🔲 Not started |
| IGMP-04 | 3 — Multi-Group & IGMP | 🔲 Not started |
| RECV-01 | 4 — Receiver Core | 🔲 Not started |
| RECV-02 | 4 — Receiver Core | 🔲 Not started |
| RECV-03 | 4 — Receiver Core | 🔲 Not started |
| RECV-04 | 4 — Receiver Core | 🔲 Not started |
| RECV-08 | 4 — Receiver Core | 🔲 Not started |
| RECV-05 | 5 — Terminal Display | 🔲 Not started |
| RECV-06 | 5 — Terminal Display | 🔲 Not started |
| RECV-07 | 5 — Terminal Display | 🔲 Not started |
| BILD-01 | 6 — Container & K8s | 🔲 Not started |
| BILD-02 | 6 — Container & K8s | 🔲 Not started |
| BILD-03 | 6 — Container & K8s | 🔲 Not started |
| BILD-04 | 6 — Container & K8s | 🔲 Not started |
| BILD-05 | 6 — Container & K8s | 🔲 Not started |

**Coverage:**
- v1 requirements: 27 total
- Mapped to phases: 27
- Unmapped: 0 ✅

---
*Requirements defined: 2026-04-20*
*Last updated: 2026-04-20 — traceability populated after roadmap creation*
