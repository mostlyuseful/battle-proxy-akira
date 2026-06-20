---
id: config.reload
title: Safe runtime config reload
status: done
priority: p3
domain: config
slice: operational-hardening
depends_on:
- router.availability-state
- config.json-loader
tags:
- config
- reload
- ops
context_files:
- docs/spec.md
---

# Safe runtime config reload

## Goal

Reload configuration without dropping in-flight requests or corrupting routing state.

## Acceptance Criteria

- [x] Provide an explicit reload trigger mechanism appropriate for the current server design.
- [x] Validate new config before swapping it into use.
- [x] Swap router/provider configuration atomically for new requests.
- [x] Avoid interrupting in-flight requests during reload.
- [x] Preserve or reconcile availability state for unchanged provider/model pairs.
- [x] Add tests for successful reload, failed validation retaining old config, and in-flight safety where practical.

## Context

Config reload is operational hardening and should build on established config and router state.

## Out of Scope

- Admin UI
- Distributed config

## Notes

Prefer correctness and clear errors over automatic filesystem watching.

Completed via a new `internal/runtime` package: `Manager` holds an `atomic.Pointer[Snapshot]` of `(config, *router.StaticRouter, providers)` and implements `router.Router` (Resolve/MarkFailure/MarkSuccess) plus `Models` by delegating to the current snapshot. `Reload` re-runs the configured loader (validates via `config.Load`/`Validate`), rebuilds providers and router, reconciles availability state for unchanged provider/model pairs (`AvailabilityTracker.Restore` + `StaticRouter.RestoreAvailability`), and atomically swaps the snapshot; on any failure the previous snapshot stays active. In-flight requests keep using their already-resolved candidates and provider instances. The explicit trigger is `SIGHUP`, handled by `runReloadLoop` in `cmd/llm-proxy/main.go`; reload failures are logged but never terminate the proxy. `main.go` now wires the manager as the chat router and model lister. Tests cover successful reload, loader-error and invalid-config retention, in-flight candidate safety, availability preservation for unchanged pairs, availability dropping for removed pairs, and the SIGHUP loop.
