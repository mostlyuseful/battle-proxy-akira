---
id: config.reload
title: Safe runtime config reload
status: active
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

- [ ] Provide an explicit reload trigger mechanism appropriate for the current server design.
- [ ] Validate new config before swapping it into use.
- [ ] Swap router/provider configuration atomically for new requests.
- [ ] Avoid interrupting in-flight requests during reload.
- [ ] Preserve or reconcile availability state for unchanged provider/model pairs.
- [ ] Add tests for successful reload, failed validation retaining old config, and in-flight safety where practical.

## Context

Config reload is operational hardening and should build on established config and router state.

## Out of Scope

- Admin UI
- Distributed config

## Notes

Prefer correctness and clear errors over automatic filesystem watching.
