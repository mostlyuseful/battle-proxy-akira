package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"battle-proxy-akira/internal/config"
	"battle-proxy-akira/internal/ir"
	openaiapi "battle-proxy-akira/internal/openai"
	"battle-proxy-akira/internal/sse"
)

func TestCodexResponsesCompletePostsStreamingResponsesRequest(t *testing.T) {
	t.Parallel()

	var captured map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+codexTestJWT(t, "acct-123") {
			t.Error("missing bearer token")
		}
		if r.Header.Get("ChatGPT-Account-ID") != "acct-123" {
			t.Errorf("account header = %q", r.Header.Get("ChatGPT-Account-ID"))
		}
		if r.Header.Get("Originator") != codexOriginator {
			t.Errorf("originator = %q", r.Header.Get("Originator"))
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		writeCodexEvent(t, w, "response.output_text.delta", `{"type":"response.output_text.delta","delta":"OK"}`)
		writeCodexEvent(t, w, "response.completed", codexCompletedEvent("resp_1", "gpt-codex", "OK"))
	}))
	defer upstream.Close()

	p := newCodexTestProvider(t, upstream, codexTestJWT(t, "acct-123"))
	maxTokens := 10
	resp, err := p.Complete(context.Background(), ir.Request{
		Model: "gpt-codex",
		Messages: []ir.Message{
			{Role: ir.RoleSystem, Content: []ir.ContentPart{{Type: ir.ContentTypeText, Text: "Be terse."}}},
			{Role: ir.RoleUser, Content: []ir.ContentPart{
				{Type: ir.ContentTypeText, Text: "Look"},
				{Type: ir.ContentTypeImageURL, ImageURL: "data:image/png;base64,AA==", Detail: "low"},
			}},
		},
		Params: ir.SamplingParams{MaxCompletionTokens: &maxTokens},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.ID != "resp_1" || responseText(resp) != "OK" || resp.Usage == nil || resp.Usage.TotalTokens != 3 {
		t.Fatalf("response = %#v", resp)
	}
	if captured["stream"] != true || captured["store"] != false || captured["instructions"] != "Be terse." {
		t.Fatalf("request flags/instructions = %#v", captured)
	}
	if _, ok := captured["max_output_tokens"]; ok {
		t.Fatal("request unexpectedly contains max_output_tokens")
	}
	input := captured["input"].([]any)
	content := input[0].(map[string]any)["content"].([]any)
	image := content[1].(map[string]any)
	if image["type"] != "input_image" || image["image_url"] != "data:image/png;base64,AA==" {
		t.Fatalf("image part = %#v", image)
	}
}

func TestCodexResponsesStreamTranslatesEventsToChatChunks(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeCodexEvent(t, w, "response.created", `{"type":"response.created","response":{"id":"resp_stream","model":"gpt-codex"}}`)
		writeCodexEvent(t, w, "response.output_text.delta", `{"type":"response.output_text.delta","delta":"O"}`)
		writeCodexEvent(t, w, "response.output_text.delta", `{"type":"response.output_text.delta","delta":"K"}`)
		writeCodexEvent(t, w, "response.completed", codexCompletedEvent("resp_stream", "gpt-codex", "OK"))
	}))
	defer upstream.Close()

	p := newCodexTestProvider(t, upstream, codexTestJWT(t, "acct-123"))
	events, err := p.Stream(context.Background(), textCodexRequest())
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var deltas strings.Builder
	var finish string
	var done bool
	for event := range events {
		if event.Type == ir.EventTypeDone {
			done = true
			continue
		}
		chunk, err := openaiapi.ParseChatCompletionChunk(event.Raw)
		if err != nil {
			t.Fatalf("ParseChatCompletionChunk: %v", err)
		}
		if len(chunk.Choices) > 0 {
			deltas.WriteString(chunk.Choices[0].Delta.Content)
			if chunk.Choices[0].FinishReason != nil {
				finish = *chunk.Choices[0].FinishReason
			}
		}
	}
	if deltas.String() != "OK" || finish != "stop" || !done {
		t.Fatalf("deltas=%q finish=%q done=%v", deltas.String(), finish, done)
	}
}

func TestCodexResponsesRejectsMalformedOrMissingAccountJWT(t *testing.T) {
	t.Parallel()

	for _, token := range []string{"not-a-jwt", codexTestJWT(t, "")} {
		token := token
		t.Run(token[:min(len(token), 10)], func(t *testing.T) {
			var calls atomic.Int32
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return nil, fmt.Errorf("unexpected request")
			})}
			p, err := NewCodexResponses("codex", codexProviderConfig("https://example.invalid/backend-api/codex"), staticTokenSource(token), client)
			if err != nil {
				t.Fatalf("NewCodexResponses: %v", err)
			}
			_, err = p.Complete(context.Background(), textCodexRequest())
			if ErrorCode(err) != ErrorProviderAuthFailed || calls.Load() != 0 {
				t.Fatalf("err=%v calls=%d", err, calls.Load())
			}
		})
	}
}

func TestCodexResponsesClassifiesUpstreamStatus(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"rate_limit_exceeded"}}`))
	}))
	defer upstream.Close()

	p := newCodexTestProvider(t, upstream, codexTestJWT(t, "acct-123"))
	_, err := p.Complete(context.Background(), textCodexRequest())
	if ErrorCode(err) != ErrorProviderRateLimited || !IsRetryable(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestCodexResponsesStreamStopsOnCancellation(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeCodexEvent(t, w, "response.output_text.delta", `{"type":"response.output_text.delta","delta":"first"}`)
		<-r.Context().Done()
	}))
	defer upstream.Close()

	p := newCodexTestProvider(t, upstream, codexTestJWT(t, "acct-123"))
	ctx, cancel := context.WithCancel(context.Background())
	events, err := p.Stream(ctx, textCodexRequest())
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	select {
	case <-events:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first event")
	}
	cancel()
	select {
	case _, ok := <-events:
		if ok {
			for range events {
			}
		}
	case <-time.After(time.Second):
		t.Fatal("stream did not close after cancellation")
	}
}

func newCodexTestProvider(t *testing.T, upstream *httptest.Server, token string) *CodexResponsesProvider {
	t.Helper()
	p, err := NewCodexResponses("codex", codexProviderConfig(upstream.URL+"/backend-api/codex"), staticTokenSource(token), upstream.Client())
	if err != nil {
		t.Fatalf("NewCodexResponses: %v", err)
	}
	return p
}

func codexProviderConfig(baseURL string) config.ProviderConfig {
	return config.ProviderConfig{
		Type: config.ProviderTypeCodexResponses, BaseURL: baseURL,
		Models: map[string]config.ModelConfig{"gpt-codex": {Modalities: []string{ir.ModalityText, ir.ModalityImage}}},
	}
}

func textCodexRequest() ir.Request {
	return ir.Request{Model: "gpt-codex", Messages: []ir.Message{{Role: ir.RoleUser, Content: []ir.ContentPart{{Type: ir.ContentTypeText, Text: "Reply OK"}}}}}
}

func codexTestJWT(t *testing.T, accountID string) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, err := json.Marshal(map[string]any{"https://api.openai.com/auth": map[string]string{"chatgpt_account_id": accountID}})
	if err != nil {
		t.Fatal(err)
	}
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

func codexCompletedEvent(id, model, text string) string {
	payload := map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id": id, "object": "response", "model": model, "status": "completed",
			"output": []any{map[string]any{
				"id": "msg_1", "type": "message", "role": "assistant", "status": "completed",
				"content": []any{map[string]any{"type": "output_text", "text": text}},
			}},
			"usage": map[string]any{"input_tokens": 2, "output_tokens": 1, "total_tokens": 3},
		},
	}
	encoded, _ := json.Marshal(payload)
	return string(encoded)
}

func writeCodexEvent(t *testing.T, w http.ResponseWriter, eventType, data string) {
	t.Helper()
	if err := sse.WriteTypedEvent(w, eventType, data); err != nil {
		t.Errorf("write event: %v", err)
	}
}

func responseText(resp *ir.Response) string {
	var out strings.Builder
	for _, part := range resp.Message.Content {
		out.WriteString(part.Text)
	}
	return out.String()
}
