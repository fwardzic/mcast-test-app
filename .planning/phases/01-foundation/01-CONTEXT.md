# Phase 1: Foundation - Context

**Gathered:** 2026-04-20
**Status:** Ready for planning

<domain>
## Phase Boundary

Establish the Go project skeleton with `go mod init`, directory tree (`cmd/sender`, `cmd/receiver`, `internal/{config,packet,multicast,display}`, `k8s/`), top-level Makefile, and two shared internal packages (`internal/config`, `internal/packet`) with unit tests.

</domain>

<decisions>
## Implementation Decisions

### Go Module & Tooling
- **D-01:** Module path: `github.com/fwardzic/mcast-test-app`
- **D-02:** Go version: 1.24 in go.mod
- **D-03:** Linter: golangci-lint with `.golangci.yml` config
- **D-04:** Makefile targets: `build`, `test`, `lint` (minimum)

### Packet Format
- **D-05:** JSON field naming: snake_case (use struct tags like `json:"sequence"`)
- **D-06:** Timestamp format: RFC3339 with milliseconds (e.g., `2026-04-20T12:34:56.789Z`)
- **D-07:** Payload content: short ticker string simulating price data (e.g., `"AAPL 182.35 +0.12"`)
- **D-08:** Encoding: `encoding/json` — simplicity over performance for a learning tool

### Code Style
- **D-09:** Comment depth: moderate (standard open-source style) — doc comments on all exported types/functions, inline "why" comments on non-obvious lines only
- **D-10:** No tutorial-level line-by-line commentary — assumes basic Go familiarity

### Claude's Discretion
- Specific golangci-lint rules/settings in `.golangci.yml`
- Exact fake ticker symbols and price ranges for test data
- Internal package API signatures (as long as they satisfy phase success criteria)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

No external specs — requirements fully captured in decisions above and ROADMAP.md Phase 1 section.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- None — greenfield project, no existing code

### Established Patterns
- None yet — this phase establishes the patterns for all subsequent phases

### Integration Points
- `.gitignore` exists (currently minimal)
- Git repository initialized

</code_context>

<specifics>
## Specific Ideas

- Packet payload should look like a real ticker line: symbol, price, change (e.g., `"GOOG 2847.12 -3.45"`)
- snake_case JSON chosen specifically for readability in `tcpdump -A` output

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 01-foundation*
*Context gathered: 2026-04-20*
