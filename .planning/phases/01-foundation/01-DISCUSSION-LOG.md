# Phase 1: Foundation - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-20
**Phase:** 01-foundation
**Areas discussed:** Go module path, Packet format details, Comment style & depth, Makefile & tooling

---

## Go Module Path

| Option | Description | Selected |
|--------|-------------|----------|
| github.com/yourorg/mcast-test-app | Standard for publishable Go projects. Enables go get. | ✓ |
| mcast-test-app | Short, simple, works fine for a learning/test tool. | |
| github.com/isovalent/mcast-test-app | Isovalent org namespace. | |

**User's choice:** github.com/fwardzic/mcast-test-app (specified exact org after selecting pattern)
**Notes:** None

---

## Packet Format Details

### JSON Field Naming

| Option | Description | Selected |
|--------|-------------|----------|
| snake_case JSON fields | Keys like "sequence", "group". Clear in tcpdump. | ✓ |
| PascalCase JSON fields | Matches Go struct names directly. | |
| Abbreviated fields | Compact but harder to read. | |

**User's choice:** snake_case JSON fields

### Timestamp Format

| Option | Description | Selected |
|--------|-------------|----------|
| RFC3339 with milliseconds | Human-readable in tcpdump. | ✓ |
| Unix epoch millis | Easy to compute deltas. | |
| Both formats | Best of both but adds size. | |

**User's choice:** RFC3339 with milliseconds

### Payload Content

| Option | Description | Selected |
|--------|-------------|----------|
| Short ticker string (symbol + fake price) | Looks like a ticker line. | ✓ |
| Minimal placeholder | Just a counter or fixed string. | |
| You decide | Claude's discretion. | |

**User's choice:** Short ticker string

---

## Comment Style & Depth

| Option | Description | Selected |
|--------|-------------|----------|
| Heavily commented (tutorial style) | Every function + key lines. | |
| Moderate (standard open-source style) | Doc comments on exports, inline on non-obvious. | ✓ |
| File-level blocks only | Package-level blocks, minimal inline. | |

**User's choice:** Moderate

---

## Makefile & Tooling

### Linter

| Option | Description | Selected |
|--------|-------------|----------|
| golangci-lint | Industry standard, catches real issues. | ✓ |
| go vet + staticcheck | Lighter, fewer config files. | |
| Skip linting for now | Add later if needed. | |

**User's choice:** golangci-lint

### Go Version

| Option | Description | Selected |
|--------|-------------|----------|
| Go 1.24 | Latest stable. Best toolchain support. | ✓ |
| Go 1.23 | One version back. | |
| You decide | Claude picks latest stable. | |

**User's choice:** Go 1.24

---

## Claude's Discretion

- golangci-lint rule configuration
- Exact ticker symbols and price ranges
- Internal package API signatures

## Deferred Ideas

None
