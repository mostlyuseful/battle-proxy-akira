package ir

import (
	"encoding/json"
	"testing"
)

func TestRequestPreservesRawBodyAndExtraFields(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"model":"coding","vendor_flag":true}`)
	extraValue := json.RawMessage(`{"nested":true}`)
	req := Request{
		ID:      "req_test",
		Model:   "coding",
		RawBody: raw,
		Extra: map[string]json.RawMessage{
			"vendor_options": extraValue,
		},
	}

	if string(req.RawBody) != string(raw) {
		t.Fatalf("RawBody = %s, want %s", req.RawBody, raw)
	}
	if string(req.Extra["vendor_options"]) != string(extraValue) {
		t.Fatalf("Extra vendor_options = %s, want %s", req.Extra["vendor_options"], extraValue)
	}
}

func TestContentPartsRepresentTextAndImagesWithoutProviderTypes(t *testing.T) {
	t.Parallel()

	req := Request{
		Messages: []Message{
			{
				Role: RoleUser,
				Content: []ContentPart{
					{Type: ContentTypeText, Text: "What is in this image?"},
					{Type: ContentTypeImageURL, ImageURL: "data:image/png;base64,abc", Detail: "low"},
				},
			},
		},
	}

	if !req.HasImages() {
		t.Fatal("HasImages() = false, want true")
	}
	modalities := req.InputModalities()
	if len(modalities) != 2 || modalities[0] != ModalityText || modalities[1] != ModalityImage {
		t.Fatalf("InputModalities() = %#v, want [text image]", modalities)
	}
}

func TestTextOnlyRequestModalities(t *testing.T) {
	t.Parallel()

	req := Request{
		Messages: []Message{
			{
				Role: RoleUser,
				Content: []ContentPart{
					{Type: ContentTypeText, Text: "hello"},
				},
			},
		},
	}

	if req.HasImages() {
		t.Fatal("HasImages() = true, want false")
	}
	modalities := req.InputModalities()
	if len(modalities) != 1 || modalities[0] != ModalityText {
		t.Fatalf("InputModalities() = %#v, want [text]", modalities)
	}
}

func TestResponseEventAndModelTypesAreUsable(t *testing.T) {
	t.Parallel()

	usage := &Usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}
	resp := Response{
		ID:    "chatcmpl-test",
		Model: "coding",
		Message: Message{
			Role:    RoleAssistant,
			Content: []ContentPart{{Type: ContentTypeText, Text: "done"}},
		},
		FinishReason: "stop",
		Usage:        usage,
	}
	event := Event{
		Type:  EventTypeMessageDelta,
		Model: "coding",
		Delta: Message{Role: RoleAssistant, Content: []ContentPart{{Type: ContentTypeText, Text: "do"}}},
		Usage: usage,
	}
	model := Model{ID: "coding", Provider: "proxy", Name: "coding", Modalities: []string{ModalityText}, Synthetic: true}

	if resp.Usage.TotalTokens != 5 {
		t.Fatalf("response total tokens = %d, want 5", resp.Usage.TotalTokens)
	}
	if event.Type != EventTypeMessageDelta {
		t.Fatalf("event type = %q, want %q", event.Type, EventTypeMessageDelta)
	}
	if !model.Synthetic || model.Modalities[0] != ModalityText {
		t.Fatalf("model = %#v", model)
	}
}
