package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
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

const (
	codexOriginator         = "battle_proxy_akira"
	codexDefaultInstruction = "You are a helpful assistant."
)

// CodexResponsesProvider calls the ChatGPT-backed Codex Responses endpoint.
// The upstream only supports streaming, so Complete buffers its SSE response.
type CodexResponsesProvider struct {
	name        string
	baseURL     *url.URL
	tokenSource auth.TokenSource
	httpClient  *http.Client
	models      map[string]config.ModelConfig
	logger      *slog.Logger
}

// NewCodexResponses constructs a ChatGPT-backed Codex provider adapter.
func NewCodexResponses(name string, cfg config.ProviderConfig, tokenSource auth.TokenSource, httpClient *http.Client) (*CodexResponsesProvider, error) {
	return NewCodexResponsesWithLogger(name, cfg, tokenSource, httpClient, nil)
}

// NewCodexResponsesWithLogger constructs a Codex provider with optional diagnostics.
func NewCodexResponsesWithLogger(name string, cfg config.ProviderConfig, tokenSource auth.TokenSource, httpClient *http.Client, logger *slog.Logger) (*CodexResponsesProvider, error) {
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
	if logger != nil {
		logger.Info("codex responses provider initialized", "provider", name, "base_url", parsed.String(), "models", len(cfg.Models))
	}
	return &CodexResponsesProvider{
		name: name, baseURL: parsed, tokenSource: tokenSource,
		httpClient: httpClient, models: cfg.Models, logger: logger,
	}, nil
}

func (p *CodexResponsesProvider) Name() string { return p.name }

// Complete buffers the mandatory upstream SSE stream into one response.
func (p *CodexResponsesProvider) Complete(ctx context.Context, req ir.Request) (*ir.Response, error) {
	httpResp, err := p.doResponses(ctx, req)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	state := codexStreamState{model: req.Model}
	reader := sse.NewReader(httpResp.Body)
	for {
		event, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, classifyNetworkError(p.name, readErr)
		}
		if err := state.consume([]byte(event.Data)); err != nil {
			return nil, &Error{Code: ErrorUpstream, Retryable: false, Provider: p.name}
		}
	}
	if state.final != nil {
		return state.final, nil
	}
	return nil, &Error{Code: ErrorUpstream, Retryable: true, Provider: p.name}
}

// Stream translates Responses events into Chat Completion chunks, which is the
// streaming contract currently consumed by both public API handlers.
func (p *CodexResponsesProvider) Stream(ctx context.Context, req ir.Request) (<-chan ir.Event, error) {
	httpResp, err := p.doResponses(ctx, req)
	if err != nil {
		return nil, err
	}

	events := make(chan ir.Event)
	go func() {
		defer close(events)
		defer httpResp.Body.Close()

		state := codexStreamState{model: req.Model}
		reader := sse.NewReader(httpResp.Body)
		for {
			event, readErr := reader.Read()
			if readErr == io.EOF {
				if state.final == nil {
					p.sendEvent(ctx, events, ir.Event{Type: ir.EventTypeError, Model: req.Model, Error: &ir.Error{Code: ErrorUpstream}})
				}
				return
			}
			if readErr != nil {
				if ctx.Err() == nil {
					p.sendEvent(ctx, events, ir.Event{Type: ir.EventTypeError, Model: req.Model, Error: &ir.Error{Code: ErrorUpstream}})
				}
				return
			}

			before := state.text.Len()
			if err := state.consume([]byte(event.Data)); err != nil {
				p.sendEvent(ctx, events, ir.Event{Type: ir.EventTypeError, Model: req.Model, Error: &ir.Error{Code: ErrorUpstream}})
				return
			}
			if state.text.Len() > before {
				delta := state.text.String()[before:]
				if !p.sendEvent(ctx, events, chatDeltaEvent(state.id, req.Model, delta)) {
					return
				}
			}
			if state.final != nil {
				if !p.sendEvent(ctx, events, chatTerminalEvent(state.id, req.Model, state.final.FinishReason, state.final.Usage)) {
					return
				}
				p.sendEvent(ctx, events, ir.Event{Type: ir.EventTypeDone, Model: req.Model, Text: sse.DoneData})
				return
			}
		}
	}()
	return events, nil
}

func (p *CodexResponsesProvider) Models(ctx context.Context) ([]ir.Model, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return configuredModels(p.name, p.models), nil
}

func (p *CodexResponsesProvider) Health(ctx context.Context) error { return ctx.Err() }

func (p *CodexResponsesProvider) doResponses(ctx context.Context, req ir.Request) (*http.Response, error) {
	token, err := p.tokenSource.Token(ctx)
	if err != nil {
		return nil, &Error{Code: ErrorProviderAuthFailed, Provider: p.name}
	}
	accountID, err := codexAccountID(token)
	if err != nil {
		return nil, &Error{Code: ErrorProviderAuthFailed, Provider: p.name}
	}
	payload, err := codexRequestFromIR(req)
	if err != nil {
		return nil, &Error{Code: ErrorInvalidRequest, Provider: p.name}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode codex responses request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint("responses"), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create codex responses request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("ChatGPT-Account-ID", accountID)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("originator", codexOriginator)

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, classifyNetworkError(p.name, err)
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		defer httpResp.Body.Close()
		responseBody, _ := io.ReadAll(httpResp.Body)
		return nil, classifyHTTPStatus(p.name, httpResp.StatusCode, httpResp.Header, responseBody)
	}
	return httpResp, nil
}

func (p *CodexResponsesProvider) endpoint(suffix string) string {
	u := *p.baseURL
	u.Path = path.Join(strings.TrimRight(u.Path, "/"), suffix)
	return u.String()
}

func (p *CodexResponsesProvider) sendEvent(ctx context.Context, out chan<- ir.Event, event ir.Event) bool {
	select {
	case out <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

type codexRequest struct {
	Model        string              `json:"model"`
	Instructions string              `json:"instructions"`
	Input        []codexInputMessage `json:"input"`
	Store        bool                `json:"store"`
	Stream       bool                `json:"stream"`
	Temperature  *float64            `json:"temperature,omitempty"`
	TopP         *float64            `json:"top_p,omitempty"`
}

type codexInputMessage struct {
	Type    string             `json:"type"`
	Role    string             `json:"role"`
	Content []codexContentPart `json:"content"`
}

type codexContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

func codexRequestFromIR(req ir.Request) (codexRequest, error) {
	if strings.TrimSpace(req.Model) == "" || len(req.Messages) == 0 {
		return codexRequest{}, errors.New("model and messages are required")
	}
	var instructions []string
	input := make([]codexInputMessage, 0, len(req.Messages))
	for i, message := range req.Messages {
		if message.Role == ir.RoleSystem || message.Role == openaiapi.ResponseRoleDeveloper {
			var text strings.Builder
			for _, part := range message.Content {
				if part.Type != ir.ContentTypeText {
					return codexRequest{}, fmt.Errorf("instruction message %d contains unsupported content type %q", i, part.Type)
				}
				text.WriteString(part.Text)
			}
			if text.Len() > 0 {
				instructions = append(instructions, text.String())
			}
			continue
		}
		if message.Role != ir.RoleUser && message.Role != ir.RoleAssistant {
			return codexRequest{}, fmt.Errorf("message %d has unsupported role %q", i, message.Role)
		}
		parts := make([]codexContentPart, 0, len(message.Content))
		for _, part := range message.Content {
			switch part.Type {
			case ir.ContentTypeText:
				parts = append(parts, codexContentPart{Type: openaiapi.ResponseInputContentTypeText, Text: part.Text})
			case ir.ContentTypeImageURL, ir.ContentTypeInputImage:
				if part.ImageURL == "" {
					return codexRequest{}, fmt.Errorf("message %d image URL is required", i)
				}
				parts = append(parts, codexContentPart{Type: openaiapi.ResponseInputContentTypeImage, ImageURL: part.ImageURL, Detail: part.Detail})
			default:
				return codexRequest{}, fmt.Errorf("message %d contains unsupported content type %q", i, part.Type)
			}
		}
		input = append(input, codexInputMessage{Type: openaiapi.ResponseInputItemTypeMessage, Role: message.Role, Content: parts})
	}
	if len(input) == 0 {
		return codexRequest{}, errors.New("at least one non-instruction message is required")
	}
	instructionText := strings.Join(instructions, "\n\n")
	if instructionText == "" {
		instructionText = codexDefaultInstruction
	}
	return codexRequest{
		Model: req.Model, Instructions: instructionText, Input: input,
		Store: false, Stream: true, Temperature: req.Params.Temperature, TopP: req.Params.TopP,
	}, nil
}

func codexAccountID(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("access token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", errors.New("access token JWT payload is malformed")
	}
	var claims struct {
		OpenAIAuth struct {
			ChatGPTAccountID string `json:"chatgpt_account_id"`
		} `json:"https://api.openai.com/auth"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", errors.New("access token JWT payload is malformed")
	}
	accountID := strings.TrimSpace(claims.OpenAIAuth.ChatGPTAccountID)
	if accountID == "" {
		return "", errors.New("access token is missing ChatGPT account ID")
	}
	return accountID, nil
}

type codexStreamState struct {
	id    string
	model string
	text  strings.Builder
	final *ir.Response
}

func (s *codexStreamState) consume(data []byte) error {
	if len(data) == 0 || string(data) == sse.DoneData {
		return nil
	}
	var envelope struct {
		Type     string             `json:"type"`
		Delta    string             `json:"delta"`
		Text     string             `json:"text"`
		Response openaiapi.Response `json:"response"`
		Code     string             `json:"code"`
		Message  string             `json:"message"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("decode codex responses event: %w", err)
	}
	switch envelope.Type {
	case openaiapi.EventResponseCreated, openaiapi.EventResponseInProgress:
		if envelope.Response.ID != "" {
			s.id = envelope.Response.ID
		}
		if envelope.Response.Model != "" {
			s.model = envelope.Response.Model
		}
	case openaiapi.EventResponseOutputTextDelta:
		s.text.WriteString(envelope.Delta)
	case openaiapi.EventResponseOutputTextDone:
		if s.text.Len() == 0 {
			s.text.WriteString(envelope.Text)
		}
	case openaiapi.EventResponseCompleted:
		rawResponse, err := json.Marshal(envelope.Response)
		if err != nil {
			return err
		}
		final := ir.Response{
			ID:      envelope.Response.ID,
			Model:   envelope.Response.Model,
			Message: ir.Message{Role: ir.RoleAssistant, Content: []ir.ContentPart{{Type: ir.ContentTypeText, Text: s.text.String()}}},
			RawBody: rawResponse,
		}
		if envelope.Response.Status == openaiapi.ResponseStatusIncomplete {
			final.FinishReason = "length"
		} else {
			final.FinishReason = "stop"
		}
		if envelope.Response.Usage != nil {
			final.Usage = &ir.Usage{
				PromptTokens:     envelope.Response.Usage.InputTokens,
				CompletionTokens: envelope.Response.Usage.OutputTokens,
				TotalTokens:      envelope.Response.Usage.TotalTokens,
			}
		}
		s.id = final.ID
		s.model = final.Model
		s.final = &final
	case openaiapi.EventResponseFailed, openaiapi.EventResponseError:
		return errors.New("codex response failed")
	}
	return nil
}

func chatDeltaEvent(id, model, delta string) ir.Event {
	chunk := openaiapi.ChatCompletionChunk{
		ID: id, Object: "chat.completion.chunk", Model: model,
		Choices: []openaiapi.ChatCompletionChunkChoice{{Index: 0, Delta: openaiapi.ChatCompletionChunkDelta{Role: ir.RoleAssistant, Content: delta}}},
	}
	encoded, _ := json.Marshal(chunk)
	return ir.Event{Type: ir.EventTypeMessageDelta, Model: model, Text: string(encoded), Raw: encoded}
}

func chatTerminalEvent(id, model, finishReason string, usage *ir.Usage) ir.Event {
	if finishReason == "" {
		finishReason = "stop"
	}
	chunk := openaiapi.ChatCompletionChunk{
		ID: id, Object: "chat.completion.chunk", Model: model,
		Choices: []openaiapi.ChatCompletionChunkChoice{{Index: 0, FinishReason: &finishReason}},
	}
	if usage != nil {
		chunk.Usage = &openaiapi.ChatCompletionUsage{
			PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens, TotalTokens: usage.TotalTokens,
		}
	}
	encoded, _ := json.Marshal(chunk)
	return ir.Event{Type: ir.EventTypeMessageDelta, Model: model, FinishReason: finishReason, Usage: usage, Text: string(encoded), Raw: encoded}
}
