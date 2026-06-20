package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"battle-proxy-akira/internal/ir"
)

func TestResponsesStreamTranslatorEmitsFullLifecycle(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	tr := NewResponsesStreamTranslator("resp_1", "msg_1", "coding", 1700000000)

	if err := tr.WriteOpening(&buf); err != nil {
		t.Fatalf("WriteOpening: %v", err)
	}

	chunks := []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":"hello"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":" world"}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`,
	}
	for _, chunk := range chunks {
		if err := tr.Translate(&buf, ir.Event{Type: ir.EventTypeMessageDelta, Text: chunk, Raw: json.RawMessage(chunk)}); err != nil {
			t.Fatalf("Translate: %v", err)
		}
	}
	// Done event should be a no-op for translation.
	if err := tr.Translate(&buf, ir.Event{Type: ir.EventTypeDone, Text: "[DONE]"}); err != nil {
		t.Fatalf("Translate done: %v", err)
	}

	if err := tr.WriteClosing(&buf); err != nil {
		t.Fatalf("WriteClosing: %v", err)
	}

	out := buf.String()
	// Expected event ordering.
	wantSequence := []string{
		"event: response.created",
		"event: response.output_item.added",
		"event: response.content_part.added",
		"event: response.output_text.delta",
		"event: response.output_text.delta",
		"event: response.output_text.done",
		"event: response.content_part.done",
		"event: response.output_item.done",
		"event: response.completed",
	}
	prev := -1
	for _, want := range wantSequence {
		idx := strings.Index(out, want)
		if idx < 0 {
			t.Fatalf("missing %q in output\n%s", want, out)
		}
		if idx < prev {
			t.Fatalf("%q appeared out of order\n%s", want, out)
		}
		prev = idx
	}
	// Role-only chunk must not produce a delta.
	if got := strings.Count(out, "event: response.output_text.delta"); got != 2 {
		t.Fatalf("delta count = %d, want 2 (role-only chunk skipped)", got)
	}
	// Aggregated text in done/completed.
	if !strings.Contains(out, `"text":"hello world"`) {
		t.Fatalf("missing aggregated text\n%s", out)
	}
	// Usage carried into completed event.
	if !strings.Contains(out, `"input_tokens":3`) || !strings.Contains(out, `"total_tokens":7`) {
		t.Fatalf("missing usage in completed event\n%s", out)
	}
	// Created event has in_progress status; completed has completed status.
	if !strings.Contains(out, `"status":"in_progress"`) {
		t.Fatalf("missing in_progress status\n%s", out)
	}
	if !strings.Contains(out, `"status":"completed"`) {
		t.Fatalf("missing completed status\n%s", out)
	}
}

func TestResponsesStreamTranslatorWriteOpeningIdempotent(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	tr := NewResponsesStreamTranslator("resp_1", "msg_1", "m", 1)
	if err := tr.WriteOpening(&buf); err != nil {
		t.Fatal(err)
	}
	if err := tr.WriteOpening(&buf); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(buf.String(), "event: response.created"); got != 1 {
		t.Fatalf("created count = %d, want 1 (opening idempotent)", got)
	}
}

func TestResponsesStreamTranslatorSkipsUnparseableChunks(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	tr := NewResponsesStreamTranslator("resp_1", "msg_1", "m", 1)
	_ = tr.WriteOpening(&buf)

	// Garbage chunk is skipped, not fatal.
	if err := tr.Translate(&buf, ir.Event{Type: ir.EventTypeMessageDelta, Text: "not json", Raw: json.RawMessage("not json")}); err != nil {
		t.Fatalf("Translate garbage: %v", err)
	}
	// Valid content still flows after garbage.
	valid := `{"choices":[{"index":0,"delta":{"content":"ok"}}]}`
	if err := tr.Translate(&buf, ir.Event{Type: ir.EventTypeMessageDelta, Text: valid, Raw: json.RawMessage(valid)}); err != nil {
		t.Fatalf("Translate valid: %v", err)
	}
	if !strings.Contains(buf.String(), `"delta":"ok"`) {
		t.Fatalf("valid chunk after garbage not emitted\n%s", buf.String())
	}
	if tr.FullText() != "ok" {
		t.Fatalf("FullText = %q, want ok", tr.FullText())
	}
}

func TestResponsesStreamTranslatorLengthFinishReasonIsIncomplete(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	tr := NewResponsesStreamTranslator("resp_1", "msg_1", "m", 1)
	_ = tr.WriteOpening(&buf)
	chunk := `{"choices":[{"index":0,"delta":{},"finish_reason":"length"}]}`
	_ = tr.Translate(&buf, ir.Event{Type: ir.EventTypeMessageDelta, Text: chunk, Raw: json.RawMessage(chunk)})
	_ = tr.WriteClosing(&buf)

	if !strings.Contains(buf.String(), `"status":"incomplete"`) {
		t.Fatalf("expected incomplete status for length finish reason\n%s", buf.String())
	}
}

func TestResponsesStreamTranslatorWriteError(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	tr := NewResponsesStreamTranslator("resp_1", "msg_1", "m", 1)
	if err := tr.WriteError(&buf, "upstream_error", "boom"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "event: error") {
		t.Fatalf("missing error event\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), `"code":"upstream_error"`) || !strings.Contains(buf.String(), `"message":"boom"`) {
		t.Fatalf("missing error payload\n%s", buf.String())
	}
}

func TestParseChatCompletionChunk(t *testing.T) {
	t.Parallel()

	chunk, err := ParseChatCompletionChunk([]byte(`{"id":"c","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`))
	if err != nil {
		t.Fatalf("ParseChatCompletionChunk: %v", err)
	}
	if len(chunk.Choices) != 1 || chunk.Choices[0].Delta.Content != "hi" {
		t.Fatalf("unexpected chunk: %+v", chunk)
	}
	if chunk.Choices[0].FinishReason == nil || *chunk.Choices[0].FinishReason != "stop" {
		t.Fatalf("finish reason = %v", chunk.Choices[0].FinishReason)
	}
	if chunk.Usage == nil || chunk.Usage.TotalTokens != 3 {
		t.Fatalf("usage = %+v", chunk.Usage)
	}
}
