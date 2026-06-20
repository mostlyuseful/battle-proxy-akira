package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"battle-proxy-akira/internal/ir"
	providerpkg "battle-proxy-akira/internal/provider"
	"battle-proxy-akira/internal/router"
)

func TestChatCompletionsStreamingRetriesBeforeFirstEvent(t *testing.T) {
	logger := &recordingLogger{}
	first := &streamRetryProvider{name: "first", streamErr: &providerpkg.Error{Code: providerpkg.ErrorProviderRateLimited, Retryable: true, Provider: "first"}}
	second := &streamRetryProvider{name: "second", events: []ir.Event{{Type: ir.EventTypeMessageDelta, Text: `{"delta":"ok"}`}, {Type: ir.EventTypeDone, Text: "[DONE]"}}}
	r := &streamRetryRouter{candidates: []router.RouteCandidate{
		{ProviderName: "first", ProviderModel: "gpt-test", RequestedModel: "coding", Provider: first},
		{ProviderName: "second", ProviderModel: "gpt-test", RequestedModel: "coding", Provider: second},
	}}
	handler := NewServer(WithChatRouter(r), WithRequestLogger(logger))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"coding","messages":[{"role":"user","content":"hello"}],"stream":true}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if first.streamCalls != 1 || second.streamCalls != 1 {
		t.Fatalf("stream calls first=%d second=%d, want 1/1", first.streamCalls, second.streamCalls)
	}
	if len(r.failures) != 1 || r.failures[0] != "first" {
		t.Fatalf("failures = %#v, want [first]", r.failures)
	}
	if len(r.successes) != 1 || r.successes[0] != "second" {
		t.Fatalf("successes = %#v, want [second]", r.successes)
	}
	want := "data: {\"delta\":\"ok\"}\n\ndata: [DONE]\n\n"
	if rec.Body.String() != want {
		t.Fatalf("stream body = %q, want %q", rec.Body.String(), want)
	}
	if len(logger.records) != 1 || logger.records[0].RetryCount != 1 || logger.records[0].ResolvedProvider != "second" {
		t.Fatalf("log records = %#v, want retry_count=1 and second provider", logger.records)
	}
}

func TestChatCompletionsStreamingDoesNotFallbackAfterFirstEvent(t *testing.T) {
	first := &streamRetryProvider{name: "first", events: []ir.Event{
		{Type: ir.EventTypeMessageDelta, Text: `{"delta":"first"}`},
		{Type: ir.EventTypeError, Error: &ir.Error{Code: providerpkg.ErrorUpstream, Message: "boom"}},
	}}
	second := &streamRetryProvider{name: "second", events: []ir.Event{{Type: ir.EventTypeMessageDelta, Text: `{"delta":"second"}`}}}
	r := &streamRetryRouter{candidates: []router.RouteCandidate{
		{ProviderName: "first", ProviderModel: "gpt-test", RequestedModel: "coding", Provider: first},
		{ProviderName: "second", ProviderModel: "gpt-test", RequestedModel: "coding", Provider: second},
	}}
	handler := NewServer(WithChatRouter(r))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"coding","messages":[{"role":"user","content":"hello"}],"stream":true}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if first.streamCalls != 1 || second.streamCalls != 0 {
		t.Fatalf("stream calls first=%d second=%d, want 1/0", first.streamCalls, second.streamCalls)
	}
	if len(r.failures) != 1 || r.failures[0] != "first" {
		t.Fatalf("failures = %#v, want [first]", r.failures)
	}
	if strings.Contains(rec.Body.String(), `"delta":"second"`) {
		t.Fatalf("stream body contains fallback provider data after first event: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), string(ErrorStreamInterrupted)) {
		t.Fatalf("stream body = %q, want stream_interrupted SSE error", rec.Body.String())
	}
}

type streamRetryRouter struct {
	candidates []router.RouteCandidate
	failures   []string
	successes  []string
}

func (r *streamRetryRouter) Resolve(context.Context, ir.Request) ([]router.RouteCandidate, error) {
	return r.candidates, nil
}

func (r *streamRetryRouter) MarkFailure(candidate router.RouteCandidate, err error) {
	r.failures = append(r.failures, candidate.ProviderName)
}

func (r *streamRetryRouter) MarkSuccess(candidate router.RouteCandidate) {
	r.successes = append(r.successes, candidate.ProviderName)
}

type streamRetryProvider struct {
	name        string
	events      []ir.Event
	streamErr   error
	streamCalls int
}

func (p *streamRetryProvider) Name() string { return p.name }

func (p *streamRetryProvider) Complete(context.Context, ir.Request) (*ir.Response, error) {
	return nil, nil
}

func (p *streamRetryProvider) Stream(context.Context, ir.Request) (<-chan ir.Event, error) {
	p.streamCalls++
	if p.streamErr != nil {
		return nil, p.streamErr
	}
	ch := make(chan ir.Event, len(p.events))
	for _, event := range p.events {
		ch <- event
	}
	close(ch)
	return ch, nil
}

func (p *streamRetryProvider) Models(context.Context) ([]ir.Model, error) { return nil, nil }
func (p *streamRetryProvider) Health(context.Context) error               { return nil }
