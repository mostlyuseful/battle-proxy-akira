// Package sse provides small Server-Sent Events helpers for OpenAI-compatible streams.
package sse

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	// DoneData is the OpenAI Chat Completions stream terminator payload.
	DoneData = "[DONE]"
)

// Event is a parsed SSE event. For OpenAI-compatible pass-through streams, Data is
// the only field most callers need.
type Event struct {
	Data string
}

// IsDone reports whether this event is the OpenAI-compatible [DONE] marker.
func (e Event) IsDone() bool {
	return e.Data == DoneData
}

// Reader parses SSE events from an upstream response body.
type Reader struct {
	r *bufio.Reader
}

// NewReader creates an SSE reader around r.
func NewReader(r io.Reader) *Reader {
	return &Reader{r: bufio.NewReader(r)}
}

// Read reads the next SSE event containing data: fields. Comments, blank events,
// and non-data fields are ignored. Multiple data: fields in one SSE event are
// joined with a newline according to SSE conventions.
func (r *Reader) Read() (Event, error) {
	var data []string
	for {
		line, err := r.r.ReadString('\n')
		if err != nil && len(line) == 0 {
			if err == io.EOF && len(data) > 0 {
				return Event{Data: strings.Join(data, "\n")}, nil
			}
			return Event{}, err
		}

		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		if line == "" {
			if len(data) == 0 {
				if err != nil {
					return Event{}, err
				}
				continue
			}
			return Event{Data: strings.Join(data, "\n")}, nil
		}

		if strings.HasPrefix(line, ":") {
			if err != nil {
				return Event{}, err
			}
			continue
		}

		if field, value, ok := strings.Cut(line, ":"); ok && field == "data" {
			value = strings.TrimPrefix(value, " ")
			data = append(data, value)
		} else if line == "data" {
			data = append(data, "")
		}

		if err != nil {
			if err == io.EOF && len(data) > 0 {
				return Event{Data: strings.Join(data, "\n")}, nil
			}
			return Event{}, err
		}
	}
}

// ReadAll reads all SSE data events until EOF.
func ReadAll(r io.Reader) ([]Event, error) {
	reader := NewReader(r)
	var events []Event
	for {
		event, err := reader.Read()
		if err == io.EOF {
			return events, nil
		}
		if err != nil {
			return events, err
		}
		events = append(events, event)
	}
}

// WriteData writes one data SSE event and flushes when w supports http.Flusher.
func WriteData(w io.Writer, data string) error {
	if data == "" {
		if _, err := io.WriteString(w, "data: \n\n"); err != nil {
			return err
		}
	} else {
		lines := strings.Split(data, "\n")
		for _, line := range lines {
			if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
	}

	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

// WriteEvent writes event.Data as one data SSE event.
func WriteEvent(w io.Writer, event Event) error {
	return WriteData(w, event.Data)
}

// WriteDone writes the OpenAI-compatible stream terminator event.
func WriteDone(w io.Writer) error {
	return WriteData(w, DoneData)
}

// FormatData returns the bytes that WriteData would write, without flushing.
func FormatData(data string) []byte {
	var buf bytes.Buffer
	_ = WriteData(&buf, data)
	return buf.Bytes()
}

// SetHeaders applies common SSE response headers.
func SetHeaders(h http.Header) {
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
}
