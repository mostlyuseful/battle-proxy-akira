// Package ir defines provider-neutral request and response types used inside the proxy.
package ir

import "encoding/json"

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

const (
	ContentTypeText       = "text"
	ContentTypeImageURL   = "image_url"
	ContentTypeInputImage = "input_image"
)

const (
	ModalityText  = "text"
	ModalityImage = "image"
)

const (
	EventTypeMessageDelta = "message_delta"
	EventTypeDone         = "done"
	EventTypeError        = "error"
)

// Request is the normalized provider-neutral request shape used after API parsing.
type Request struct {
	ID       string
	Model    string
	Messages []Message
	Params   SamplingParams
	Stream   bool
	Metadata map[string]string
	RawBody  json.RawMessage
	Extra    map[string]json.RawMessage
}

// Message is a normalized conversation message.
type Message struct {
	Role    string
	Content []ContentPart
}

// ContentPart is one provider-neutral text or image input part.
type ContentPart struct {
	Type     string
	Text     string
	ImageURL string
	Detail   string
}

// SamplingParams contains common sampling and output controls shared by providers.
type SamplingParams struct {
	Temperature         *float64
	TopP                *float64
	MaxTokens           *int
	MaxCompletionTokens *int
	Stop                []string
	PresencePenalty     *float64
	FrequencyPenalty    *float64
	Seed                *int
}

// Response is the normalized provider-neutral response shape for non-streaming completions.
type Response struct {
	ID           string
	Model        string
	Message      Message
	FinishReason string
	Usage        *Usage
	Metadata     map[string]string
	RawBody      json.RawMessage
}

// Usage contains token accounting when an upstream provider returns it.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// Event is the normalized provider-neutral streaming event shape.
type Event struct {
	Type         string
	Model        string
	Delta        Message
	Text         string
	FinishReason string
	Usage        *Usage
	Error        *Error
	Metadata     map[string]string
	Raw          json.RawMessage
}

// Error describes a provider-neutral stream or response error.
type Error struct {
	Message string
	Code    string
	Param   string
}

// Model describes a model known to the proxy or an upstream provider.
type Model struct {
	ID         string
	Provider   string
	Name       string
	Modalities []string
	Synthetic  bool
	Metadata   map[string]string
}

// HasImages reports whether the request contains any image content parts.
func (r Request) HasImages() bool {
	for _, message := range r.Messages {
		for _, part := range message.Content {
			if part.Type == ContentTypeImageURL || part.Type == ContentTypeInputImage || part.ImageURL != "" {
				return true
			}
		}
	}
	return false
}

// InputModalities returns the unique modalities required by the request.
func (r Request) InputModalities() []string {
	modalities := []string{ModalityText}
	if r.HasImages() {
		modalities = append(modalities, ModalityImage)
	}
	return modalities
}
