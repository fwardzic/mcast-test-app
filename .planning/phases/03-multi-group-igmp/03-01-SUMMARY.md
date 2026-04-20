# Plan 03-01 Summary — IGMP Helpers & SSM Range Check

## What Was Built

- **IsSSMAddress helper** in `internal/config/config.go`: validates whether an IP falls in the IANA SSM range `232.0.0.0/8`.
- **IGMPConn interface** in `internal/multicast/receiver.go`: defines `JoinGroup`, `LeaveGroup`, `JoinSourceSpecificGroup`, `LeaveSourceSpecificGroup` methods, satisfied by `*ipv4.PacketConn`.
- **Four IGMP helper functions** in `receiver.go`: `JoinASM`, `LeaveASM`, `JoinSSM`, `LeaveSSM` — each wraps the corresponding `IGMPConn` method with structured error messages.
- **PC() accessor** on `SenderConn` in `sender.go`: exposes the underlying `*ipv4.PacketConn` for IGMP membership management.

## Key Files Created/Modified

| File | Action |
|------|--------|
| `internal/config/config.go` | Added `net` import, `ssmRange` var, `IsSSMAddress` function |
| `internal/config/config_test.go` | Added `TestIsSSMAddress` with 7 table-driven cases |
| `internal/multicast/receiver.go` | Created — `IGMPConn` interface + 4 helper functions |
| `internal/multicast/receiver_test.go` | Created — `mockIGMPConn` + `TestJoinASM/LeaveASM/JoinSSM/LeaveSSM` |
| `internal/multicast/sender.go` | Added `PC() *ipv4.PacketConn` accessor |

## Test Results

```
ok  github.com/fwardzic/mcast-test-app/internal/config
ok  github.com/fwardzic/mcast-test-app/internal/multicast
go build ./... — exit 0
```

## Self-Check Status

- [x] All 5 tasks executed
- [x] Each task committed individually with `feat(03-01): <description>`
- [x] `go test ./internal/config/... ./internal/multicast/...` passes
- [x] `go build ./...` passes
- [x] SUMMARY.md created and committed
