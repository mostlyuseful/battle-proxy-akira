package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"battle-proxy-akira/internal/ir"
	"battle-proxy-akira/internal/sse"
)

// Responses streaming event type names. Only the text-output lifecycle is
// modeled for MVP; tool/refusal/file/reasoning event types are intentionally
// unsupported and not emitted.
const (
	EventResponseCreated          = "response.created"
	EventResponseInProgress       = "response.in_progress"
	EventResponseOutputItemAdded  = "response.output_item.added"
	EventResponseContentPartAdded = "response.content_part.added"
	EventResponseOutputTextDelta  = "response.output_text.delta"
	EventResponseOutputTextDone   = "response.output_text.done"
	EventResponseContentPartDone  = "response.content_part.done"
	EventResponseOutputItemDone   = "response.output_item.done"
	EventResponseCompleted        = "response.completed"
	EventResponseFailed           = "response.failed"
	EventResponseError            = "error"
)

// ChatCompletionChunk is the OpenAI-compatible streaming chunk shape emitted by
// upstream providers. It is used to extract text deltas and finish reasons when
// translating a Chat Completions stream into Responses SSE events.
type ChatCompletionChunk struct {
	ID      string                      `json:"id,omitempty"`
	Object  string                      `json:"object,omitempty"`
	Created int64                       `json:"created,omitempty"`
	Model   string                      `json:"model,omitempty"`
	Choices []ChatCompletionChunkChoice `json:"choices,omitempty"`
	Usage   *ChatCompletionUsage        `json:"usage,omitempty"`
}

// ChatCompletionChunkChoice is one choice in a streaming chunk.
type ChatCompletionChunkChoice struct {
	Index        int                      `json:"index"`
	Delta        ChatCompletionChunkDelta `json:"delta"`
	FinishReason *string                  `json:"finish_reason"`
}

// ChatCompletionChunkDelta is the incremental delta within a streaming chunk.
type ChatCompletionChunkDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// ParseChatCompletionChunk decodes an upstream Chat Completions streaming chunk.
func ParseChatCompletionChunk(data []byte) (ChatCompletionChunk, error) {
	var chunk ChatCompletionChunk
	if err := json.Unmarshal(data, &chunk); err != nil {
		return ChatCompletionChunk{}, err
	}
	return chunk, nil
}

// responsesContentPart is the output_text content part shape used in streaming events.
type responsesContentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// responsesStreamItem is the message item shape used in streaming output_item events.
type responsesStreamItem struct {
	ID      string                 `json:"id"`
	Type    string                 `json:"type"`
	Role    string                 `json:"role"`
	Status  string                 `json:"status"`
	Content []responsesContentPart `json:"content"`
}

type responseCreatedEvent struct {
	Type     string   `json:"type"`
	Response Response `json:"response"`
}

type responseOutputItemAddedEvent struct {
	Type        string              `json:"type"`
	OutputIndex int                 `json:"output_index"`
	Item        responsesStreamItem `json:"item"`
}

type responseContentPartAddedEvent struct {
	Type         string               `json:"type"`
	ItemID       string               `json:"item_id"`
	OutputIndex  int                  `json:"output_index"`
	ContentIndex int                  `json:"content_index"`
	Part         responsesContentPart `json:"part"`
}

type responseOutputTextDeltaEvent struct {
	Type         string `json:"type"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"`
}

type responseOutputTextDoneEvent struct {
	Type         string `json:"type"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Text         string `json:"text"`
}

type responseContentPartDoneEvent struct {
	Type         string               `json:"type"`
	ItemID       string               `json:"item_id"`
	OutputIndex  int                  `json:"output_index"`
	ContentIndex int                  `json:"content_index"`
	Part         responsesContentPart `json:"part"`
}

type responseOutputItemDoneEvent struct {
	Type        string              `json:"type"`
	OutputIndex int                 `json:"output_index"`
	Item        responsesStreamItem `json:"item"`
}

type responseCompletedEvent struct {
	Type     string   `json:"type"`
	Response Response `json:"response"`
}

type responseFailedEvent struct {
	Type     string   `json:"type"`
	Response Response `json:"response"`
}

type responseErrorEvent struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ResponsesStreamTranslator translates provider-neutral ir.Events (which carry
// raw Chat Completions streaming chunks) into OpenAI Responses API SSE events.
// It models the text-output lifecycle only.
type ResponsesStreamTranslator struct {
	responseID    string
	itemID        string
	model         string
	createdAtUnix int64
	fullText      strings.Builder
	finishReason  string
	usage         *ResponseUsage
	opened        bool
}

// NewResponsesStreamTranslator creates a translator for one streaming response.
// responseID and itemID are used as stable identifiers in the emitted events.
// model is the client-requested model name returned in response metadata.
func NewResponsesStreamTranslator(responseID, itemID, model string, createdAtUnix int64) *ResponsesStreamTranslator {
	return &ResponsesStreamTranslator{
		responseID:    responseID,
		itemID:        itemID,
		model:         model,
		createdAtUnix: createdAtUnix,
	}
}

// WriteOpening emits the response.created, output_item.added, and
// content_part.added lifecycle events. It is idempotent.
func (t *ResponsesStreamTranslator) WriteOpening(w io.Writer) error {
	if t.opened {
		return nil
	}
	t.opened = true

	created := Response{
		ID:        t.responseID,
		Object:    ResponseObject,
		CreatedAt: t.createdAtUnix,
		Model:     t.model,
		Status:    ResponseStatusInProgress,
		Output:    []ResponseOutputItem{},
	}
	if err := writeTypedEvent(w, EventResponseCreated, responseCreatedEvent{Type: EventResponseCreated, Response: created}); err != nil {
		return err
	}

	item := responsesStreamItem{
		ID:      t.itemID,
		Type:    ResponseItemTypeMessage,
		Role:    ir.RoleAssistant,
		Status:  ResponseStatusInProgress,
		Content: []responsesContentPart{},
	}
	if err := writeTypedEvent(w, EventResponseOutputItemAdded, responseOutputItemAddedEvent{Type: EventResponseOutputItemAdded, OutputIndex: 0, Item: item}); err != nil {
		return err
	}

	part := responsesContentPart{Type: ResponseContentTypeOutputText, Text: ""}
	return writeTypedEvent(w, EventResponseContentPartAdded, responseContentPartAddedEvent{
		Type:         EventResponseContentPartAdded,
		ItemID:       t.itemID,
		OutputIndex:  0,
		ContentIndex: 0,
		Part:         part,
	})
}

// Translate consumes one provider event and emits any corresponding
// output_text.delta event. It returns an error only if writing fails.
func (t *ResponsesStreamTranslator) Translate(w io.Writer, event ir.Event) error {
	if event.Type == ir.EventTypeDone {
		return nil
	}
	if event.Type == ir.EventTypeError || event.Error != nil {
		return nil
	}

	// Provider events carry raw Chat Completions chunk JSON in Text/Raw.
	data := event.Raw
	if len(data) == 0 {
		data = json.RawMessage(event.Text)
	}
	if len(data) == 0 {
		return nil
	}

	chunk, err := ParseChatCompletionChunk(data)
	if err != nil {
		// Unparseable chunks are skipped rather than aborting the stream.
		return nil
	}
	if chunk.Usage != nil {
		t.usage = &ResponseUsage{
			InputTokens:  chunk.Usage.PromptTokens,
			OutputTokens: chunk.Usage.CompletionTokens,
			TotalTokens:  chunk.Usage.TotalTokens,
		}
	}
	if len(chunk.Choices) > 0 {
		choice := chunk.Choices[0]
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			t.finishReason = *choice.FinishReason
		}
		if choice.Delta.Content != "" {
			t.fullText.WriteString(choice.Delta.Content)
			if err := writeTypedEvent(w, EventResponseOutputTextDelta, responseOutputTextDeltaEvent{
				Type:         EventResponseOutputTextDelta,
				ItemID:       t.itemID,
				OutputIndex:  0,
				ContentIndex: 0,
				Delta:        choice.Delta.Content,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// FullText returns the accumulated output text so far.
func (t *ResponsesStreamTranslator) FullText() string {
	return t.fullText.String()
}

// WriteClosing emits the output_text.done, content_part.done, output_item.done,
// and response.completed lifecycle events.
func (t *ResponsesStreamTranslator) WriteClosing(w io.Writer) error {
	text := t.fullText.String()

	if err := writeTypedEvent(w, EventResponseOutputTextDone, responseOutputTextDoneEvent{
		Type:         EventResponseOutputTextDone,
		ItemID:       t.itemID,
		OutputIndex:  0,
		ContentIndex: 0,
		Text:         text,
	}); err != nil {
		return err
	}

	part := responsesContentPart{Type: ResponseContentTypeOutputText, Text: text}
	if err := writeTypedEvent(w, EventResponseContentPartDone, responseContentPartDoneEvent{
		Type:         EventResponseContentPartDone,
		ItemID:       t.itemID,
		OutputIndex:  0,
		ContentIndex: 0,
		Part:         part,
	}); err != nil {
		return err
	}

	item := responsesStreamItem{
		ID:      t.itemID,
		Type:    ResponseItemTypeMessage,
		Role:    ir.RoleAssistant,
		Status:  responseStatusForFinishReason(t.finishReason),
		Content: []responsesContentPart{part},
	}
	if err := writeTypedEvent(w, EventResponseOutputItemDone, responseOutputItemDoneEvent{
		Type:        EventResponseOutputItemDone,
		OutputIndex: 0,
		Item:        item,
	}); err != nil {
		return err
	}

	completed := Response{
		ID:        t.responseID,
		Object:    ResponseObject,
		CreatedAt: t.createdAtUnix,
		Model:     t.model,
		Status:    responseStatusForFinishReason(t.finishReason),
		Output: []ResponseOutputItem{{
			ID:      t.itemID,
			Type:    ResponseItemTypeMessage,
			Role:    ir.RoleAssistant,
			Status:  responseStatusForFinishReason(t.finishReason),
			Content: []ResponseOutputContent{{Type: ResponseContentTypeOutputText, Text: text}},
		}},
		Usage: t.usage,
	}
	return writeTypedEvent(w, EventResponseCompleted, responseCompletedEvent{Type: EventResponseCompleted, Response: completed})
}

// WriteError emits an error SSE event for a mid-stream failure.
func (t *ResponsesStreamTranslator) WriteError(w io.Writer, code, message string) error {
	return writeTypedEvent(w, EventResponseError, responseErrorEvent{
		Type:    EventResponseError,
		Code:    code,
		Message: message,
	})
}

// writeTypedEvent marshals payload and writes a named SSE event.
func writeTypedEvent(w io.Writer, eventType string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode %s event: %w", eventType, err)
	}
	return sse.WriteTypedEvent(w, eventType, string(encoded))
}
