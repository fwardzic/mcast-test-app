# Stack: Go Multicast Test App

Prescriptive technology choices for the mcast-test-app. Each entry includes version, rationale, and confidence level. This feeds directly into roadmap creation.

**Research date:** 2026-04-20  
**Go version used for resolution:** go1.25.1 (local); latest stable is **go1.26.2**

---

## 1. Language Runtime

### Go 1.22+ (target: 1.26.2)

**Version:** `go 1.22` minimum in `go.mod`; build with `go1.26.2`.

**Rationale:**
- Go 1.22 is the minimum for loop-variable semantics fix (range variable capture bug fixed) — important for goroutine-heavy code patterns.
- Go 1.21+ `log/slog` structured logging is in stdlib — useful for debug output without importing a third-party logger.
- Static binary compilation with `CGO_ENABLED=0` is first-class since early versions but tooling around `go build -tags netgo` is well-documented from 1.21+.
- 1.26.2 is the current stable; use it for the build image. The module minimum can stay at 1.22 to document the actual floor.

**Do NOT use:** Go generics heavy patterns or `any` abuse — the project explicitly requires beginner-friendly code.

**Confidence:** High ✅

---

## 2. Networking — Multicast Core

### `net` (stdlib) + `golang.org/x/net v0.53.0`

**Packages used:**
- `net` (stdlib) — `net.ListenUDP`, `net.UDPAddr`, `net.ParseIP`, `net.InterfaceByName`
- `golang.org/x/net/ipv4` — `ipv4.NewPacketConn`, `JoinGroup`, `JoinSourceSpecificGroup`, `SetMulticastInterface`, `SetMulticastLoopback`, `SetControlMessage`

**Version:** `golang.org/x/net v0.53.0` (latest as of 2026-04-20, verified via `go get golang.org/x/net@latest`)

**Rationale:**
- `net.ListenUDP("udp4", addr)` returns a concrete `*net.UDPConn` which wraps cleanly into `ipv4.NewPacketConn`. This is the only correct path — `net.ListenPacket` returns an interface that requires a type assertion and is fragile (see PITFALLS §1.5).
- `golang.org/x/net/ipv4` is the only Go library that exposes `IP_ADD_SOURCE_MEMBERSHIP` (SSM) via `JoinSourceSpecificGroup`. There is no stdlib equivalent. SSM is a hard requirement.
- `SetControlMessage(ipv4.FlagTTL | ipv4.FlagSrc | ipv4.FlagDst, true)` enables IP header ancillary data on receive — required to display TTL and source IP without raw sockets (see PITFALLS §1.4).
- `golang.org/x/net` is the official Go extended stdlib (golang/x). It is maintained by the Go team, uses Go's syscall layer (no libc), and produces fully static binaries under `CGO_ENABLED=0`.

**Do NOT use:**
- `golang.org/x/net/ipv6` — IPv6/MLD is explicitly out of scope.
- `github.com/google/gopacket` — raw packet crafting library; overkill for this use case and requires cgo on some platforms.
- Any third-party multicast abstraction library — they wrap `x/net/ipv4` without adding value and obscure the learning objective.

**Confidence:** High ✅

---

## 3. Terminal UI

### Raw ANSI escape codes + `golang.org/x/term v0.42.0`

**Packages used:**
- `golang.org/x/term` — `term.IsTerminal(fd)`, `term.GetSize(fd)` only
- Raw ANSI writes to `os.Stdout` for all rendering

**Version:** `golang.org/x/term v0.42.0` (latest as of 2026-04-20, verified via `go get golang.org/x/term@latest`)

**Rationale:**
- The PROJECT.md explicitly states: "ANSI terminal scrolling region — avoids ncurses/tui library complexity; keeps code simple."
- `golang.org/x/term` is the minimal stdlib extension needed: TTY detection (`IsTerminal`) and terminal dimension query (`GetSize`). Both are needed per PITFALLS §6.1 and §6.4.
- Writing raw ANSI to `os.Stdout` (unbuffered) avoids the flush-forgetting bug (PITFALLS §6.2).
- A single `render.go` file handling all escape sequences (scrolling region `\033[<t>;<b>r`, cursor positioning `\033[<r>;<c>H`, cursor hide/show `\033[?25l`/`\033[?25h`) keeps the rendering isolated and swappable.
- The ~20-line fixed display is simple enough that a full TUI library would be massive overkill.

**Do NOT use:**
- `github.com/charmbracelet/bubbletea` — elegant but introduces Elm-architecture concepts that obscure Go basics for a beginner.
- `github.com/rivo/tview` — ncurses-style widget library, heavy, complex event model.
- `github.com/gdamore/tcell` — lower-level than bubbletea but still a large API surface.
- `github.com/nsf/termbox-go` — unmaintained since 2022.
- Any library that takes over the event loop — the app needs to own its goroutine architecture (UDP reader + stdin reader + render ticker).

**Confidence:** High ✅

---

## 4. Standard Library — Supporting Packages

These are all `stdlib`; no third-party alternatives needed.

| Package | Purpose | Why stdlib is sufficient |
|---------|---------|--------------------------|
| `flag` | CLI flags (`-group`, `-iface`, `-port`, `-mode`) | Simple key=value flags; no subcommands needed |
| `log/slog` | Structured debug logging | Built-in since Go 1.21; zero dependencies |
| `os/signal` | Catch `SIGINT`/`SIGTERM` for terminal cleanup | See PITFALLS §6.3 |
| `sync` | `sync.RWMutex` for packet ring buffer | Protects shared state between UDP goroutines and render |
| `bufio` | Stdin command reader (line-by-line) | Simple; pair with a goroutine channel (PITFALLS §7.2) |
| `fmt` | Formatted output | — |
| `time` | Render ticker (`time.NewTicker`) | Drives periodic display updates |
| `net` | Interface enumeration, IP parsing | Core; already required for multicast |

**Do NOT use:**
- `github.com/spf13/cobra` / `github.com/urfave/cli` — no subcommands needed; `flag` is sufficient and keeps the code beginner-friendly.
- `github.com/sirupsen/logrus` / `go.uber.org/zap` — `log/slog` satisfies the logging need since Go 1.21.

**Confidence:** High ✅

---

## 5. Container Build Pattern

### Multi-stage Dockerfile: `golang:1.26.2-alpine` builder → `nicolaka/netshoot:v0.15`

```dockerfile
# Stage 1: Build static binaries
FROM golang:1.26.2-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-extldflags '-static'" -tags netgo \
    -o /out/sender ./cmd/sender
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-extldflags '-static'" -tags netgo \
    -o /out/receiver ./cmd/receiver
# Verify static linkage (fails build if not static)
RUN file /out/sender | grep -q 'statically linked'
RUN file /out/receiver | grep -q 'statically linked'

# Stage 2: Runtime image
FROM nicolaka/netshoot:v0.15
COPY --from=builder /out/sender /usr/local/bin/sender
COPY --from=builder /out/receiver /usr/local/bin/receiver
```

**Rationale:**
- `golang:1.26.2-alpine` is the official Go builder image on Alpine — minimal, reproducible, and produces a clean `CGO_ENABLED=0` build environment.
- `nicolaka/netshoot:v0.15` (latest stable as of 2026-01-07, supports amd64/arm64) is the mandated runtime. It is based on Alpine 3.23.2 and includes tcpdump, ip, wireshark-tshark, nmap, and ~40 other network tools that are essential for debugging multicast issues alongside the app.
- Static binaries (`-tags netgo`, `-extldflags '-static'`) are required because netshoot's musl/glibc environment may differ from the builder. Static binaries eliminate this dependency entirely (see PITFALLS §5.2).
- The `file` check in the builder stage makes a static-linkage failure a **build error**, not a runtime surprise (see PITFALLS §5.2).
- Multi-stage keeps the final image lean — only the netshoot base + two small static binaries.

**Do NOT use:**
- `FROM scratch` — loses all the netshoot debug tools that are the point of the base image.
- `FROM alpine` as runtime — misses the entire netshoot toolkit.
- `CGO_ENABLED=1` — introduces libc/musl linkage; breaks in netshoot or requires matching glibc.
- `GOARCH=arm64` hardcoded in CI — build for the target arch; use `--platform` in Docker buildx for multi-arch if needed.

**Confidence:** High ✅ (netshoot version confirmed via Docker Hub API; base image Alpine 3.23.2 confirmed from Dockerfile)

---

## 6. Kubernetes Manifest Patterns

### DaemonSet or Deployment with explicit security context

**Key manifest decisions:**

```yaml
# Network-sensitive pod spec
spec:
  # Option A: hostNetwork for CNI-independent testing
  hostNetwork: true       # shares node network namespace; pod IP = node IP
  dnsPolicy: ClusterFirstWithHostNet

  # Option B: Pod network (CNI must support multicast)
  # hostNetwork: false

  containers:
  - name: receiver
    image: nicolaka/netshoot:v0.15
    command: ["/usr/local/bin/receiver"]
    args: ["-group", "239.1.1.1", "-port", "5000", "-iface", "eth0"]
    securityContext:
      capabilities:
        add:
          - NET_RAW    # required for multicast socket control messages
          - NET_ADMIN  # required for reading /proc/net/igmp, setting sysctl
      runAsNonRoot: false  # netshoot runs as root; required for raw socket ops
    stdin: true
    tty: true           # required for ANSI terminal UI
```

**Rationale:**
- `NET_RAW` is needed for `SetControlMessage` and some `setsockopt` calls (PITFALLS §5.3).
- `NET_ADMIN` is needed for reading `/proc/net/igmp` and `/proc/net/igmp6` at startup to detect IGMP version behavior (PITFALLS §2.2).
- `stdin: true` + `tty: true` enables `kubectl exec -it` with a proper TTY — required for the ANSI scrolling display to function (without TTY, the app must fall back to plain text).
- `hostNetwork: true` is the recommended first-test mode because it bypasses CNI multicast support questions entirely (PITFALLS §4.1). The manifest should provide both variants with clear comments.
- No `PodSecurityPolicy` / `Pod Security Standards` `restricted` profile — this app requires elevated network capabilities by design. Use `baseline` PSS or document that `restricted` will break it.

**Node selector / toleration pattern:**
```yaml
  nodeSelector:
    kubernetes.io/os: linux     # x/net/ipv4 is Linux-only for multicast
```

**Do NOT use:**
- `privileged: true` — grants all capabilities; NET_RAW + NET_ADMIN is the minimum necessary scope.
- `runAsNonRoot: true` without confirming all socket ops succeed — IGMP joins at the kernel level may require root for some socket options.
- `hostPID: true` — not needed; avoid unnecessary privilege.

**Confidence:** High ✅

---

## 7. Module Summary (`go.mod`)

```
module github.com/YOUR_ORG/mcast-test-app

go 1.22

require (
    golang.org/x/net  v0.53.0
    golang.org/x/term v0.42.0
)
```

That is the complete third-party dependency list. Two packages. Both are official Go extended stdlib, maintained by the Go team, zero indirect security risk.

**Do NOT add:**
- Any logging library (use `log/slog`)
- Any CLI framework (use `flag`)
- Any TUI library (use raw ANSI + `x/term`)
- Any test assertion library for v1 — `testing` stdlib is sufficient for beginner-friendly tests

---

## 8. What NOT to Use — Summary

| Library | Category | Reason to avoid |
|---------|----------|-----------------|
| `github.com/charmbracelet/bubbletea` | TUI | Elm architecture; hides Go fundamentals from learner |
| `github.com/rivo/tview` | TUI | Widget model; heavy; wrong abstraction level |
| `github.com/nsf/termbox-go` | TUI | Unmaintained |
| `github.com/spf13/cobra` | CLI | Subcommand model; overkill for 2-binary tool |
| `github.com/urfave/cli` | CLI | Same; `flag` is sufficient |
| `github.com/sirupsen/logrus` | Logging | `log/slog` is stdlib since Go 1.21 |
| `go.uber.org/zap` | Logging | `log/slog` is stdlib |
| `github.com/google/gopacket` | Networking | Requires cgo; raw packet crafting not needed |
| Any multicast abstraction lib | Networking | Wraps `x/net/ipv4`; obscures learning objective |
| `golang.org/x/net/ipv6` | Networking | IPv6/MLD out of scope for v1 |

---

## 9. Confidence Summary

| Decision | Confidence | Verification method |
|----------|-----------|---------------------|
| `golang.org/x/net v0.53.0` | ✅ High | `go get golang.org/x/net@latest` on go1.25.1 |
| `golang.org/x/term v0.42.0` | ✅ High | `go get golang.org/x/term@latest` on go1.25.1 |
| Go 1.26.2 latest stable | ✅ High | go.dev/dl/ confirmed |
| nicolaka/netshoot v0.15 | ✅ High | Docker Hub API confirmed, 2026-01-07 |
| netshoot base: Alpine 3.23.2 | ✅ High | GitHub Dockerfile confirmed |
| Multi-stage build pattern | ✅ High | Standard Go container best practice |
| NET_RAW + NET_ADMIN caps | ✅ High | Cross-referenced with PITFALLS §5.3 |
| Raw ANSI over TUI library | ✅ High | PROJECT.md explicit decision |
| `flag` over cobra/urfave | ✅ High | PROJECT.md "simple patterns only" constraint |

---

*Research date: 2026-04-20*
