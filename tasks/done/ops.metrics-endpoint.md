---
id: ops.metrics-endpoint
title: Basic structured metrics endpoint
status: done
priority: p3
domain: ops
slice: operational-hardening
depends_on:
- logging.jsonl-metadata
tags:
- ops
- metrics
- observability
context_files:
- docs/spec.md
---

# Basic structured metrics endpoint

## Goal

Expose basic structured runtime counters for operations without adding a metrics dependency.

## Acceptance Criteria

- [x] Track request counts by endpoint/status class.
- [x] Track error counts by internal error code where available.
- [x] Track simple latency summaries or recent aggregate durations.
- [x] Expose metrics through a JSON endpoint.
- [x] Add tests for counter increments and endpoint response shape.

## Context

The spec lists a structured metrics endpoint under operational hardening.

## Out of Scope

- Prometheus exposition format
- Distributed metrics

## Notes

Use stdlib synchronization primitives and keep overhead low.

Implemented in `internal/metrics/collector.go` (mutex-protected `Collector` with request counts keyed by endpoint/status-class, error counts by internal code, per-bucket latency summaries with count/sum/min/max/mean plus a 128-sample ring buffer for p50/p95/p99), exposed via `GET /metrics` as JSON (`Snapshot` with `requests`, `errors`, `latency`, totals, and start/generated timestamps). Wired into `internal/api` via `WithMetrics`, a `metricsMiddleware` that records endpoint/status-class/latency for every request using a `statusRecorder`, and context-stored collector access so `writeLoggedOpenAIError` records internal error codes (the single proxy-error choke point). `main.go` constructs a collector and passes it to `NewServer`. Tests cover counter increments, error-code recording, latency aggregation/percentiles, sorted/deterministic snapshots, nil-collector safety, the `/metrics` endpoint shape (incl. nil collector returning empty), middleware recording of endpoint/status/latency, error-code recording through the error path, the health endpoint, and context plumbing. Prometheus format and distributed metrics remain out of scope.
