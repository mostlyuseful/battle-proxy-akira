---
id: server.timeouts-shutdown
title: Server timeouts and graceful shutdown
status: done
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

- [x] Apply read_timeout_seconds, write_timeout_seconds, and idle_timeout_seconds from config.
- [x] Support write_timeout_seconds=0 for long-lived streaming when configured.
- [x] Handle SIGINT/SIGTERM with graceful shutdown.
- [x] Allow in-flight requests a bounded shutdown window.
- [x] Add tests or a small integration check for timeout configuration and shutdown wiring where practical.

## Context

Streaming may require different write timeout behavior than ordinary JSON endpoints.

## Out of Scope

- Config reload
- Kubernetes readiness management

## Notes

Keep defaults safe for local use.

Completed in `cmd/llm-proxy`: runtime config is loaded from `LLM_PROXY_CONFIG`, `LLM_PROXY_ADDR` remains an optional development address override, and the HTTP server applies configured read/write/idle timeouts. A zero write timeout is preserved for streaming. SIGINT/SIGTERM trigger graceful shutdown with a 10s bounded grace window before force-close. Tests cover timeout mapping, zero write timeout behavior, graceful waiting for an in-flight request, and bounded shutdown timeout behavior.
