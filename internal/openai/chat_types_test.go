package openai

import (
	"encoding/json"
	"testing"
	"time"

	"battle-proxy-akira/internal/ir"
)

func TestParseChatCompletionRequestToIRTextOnly(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"model": "coding",
		"messages": [
			{"role": "system", "content": "You are concise."},
			{"role": "user", "content": "write a test"}
		],
		"stream": true,
		"x_provider_option": {"enabled": true}
	}`)

	req, err := ParseChatCompletionRequest(body)
	if err != nil {
		t.Fatalf("ParseChatCompletionRequest: %v", err)
	}
	got, err := req.ToIR()
	if err != nil {
		t.Fatalf("ToIR: %v", err)
	}

	if got.Model != "coding" {
		t.Fatalf("Model = %q, want coding", got.Model)
	}
	if !got.Stream {
		t.Fatal("Stream = false, want true")
	}
	if len(got.Messages) != 2 {
		t.Fatalf("Messages length = %d, want 2", len(got.Messages))
	}
	if got.Messages[1].Role != ir.RoleUser {
		t.Fatalf("second role = %q, want user", got.Messages[1].Role)
	}
	part := got.Messages[1].Content[0]
	if part.Type != ir.ContentTypeText || part.Text != "write a test" {
		t.Fatalf("content part = %#v, want text write a test", part)
	}
	if string(got.RawBody) != string(body) {
		t.Fatalf("RawBody = %s, want original body", got.RawBody)
	}
	if string(got.Extra["x_provider_option"]) != `{"enabled": true}` {
		t.Fatalf("Extra[x_provider_option] = %s", got.Extra["x_provider_option"])
	}
}

func TestParseChatCompletionRequestSamplingParameters(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"model": "coding",
		"messages": [{"role": "user", "content": "hello"}],
		"temperature": 0.2,
		"top_p": 0.9,
		"max_tokens": 100,
		"max_completion_tokens": 80,
		"stop": ["END", "STOP"],
		"presence_penalty": 0.1,
		"frequency_penalty": 0.3,
		"seed": 1234
	}`)

	req, err := ParseChatCompletionRequest(body)
	if err != nil {
		t.Fatalf("ParseChatCompletionRequest: %v", err)
	}
	got, err := req.ToIR()
	if err != nil {
		t.Fatalf("ToIR: %v", err)
	}

	assertFloatPtr(t, "Temperature", got.Params.Temperature, 0.2)
	assertFloatPtr(t, "TopP", got.Params.TopP, 0.9)
	assertIntPtr(t, "MaxTokens", got.Params.MaxTokens, 100)
	assertIntPtr(t, "MaxCompletionTokens", got.Params.MaxCompletionTokens, 80)
	if len(got.Params.Stop) != 2 || got.Params.Stop[0] != "END" || got.Params.Stop[1] != "STOP" {
		t.Fatalf("Stop = %#v, want [END STOP]", got.Params.Stop)
	}
	assertFloatPtr(t, "PresencePenalty", got.Params.PresencePenalty, 0.1)
	assertFloatPtr(t, "FrequencyPenalty", got.Params.FrequencyPenalty, 0.3)
	assertIntPtr(t, "Seed", got.Params.Seed, 1234)
}

func TestParseChatCompletionRequestSingleStopString(t *testing.T) {
	t.Parallel()

	req, err := ParseChatCompletionRequest([]byte(`{
		"model": "coding",
		"messages": [{"role": "user", "content": "hello"}],
		"stop": "END"
	}`))
	if err != nil {
		t.Fatalf("ParseChatCompletionRequest: %v", err)
	}
	got, err := req.ToIR()
	if err != nil {
		t.Fatalf("ToIR: %v", err)
	}
	if len(got.Params.Stop) != 1 || got.Params.Stop[0] != "END" {
		t.Fatalf("Stop = %#v, want [END]", got.Params.Stop)
	}
}

func TestParseChatCompletionRequestToIRTextAndImageParts(t *testing.T) {
	t.Parallel()

	req, err := ParseChatCompletionRequest([]byte(`{
		"model": "vision-model",
		"messages": [{
			"role": "user",
			"content": [
				{"type": "text", "text": "what is in this image?"},
				{"type": "image_url", "image_url": {"url": "https://example.test/cat.png", "detail": "high"}}
			]
		}]
	}`))
	if err != nil {
		t.Fatalf("ParseChatCompletionRequest: %v", err)
	}
	got, err := req.ToIR()
	if err != nil {
		t.Fatalf("ToIR: %v", err)
	}

	parts := got.Messages[0].Content
	if len(parts) != 2 {
		t.Fatalf("content parts length = %d, want 2", len(parts))
	}
	if parts[0].Type != ir.ContentTypeText || parts[0].Text != "what is in this image?" {
		t.Fatalf("text part = %#v", parts[0])
	}
	if parts[1].Type != ir.ContentTypeImageURL || parts[1].ImageURL != "https://example.test/cat.png" || parts[1].Detail != "high" {
		t.Fatalf("image part = %#v", parts[1])
	}
	if !got.HasImages() {
		t.Fatal("HasImages = false, want true")
	}
}

func TestParseChatCompletionRequestToIRMultipleImagesAndDataURL(t *testing.T) {
	t.Parallel()

	dataURL := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAAB"
	req, err := ParseChatCompletionRequest([]byte(`{
		"model": "vision-model",
		"messages": [{
			"role": "user",
			"content": [
				{"type": "image_url", "image_url": {"url": "` + dataURL + `", "detail": "low"}},
				{"type": "image_url", "image_url": {"url": "https://example.test/second.jpg"}}
			]
		}]
	}`))
	if err != nil {
		t.Fatalf("ParseChatCompletionRequest: %v", err)
	}
	got, err := req.ToIR()
	if err != nil {
		t.Fatalf("ToIR: %v", err)
	}

	parts := got.Messages[0].Content
	if len(parts) != 2 {
		t.Fatalf("content parts length = %d, want 2", len(parts))
	}
	if parts[0].ImageURL != dataURL || parts[0].Detail != "low" {
		t.Fatalf("first image part = %#v", parts[0])
	}
	if parts[1].ImageURL != "https://example.test/second.jpg" || parts[1].Detail != "" {
		t.Fatalf("second image part = %#v", parts[1])
	}
}

func TestParseChatCompletionRequestRejectsMalformedContentParts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "content object", body: `{"model":"coding","messages":[{"role":"user","content":{"type":"text","text":"hello"}}]}`},
		{name: "missing type", body: `{"model":"coding","messages":[{"role":"user","content":[{"text":"hello"}]}]}`},
		{name: "unknown type", body: `{"model":"coding","messages":[{"role":"user","content":[{"type":"input_image","image_url":{"url":"https://example.test/a.png"}}]}]}`},
		{name: "missing text", body: `{"model":"coding","messages":[{"role":"user","content":[{"type":"text"}]}]}`},
		{name: "image url string", body: `{"model":"coding","messages":[{"role":"user","content":[{"type":"image_url","image_url":"https://example.test/a.png"}]}]}`},
		{name: "missing image url", body: `{"model":"coding","messages":[{"role":"user","content":[{"type":"image_url","image_url":{}}]}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := ParseChatCompletionRequest([]byte(tt.body)); err == nil {
				t.Fatal("ParseChatCompletionRequest returned nil error, want malformed content error")
			}
		})
	}
}

func TestChatCompletionRequestFromIRImageParts(t *testing.T) {
	t.Parallel()

	req, err := ChatCompletionRequestFromIR(ir.Request{
		Model: "vision-model",
		Messages: []ir.Message{{
			Role: ir.RoleUser,
			Content: []ir.ContentPart{
				{Type: ir.ContentTypeText, Text: "describe"},
				{Type: ir.ContentTypeImageURL, ImageURL: "data:image/jpeg;base64,/9j/4AAQ", Detail: "auto"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("ChatCompletionRequestFromIR: %v", err)
	}

	encoded, err := json.Marshal(req.Messages[0].Content)
	if err != nil {
		t.Fatalf("Marshal content: %v", err)
	}
	var parts []ChatContentPart
	if err := json.Unmarshal(encoded, &parts); err != nil {
		t.Fatalf("Unmarshal content: %v", err)
	}
	if len(parts) != 2 || parts[0].Text != "describe" || parts[1].ImageURL == nil || parts[1].ImageURL.Detail != "auto" {
		t.Fatalf("encoded parts = %#v", parts)
	}
}

func TestChatMessagePreservesUnknownFields(t *testing.T) {
	t.Parallel()

	req, err := ParseChatCompletionRequest([]byte(`{
		"model": "coding",
		"messages": [{"role": "user", "content": "hello", "experimental": 42}]
	}`))
	if err != nil {
		t.Fatalf("ParseChatCompletionRequest: %v", err)
	}
	if string(req.Messages[0].Extra["experimental"]) != "42" {
		t.Fatalf("message Extra[experimental] = %s, want 42", req.Messages[0].Extra["experimental"])
	}
}

func TestChatCompletionResponseFromIR(t *testing.T) {
	t.Parallel()

	resp := ChatCompletionResponseFromIR(ir.Response{
		ID:    "chatcmpl-proxy-test",
		Model: "coding",
		Message: ir.Message{
			Role: ir.RoleAssistant,
			Content: []ir.ContentPart{
				{Type: ir.ContentTypeText, Text: "hello"},
				{Type: ir.ContentTypeText, Text: " world"},
			},
		},
		FinishReason: "stop",
		Usage:        &ir.Usage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3},
	}, time.Unix(123, 0))

	if resp.Object != ChatCompletionsObject {
		t.Fatalf("Object = %q, want %q", resp.Object, ChatCompletionsObject)
	}
	if resp.Created != 123 {
		t.Fatalf("Created = %d, want 123", resp.Created)
	}
	if resp.Choices[0].Message.Content.Text != "hello world" {
		t.Fatalf("message content = %q, want hello world", resp.Choices[0].Message.Content.Text)
	}
	if resp.Usage.TotalTokens != 3 {
		t.Fatalf("total tokens = %d, want 3", resp.Usage.TotalTokens)
	}

	encoded, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal response: %v", err)
	}
	if !json.Valid(encoded) {
		t.Fatalf("encoded response is invalid JSON: %s", encoded)
	}
}

func assertFloatPtr(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func assertIntPtr(t *testing.T, name string, got *int, want int) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}
