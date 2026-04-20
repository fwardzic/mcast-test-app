# Roadmap: mcast-test-app

**Created:** 2026-04-20
**Granularity:** standard (6 phases)
**Mode:** interactive
**v1 Requirements covered:** 27 / 27

---

## Overview

Six phases, each building directly on the last. Phases are ordered so a Go beginner can run and inspect something real at every step — no phase ends with code that can't be exercised yet.

```
Phase 1 → Foundation         shared packages, wire format, project skeleton
Phase 2 → Sender Core        UDP multicast send, sequence numbering, CLI flags, shutdown
Phase 3 → Multi-Group & IGMP multiple groups, ticker symbols, IGMPv2/v3/SSM
Phase 4 → Receiver Core      ASM join, receive loop, gap detection, graceful shutdown
Phase 5 → Terminal Display   ANSI scroll region, IP header summary, per-group stats
Phase 6 → Container & K8s   static builds, Dockerfile, Kubernetes manifests
```

---

## Phase 1 — Foundation

**Goal:** Establish the project skeleton and shared internal packages so every later phase builds on a stable, tested base. A Go beginner reading the code here sees how a multi-package Go project is laid out and why `internal/` packages exist.

**Requirements mapped:** CODE-01, CODE-02

### Plans
1. ✅ **Repo & module init** — `go mod init`, directory tree (`cmd/sender`, `cmd/receiver`, `internal/{config,packet,multicast,display}`, `k8s/`), `.gitignore`, top-level `Makefile` with `build`, `test`, `lint` targets.
2. ✅ **`internal/config` package** — `GroupSpec` struct (group address, port, interface, TTL, symbol, source IP for SSM). Beginner-friendly comments explaining each field's role in multicast. Unit tests.
3. **`internal/packet` package** — `Packet` struct (Sequence, Group, Source, Symbol, Timestamp, Payload). `Encode()`/`Decode()` using `encoding/json`. Unit tests verifying round-trip. Comment block explaining why JSON over a binary format.

### Success Criteria
1. `go test ./internal/...` passes with zero failures — a learner can verify the foundation is sound.
2. `go build ./...` compiles cleanly from a fresh `go env` — no missing imports, no placeholder stubs.
3. A reviewer reading `internal/config` and `internal/packet` understands each field and function without reading any other file — comments carry the "why".
4. The directory layout matches the architecture document exactly — no surprise locations for later phases.

---

## Phase 2 — Sender Core

**Goal:** Build a working single-group multicast sender that a beginner can run locally and observe with `tcpdump`. Establishes the send loop, CLI flag pattern, `time.Ticker` usage, and signal-based graceful shutdown — all patterns reused in Phase 3 and Phase 4.

**Requirements mapped:** SEND-01, SEND-02, SEND-03, SEND-04, SEND-08

### Plans
1. **`internal/multicast` — sender socket helpers** — `NewSenderConn(iface, ttl)` wrapping `net.ListenUDP` + `ipv4.NewPacketConn`; `SetMulticastInterface`; `SetMulticastLoopback(false)`. Comment explaining why loopback is disabled by default and the `--loopback` flag escape hatch.
2. **`cmd/sender` — single-group send loop** — `flag` parsing for `-group`, `-port`, `-iface`, `-ttl`, `-rate`, `-loopback`; `sendLoop` goroutine using `time.Ticker`; monotonically increasing sequence counter; `packet.Encode` → `WriteTo`; basic `log/slog` output per send.
3. **Graceful shutdown** — `signal.NotifyContext` for `SIGINT`/`SIGTERM`; `context.Cancel` propagates to `sendLoop`; `wg.Wait()` before exit. Inline comments walking a beginner through the goroutine lifecycle.

### Success Criteria
1. Running `./sender -group 239.1.1.1 -port 5000 -iface eth0` produces one log line per packet — a learner can see the send rate in their terminal.
2. `tcpdump -i eth0 -A udp port 5000` on the same host shows human-readable JSON payloads with incrementing sequence numbers.
3. Pressing `Ctrl-C` exits cleanly within one tick interval — no zombie goroutines, no error messages.
4. Passing `-rate 10` changes packet cadence visibly in the log output — learner can tune the ticker.
5. `-help` prints all flags with descriptions — self-documenting binary.

---

## Phase 3 — Multi-Group & IGMP

**Goal:** Extend the sender to drive multiple groups simultaneously and implement the full IGMP join/leave matrix (IGMPv2 ASM, IGMPv3 ASM, IGMPv3 SSM). A beginner sees goroutines-per-group in practice and learns the distinction between ASM and SSM at the socket API level.

**Requirements mapped:** SEND-05, SEND-06, SEND-07, IGMP-01, IGMP-02, IGMP-03, IGMP-04

### Plans
1. **`internal/multicast` — receiver socket helpers** — `JoinASM(conn, iface, group)`, `LeaveASM(...)`, `JoinSSM(conn, iface, group, source)`, `LeaveSSM(...)`. Each function is ~10 lines with a comment block explaining the underlying IP socket option and when to use it. Unit tests using loopback.
2. **Sender: multi-group goroutines** — parse multiple `-group` values (comma-separated or repeated flag); spawn one `sendLoop` goroutine per group; per-group `GroupSpec` carries symbol name (`-symbol AAPL,GOOG`) and timestamp in payload. Comment on goroutine fan-out pattern.
3. **SSM validation & sender SSM mode** — `-source` flag on sender; `232.0.0.0/8` range check at flag-parse time with clear error message; `SetMulticastInterface` correctly derived when `hostNetwork` changes pod IP. IGMP-04 enforcement in `validateFlags()`.

### Success Criteria
1. Running the sender with two `-group` values produces interleaved log lines from two goroutines — learner sees concurrent sends in real time.
2. Each packet's decoded JSON contains a non-empty `Symbol` field and a `Timestamp` — learner can inspect payloads with `tcpdump -A`.
3. Passing a non-SSM address (e.g. `239.1.1.1`) with `-source` prints a validation error and exits with code 1 — flag-parse-time rejection is visible.
4. `internal/multicast` unit tests for `JoinASM` / `JoinSSM` pass on loopback — a learner can run `go test` and see the IGMP calls succeed.

---

## Phase 4 — Receiver Core

**Goal:** Build a working receiver that joins a multicast group, reads packets, detects sequence gaps, and shuts down cleanly. No fancy UI yet — just log lines. A beginner sees the full goroutine architecture (receiveLoop + groupManager + stdinLoop) and understands channel-based decoupling.

**Requirements mapped:** RECV-01, RECV-02, RECV-03, RECV-04, RECV-08

### Plans
1. **`cmd/receiver` — socket setup & receive loop** — `flag` parsing for `-group`, `-port`, `-iface`; `net.ListenUDP` → `ipv4.NewPacketConn`; `SetControlMessage(FlagTTL|FlagSrc|FlagDst, true)`; `receiveLoop` goroutine reads into buffered `packetCh (64)`.
2. **`groupManager` goroutine** — owns the `map[string]*GroupStats` (packets received, last sequence, gap count); processes decoded packets from `packetCh`; gap detection: `if seq != lastSeq+1 { gaps += seq - lastSeq - 1 }`; plain `log/slog` output per packet with counter summary.
3. **Graceful shutdown** — `signal.NotifyContext` → `context.Cancel`; `receiveLoop` exits → closes `packetCh` → `groupManager` drains and leaves groups; `LeaveASM`/`LeaveSSM` called per joined group; `main` waits on `wg`. Comments on shutdown ordering.

### Success Criteria
1. Starting the receiver while the sender runs produces a log line per received packet including sequence number — end-to-end multicast flow is visible.
2. Killing the sender and restarting it causes the receiver to log a gap count greater than zero — gap detection is observable.
3. `RECV-03` stat line (total received, total gaps) appears in every log line — a learner can watch counters grow.
4. `Ctrl-C` on the receiver logs "leaving group" then exits cleanly — no terminal hang, no error from the kernel IGMP leave.
5. Passing an unresolvable `-iface` value prints a startup error and exits before binding — no silent failures.

---

## Phase 5 — Terminal Display

**Goal:** Replace the plain log output with the signature fixed-height ANSI scrolling terminal UI. A beginner sees how `ESC` sequences work, learns TTY detection, and understands why display and receive are separate goroutines.

**Requirements mapped:** RECV-05, RECV-06, RECV-07

### Plans
1. **`internal/display` package** — `Init(rows int)`: hide cursor + set scroll region; `Render(lines []string, status string)`: position cursor, write lines from ring buffer, write status line below region; `Teardown()`: reset scroll region + show cursor. TTY detection via `term.IsTerminal`; plain `fmt.Println` fallback. Unit tests with mock writer.
2. **`displayLoop` goroutine in receiver** — reads from `packetCh`; formats each packet as `"[SEQ] src:port → dst:port  TTL=N  SYMBOL  payload"`; maintains 20-line ring buffer; calls `display.Render` at ~10 Hz via `time.Ticker` to avoid per-packet redraws; IP header fields sourced from `ipv4.ControlMessage`.
3. **Per-group stats status line** — `groupManager` maintains `GroupStats{pkts, gaps, rate}` and sends a formatted summary string to `displayLoop` via a separate `statsCh`; status line rendered below scroll region showing one line per joined group.

### Success Criteria
1. Running the receiver in a real TTY shows exactly ~20 scrolling lines of packet data with no full-screen flicker — the scroll region stays fixed.
2. Each displayed line contains `src IP → dst IP`, `TTL`, symbol, and sequence number — IP header summary is readable at a glance.
3. The status line below the scroll region updates with per-group packet count and gap count without disturbing the scroll region.
4. Running `kubectl logs` (non-TTY) produces readable plain-text output with no ANSI garbage — TTY fallback works.
5. `Ctrl-C` restores the terminal cursor and scroll region immediately — `display.Teardown()` runs before process exit.

---

## Phase 6 — Container & Kubernetes

**Goal:** Package both binaries into a production-ready container image and provide Kubernetes manifests that a learner can `kubectl apply` to see multicast flow across pods. Establishes the static-build pipeline and documents the K8s multicast prerequisites.

**Requirements mapped:** BILD-01, BILD-02, BILD-03, BILD-04, BILD-05

### Plans
1. **Static build pipeline** — `Makefile` targets: `build-linux-amd64` and `build-linux-arm64` using `CGO_ENABLED=0 GOOS=linux GOARCH=... go build -tags netgo -ldflags="-extldflags '-static'"`. Verification step: `file bin/sender | grep 'statically linked'` fails the build if not static.
2. **Multi-stage Dockerfile** — `FROM golang:1.26.2-alpine AS builder`: copies source, runs static build + static verification; `FROM nicolaka/netshoot:v0.15`: copies both binaries. Comment block explaining why two stages and why `netshoot` as runtime.
3. **Kubernetes manifests** — `k8s/sender.yaml` and `k8s/receiver.yaml`: `securityContext.capabilities.add: [NET_RAW, NET_ADMIN]`; `stdin: true`; `tty: true`; `hostNetwork: true` with inline comment explaining CNI multicast caveat; `nodeSelector: kubernetes.io/os: linux`. README section on prerequisites (CNI requirements, IGMP snooping, same-node vs cross-node).

### Success Criteria
1. `make build-linux-amd64` produces a binary that `file` reports as `ELF 64-bit LSB executable, statically linked` — no dynamic libraries.
2. `docker build .` completes without error and the image runs `sender --help` successfully — image is self-contained.
3. `kubectl apply -f k8s/` creates sender and receiver pods that reach `Running` status with the correct capabilities — manifest is valid.
4. `kubectl attach -it receiver` shows the ANSI display with live packets from the sender pod — full end-to-end flow in Kubernetes.
5. `kubectl logs receiver` (non-attach) produces readable plain-text packet lines — plain-text fallback confirmed in-cluster.

---

## Coverage Audit

| Phase | Requirements |
|-------|-------------|
| 1 — Foundation | CODE-01, CODE-02 |
| 2 — Sender Core | SEND-01, SEND-02, SEND-03, SEND-04, SEND-08 |
| 3 — Multi-Group & IGMP | SEND-05, SEND-06, SEND-07, IGMP-01, IGMP-02, IGMP-03, IGMP-04 |
| 4 — Receiver Core | RECV-01, RECV-02, RECV-03, RECV-04, RECV-08 |
| 5 — Terminal Display | RECV-05, RECV-06, RECV-07 |
| 6 — Container & K8s | BILD-01, BILD-02, BILD-03, BILD-04, BILD-05 |
| **Total** | **27 / 27** ✅ |

---

*Roadmap created: 2026-04-20*
