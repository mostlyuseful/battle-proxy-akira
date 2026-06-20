package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"battle-proxy-akira/internal/ir"
)

const (
	// ResponseObject is the OpenAI Responses API top-level object type.
	ResponseObject = "response"

	// ResponseItemTypeMessage is the output item type for an assistant message.
	ResponseItemTypeMessage = "message"
	// ResponseInputItemTypeMessage is the input item type for an EasyInputMessage.
	ResponseInputItemTypeMessage = "message"

	// ResponseContentTypeOutputText is the output content part type for text.
	ResponseContentTypeOutputText = "output_text"
	// ResponseContentTypeRefusal is the output content part type for a refusal.
	ResponseContentTypeRefusal = "refusal"

	// ResponseInputContentTypeText is the input content part type for text.
	ResponseInputContentTypeText = "input_text"
	// ResponseInputContentTypeImage is the input content part type for an image.
	ResponseInputContentTypeImage = "input_image"

	// ResponseRoleDeveloper is the role used for instructions injected into a Responses request.
	ResponseRoleDeveloper = "developer"
)

// ResponseRequest is the supported OpenAI Responses API request shape for MVP
// text and image inputs plus common sampling parameters.
type ResponseRequest struct {
	Model           string                     `json:"model"`
	Input           ResponseInput              `json:"input"`
	Instructions    string                     `json:"instructions,omitempty"`
	Stream          bool                       `json:"stream,omitempty"`
	Temperature     *float64                   `json:"temperature,omitempty"`
	TopP            *float64                   `json:"top_p,omitempty"`
	MaxOutputTokens *int                       `json:"max_output_tokens,omitempty"`
	Extra           map[string]json.RawMessage `json:"-"`
	RawBody         json.RawMessage            `json:"-"`
}

// ResponseInput accepts either a plain text string (a single user message) or
// an array of input items (EasyInputMessage items for MVP).
type ResponseInput struct {
	Text  string
	Items []ResponseInputItem
}

// ResponseInputItem is one Responses API input item. MVP supports EasyInputMessage
// items (type "message" with a role and string or multimodal content). Unknown
// item types are preserved verbatim in Raw for forwarding.
type ResponseInputItem struct {
	Type    string                     `json:"type,omitempty"`
	Role    string                     `json:"role,omitempty"`
	Content ResponseInputContent       `json:"content"`
	Raw     json.RawMessage            `json:"-"`
	Extra   map[string]json.RawMessage `json:"-"`
}

// ResponseInputContent accepts either a simple string or an array of input
// content parts (input_text / input_image).
type ResponseInputContent struct {
	Text  string
	Parts []ResponseInputContentPart
}

// ResponseInputContentPart is one Responses API input content part.
type ResponseInputContentPart struct {
	Type     string                     `json:"type"`
	Text     string                     `json:"text,omitempty"`
	ImageURL string                     `json:"image_url,omitempty"`
	FileID   string                     `json:"file_id,omitempty"`
	Detail   string                     `json:"detail,omitempty"`
	Extra    map[string]json.RawMessage `json:"-"`
}

// Response is the OpenAI Responses API non-streaming response object.
type Response struct {
	ID         string                     `json:"id"`
	Object     string                     `json:"object"`
	CreatedAt  int64                      `json:"created_at"`
	Model      string                     `json:"model"`
	Status     string                     `json:"status,omitempty"`
	Output     []ResponseOutputItem       `json:"output"`
	Usage      *ResponseUsage             `json:"usage,omitempty"`
	Error      *ResponseError             `json:"error,omitempty"`
	Extra      map[string]json.RawMessage `json:"-"`
	RawBody    json.RawMessage            `json:"-"`
}

// ResponseOutputItem is one item in a Responses output array. MVP models the
// assistant "message" item; other item types are preserved verbatim in Raw.
type ResponseOutputItem struct {
	ID      string                     `json:"id,omitempty"`
	Type    string                     `json:"type"`
	Role    string                     `json:"role,omitempty"`
	Status  string                     `json:"status,omitempty"`
	Content []ResponseOutputContent    `json:"content,omitempty"`
	Raw     json.RawMessage            `json:"-"`
	Extra   map[string]json.RawMessage `json:"-"`
}

// ResponseOutputContent is one content part of a Responses output message.
// MVP models output_text; refusal and other types are preserved in Raw.
type ResponseOutputContent struct {
	Type string                     `json:"type"`
	Text string                     `json:"text,omitempty"`
	Raw  json.RawMessage            `json:"-"`
	Extra map[string]json.RawMessage `json:"-"`
}

// ResponseUsage mirrors OpenAI Responses token accounting.
type ResponseUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// ResponseError is the error object returned on a failed model response.
type ResponseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ParseResponseRequest decodes a Responses API request body, preserving the raw
// body and unknown top-level fields for forwarding.
func ParseResponseRequest(body []byte) (*ResponseRequest, error) {
	var req ResponseRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("decode responses request: %w", err)
	}
	req.RawBody = append(req.RawBody[:0], body...)

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, fmt.Errorf("decode responses request fields: %w", err)
	}
	for _, key := range []string{
		"model",
		"input",
		"instructions",
		"stream",
		"temperature",
		"top_p",
		"max_output_tokens",
	} {
		delete(fields, key)
	}
	req.Extra = fields
	return &req, nil
}

// MarshalJSON encodes known Responses request fields plus preserved unknown fields.
func (r ResponseRequest) MarshalJSON() ([]byte, error) {
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
	if err := put("input", r.Input); err != nil {
		return nil, err
	}
	if r.Instructions != "" {
		if err := put("instructions", r.Instructions); err != nil {
			return nil, err
		}
	}
	if r.Stream {
		if err := put("stream", r.Stream); err != nil {
			return nil, err
		}
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
	if r.MaxOutputTokens != nil {
		if err := put("max_output_tokens", r.MaxOutputTokens); err != nil {
			return nil, err
		}
	}
	return json.Marshal(fields)
}

// ToIR normalizes a Responses API request to the provider-neutral IR.
func (r ResponseRequest) ToIR() (ir.Request, error) {
	if r.Model == "" {
		return ir.Request{}, errors.New("responses request model is required")
	}
	if r.Input.Text == "" && len(r.Input.Items) == 0 {
		return ir.Request{}, errors.New("responses request input must not be empty")
	}

	var messages []ir.Message
	if r.Instructions != "" {
		messages = append(messages, ir.Message{
			Role:    ResponseRoleDeveloper,
			Content: []ir.ContentPart{{Type: ir.ContentTypeText, Text: r.Instructions}},
		})
	}

	if r.Input.Text != "" {
		messages = append(messages, ir.Message{
			Role:    ir.RoleUser,
			Content: []ir.ContentPart{{Type: ir.ContentTypeText, Text: r.Input.Text}},
		})
	}

	for i, item := range r.Input.Items {
		itemType := item.Type
		if itemType == "" {
			itemType = ResponseInputItemTypeMessage
		}
		if itemType != ResponseInputItemTypeMessage {
			return ir.Request{}, fmt.Errorf("responses input item %d has unsupported type %q", i, itemType)
		}
		if item.Role == "" {
			return ir.Request{}, fmt.Errorf("responses input item %d role is required", i)
		}
		content, err := item.Content.ToIRContentParts()
		if err != nil {
			return ir.Request{}, fmt.Errorf("responses input item %d content: %w", i, err)
		}
		messages = append(messages, ir.Message{
			Role:    item.Role,
			Content: content,
		})
	}

	return ir.Request{
		Model:    r.Model,
		Messages: messages,
		Params: ir.SamplingParams{
			Temperature:         r.Temperature,
			TopP:                r.TopP,
			MaxCompletionTokens: r.MaxOutputTokens,
		},
		Stream:  r.Stream,
		RawBody: append(json.RawMessage(nil), r.RawBody...),
		Extra:   cloneRawMap(r.Extra),
	}, nil
}

// ResponseFromIR converts a normalized response to an OpenAI Responses response object.
func ResponseFromIR(resp ir.Response, createdAt time.Time) Response {
	out := Response{
		ID:        resp.ID,
		Object:    ResponseObject,
		CreatedAt: createdAt.Unix(),
		Model:     resp.Model,
		Status:    responseStatusForFinishReason(resp.FinishReason),
		Output:    []ResponseOutputItem{responseOutputItemFromIR(resp.Message, resp.FinishReason)},
	}
	if resp.Usage != nil {
		out.Usage = &ResponseUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		}
	}
	return out
}

// ToIR normalizes an OpenAI Responses response object to provider-neutral IR.
func (r Response) ToIR(raw json.RawMessage) (ir.Response, error) {
	message, finishReason, err := r.firstMessage()
	if err != nil {
		return ir.Response{}, err
	}
	resp := ir.Response{
		ID:           r.ID,
		Model:        r.Model,
		Message:      message,
		FinishReason: finishReason,
		RawBody:      append(json.RawMessage(nil), raw...),
	}
	if r.Usage != nil {
		resp.Usage = &ir.Usage{
			PromptTokens:     r.Usage.InputTokens,
			CompletionTokens: r.Usage.OutputTokens,
			TotalTokens:      r.Usage.TotalTokens,
		}
	}
	return resp, nil
}

func (r Response) firstMessage() (ir.Message, string, error) {
	for _, item := range r.Output {
		if item.Type != ResponseItemTypeMessage {
			continue
		}
		content, err := responseOutputContentToIR(item.Content)
		if err != nil {
			return ir.Message{}, "", err
		}
		finishReason := responseFinishReasonForStatus(item.Status)
		return ir.Message{Role: item.Role, Content: content}, finishReason, nil
	}
	return ir.Message{}, "", errors.New("responses output contains no message item")
}

func responseOutputItemFromIR(message ir.Message, finishReason string) ResponseOutputItem {
	parts := make([]ResponseOutputContent, 0, len(message.Content))
	for _, part := range message.Content {
		if part.Type != ir.ContentTypeText {
			continue
		}
		parts = append(parts, ResponseOutputContent{
			Type: ResponseContentTypeOutputText,
			Text: part.Text,
		})
	}
	return ResponseOutputItem{
		Type:    ResponseItemTypeMessage,
		Role:    message.Role,
		Status:  responseStatusForFinishReason(finishReason),
		Content: parts,
	}
}

func responseOutputContentToIR(parts []ResponseOutputContent) ([]ir.ContentPart, error) {
	if len(parts) == 0 {
		return []ir.ContentPart{}, nil
	}
	out := make([]ir.ContentPart, 0, len(parts))
	for i, part := range parts {
		switch part.Type {
		case ResponseContentTypeOutputText:
			out = append(out, ir.ContentPart{Type: ir.ContentTypeText, Text: part.Text})
		case ResponseContentTypeRefusal:
			// Preserve refusals as text so they are not silently dropped.
			out = append(out, ir.ContentPart{Type: ir.ContentTypeText, Text: part.Text})
		case "":
			return nil, fmt.Errorf("responses output content part %d type is required", i)
		default:
			return nil, fmt.Errorf("responses output content part %d has unsupported type %q", i, part.Type)
		}
	}
	return out, nil
}

// ResponseStatusCompleted and friends mirror OpenAI Responses item statuses.
const (
	ResponseStatusCompleted   = "completed"
	ResponseStatusIncomplete  = "incomplete"
	ResponseStatusInProgress  = "in_progress"
)

func responseStatusForFinishReason(reason string) string {
	switch reason {
	case "length", "max_output_tokens":
		return ResponseStatusIncomplete
	case "":
		return ResponseStatusCompleted
	default:
		return ResponseStatusCompleted
	}
}

func responseFinishReasonForStatus(status string) string {
	switch status {
	case ResponseStatusIncomplete:
		return "length"
	case ResponseStatusCompleted, ResponseStatusInProgress, "":
		return "stop"
	default:
		return "stop"
	}
}

// UnmarshalJSON decodes Responses input as either a plain string or an array of input items.
func (i *ResponseInput) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		i.Text = text
		i.Items = nil
		return nil
	}
	var items []ResponseInputItem
	if err := json.Unmarshal(data, &items); err != nil {
		return errors.New("responses input must be a string or an array of input items")
	}
	i.Text = ""
	i.Items = items
	return nil
}

// MarshalJSON emits Responses input as a string when set, otherwise as an array of items.
func (i ResponseInput) MarshalJSON() ([]byte, error) {
	if len(i.Items) == 0 {
		return json.Marshal(i.Text)
	}
	return json.Marshal(i.Items)
}

// UnmarshalJSON decodes Responses input content as either a string or an array of content parts.
func (c *ResponseInputContent) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		c.Text = text
		c.Parts = nil
		return nil
	}
	var parts []ResponseInputContentPart
	if err := json.Unmarshal(data, &parts); err != nil {
		return errors.New("responses input content must be a string or an array of content parts")
	}
	c.Text = ""
	c.Parts = parts
	return nil
}

// MarshalJSON emits Responses input content as a string when no parts are present.
func (c ResponseInputContent) MarshalJSON() ([]byte, error) {
	if c.Parts == nil {
		return json.Marshal(c.Text)
	}
	return json.Marshal(c.Parts)
}

// ToIRContentParts normalizes Responses input content into provider-neutral IR content parts.
func (c ResponseInputContent) ToIRContentParts() ([]ir.ContentPart, error) {
	if c.Parts == nil {
		return []ir.ContentPart{{Type: ir.ContentTypeText, Text: c.Text}}, nil
	}
	parts := make([]ir.ContentPart, 0, len(c.Parts))
	for i, part := range c.Parts {
		switch part.Type {
		case ResponseInputContentTypeText:
			parts = append(parts, ir.ContentPart{Type: ir.ContentTypeText, Text: part.Text})
		case ResponseInputContentTypeImage:
			if part.ImageURL == "" && part.FileID == "" {
				return nil, fmt.Errorf("responses content part %d input_image requires image_url or file_id", i)
			}
			parts = append(parts, ir.ContentPart{
				Type:     ir.ContentTypeInputImage,
				ImageURL: part.ImageURL,
				Detail:   part.Detail,
			})
		default:
			return nil, fmt.Errorf("responses content part %d has unsupported type %q", i, part.Type)
		}
	}
	return parts, nil
}

// UnmarshalJSON decodes and validates supported Responses input content part shapes,
// preserving unknown fields.
func (p *ResponseInputContentPart) UnmarshalJSON(data []byte) error {
	type alias ResponseInputContentPart
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	switch decoded.Type {
	case ResponseInputContentTypeText:
		raw, ok := fields["text"]
		if !ok {
			return errors.New("input_text content part requires text")
		}
		if err := json.Unmarshal(raw, &decoded.Text); err != nil {
			return errors.New("input_text content part text must be a string")
		}
		decoded.ImageURL = ""
		decoded.FileID = ""
		decoded.Detail = ""
	case ResponseInputContentTypeImage:
		// image_url and file_id are both optional individually; validation of at
		// least one happens in ToIRContentParts where context is available.
		decoded.Text = ""
	default:
		return fmt.Errorf("unsupported responses content part type %q", decoded.Type)
	}

	for _, key := range []string{"type", "text", "image_url", "file_id", "detail"} {
		delete(fields, key)
	}
	decoded.Extra = fields
	*p = ResponseInputContentPart(decoded)
	return nil
}

// UnmarshalJSON preserves unknown Responses input item fields while decoding known fields.
func (i *ResponseInputItem) UnmarshalJSON(data []byte) error {
	type alias ResponseInputItem
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	i.Raw = append(json.RawMessage(nil), data...)

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range []string{"type", "role", "content"} {
		delete(fields, key)
	}
	decoded.Extra = fields
	*i = ResponseInputItem(decoded)
	return nil
}

// UnmarshalJSON decodes a Responses output item, preserving unknown item types verbatim.
func (i *ResponseOutputItem) UnmarshalJSON(data []byte) error {
	type alias ResponseOutputItem
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	i.Raw = append(json.RawMessage(nil), data...)

	itemType := decoded.Type
	if itemType == "" {
		itemType = ResponseItemTypeMessage
	}
	if itemType != ResponseItemTypeMessage {
		// Unknown output item type: keep raw only, clear modeled fields.
		decoded = alias{Type: decoded.Type, Raw: nil}
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range []string{"type", "role", "status", "content", "id"} {
		delete(fields, key)
	}
	decoded.Extra = fields
	*i = ResponseOutputItem(decoded)
	return nil
}

// UnmarshalJSON decodes a Responses output content part, preserving unknown types verbatim.
func (c *ResponseOutputContent) UnmarshalJSON(data []byte) error {
	type alias ResponseOutputContent
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	c.Raw = append(json.RawMessage(nil), data...)

	switch decoded.Type {
	case ResponseContentTypeOutputText, ResponseContentTypeRefusal:
		// Modeled text/refusal content; text decoded above.
	default:
		// Unknown content type: keep raw only, clear modeled fields.
		decoded = alias{Type: decoded.Type, Raw: nil}
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range []string{"type", "text"} {
		delete(fields, key)
	}
	decoded.Extra = fields
	*c = ResponseOutputContent(decoded)
	return nil
}

// MarshalJSON encodes a Responses output content part, preserving unknown fields.
func (c ResponseOutputContent) MarshalJSON() ([]byte, error) {
	fields := cloneRawMap(c.Extra)
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
	if err := put("type", c.Type); err != nil {
		return nil, err
	}
	if c.Text != "" {
		if err := put("text", c.Text); err != nil {
			return nil, err
		}
	}
	return json.Marshal(fields)
}
