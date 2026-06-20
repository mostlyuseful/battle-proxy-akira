---
id: ops.metrics-endpoint
title: Basic structured metrics endpoint
status: active
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

- [ ] Track request counts by endpoint/status class.
- [ ] Track error counts by internal error code where available.
- [ ] Track simple latency summaries or recent aggregate durations.
- [ ] Expose metrics through a JSON endpoint.
- [ ] Add tests for counter increments and endpoint response shape.

## Context

The spec lists a structured metrics endpoint under operational hardening.

## Out of Scope

- Prometheus exposition format
- Distributed metrics

## Notes

Use stdlib synchronization primitives and keep overhead low.
