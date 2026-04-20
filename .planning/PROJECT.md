# Multicast Test App (mcast-test-app)

## What This Is

A Go-based multicast traffic generator and receiver for testing multicast networking in containerized environments. It simulates a simplified financial ticker feed where a sender publishes sequenced data to multicast groups and receivers display the incoming traffic in a fixed-height terminal UI. Designed to run in nicolaka/netshoot containers on Kubernetes, providing hands-on visibility into IGMP group membership and multicast data flow.

## Core Value

Clearly show multicast traffic flowing between sender and receiver with enough detail (headers, sequence numbers, payloads) to verify multicast routing works correctly — while keeping the code simple enough for a Go beginner to understand and learn from.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] Go sender binary that sends sequenced ticker-style payloads to multicast groups
- [ ] Go receiver binary that joins multicast groups and displays received traffic
- [ ] Simple payload format: sequence number + source info + short data (not realistic market data)
- [ ] Fixed ~20-line terminal display that scrolls within a region (no full-screen scroll)
- [ ] Display IP packet header summary alongside payload
- [ ] Support for multiple multicast groups simultaneously
- [ ] ASM (any-source multicast) group join/leave
- [ ] SSM (source-specific multicast) group join/leave with source IP filtering
- [ ] IGMPv2 support
- [ ] IGMPv3 support
- [ ] Command-line flags for initial config (group, port, interface)
- [ ] Interactive commands while running (join, leave groups dynamically)
- [ ] Kubernetes manifests using nicolaka/netshoot as base image
- [ ] Beginner-friendly Go code with clear comments explaining why, not just what

### Out of Scope

- Realistic financial market data protocol (FIX, ITCH, etc.) — too complex, defeats learning purpose
- GUI or web interface — terminal-only
- Performance benchmarking or latency measurement — this is a functional test tool
- Docker Compose setup — Kubernetes only
- IPv6 multicast / MLD — IPv4 IGMP only for v1

## Context

- The author is learning Go — code must be straightforward, well-commented, and use simple patterns
- nicolaka/netshoot is the target container image (already has tcpdump, ip, etc. for debugging)
- The app simulates high-frequency trading ticker feeds at a conceptual level — sequenced lines of data, not actual market protocols
- Primary use case is verifying multicast routing in Kubernetes/CNI environments
- IGMP version differences (v2 vs v3) are important for testing — v3 is required for SSM

## Constraints

- **Language**: Go — simple, idiomatic patterns only (no generics abuse, no complex interfaces)
- **Container image**: nicolaka/netshoot as base — binaries must be statically compiled (CGO_DISABLED=1)
- **Display**: Terminal-only, ~20 fixed lines, ANSI escape codes for in-place updates
- **Code style**: Every non-obvious line gets a comment explaining the "why"

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Go over Python/C | Learning Go is an explicit goal; Go has good net/multicast stdlib support | — Pending |
| netshoot base image | Already has networking debug tools (tcpdump, ip, nslookup) for troubleshooting | — Pending |
| Simple payload over real market protocol | Code must be understandable by a Go beginner | — Pending |
| Kubernetes over Docker Compose | Target environment is Kubernetes | — Pending |
| ANSI terminal scrolling region | Avoids ncurses/tui library complexity; keeps code simple | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? -> Move to Out of Scope with reason
2. Requirements validated? -> Move to Validated with phase reference
3. New requirements emerged? -> Add to Active
4. Decisions to log? -> Add to Key Decisions
5. "What This Is" still accurate? -> Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-04-20 after initialization*
