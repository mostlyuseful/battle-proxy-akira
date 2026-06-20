package openai

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"battle-proxy-akira/internal/ir"
)

// parseResponsesTime is a fixed timestamp used for deterministic response creation.
var parseResponsesTime = time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)

func TestParseResponseRequestTextInput(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"model": "coding",
		"input": "write a test",
		"instructions": "be concise",
		"temperature": 0.2,
		"stream": true
	}`)

	req, err := ParseResponseRequest(body)
	if err != nil {
		t.Fatalf("ParseResponseRequest: %v", err)
	}
	if req.Model != "coding" {
		t.Fatalf("Model = %q", req.Model)
	}
	if req.Input.Text != "write a test" {
		t.Fatalf("Input.Text = %q", req.Input.Text)
	}
	if len(req.Input.Items) != 0 {
		t.Fatalf("Input.Items = %v, want empty", req.Input.Items)
	}
	if req.Instructions != "be concise" {
		t.Fatalf("Instructions = %q", req.Instructions)
	}
	if req.Stream != true {
		t.Fatal("Stream = false, want true")
	}
	if req.Temperature == nil || *req.Temperature != 0.2 {
		t.Fatalf("Temperature = %v", req.Temperature)
	}
	if req.RawBody == nil {
		t.Fatal("RawBody not preserved")
	}
}

func TestResponseRequestToIRTextInput(t *testing.T) {
	t.Parallel()

	req := ResponseRequest{
		Model:        "coding",
		Instructions: "be concise",
		Input:        ResponseInput{Text: "write a test"},
	}
	temp := 0.2
	topP := 0.9
	maxTokens := 512
	req.Temperature = &temp
	req.TopP = &topP
	req.MaxOutputTokens = &maxTokens

	got, err := req.ToIR()
	if err != nil {
		t.Fatalf("ToIR: %v", err)
	}
	if got.Model != "coding" {
		t.Fatalf("Model = %q", got.Model)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("Messages len = %d, want 2 (instructions + user)", len(got.Messages))
	}
	if got.Messages[0].Role != ResponseRoleDeveloper {
		t.Fatalf("first message role = %q, want developer", got.Messages[0].Role)
	}
	if got.Messages[0].Content[0].Text != "be concise" {
		t.Fatalf("instructions text = %q", got.Messages[0].Content[0].Text)
	}
	if got.Messages[1].Role != ir.RoleUser {
		t.Fatalf("second message role = %q, want user", got.Messages[1].Role)
	}
	if got.Messages[1].Content[0].Text != "write a test" {
		t.Fatalf("user text = %q", got.Messages[1].Content[0].Text)
	}
	if got.Params.Temperature == nil || *got.Params.Temperature != 0.2 {
		t.Fatalf("Temperature = %v", got.Params.Temperature)
	}
	if got.Params.TopP == nil || *got.Params.TopP != 0.9 {
		t.Fatalf("TopP = %v", got.Params.TopP)
	}
	if got.Params.MaxCompletionTokens == nil || *got.Params.MaxCompletionTokens != 512 {
		t.Fatalf("MaxCompletionTokens = %v", got.Params.MaxCompletionTokens)
	}
	if got.Params.MaxTokens != nil {
		t.Fatalf("MaxTokens should be nil for responses request, got %v", got.Params.MaxTokens)
	}
}

func TestParseResponseRequestImageInputNormalizesToIR(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"model": "vision-model",
		"input": [
			{
				"type": "message",
				"role": "user",
				"content": [
					{"type": "input_text", "text": "What is wrong with this UI?"},
					{"type": "input_image", "image_url": "data:image/png;base64,iVBOR=", "detail": "high"}
				]
			}
		]
	}`)

	req, err := ParseResponseRequest(body)
	if err != nil {
		t.Fatalf("ParseResponseRequest: %v", err)
	}
	if req.Input.Text != "" {
		t.Fatalf("Input.Text = %q, want empty", req.Input.Text)
	}
	if len(req.Input.Items) != 1 {
		t.Fatalf("Input.Items len = %d", len(req.Input.Items))
	}
	item := req.Input.Items[0]
	if item.Type != "message" {
		t.Fatalf("item Type = %q", item.Type)
	}
	if item.Role != "user" {
		t.Fatalf("item Role = %q", item.Role)
	}
	if len(item.Content.Parts) != 2 {
		t.Fatalf("content parts len = %d", len(item.Content.Parts))
	}
	if item.Content.Parts[0].Type != ResponseInputContentTypeText {
		t.Fatalf("part 0 Type = %q", item.Content.Parts[0].Type)
	}
	if item.Content.Parts[0].Text != "What is wrong with this UI?" {
		t.Fatalf("part 0 Text = %q", item.Content.Parts[0].Text)
	}
	if item.Content.Parts[1].Type != ResponseInputContentTypeImage {
		t.Fatalf("part 1 Type = %q", item.Content.Parts[1].Type)
	}
	if item.Content.Parts[1].ImageURL != "data:image/png;base64,iVBOR=" {
		t.Fatalf("part 1 ImageURL = %q", item.Content.Parts[1].ImageURL)
	}
	if item.Content.Parts[1].Detail != "high" {
		t.Fatalf("part 1 Detail = %q", item.Content.Parts[1].Detail)
	}

	got, err := req.ToIR()
	if err != nil {
		t.Fatalf("ToIR: %v", err)
	}
	if !got.HasImages() {
		t.Fatal("expected IR request to report images")
	}
	if got.Messages[0].Role != ir.RoleUser {
		t.Fatalf("message role = %q", got.Messages[0].Role)
	}
	if len(got.Messages[0].Content) != 2 {
		t.Fatalf("IR content len = %d", len(got.Messages[0].Content))
	}
	if got.Messages[0].Content[0].Type != ir.ContentTypeText {
		t.Fatalf("IR part 0 Type = %q", got.Messages[0].Content[0].Type)
	}
	img := got.Messages[0].Content[1]
	if img.Type != ir.ContentTypeInputImage {
		t.Fatalf("IR image part Type = %q, want input_image", img.Type)
	}
	if img.ImageURL != "data:image/png;base64,iVBOR=" {
		t.Fatalf("IR image ImageURL = %q", img.ImageURL)
	}
	if img.Detail != "high" {
		t.Fatalf("IR image Detail = %q", img.Detail)
	}
	if got.InputModalities()[1] != ir.ModalityImage {
		t.Fatalf("expected image modality, got %v", got.InputModalities())
	}
}

func TestParseResponseRequestImageInputStringContent(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"model": "m",
		"input": [
			{"type": "message", "role": "system", "content": "you are helpful"},
			{"type": "message", "role": "user", "content": "hello"}
		]
	}`)

	req, err := ParseResponseRequest(body)
	if err != nil {
		t.Fatalf("ParseResponseRequest: %v", err)
	}
	if len(req.Input.Items) != 2 {
		t.Fatalf("items len = %d", len(req.Input.Items))
	}
	if req.Input.Items[0].Content.Text != "you are helpful" {
		t.Fatalf("item 0 content = %q", req.Input.Items[0].Content.Text)
	}
	if req.Input.Items[0].Content.Parts != nil {
		t.Fatalf("item 0 Parts should be nil for string content")
	}

	got, err := req.ToIR()
	if err != nil {
		t.Fatalf("ToIR: %v", err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("IR messages len = %d", len(got.Messages))
	}
	if got.Messages[0].Role != "system" {
		t.Fatalf("IR message 0 role = %q", got.Messages[0].Role)
	}
	if got.Messages[0].Content[0].Type != ir.ContentTypeText || got.Messages[0].Content[0].Text != "you are helpful" {
		t.Fatalf("IR message 0 content = %+v", got.Messages[0].Content[0])
	}
}

func TestParseResponseRequestPreservesUnknownFields(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"model": "m",
		"input": "hi",
		"top_p": 0.5,
		"reasoning": {"effort": "low"},
		"previous_response_id": "resp_123"
	}`)

	req, err := ParseResponseRequest(body)
	if err != nil {
		t.Fatalf("ParseResponseRequest: %v", err)
	}
	if req.TopP == nil || *req.TopP != 0.5 {
		t.Fatalf("TopP = %v", req.TopP)
	}
	if _, ok := req.Extra["reasoning"]; !ok {
		t.Fatal("expected reasoning preserved in Extra")
	}
	if _, ok := req.Extra["previous_response_id"]; !ok {
		t.Fatal("expected previous_response_id preserved in Extra")
	}
	if _, ok := req.Extra["model"]; ok {
		t.Fatal("known field model should not be in Extra")
	}

	// Round-trip through MarshalJSON keeps unknown fields and known fields.
	encoded, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if !strings.Contains(string(encoded), `"reasoning"`) {
		t.Fatalf("marshaled output missing reasoning: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"top_p":0.5`) {
		t.Fatalf("marshaled output missing top_p: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"input":"hi"`) {
		t.Fatalf("marshaled output missing input string: %s", encoded)
	}
}

func TestParseResponseRequestRejectsMissingModel(t *testing.T) {
	t.Parallel()

	body := []byte(`{"input": "hi"}`)
	req, err := ParseResponseRequest(body)
	if err != nil {
		t.Fatalf("ParseResponseRequest: %v", err)
	}
	if _, err := req.ToIR(); err == nil {
		t.Fatal("ToIR with missing model should error")
	}
}

func TestParseResponseRequestRejectsEmptyInput(t *testing.T) {
	t.Parallel()

	body := []byte(`{"model": "m", "input": []}`)
	req, err := ParseResponseRequest(body)
	if err != nil {
		t.Fatalf("ParseResponseRequest: %v", err)
	}
	if _, err := req.ToIR(); err == nil {
		t.Fatal("ToIR with empty input should error")
	}
}

func TestParseResponseRequestRejectsUnsupportedInputItemType(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"model": "m",
		"input": [
			{"type": "function_call", "arguments": "{}"}
		]
	}`)
	req, err := ParseResponseRequest(body)
	if err != nil {
		t.Fatalf("ParseResponseRequest: %v", err)
	}
	if _, err := req.ToIR(); err == nil {
		t.Fatal("ToIR with unsupported input item type should error")
	}
}

func TestParseResponseRequestRejectsImagePartWithoutSource(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"model": "m",
		"input": [
			{"type": "message", "role": "user", "content": [
				{"type": "input_image", "detail": "auto"}
			]}
		]
	}`)
	req, err := ParseResponseRequest(body)
	if err != nil {
		t.Fatalf("ParseResponseRequest: %v", err)
	}
	if _, err := req.ToIR(); err == nil {
		t.Fatal("ToIR with image part lacking image_url/file_id should error")
	}
}

func TestResponseFromIRRoundTripsThroughParse(t *testing.T) {
	t.Parallel()

	irResp := ir.Response{
		ID:    "resp_123",
		Model: "coding",
		Message: ir.Message{
			Role:    ir.RoleAssistant,
			Content: []ir.ContentPart{{Type: ir.ContentTypeText, Text: "here is a test"}},
		},
		FinishReason: "stop",
		Usage: &ir.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	resp := ResponseFromIR(irResp, parseResponsesTime)
	if resp.ID != "resp_123" {
		t.Fatalf("ID = %q", resp.ID)
	}
	if resp.Object != ResponseObject {
		t.Fatalf("Object = %q", resp.Object)
	}
	if resp.Model != "coding" {
		t.Fatalf("Model = %q", resp.Model)
	}
	if resp.Status != ResponseStatusCompleted {
		t.Fatalf("Status = %q", resp.Status)
	}
	if len(resp.Output) != 1 {
		t.Fatalf("Output len = %d", len(resp.Output))
	}
	item := resp.Output[0]
	if item.Type != ResponseItemTypeMessage {
		t.Fatalf("output item Type = %q", item.Type)
	}
	if item.Role != ir.RoleAssistant {
		t.Fatalf("output item Role = %q", item.Role)
	}
	if len(item.Content) != 1 || item.Content[0].Type != ResponseContentTypeOutputText {
		t.Fatalf("output content = %+v", item.Content)
	}
	if item.Content[0].Text != "here is a test" {
		t.Fatalf("output text = %q", item.Content[0].Text)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 15 {
		t.Fatalf("Usage = %+v", resp.Usage)
	}

	encoded, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var parsed Response
	if err := json.Unmarshal(encoded, &parsed); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	roundTripped, err := parsed.ToIR(encoded)
	if err != nil {
		t.Fatalf("ToIR: %v", err)
	}
	if roundTripped.ID != irResp.ID {
		t.Fatalf("round-trip ID = %q", roundTripped.ID)
	}
	if roundTripped.Model != irResp.Model {
		t.Fatalf("round-trip Model = %q", roundTripped.Model)
	}
	if roundTripped.Message.Role != ir.RoleAssistant {
		t.Fatalf("round-trip role = %q", roundTripped.Message.Role)
	}
	if roundTripped.Message.Content[0].Text != "here is a test" {
		t.Fatalf("round-trip text = %q", roundTripped.Message.Content[0].Text)
	}
	if roundTripped.Usage == nil || roundTripped.Usage.TotalTokens != 15 {
		t.Fatalf("round-trip Usage = %+v", roundTripped.Usage)
	}
	if roundTripped.FinishReason != "stop" {
		t.Fatalf("round-trip FinishReason = %q", roundTripped.FinishReason)
	}
}

func TestResponseFromIRLengthFinishReasonIsIncomplete(t *testing.T) {
	t.Parallel()

	irResp := ir.Response{
		ID:           "resp_len",
		Model:        "m",
		Message:      ir.Message{Role: ir.RoleAssistant, Content: []ir.ContentPart{{Type: ir.ContentTypeText, Text: "partial"}}},
		FinishReason: "length",
	}
	resp := ResponseFromIR(irResp, parseResponsesTime)
	if resp.Status != ResponseStatusIncomplete {
		t.Fatalf("Status = %q, want incomplete", resp.Status)
	}

	// Round-trip maps incomplete status back to a length finish reason.
	encoded, _ := json.Marshal(resp)
	var parsed Response
	if err := json.Unmarshal(encoded, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, err := parsed.ToIR(encoded)
	if err != nil {
		t.Fatalf("ToIR: %v", err)
	}
	if got.FinishReason != "length" {
		t.Fatalf("FinishReason = %q, want length", got.FinishReason)
	}
}

func TestResponseToIRRejectsOutputWithoutMessage(t *testing.T) {
	t.Parallel()

	resp := Response{
		ID:     "resp_x",
		Model:  "m",
		Output: []ResponseOutputItem{{Type: "reasoning"}},
	}
	if _, err := resp.ToIR(json.RawMessage(`{}`)); err == nil {
		t.Fatal("ToIR with no message item should error")
	}
}

func TestResponseToIRRejectsUnsupportedOutputContentType(t *testing.T) {
	t.Parallel()

	resp := Response{
		ID:    "resp_y",
		Model: "m",
		Output: []ResponseOutputItem{{
			Type:    ResponseItemTypeMessage,
			Role:    ir.RoleAssistant,
			Content: []ResponseOutputContent{{Type: "output_audio"}},
		}},
	}
	if _, err := resp.ToIR(json.RawMessage(`{}`)); err == nil {
		t.Fatal("ToIR with unsupported output content type should error")
	}
}
