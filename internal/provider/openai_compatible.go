package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"battle-proxy-akira/internal/auth"
	"battle-proxy-akira/internal/config"
	"battle-proxy-akira/internal/ir"
	openaiapi "battle-proxy-akira/internal/openai"
	"battle-proxy-akira/internal/sse"
)

// OpenAICompatibleProvider calls an OpenAI-compatible upstream API using raw HTTP.
type OpenAICompatibleProvider struct {
	name        string
	baseURL     *url.URL
	tokenSource auth.TokenSource
	httpClient  *http.Client
	models      map[string]config.ModelConfig
}

// NewOpenAICompatible constructs an OpenAI-compatible provider adapter.
func NewOpenAICompatible(name string, cfg config.ProviderConfig, tokenSource auth.TokenSource, httpClient *http.Client) (*OpenAICompatibleProvider, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("provider name is required")
	}
	if tokenSource == nil {
		return nil, fmt.Errorf("provider %q token source is required", name)
	}
	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return nil, fmt.Errorf("provider %q base_url must be an absolute URL", name)
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &OpenAICompatibleProvider{
		name:        name,
		baseURL:     parsed,
		tokenSource: tokenSource,
		httpClient:  httpClient,
		models:      cfg.Models,
	}, nil
}

// Name returns the configured provider name.
func (p *OpenAICompatibleProvider) Name() string {
	return p.name
}

// Complete sends a non-streaming Chat Completions request upstream and normalizes the response.
func (p *OpenAICompatibleProvider) Complete(ctx context.Context, req ir.Request) (*ir.Response, error) {
	httpResp, err := p.doChatCompletion(ctx, req, false)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read upstream chat completion response: %w", err)
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return nil, classifyHTTPStatus(p.name, httpResp.StatusCode, httpResp.Header, respBody)
	}

	var chatResp openaiapi.ChatCompletionResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("decode upstream chat completion response: %w", err)
	}
	irResp, err := chatResp.ToIR(respBody)
	if err != nil {
		return nil, fmt.Errorf("normalize upstream chat completion response: %w", err)
	}
	return &irResp, nil
}

// Stream sends a streaming Chat Completions request upstream and returns parsed SSE events.
func (p *OpenAICompatibleProvider) Stream(ctx context.Context, req ir.Request) (<-chan ir.Event, error) {
	httpResp, err := p.doChatCompletion(ctx, req, true)
	if err != nil {
		return nil, err
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		defer httpResp.Body.Close()
		respBody, _ := io.ReadAll(httpResp.Body)
		return nil, classifyHTTPStatus(p.name, httpResp.StatusCode, httpResp.Header, respBody)
	}

	events := make(chan ir.Event)
	go func() {
		defer close(events)
		defer httpResp.Body.Close()

		reader := sse.NewReader(httpResp.Body)
		for {
			event, err := reader.Read()
			if err == io.EOF || ctx.Err() != nil {
				return
			}
			if err != nil {
				return
			}

			irEvent := ir.Event{
				Type:  ir.EventTypeMessageDelta,
				Model: req.Model,
				Text:  event.Data,
				Raw:   json.RawMessage(event.Data),
			}
			if event.IsDone() {
				irEvent.Type = ir.EventTypeDone
				irEvent.Raw = nil
			}

			select {
			case events <- irEvent:
			case <-ctx.Done():
				return
			}
		}
	}()
	return events, nil
}

// Models returns models configured for this provider.
func (p *OpenAICompatibleProvider) Models(ctx context.Context) ([]ir.Model, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	models := make([]ir.Model, 0, len(p.models))
	for modelName, model := range p.models {
		models = append(models, ir.Model{
			ID:         modelName,
			Provider:   p.name,
			Name:       modelName,
			Modalities: append([]string(nil), model.Modalities...),
		})
	}
	return models, nil
}

// Health returns nil for now if the provider is configured and the context is active.
func (p *OpenAICompatibleProvider) Health(ctx context.Context) error {
	return ctx.Err()
}

func (p *OpenAICompatibleProvider) doChatCompletion(ctx context.Context, req ir.Request, stream bool) (*http.Response, error) {
	token, err := p.tokenSource.Token(ctx)
	if err != nil {
		return nil, &Error{Code: ErrorProviderAuthFailed, Retryable: false, Provider: p.name}
	}

	chatReq, err := openaiapi.ChatCompletionRequestFromIR(req)
	if err != nil {
		return nil, &Error{Code: ErrorInvalidRequest, Retryable: false, Provider: p.name}
	}
	chatReq.Stream = stream

	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("encode chat completion request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint("chat/completions"), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create upstream chat completion request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")
	if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	} else {
		httpReq.Header.Set("Accept", "application/json")
	}

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, classifyNetworkError(p.name, err)
	}
	return httpResp, nil
}

func (p *OpenAICompatibleProvider) endpoint(suffix string) string {
	u := *p.baseURL
	basePath := strings.TrimRight(u.Path, "/")
	u.Path = path.Join(basePath, suffix)
	return u.String()
}
