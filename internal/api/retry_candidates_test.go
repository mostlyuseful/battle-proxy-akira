package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"battle-proxy-akira/internal/ir"
	providerpkg "battle-proxy-akira/internal/provider"
	"battle-proxy-akira/internal/router"
)

func TestChatCompletionsNonStreamingRetriesRetryableCandidates(t *testing.T) {
	logger := &recordingLogger{}
	first := &candidateRetryProvider{name: "first", completeErr: &providerpkg.Error{Code: providerpkg.ErrorProviderRateLimited, Retryable: true, Provider: "first"}}
	second := &candidateRetryProvider{name: "second", responseText: "from second"}
	r := &candidateRetryRouter{candidates: []router.RouteCandidate{
		{ProviderName: "first", ProviderModel: "gpt-first", RequestedModel: "coding", Provider: first},
		{ProviderName: "second", ProviderModel: "gpt-second", RequestedModel: "coding", Provider: second},
	}}
	handler := NewServer(WithChatRouter(r), WithRequestLogger(logger))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"coding","messages":[{"role":"user","content":"hello"}]}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if first.completeCalls != 1 || second.completeCalls != 1 {
		t.Fatalf("complete calls first=%d second=%d, want 1/1", first.completeCalls, second.completeCalls)
	}
	if len(r.failures) != 1 || r.failures[0] != "first" {
		t.Fatalf("failures = %#v, want [first]", r.failures)
	}
	if len(r.successes) != 1 || r.successes[0] != "second" {
		t.Fatalf("successes = %#v, want [second]", r.successes)
	}
	var body struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Model != "coding" || body.Choices[0].Message.Content != "from second" {
		t.Fatalf("response body = %#v, want coding/from second", body)
	}
	if len(logger.records) != 1 || logger.records[0].RetryCount != 1 || logger.records[0].ResolvedProvider != "second" || logger.records[0].ResolvedModel != "gpt-second" {
		t.Fatalf("log records = %#v, want retry_count=1 and second provider", logger.records)
	}
}

func TestChatCompletionsNonStreamingStopsOnNonRetryableCandidate(t *testing.T) {
	logger := &recordingLogger{}
	first := &candidateRetryProvider{name: "first", completeErr: &providerpkg.Error{Code: providerpkg.ErrorPolicyDenied, Retryable: false, Provider: "first"}}
	second := &candidateRetryProvider{name: "second", responseText: "should not run"}
	r := &candidateRetryRouter{candidates: []router.RouteCandidate{
		{ProviderName: "first", ProviderModel: "gpt-first", RequestedModel: "coding", Provider: first},
		{ProviderName: "second", ProviderModel: "gpt-second", RequestedModel: "coding", Provider: second},
	}}
	handler := NewServer(WithChatRouter(r), WithRequestLogger(logger))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"coding","messages":[{"role":"user","content":"hello"}]}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status code = %d, want %d, body %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if first.completeCalls != 1 || second.completeCalls != 0 {
		t.Fatalf("complete calls first=%d second=%d, want 1/0", first.completeCalls, second.completeCalls)
	}
	if len(r.failures) != 1 || r.failures[0] != "first" || len(r.successes) != 0 {
		t.Fatalf("failures/successes = %#v/%#v, want [first]/[]", r.failures, r.successes)
	}
	if len(logger.records) != 1 || logger.records[0].RetryCount != 0 || logger.records[0].ResolvedProvider != "first" || logger.records[0].Status != http.StatusForbidden {
		t.Fatalf("log records = %#v, want retry_count=0 first forbidden", logger.records)
	}
}

func TestChatCompletionsNonStreamingAllRetryableCandidatesFail(t *testing.T) {
	logger := &recordingLogger{}
	first := &candidateRetryProvider{name: "first", completeErr: &providerpkg.Error{Code: providerpkg.ErrorProviderRateLimited, Retryable: true, Provider: "first"}}
	second := &candidateRetryProvider{name: "second", completeErr: &providerpkg.Error{Code: providerpkg.ErrorUpstream, Retryable: true, Provider: "second"}}
	r := &candidateRetryRouter{candidates: []router.RouteCandidate{
		{ProviderName: "first", ProviderModel: "gpt-first", RequestedModel: "coding", Provider: first},
		{ProviderName: "second", ProviderModel: "gpt-second", RequestedModel: "coding", Provider: second},
	}}
	handler := NewServer(WithChatRouter(r), WithRequestLogger(logger))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"coding","messages":[{"role":"user","content":"hello"}]}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status code = %d, want %d, body %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
	if first.completeCalls != 1 || second.completeCalls != 1 {
		t.Fatalf("complete calls first=%d second=%d, want 1/1", first.completeCalls, second.completeCalls)
	}
	if len(r.failures) != 2 || r.failures[0] != "first" || r.failures[1] != "second" || len(r.successes) != 0 {
		t.Fatalf("failures/successes = %#v/%#v, want [first second]/[]", r.failures, r.successes)
	}
	if len(logger.records) != 1 || logger.records[0].RetryCount != 1 || logger.records[0].ResolvedProvider != "second" || logger.records[0].Status != http.StatusBadGateway {
		t.Fatalf("log records = %#v, want retry_count=1 second bad gateway", logger.records)
	}
}

type candidateRetryRouter struct {
	candidates []router.RouteCandidate
	failures   []string
	successes  []string
}

func (r *candidateRetryRouter) Resolve(context.Context, ir.Request) ([]router.RouteCandidate, error) {
	return r.candidates, nil
}

func (r *candidateRetryRouter) MarkFailure(candidate router.RouteCandidate, err error) {
	r.failures = append(r.failures, candidate.ProviderName)
}

func (r *candidateRetryRouter) MarkSuccess(candidate router.RouteCandidate) {
	r.successes = append(r.successes, candidate.ProviderName)
}

type candidateRetryProvider struct {
	name          string
	responseText  string
	completeErr   error
	completeCalls int
}

func (p *candidateRetryProvider) Name() string { return p.name }

func (p *candidateRetryProvider) Complete(ctx context.Context, req ir.Request) (*ir.Response, error) {
	p.completeCalls++
	if p.completeErr != nil {
		return nil, p.completeErr
	}
	return &ir.Response{
		ID:    "chatcmpl-" + p.name,
		Model: req.Model,
		Message: ir.Message{
			Role:    ir.RoleAssistant,
			Content: []ir.ContentPart{{Type: ir.ContentTypeText, Text: p.responseText}},
		},
		FinishReason: "stop",
	}, nil
}

func (p *candidateRetryProvider) Stream(context.Context, ir.Request) (<-chan ir.Event, error) {
	return nil, nil
}

func (p *candidateRetryProvider) Models(context.Context) ([]ir.Model, error) { return nil, nil }
func (p *candidateRetryProvider) Health(context.Context) error               { return nil }
