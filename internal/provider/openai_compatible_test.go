package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"battle-proxy-akira/internal/config"
	"battle-proxy-akira/internal/ir"
)

type staticTokenSource string

func (s staticTokenSource) Token(context.Context) (string, error) { return string(s), nil }

func TestOpenAICompatibleProviderCompletePostsChatCompletion(t *testing.T) {
	t.Parallel()

	var captured struct {
		Path          string
		Authorization string
		ContentType   string
		Body          map[string]any
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Path = r.URL.Path
		captured.Authorization = r.Header.Get("Authorization")
		captured.ContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&captured.Body); err != nil {
			t.Fatalf("decode upstream request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "chatcmpl-upstream",
			"object": "chat.completion",
			"created": 123,
			"model": "gpt-test",
			"choices": [
				{
					"index": 0,
					"message": {"role": "assistant", "content": "hello back"},
					"finish_reason": "stop"
				}
			],
			"usage": {"prompt_tokens": 2, "completion_tokens": 3, "total_tokens": 5}
		}`))
	}))
	defer upstream.Close()

	provider, err := NewOpenAICompatible("openai_api", config.ProviderConfig{
		BaseURL: upstream.URL + "/v1",
		Models: map[string]config.ModelConfig{
			"gpt-test": {Modalities: []string{ir.ModalityText}},
		},
	}, staticTokenSource("test-token"), upstream.Client())
	if err != nil {
		t.Fatalf("NewOpenAICompatible: %v", err)
	}

	resp, err := provider.Complete(context.Background(), ir.Request{
		Model: "gpt-test",
		Messages: []ir.Message{
			{Role: ir.RoleUser, Content: []ir.ContentPart{{Type: ir.ContentTypeText, Text: "hello"}}},
		},
		Params: ir.SamplingParams{Temperature: ptr(0.2), Stop: []string{"END"}},
		Stream: true,
		Extra: map[string]json.RawMessage{
			"x_provider_option": json.RawMessage(`{"enabled":true}`),
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if captured.Path != "/v1/chat/completions" {
		t.Fatalf("path = %q, want /v1/chat/completions", captured.Path)
	}
	if captured.Authorization != "Bearer test-token" {
		t.Fatalf("authorization = %q, want bearer token", captured.Authorization)
	}
	if !strings.HasPrefix(captured.ContentType, "application/json") {
		t.Fatalf("content-type = %q, want application/json", captured.ContentType)
	}
	if captured.Body["model"] != "gpt-test" {
		t.Fatalf("body model = %#v, want gpt-test", captured.Body["model"])
	}
	if captured.Body["stream"] != nil {
		t.Fatalf("body stream = %#v, want absent/false for non-stream Complete", captured.Body["stream"])
	}
	messages := captured.Body["messages"].([]any)
	message := messages[0].(map[string]any)
	if message["role"] != "user" || message["content"] != "hello" {
		t.Fatalf("message = %#v", message)
	}
	if captured.Body["temperature"] != 0.2 {
		t.Fatalf("temperature = %#v, want 0.2", captured.Body["temperature"])
	}
	if captured.Body["stop"] != "END" {
		t.Fatalf("stop = %#v, want END", captured.Body["stop"])
	}
	providerOption := captured.Body["x_provider_option"].(map[string]any)
	if providerOption["enabled"] != true {
		t.Fatalf("x_provider_option = %#v", providerOption)
	}

	if resp.ID != "chatcmpl-upstream" {
		t.Fatalf("response ID = %q", resp.ID)
	}
	if resp.Model != "gpt-test" {
		t.Fatalf("response model = %q", resp.Model)
	}
	if resp.Message.Role != ir.RoleAssistant || resp.Message.Content[0].Text != "hello back" {
		t.Fatalf("response message = %#v", resp.Message)
	}
	if resp.FinishReason != "stop" {
		t.Fatalf("finish reason = %q", resp.FinishReason)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 5 {
		t.Fatalf("usage = %#v, want total 5", resp.Usage)
	}
	if !json.Valid(resp.RawBody) {
		t.Fatalf("response RawBody is invalid JSON: %s", resp.RawBody)
	}
}

func TestOpenAICompatibleProviderCompleteReturnsUpstreamStatusErrorWithoutBody(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "secret-token-in-body", http.StatusUnauthorized)
	}))
	defer upstream.Close()

	provider, err := NewOpenAICompatible("openai_api", config.ProviderConfig{BaseURL: upstream.URL}, staticTokenSource("test-token"), upstream.Client())
	if err != nil {
		t.Fatalf("NewOpenAICompatible: %v", err)
	}

	_, err = provider.Complete(context.Background(), ir.Request{
		Model:    "gpt-test",
		Messages: []ir.Message{{Role: ir.RoleUser, Content: []ir.ContentPart{{Type: ir.ContentTypeText, Text: "hello"}}}},
	})
	if err == nil {
		t.Fatal("Complete returned nil error, want upstream status error")
	}
	if !strings.Contains(err.Error(), "status 401") {
		t.Fatalf("error = %q, want status 401", err.Error())
	}
	if strings.Contains(err.Error(), "secret-token-in-body") || strings.Contains(err.Error(), "test-token") {
		t.Fatalf("error leaked secret data: %q", err.Error())
	}
}

func TestOpenAICompatibleProviderStreamReadsSSEIncrementally(t *testing.T) {
	t.Parallel()

	var captured struct {
		Path          string
		Authorization string
		Accept        string
		Body          map[string]any
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Path = r.URL.Path
		captured.Authorization = r.Header.Get("Authorization")
		captured.Accept = r.Header.Get("Accept")
		if err := json.NewDecoder(r.Body).Decode(&captured.Body); err != nil {
			t.Errorf("decode upstream request body: %v", err)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"delta\":\"hello\"}\n\n"))
		flusher.Flush()
		_, _ = w.Write([]byte("data: {\"delta\":\" world\"}\n\n"))
		flusher.Flush()
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer upstream.Close()

	provider, err := NewOpenAICompatible("openai_api", config.ProviderConfig{BaseURL: upstream.URL + "/v1"}, staticTokenSource("test-token"), upstream.Client())
	if err != nil {
		t.Fatalf("NewOpenAICompatible: %v", err)
	}

	events, err := provider.Stream(context.Background(), ir.Request{
		Model:    "gpt-test",
		Messages: []ir.Message{{Role: ir.RoleUser, Content: []ir.ContentPart{{Type: ir.ContentTypeText, Text: "hello"}}}},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var got []ir.Event
	for event := range events {
		got = append(got, event)
	}
	if len(got) != 3 {
		t.Fatalf("events length = %d, want 3", len(got))
	}
	if got[0].Type != ir.EventTypeMessageDelta || got[0].Text != `{"delta":"hello"}` {
		t.Fatalf("event 0 = %#v", got[0])
	}
	if got[1].Type != ir.EventTypeMessageDelta || got[1].Text != `{"delta":" world"}` {
		t.Fatalf("event 1 = %#v", got[1])
	}
	if got[2].Type != ir.EventTypeDone || got[2].Text != "[DONE]" {
		t.Fatalf("event 2 = %#v, want done", got[2])
	}
	if captured.Path != "/v1/chat/completions" {
		t.Fatalf("path = %q, want /v1/chat/completions", captured.Path)
	}
	if captured.Authorization != "Bearer test-token" {
		t.Fatalf("authorization = %q, want bearer token", captured.Authorization)
	}
	if captured.Accept != "text/event-stream" {
		t.Fatalf("accept = %q, want text/event-stream", captured.Accept)
	}
	if captured.Body["stream"] != true {
		t.Fatalf("body stream = %#v, want true", captured.Body["stream"])
	}
}

func TestOpenAICompatibleProviderStreamReturnsPreStreamStatusErrorWithoutBody(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "secret-token-in-body", http.StatusTooManyRequests)
	}))
	defer upstream.Close()

	provider, err := NewOpenAICompatible("openai_api", config.ProviderConfig{BaseURL: upstream.URL}, staticTokenSource("test-token"), upstream.Client())
	if err != nil {
		t.Fatalf("NewOpenAICompatible: %v", err)
	}

	_, err = provider.Stream(context.Background(), ir.Request{
		Model:    "gpt-test",
		Messages: []ir.Message{{Role: ir.RoleUser, Content: []ir.ContentPart{{Type: ir.ContentTypeText, Text: "hello"}}}},
	})
	if err == nil {
		t.Fatal("Stream returned nil error, want upstream status error")
	}
	if !strings.Contains(err.Error(), "status 429") {
		t.Fatalf("error = %q, want status 429", err.Error())
	}
	if strings.Contains(err.Error(), "secret-token-in-body") || strings.Contains(err.Error(), "test-token") {
		t.Fatalf("error leaked secret data: %q", err.Error())
	}
}

func TestOpenAICompatibleProviderStreamPropagatesContextCancellation(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"delta\":\"hello\"}\n\n"))
		flusher.Flush()
		<-r.Context().Done()
		close(canceled)
	}))
	defer upstream.Close()

	provider, err := NewOpenAICompatible("openai_api", config.ProviderConfig{BaseURL: upstream.URL}, staticTokenSource("test-token"), upstream.Client())
	if err != nil {
		t.Fatalf("NewOpenAICompatible: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	events, err := provider.Stream(ctx, ir.Request{
		Model:    "gpt-test",
		Messages: []ir.Message{{Role: ir.RoleUser, Content: []ir.ContentPart{{Type: ir.ContentTypeText, Text: "hello"}}}},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	<-started
	if event := <-events; event.Text != `{"delta":"hello"}` {
		t.Fatalf("first event = %#v", event)
	}
	cancel()

	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream request context was not canceled")
	}
}

func TestOpenAICompatibleProviderModelsReturnsConfiguredModels(t *testing.T) {
	t.Parallel()

	provider, err := NewOpenAICompatible("openai_api", config.ProviderConfig{
		BaseURL: "https://example.invalid/v1",
		Models: map[string]config.ModelConfig{
			"gpt-test": {Modalities: []string{ir.ModalityText, ir.ModalityImage}},
		},
	}, staticTokenSource("test-token"), nil)
	if err != nil {
		t.Fatalf("NewOpenAICompatible: %v", err)
	}

	models, err := provider.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(models) != 1 || models[0].ID != "gpt-test" || models[0].Provider != "openai_api" {
		t.Fatalf("models = %#v", models)
	}
}

func ptr(v float64) *float64 { return &v }
