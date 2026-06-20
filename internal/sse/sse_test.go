package sse

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestReaderParsesDataEventsAndDone(t *testing.T) {
	t.Parallel()

	input := strings.NewReader("data: {\"delta\":\"hello\"}\n\n" +
		"data: {\"delta\":\" world\"}\n\n" +
		"data: [DONE]\n\n")

	events, err := ReadAll(input)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("events length = %d, want 3", len(events))
	}
	if events[0].Data != `{"delta":"hello"}` {
		t.Fatalf("event 0 data = %q", events[0].Data)
	}
	if events[1].Data != `{"delta":" world"}` {
		t.Fatalf("event 1 data = %q", events[1].Data)
	}
	if !events[2].IsDone() {
		t.Fatalf("event 2 = %#v, want done", events[2])
	}
}

func TestReaderIgnoresCommentsBlankEventsAndNonDataFields(t *testing.T) {
	t.Parallel()

	input := strings.NewReader("\n" +
		": keep-alive\n\n" +
		"event: ignored\n" +
		"id: 1\n" +
		"\n" +
		"data: payload\n\n")

	reader := NewReader(input)
	event, err := reader.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if event.Data != "payload" {
		t.Fatalf("event data = %q, want payload", event.Data)
	}
	_, err = reader.Read()
	if err != io.EOF {
		t.Fatalf("second Read err = %v, want EOF", err)
	}
}

func TestReaderJoinsMultipleDataLines(t *testing.T) {
	t.Parallel()

	reader := NewReader(strings.NewReader("data: hello\ndata: world\n\n"))
	event, err := reader.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if event.Data != "hello\nworld" {
		t.Fatalf("event data = %q, want joined lines", event.Data)
	}
}

func TestReaderReturnsFinalEventAtEOF(t *testing.T) {
	t.Parallel()

	reader := NewReader(strings.NewReader("data: [DONE]"))
	event, err := reader.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !event.IsDone() {
		t.Fatalf("event = %#v, want done", event)
	}
}

func TestWriteDataWritesBlankLineAndFlushes(t *testing.T) {
	t.Parallel()

	w := &flushRecorder{header: http.Header{}}
	if err := WriteData(w, `{"delta":"hello"}`); err != nil {
		t.Fatalf("WriteData: %v", err)
	}

	want := "data: {\"delta\":\"hello\"}\n\n"
	if w.builder.String() != want {
		t.Fatalf("written = %q, want %q", w.builder.String(), want)
	}
	if w.flushes != 1 {
		t.Fatalf("flushes = %d, want 1", w.flushes)
	}
}

func TestWriteDonePreservesDoneMarker(t *testing.T) {
	t.Parallel()

	w := &flushRecorder{header: http.Header{}}
	if err := WriteDone(w); err != nil {
		t.Fatalf("WriteDone: %v", err)
	}
	if got := w.builder.String(); got != "data: [DONE]\n\n" {
		t.Fatalf("written = %q, want done event", got)
	}
}

func TestWriteDataSplitsMultilinePayload(t *testing.T) {
	t.Parallel()

	got := string(FormatData("hello\nworld"))
	want := "data: hello\ndata: world\n\n"
	if got != want {
		t.Fatalf("FormatData = %q, want %q", got, want)
	}
}

func TestSetHeaders(t *testing.T) {
	t.Parallel()

	h := http.Header{}
	SetHeaders(h)
	if h.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("content-type = %q", h.Get("Content-Type"))
	}
	if h.Get("Cache-Control") != "no-cache" {
		t.Fatalf("cache-control = %q", h.Get("Cache-Control"))
	}
}

type flushRecorder struct {
	header  http.Header
	builder strings.Builder
	flushes int
}

func (w *flushRecorder) Header() http.Header { return w.header }
func (w *flushRecorder) WriteHeader(int)     {}
func (w *flushRecorder) Write(p []byte) (int, error) {
	return w.builder.Write(p)
}
func (w *flushRecorder) Flush() { w.flushes++ }
