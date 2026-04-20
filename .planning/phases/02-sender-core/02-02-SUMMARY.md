# Summary: Plan 02-02 — Sender Binary

**Phase:** 02-sender-core
**Plan:** 02-02
**Executed:** 2026-04-20

## What Was Done

1. **Added pflag dependency** — `github.com/spf13/pflag` for GNU-style CLI flags
2. **Implemented cmd/sender/main.go** — Complete sender binary with:
   - 6 CLI flags: `--group/-g`, `--port/-p`, `--iface/-i`, `--ttl/-t`, `--rate/-r`, `--loopback/-l`
   - `validateFlags()` — checks iface required, group is multicast, port/TTL ranges, rate >= 1
   - `sendLoop()` — ticker-based send loop with context cancellation for clean shutdown
   - `signal.NotifyContext` for SIGINT/SIGTERM handling
   - `sync.WaitGroup` for goroutine join before exit
3. **Added unit tests** — table-driven `TestValidateFlags` (6 cases), `TestSendLoop_CancelledContext`, `TestSendLoop_SendsPackets`

## Decisions Made

| Decision | Rationale |
|----------|-----------|
| TTL default = 2 (not config.DefaultTTL=1) | Per D-03: cross-node testing needs one router hop |
| Wide test tolerance (5-25 packets at 100pps/150ms) | Avoids flaky CI on slow machines |
| `contains` helper over `strings.Contains` | Consistent with Phase 1 test pattern |

## Verification

- `go build ./cmd/sender/...` — pass
- `go vet ./cmd/sender/...` — pass
- `go test ./cmd/sender/...` — pass
- `go test -race ./cmd/sender/...` — pass

## Commits

1. `5d1e580` — Add spf13/pflag dependency
2. `54c1950` — Implement sender binary
3. `4349458` — Add unit tests
