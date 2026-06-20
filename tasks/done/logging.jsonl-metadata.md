---
id: logging.jsonl-metadata
title: Metadata-only JSONL request logging
status: done
priority: p1
domain: logging
slice: metadata-logging-and-baseline-safety
depends_on:
- api.chat-nonstream
- api.chat-stream
tags:
- logging
- jsonl
- observability
context_files:
- docs/spec.md
---

# Metadata-only JSONL request logging

## Goal

Write opt-in metadata-only request logs as JSONL records.

## Acceptance Criteria

- [x] Implement logging modes off and metadata_only.
- [x] Write one JSON object per completed request to the configured path.
- [x] Include timestamp, request_id, requested_model, resolved_provider, resolved_model, stream flag, status, latency_ms, and retry_count where available.
- [x] Ensure logging failures do not crash successful requests unless explicitly configured otherwise.
- [x] Add tests for disabled logging and metadata log record content.

## Context

Full transcript logging is intentionally out of scope for this task.

## Out of Scope

- Full transcript logging
- Image hash logging
- Metrics endpoint

## Notes

Make the logger interface easy to mock in handler tests.

Completed with `internal/logging` logger interface, no-op/off mode, metadata-only JSONL appender, `WithRequestLogger` API hook, chat handler metadata records for success/error paths, generated/request-header request IDs, ignored logger errors for successful requests, and tests for disabled logging, JSONL record content, API metadata content, and logging failure resilience.
