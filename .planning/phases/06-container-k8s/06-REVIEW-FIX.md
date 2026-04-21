---
phase: 06-container-k8s
status: all_findings_fixed
findings_fixed: 7
---

# Phase 06 — Review Fix Summary

All 7 findings from `06-REVIEW.md` have been addressed in atomic commits.

## Fixes Applied

| ID | Severity | Fix | Commit |
|----|----------|-----|--------|
| H1 | High | Changed `golang:1.25-alpine` to `golang:1.24-alpine` in Dockerfile | `fix(Dockerfile): pin builder image to golang:1.24-alpine` |
| H2 | High | Added `podAntiAffinity` to both sender and receiver so they are scheduled on different nodes | `fix(k8s): add podAntiAffinity to prevent sender/receiver node collision` |
| M1 | Medium | Added `imagePullPolicy: IfNotPresent` with comments about `kind load docker-image` workflow | `fix(k8s): add imagePullPolicy: IfNotPresent to both pods` |
| M2 | Medium | Added resource requests (cpu: 50m, memory: 32Mi) and limits (cpu: 200m, memory: 64Mi) to both pods | `fix(k8s): add resource requests and limits to both pods` |
| L1 | Low | Added `docker-buildx` Makefile target for multi-arch (`linux/amd64,linux/arm64`) builds | `feat(Makefile): add docker-buildx target for multi-arch builds` |
| L2 | Low | Removed `NET_ADMIN` from sender.yaml, kept only `NET_RAW`; added comment explaining why | `fix(k8s): remove unnecessary NET_ADMIN capability from sender` |
| L3 | Low | Added `restartPolicy: Never` to both pod specs | `fix(k8s): add restartPolicy: Never to both pods` |
