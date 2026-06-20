// Package openai contains OpenAI-compatible API shapes and edge translators.
package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"battle-proxy-akira/internal/ir"
)

const (
	ChatCompletionsObject = "chat.completion"
)

// ChatCompletionRequest is the supported OpenAI-compatible Chat Completions request shape.
type ChatCompletionRequest struct {
	Model               string                     `json:"model"`
	Messages            []ChatMessage              `json:"messages"`
	Temperature         *float64                   `json:"temperature,omitempty"`
	TopP                *float64                   `json:"top_p,omitempty"`
	MaxTokens           *int                       `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int                       `json:"max_completion_tokens,omitempty"`
	Stop                StopSequences              `json:"stop,omitempty"`
	PresencePenalty     *float64                   `json:"presence_penalty,omitempty"`
	FrequencyPenalty    *float64                   `json:"frequency_penalty,omitempty"`
	Seed                *int                       `json:"seed,omitempty"`
	Stream              bool                       `json:"stream,omitempty"`
	Extra               map[string]json.RawMessage `json:"-"`
	RawBody             json.RawMessage            `json:"-"`
}

// ChatMessage is the text-only subset of an OpenAI chat message supported by this task.
type ChatMessage struct {
	Role    string                     `json:"role"`
	Content ChatMessageContent         `json:"content"`
	Name    string                     `json:"name,omitempty"`
	Extra   map[string]json.RawMessage `json:"-"`
}

// ChatMessageContent currently supports simple string content. Multimodal arrays are added later.
type ChatMessageContent struct {
	Text string
}

// StopSequences accepts either a single stop string or an array of stop strings.
type StopSequences []string

// ChatCompletionResponse is the OpenAI-compatible non-streaming Chat Completions response shape.
type ChatCompletionResponse struct {
	ID      string                     `json:"id"`
	Object  string                     `json:"object"`
	Created int64                      `json:"created"`
	Model   string                     `json:"model"`
	Choices []ChatCompletionChoice     `json:"choices"`
	Usage   *ChatCompletionUsage       `json:"usage,omitempty"`
	Extra   map[string]json.RawMessage `json:"-"`
}

// ChatCompletionChoice is one assistant response choice.
type ChatCompletionChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason,omitempty"`
}

// ChatCompletionUsage mirrors OpenAI token accounting fields.
type ChatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatCompletionRequestFromIR converts a provider-neutral request to an OpenAI-compatible request.
func ChatCompletionRequestFromIR(req ir.Request) (ChatCompletionRequest, error) {
	messages := make([]ChatMessage, 0, len(req.Messages))
	for i, message := range req.Messages {
		chatMessage := ChatMessage{Role: message.Role}
		for _, part := range message.Content {
			if part.Type != ir.ContentTypeText {
				return ChatCompletionRequest{}, fmt.Errorf("chat message %d contains unsupported content part type %q", i, part.Type)
			}
			chatMessage.Content.Text += part.Text
		}
		messages = append(messages, chatMessage)
	}

	return ChatCompletionRequest{
		Model:               req.Model,
		Messages:            messages,
		Temperature:         req.Params.Temperature,
		TopP:                req.Params.TopP,
		MaxTokens:           req.Params.MaxTokens,
		MaxCompletionTokens: req.Params.MaxCompletionTokens,
		Stop:                StopSequences(req.Params.Stop),
		PresencePenalty:     req.Params.PresencePenalty,
		FrequencyPenalty:    req.Params.FrequencyPenalty,
		Seed:                req.Params.Seed,
		Stream:              req.Stream,
		Extra:               cloneRawMap(req.Extra),
		RawBody:             append(json.RawMessage(nil), req.RawBody...),
	}, nil
}

// ParseChatCompletionRequest decodes a request body, preserving raw JSON and unknown top-level fields.
func ParseChatCompletionRequest(body []byte) (*ChatCompletionRequest, error) {
	var req ChatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("decode chat completion request: %w", err)
	}
	req.RawBody = append(req.RawBody[:0], body...)

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, fmt.Errorf("decode chat completion request fields: %w", err)
	}
	for _, key := range []string{
		"model",
		"messages",
		"temperature",
		"top_p",
		"max_tokens",
		"max_completion_tokens",
		"stop",
		"presence_penalty",
		"frequency_penalty",
		"seed",
		"stream",
	} {
		delete(fields, key)
	}
	req.Extra = fields
	return &req, nil
}

// MarshalJSON encodes known request fields plus preserved unknown top-level fields.
func (r ChatCompletionRequest) MarshalJSON() ([]byte, error) {
	fields := cloneRawMap(r.Extra)
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}
	put := func(key string, value any) error {
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		fields[key] = encoded
		return nil
	}

	if err := put("model", r.Model); err != nil {
		return nil, err
	}
	if err := put("messages", r.Messages); err != nil {
		return nil, err
	}
	if r.Temperature != nil {
		if err := put("temperature", r.Temperature); err != nil {
			return nil, err
		}
	}
	if r.TopP != nil {
		if err := put("top_p", r.TopP); err != nil {
			return nil, err
		}
	}
	if r.MaxTokens != nil {
		if err := put("max_tokens", r.MaxTokens); err != nil {
			return nil, err
		}
	}
	if r.MaxCompletionTokens != nil {
		if err := put("max_completion_tokens", r.MaxCompletionTokens); err != nil {
			return nil, err
		}
	}
	if len(r.Stop) > 0 {
		if err := put("stop", r.Stop); err != nil {
			return nil, err
		}
	}
	if r.PresencePenalty != nil {
		if err := put("presence_penalty", r.PresencePenalty); err != nil {
			return nil, err
		}
	}
	if r.FrequencyPenalty != nil {
		if err := put("frequency_penalty", r.FrequencyPenalty); err != nil {
			return nil, err
		}
	}
	if r.Seed != nil {
		if err := put("seed", r.Seed); err != nil {
			return nil, err
		}
	}
	if r.Stream {
		if err := put("stream", r.Stream); err != nil {
			return nil, err
		}
	}
	return json.Marshal(fields)
}

// ToIR normalizes a Chat Completions request to the provider-neutral IR.
func (r ChatCompletionRequest) ToIR() (ir.Request, error) {
	if r.Model == "" {
		return ir.Request{}, errors.New("chat completion request model is required")
	}
	if len(r.Messages) == 0 {
		return ir.Request{}, errors.New("chat completion request messages must not be empty")
	}

	messages := make([]ir.Message, 0, len(r.Messages))
	for i, message := range r.Messages {
		if message.Role == "" {
			return ir.Request{}, fmt.Errorf("chat completion message %d role is required", i)
		}
		messages = append(messages, ir.Message{
			Role: message.Role,
			Content: []ir.ContentPart{
				{
					Type: ir.ContentTypeText,
					Text: message.Content.Text,
				},
			},
		})
	}

	return ir.Request{
		Model:    r.Model,
		Messages: messages,
		Params: ir.SamplingParams{
			Temperature:         r.Temperature,
			TopP:                r.TopP,
			MaxTokens:           r.MaxTokens,
			MaxCompletionTokens: r.MaxCompletionTokens,
			Stop:                []string(r.Stop),
			PresencePenalty:     r.PresencePenalty,
			FrequencyPenalty:    r.FrequencyPenalty,
			Seed:                r.Seed,
		},
		Stream:  r.Stream,
		RawBody: append(json.RawMessage(nil), r.RawBody...),
		Extra:   cloneRawMap(r.Extra),
	}, nil
}

// ToIR converts an OpenAI-compatible Chat Completions response to provider-neutral IR.
func (r ChatCompletionResponse) ToIR(raw json.RawMessage) (ir.Response, error) {
	if len(r.Choices) == 0 {
		return ir.Response{}, errors.New("chat completion response contains no choices")
	}
	choice := r.Choices[0]
	resp := ir.Response{
		ID:           r.ID,
		Model:        r.Model,
		Message:      chatMessageToIR(choice.Message),
		FinishReason: choice.FinishReason,
		RawBody:      append(json.RawMessage(nil), raw...),
	}
	if r.Usage != nil {
		resp.Usage = &ir.Usage{
			PromptTokens:     r.Usage.PromptTokens,
			CompletionTokens: r.Usage.CompletionTokens,
			TotalTokens:      r.Usage.TotalTokens,
		}
	}
	return resp, nil
}

// ChatCompletionResponseFromIR converts a normalized response to an OpenAI-compatible response.
func ChatCompletionResponseFromIR(resp ir.Response, created time.Time) ChatCompletionResponse {
	out := ChatCompletionResponse{
		ID:      resp.ID,
		Object:  ChatCompletionsObject,
		Created: created.Unix(),
		Model:   resp.Model,
		Choices: []ChatCompletionChoice{
			{
				Index:        0,
				Message:      ChatMessageFromIR(resp.Message),
				FinishReason: resp.FinishReason,
			},
		},
	}
	if resp.Usage != nil {
		out.Usage = &ChatCompletionUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}
	return out
}

func chatMessageToIR(message ChatMessage) ir.Message {
	return ir.Message{
		Role: message.Role,
		Content: []ir.ContentPart{
			{
				Type: ir.ContentTypeText,
				Text: message.Content.Text,
			},
		},
	}
}

// ChatMessageFromIR converts a text IR message to an OpenAI-compatible chat message.
func ChatMessageFromIR(message ir.Message) ChatMessage {
	var text bytes.Buffer
	for _, part := range message.Content {
		if part.Type == ir.ContentTypeText {
			text.WriteString(part.Text)
		}
	}
	return ChatMessage{
		Role:    message.Role,
		Content: ChatMessageContent{Text: text.String()},
	}
}

// UnmarshalJSON decodes only simple string message content for this MVP task.
func (c *ChatMessageContent) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return errors.New("chat message content must be a string; multimodal content parts are not supported yet")
	}
	c.Text = text
	return nil
}

// MarshalJSON emits simple string message content.
func (c ChatMessageContent) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.Text)
}

// UnmarshalJSON preserves unknown message fields while decoding known message fields.
func (m *ChatMessage) UnmarshalJSON(data []byte) error {
	type alias ChatMessage
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*m = ChatMessage(decoded)

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range []string{"role", "content", "name"} {
		delete(fields, key)
	}
	m.Extra = fields
	return nil
}

// UnmarshalJSON decodes stop as either a string, an array of strings, or null.
func (s *StopSequences) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		*s = nil
		return nil
	}
	var one string
	if err := json.Unmarshal(data, &one); err == nil {
		*s = []string{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return errors.New("stop must be a string or an array of strings")
	}
	*s = many
	return nil
}

// MarshalJSON emits null, a single string, or an array to preserve common OpenAI request forms.
func (s StopSequences) MarshalJSON() ([]byte, error) {
	switch len(s) {
	case 0:
		return []byte("null"), nil
	case 1:
		return json.Marshal(s[0])
	default:
		return json.Marshal([]string(s))
	}
}

func cloneRawMap(in map[string]json.RawMessage) map[string]json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]json.RawMessage, len(in))
	for key, value := range in {
		out[key] = append(json.RawMessage(nil), value...)
	}
	return out
}
