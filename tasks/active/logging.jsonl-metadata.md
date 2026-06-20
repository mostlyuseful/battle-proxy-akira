---
id: logging.jsonl-metadata
title: Metadata-only JSONL request logging
status: active
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

- [ ] Implement logging modes off and metadata_only.
- [ ] Write one JSON object per completed request to the configured path.
- [ ] Include timestamp, request_id, requested_model, resolved_provider, resolved_model, stream flag, status, latency_ms, and retry_count where available.
- [ ] Ensure logging failures do not crash successful requests unless explicitly configured otherwise.
- [ ] Add tests for disabled logging and metadata log record content.

## Context

Full transcript logging is intentionally out of scope for this task.

## Out of Scope

- Full transcript logging
- Image hash logging
- Metrics endpoint

## Notes

Make the logger interface easy to mock in handler tests.
