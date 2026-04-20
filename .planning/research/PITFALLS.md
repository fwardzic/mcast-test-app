# Pitfalls: Go Multicast Test App

Specific known-bad patterns for this project (Go + IGMPv2/v3 + ASM/SSM + containers + Kubernetes + static binaries + ANSI terminal UI).

---

## 1. Go Multicast Socket Gotchas

### 1.1 Binding the receiver to `0.0.0.0` instead of the multicast group address

**What goes wrong:** Binding a UDP socket to `0.0.0.0:<port>` receives all UDP traffic on that port from any source. While this appears to work on a single-host loopback test, on a real network it means you receive unicast traffic that happens to use the same port, and it conflates traffic from different multicast groups sharing the same port.

**Warning signs:** Receiver shows packets it shouldn't; debugging with tcpdump shows non-multicast UDP arriving on the socket. Tests pass on `lo` but behave oddly in Kubernetes.

**Prevention:** Bind the socket to the multicast group address itself (`net.UDPAddr{IP: groupAddr, Port: port}`). This is the canonical form and ensures the kernel only delivers datagrams destined for that group. Per group, per socket.

**Phase:** Sender/Receiver foundation (Phase 1).

---

### 1.2 Forgetting to set `IP_MULTICAST_IF` — wrong interface joins

**What goes wrong:** `golang.org/x/net/ipv4` lets you call `JoinGroup` with an interface, but if the underlying socket's outbound interface (`IP_MULTICAST_IF`) is not explicitly set, the OS picks it from the routing table. In a multi-interface container (e.g., eth0 + lo), the OS often routes multicast out of lo, meaning IGMP reports go to the wrong interface and routers never see the join.

**Warning signs:** `ip mroute` on the router/node shows no join; tcpdump on eth0 sees no IGMP Membership Report; traffic only works when sender and receiver are on the same node.

**Prevention:** Always call `SetMulticastInterface(iface)` on the IPv4PacketConn for the sender. Always pass the explicit interface to `JoinGroup(iface, groupAddr)` on the receiver. Make the `-iface` flag mandatory rather than defaulting to `""`.

**Phase:** Phase 1 (socket setup). Validate in Phase 2 (Kubernetes).

---

### 1.3 Multicast loopback (`IP_MULTICAST_LOOP`) on by default

**What goes wrong:** The OS enables multicast loopback by default — the sender receives its own multicast packets. In a demo tool this looks like the receiver is working when it might actually be talking to itself on the same socket or same node. It also causes double-counting if sender and receiver run in the same pod.

**Warning signs:** Receiver shows data even when no other receiver pods exist; sequence numbers match sender's output exactly with zero network latency.

**Prevention:** On the sender socket, explicitly call `SetMulticastLoopback(false)` unless loopback testing is intentionally required. Document this as a flag (`--loopback`) for local testing mode.

**Phase:** Phase 1. Note it in comments as a deliberate choice.

---

### 1.4 Not reading the raw IP header — missing header display feature

**What goes wrong:** Standard `net.UDPConn` strips the IP header before delivering to userspace. The requirement to "display IP packet header summary" (TTL, source IP, ToS/DSCP) cannot be satisfied with a plain UDP conn.

**Warning signs:** Only UDP payload is available; no TTL or IP flags visible.

**Prevention:** Use `ipv4.NewPacketConn` and call `SetControlMessage(ipv4.FlagTTL | ipv4.FlagSrc | ipv4.FlagDst | ipv4.FlagInterface, true)` to receive ancillary `ControlMessage` with header fields alongside each packet. No raw socket needed.

**Phase:** Phase 1 (receiver design). Must be decided before the read loop is written.

---

### 1.5 Using `net.ListenPacket("udp4", ...)` vs `net.ListenUDP` — subtle differences with multicast

**What goes wrong:** `net.ListenPacket` with address `"udp4"` creates a socket suitable for multicast but the returned `net.PacketConn` cannot be directly wrapped by `ipv4.NewPacketConn` in all Go versions without a type assertion. Misusing the interface leads to silent no-ops on `JoinGroup`.

**Prevention:** Use `net.ListenUDP("udp4", addr)` to get a concrete `*net.UDPConn`, then wrap: `ipv4.NewPacketConn(udpConn)`. Always check the error from `JoinGroup` — it returns non-nil if the socket type is wrong.

**Phase:** Phase 1.

---

## 2. IGMPv2 vs IGMPv3 Socket-Level Differences

### 2.1 Using `JoinGroup` for SSM — silently falls back to ASM

**What goes wrong:** `golang.org/x/net/ipv4.PacketConn.JoinGroup()` issues a `IP_ADD_MEMBERSHIP` socket option, which is an ASM join. SSM requires `IP_ADD_SOURCE_MEMBERSHIP` (IGMPv3 INCLUDE mode). Calling `JoinGroup` for an SSM group (232.0.0.0/8) will either be rejected by the kernel or treated as ASM, receiving traffic from all sources — defeating the purpose of SSM.

**Warning signs:** SSM receiver gets traffic from unexpected sources; `ip maddr` shows `(S,G)` not registered; no compile error (this is a runtime mistake).

**Prevention:** For SSM, use `ipv4.PacketConn.JoinSourceSpecificGroup(iface, groupAddr, sourceAddr)` which issues `IP_ADD_SOURCE_MEMBERSHIP`. Wrap this in a clearly-named helper `joinSSM()` vs `joinASM()` to prevent confusion. Assert at startup that the group IP is in 232.0.0.0/8 before calling the SSM path.

**Phase:** Phase 1 (socket abstraction layer). Critical to get right before any join/leave feature work.

---

### 2.2 Kernel IGMPv3 not enabled or forced to v2 by sysctl

**What goes wrong:** Even with correct `IP_ADD_SOURCE_MEMBERSHIP` calls, if the kernel or network device forces IGMPv2 compatibility mode (`net.ipv4.conf.all.force_igmp_version=2`), SSM joins are downgraded or rejected. In Kubernetes, node-level sysctl and CNI configuration both affect this.

**Warning signs:** SSM joins succeed at the socket level but router shows only `(*,G)` — not `(S,G)` entries; Wireshark shows IGMPv2 Membership Reports instead of IGMPv3.

**Prevention:** At startup, log the detected IGMP version behavior (by inspecting `/proc/net/igmp` and `/proc/net/igmp6`). Add a startup check that warns if `force_igmp_version` != 0. In Kubernetes manifests, document that `privileged: true` or `CAP_NET_ADMIN` may be needed to read/set these values.

**Phase:** Phase 2 (Kubernetes manifests and startup validation).

---

### 2.3 Leaving an SSM group with the wrong leave call

**What goes wrong:** An SSM group joined with `JoinSourceSpecificGroup` must be left with `LeaveSourceSpecificGroup`. Calling the ASM `LeaveGroup` on an SSM socket has no effect — the membership persists until the socket closes. This makes interactive leave/rejoin logic silently broken.

**Prevention:** Mirror every join call with the exact matching leave call. Encapsulate join state in a struct that records whether each group is ASM or SSM, and dispatch the correct leave function.

**Phase:** Phase 1 (join/leave state management).

---

## 3. SSM Group Range Restrictions

### 3.1 Using SSM group addresses outside 232.0.0.0/8

**What goes wrong:** RFC 4607 defines 232.0.0.0/8 as the SSM range. Using an arbitrary multicast address (e.g., 239.x.x.x) with `IP_ADD_SOURCE_MEMBERSHIP` may work on some kernels (Linux is permissive) but will be silently ignored or cause errors on routers and strict kernels. The behavior is undefined outside the SSM range.

**Prevention:** Validate at flag-parse time: if `--ssm` mode is set, reject any group address not in 232.0.0.0/8. Print a clear error. Provide sane defaults (e.g., `232.1.1.1`) in SSM mode.

**Phase:** Phase 1 (flag parsing and validation).

---

### 3.2 Confusing ASM group range (239.0.0.0/8 for private use) with routable multicast

**What goes wrong:** 239.0.0.0/8 (RFC 2365, administratively scoped) is fine for local testing but won't route between sites. Using 224.x.x.x groups casually may collide with well-known assignments (224.0.0.1 = all hosts, 224.0.0.2 = all routers, etc.).

**Prevention:** Default ASM groups to 239.1.1.x range in the tool. Add a comment in the code explaining why. Warn if user specifies a group in 224.0.0.0/24 (well-known range).

**Phase:** Phase 1 (defaults and flag validation).

---

## 4. Container / Kubernetes Multicast Challenges

### 4.1 CNI plugin does not support multicast — silent packet drop

**What goes wrong:** Many CNI plugins (Flannel VXLAN, Calico default BGP mode, AWS VPC CNI) do not forward multicast traffic between pods. IGMP joins succeed at the socket level, senders transmit, but packets are silently dropped at the CNI/overlay layer. This is the most common cause of "multicast not working in Kubernetes."

**Warning signs:** `tcpdump` on the sender pod's eth0 shows multicast packets leaving; `tcpdump` on the receiver pod's eth0 shows nothing; no ICMP errors.

**Prevention:**
- Document which CNIs support multicast (Cilium with native routing, Multus + macvlan/ipvlan, SR-IOV). This tool targets CNI testing, so the README/manifests must clearly state CNI requirements.
- Provide a host-network mode (`hostNetwork: true`) as a fallback for CNI-independent testing.
- Include a startup connectivity check: sender sends one packet to the multicast group, receiver waits N seconds; fail fast with a diagnostic message rather than silently showing zero packets.

**Phase:** Phase 2 (Kubernetes manifests). Call out in Phase 1 design that host networking is a required test mode.

---

### 4.2 IGMP snooping drops multicast at the switch/bridge level

**What goes wrong:** In Kubernetes node-level networking, the Linux bridge (used by many CNIs) has IGMP snooping enabled by default. If IGMP Membership Reports from pods are not forwarded to the bridge's querier, the bridge will not add the pod port to the multicast forwarding table and will drop multicast frames — even if routing is otherwise correct.

**Warning signs:** Multicast works when pods are on the same node (same bridge) but not across nodes. `bridge mdb show` shows no entries for the multicast group.

**Prevention:**
- Add a note in manifests to check `net.bridge.bridge-nf-call-iptables` and bridge IGMP snooping: `cat /sys/devices/virtual/net/<bridge>/bridge/multicast_snooping`.
- For demo/test use, disable snooping on the bridge: `echo 0 > /sys/devices/virtual/net/<bridge>/bridge/multicast_snooping` (document this as a debugging step, not a production recommendation).
- In CI/testing docs, call out that Kind clusters use bridge networking where snooping can interfere.

**Phase:** Phase 2. Document in the troubleshooting section of manifests.

---

### 4.3 `hostNetwork: true` required for raw IGMP visibility but changes pod IP

**What goes wrong:** With `hostNetwork: true`, the pod shares the node's network namespace. This changes the pod's IP to the node IP, which can break SSM source filtering if the source IP is expected to be a pod IP. It also means multicast group joins affect the node's network stack — potentially affecting other workloads.

**Warning signs:** SSM receiver gets no packets because the source IP filter was set to the pod's cluster IP, but packets arrive from the node IP.

**Prevention:** When using host networking, re-derive the source IP from the actual interface address at startup (don't assume the pod IP). Document this behavior. Provide a `--source-ip` override flag.

**Phase:** Phase 2 (Kubernetes manifest design and sender flag design).

---

### 4.4 No multicast routing daemon in Kubernetes nodes — `(*,G)` joins go nowhere

**What goes wrong:** IGMP handles group membership on a single link. For multicast to traverse multiple nodes/subnets, a multicast routing protocol (PIM-SM, PIM-SSM) must run on the nodes or router. Most Kubernetes clusters have no multicast routing daemon. IGMP joins are seen on the local link but traffic doesn't cross nodes.

**Warning signs:** Works when sender and receiver are on the same node; fails across nodes even with correct CNI support.

**Prevention:** Document this as a prerequisite: this tool tests multicast data-plane forwarding — the routing plane (PIM, MRIB) must be pre-configured. The README must state "requires multicast routing to be configured in the cluster or use same-node testing." The tool is a test tool, not a routing daemon.

**Phase:** Phase 1 (README/design), Phase 2 (manifest prerequisites).

---

## 5. Static Compilation Gotchas with the `net` Package

### 5.1 `CGO_ENABLED=0` breaks DNS resolution (irrelevant here but traps beginners)

**What goes wrong:** `CGO_ENABLED=0` forces the pure-Go net resolver. For this app, there is no DNS resolution of multicast addresses (they're dotted-decimal IPs), so this is not a functional problem — but beginners may add hostname resolution features and be confused when they work locally (CGO enabled) but fail in containers.

**Prevention:** Document in the build section: "This app uses only IP addresses, no hostnames, so `CGO_ENABLED=0` is safe." If hostname resolution is ever added, use `net.DefaultResolver` with `PreferGo: true` explicitly.

**Phase:** Phase 1 (Makefile/build setup).

---

### 5.2 `golang.org/x/net/ipv4` requires `syscall` — check static link compatibility

**What goes wrong:** `golang.org/x/net/ipv4` uses `syscall.SetsockoptIPMreq` and related calls directly. These use the Go runtime's syscall layer — no libc dependency — so static compilation works correctly. However, if any imported package silently pulls in `net/http` or `os/user` with CGO resolvers, the binary may not be fully static.

**Warning signs:** `ldd ./receiver` shows `libc.so` or `libnss` dependencies. `file ./receiver` shows "dynamically linked."

**Prevention:** Add to the Makefile: `go build -ldflags="-extldflags '-static'" -tags netgo` and post-build verify with `file ./receiver | grep 'statically linked'`. Make this part of CI. Use `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`.

**Phase:** Phase 1 (Makefile). Catch this before any container testing.

---

### 5.3 `netshoot` base image runs as non-root by default — raw socket permission denied

**What goes wrong:** Reading ancillary control messages (`SetControlMessage`) and joining multicast groups require `CAP_NET_RAW` or running as root for some socket options. `nicolaka/netshoot` runs as root by default, so this is usually fine — but if the Kubernetes manifest sets `securityContext.runAsNonRoot: true` or drops capabilities, `JoinGroup` or `SetControlMessage` will return `permission denied`.

**Prevention:** In Kubernetes manifests, explicitly set:
```yaml
securityContext:
  capabilities:
    add: ["NET_RAW", "NET_ADMIN"]
```
Document why each capability is needed. Do not run as root unnecessarily.

**Phase:** Phase 2 (manifest security context).

---

## 6. Terminal Raw Mode / ANSI Escape Code Portability

### 6.1 ANSI scrolling region (`\033[r`) not supported in all terminals

**What goes wrong:** The plan is to use ANSI escape codes for a fixed-height scrolling display. `\033[<top>;<bottom>r` sets the scrolling region and `\033[<n>H` positions the cursor. These work in xterm/VTE-based terminals and most SSH sessions, but not in Windows `cmd.exe`, some CI log captors, or when stdout is piped (not a TTY).

**Warning signs:** Garbage characters appear in logs; `kubectl logs` shows ANSI codes as literal text; CI output is unreadable.

**Prevention:**
- Check `isatty(os.Stdout.Fd())` at startup. If not a TTY, fall back to plain-text scrolling line output (just `fmt.Println`).
- Use `golang.org/x/term` for TTY detection (`term.IsTerminal(int(os.Stdout.Fd()))`).
- Keep the ANSI rendering isolated in a single `render.go` file behind an interface so it's swappable.

**Phase:** Phase 1 (terminal UI design). Decide the fallback behavior before writing the render loop.

---

### 6.2 Not flushing stdout — display appears to hang or batch-update

**What goes wrong:** Go's `os.Stdout` is unbuffered for writes, but wrapping it in a `bufio.Writer` (common pattern) without calling `Flush()` means ANSI escape sequences accumulate and the display appears frozen, then jumps.

**Prevention:** If using `bufio.Writer` for stdout, call `Flush()` at the end of every render frame. Alternatively, write ANSI sequences directly to `os.Stdout` (unbuffered) rather than through a buffered writer.

**Phase:** Phase 1.

---

### 6.3 Not restoring terminal state on exit — cursor disappears or scrolling region persists

**What goes wrong:** If the program sets a scrolling region (`\033[r`) or hides the cursor (`\033[?25l`) and then exits abnormally (signal, panic), the terminal is left in a broken state — cursor invisible, scroll region locked to N lines.

**Prevention:**
- Register a `defer` that sends reset sequences: `\033[?25h` (show cursor), `\033[r` (reset scroll region), `\033[2J\033[H` (optional clear).
- Use `signal.Notify` to catch `SIGINT`/`SIGTERM` and run cleanup before exit.
- Test by hitting Ctrl+C — the terminal must be fully usable afterwards.

**Phase:** Phase 1 (signal handling and deferred cleanup). Do not leave this for later.

---

### 6.4 Hardcoded terminal width/height — breaks on small terminals or wide displays

**What goes wrong:** The requirement says "~20 fixed lines." If the code hardcodes row offsets for ANSI positioning without checking actual terminal size, the display is corrupted on non-standard terminals.

**Prevention:** Use `golang.org/x/term.GetSize(fd)` to get actual terminal dimensions. Cap the display area to `min(20, termHeight-4)`. Redraw on `SIGWINCH`.

**Phase:** Phase 1. Lower priority than the join/leave logic, but design the render layer to accept dimensions as a parameter from the start.

---

## 7. Interactive Command Handling

### 7.1 Reading stdin while also reading from UDP — goroutine coordination mistakes

**What goes wrong:** The app needs concurrent: (a) UDP receive loop, (b) stdin command reader (for join/leave), (c) render/display loop. A common beginner mistake is to share the group membership map without synchronization, leading to map concurrent write panics (Go's map is not goroutine-safe).

**Warning signs:** Intermittent `concurrent map write` panic; only appears when commands are sent quickly.

**Prevention:**
- Use a single `chan Command` to pass join/leave commands from the stdin reader to the membership manager goroutine.
- Never share the membership map across goroutines without a mutex or by confining it to one goroutine.
- Use `sync.RWMutex` for the packet display buffer (many writers from receive loop, one reader for display).

**Phase:** Phase 1. Establish the goroutine architecture before any feature work.

---

### 7.2 Blocking stdin read interfering with ANSI display updates

**What goes wrong:** `bufio.Scanner.Scan()` or `fmt.Scan()` blocks the goroutine. If this is on the main goroutine and ANSI rendering is also on the main goroutine, the display freezes while waiting for input.

**Prevention:** Run the stdin reader in its own goroutine, sending commands to a channel. The main goroutine drives the render ticker and receives from both the packet channel and command channel with `select`.

**Phase:** Phase 1 (architecture).

---

## Priority Summary

| Pitfall | Impact | Phase |
|---------|--------|-------|
| Bind to group address, not 0.0.0.0 | High — silent wrong behavior | 1 |
| Set `IP_MULTICAST_IF` explicitly | High — IGMP joins on wrong interface | 1 |
| Use `JoinSourceSpecificGroup` for SSM | Critical — SSM doesn't work otherwise | 1 |
| SSM address range validation | Medium — confusing errors | 1 |
| Static binary verification in Makefile | High — container failures | 1 |
| TTY detection + ANSI fallback | Medium — broken `kubectl logs` | 1 |
| Terminal cleanup on exit | Medium — UX/debugging quality | 1 |
| Goroutine-safe membership map | High — panic under load | 1 |
| `CAP_NET_RAW`/`CAP_NET_ADMIN` in manifests | High — permission denied in K8s | 2 |
| CNI multicast support documentation | Critical — most common failure mode | 2 |
| IGMP snooping on bridge | High — cross-node fails | 2 |
| Host networking IP source mismatch | Medium — SSM filter breaks | 2 |
| `multicast_loop` disabled on sender | Low — confusing in single-node test | 1 |

---
*Research date: 2026-04-20*
