---
id: server.timeouts-shutdown
title: Server timeouts and graceful shutdown
status: active
priority: p2
domain: server
slice: operational-hardening
depends_on:
  - project.bootstrap
  - config.json-loader
tags:
  - server
  - ops
  - reliability
context_files:
  - docs/spec.md
---

# Server timeouts and graceful shutdown

## Goal

Configure HTTP server timeouts and graceful shutdown behavior from config.

## Acceptance Criteria

- [ ] Apply read_timeout_seconds, write_timeout_seconds, and idle_timeout_seconds from config.
- [ ] Support write_timeout_seconds=0 for long-lived streaming when configured.
- [ ] Handle SIGINT/SIGTERM with graceful shutdown.
- [ ] Allow in-flight requests a bounded shutdown window.
- [ ] Add tests or a small integration check for timeout configuration and shutdown wiring where practical.

## Context

Streaming may require different write timeout behavior than ordinary JSON endpoints.

## Out of Scope

- Config reload
- Kubernetes readiness management

## Notes

Keep defaults safe for local use.
