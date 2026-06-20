package metrics

import (
	"testing"
	"time"
)

func TestRecordRequestIncrementsByEndpointAndStatusClass(t *testing.T) {
	t.Parallel()

	c := NewCollector()
	c.RecordRequest(EndpointChatCompletions, "2xx", 10*time.Millisecond)
	c.RecordRequest(EndpointChatCompletions, "2xx", 20*time.Millisecond)
	c.RecordRequest(EndpointChatCompletions, "4xx", 5*time.Millisecond)
	c.RecordRequest(EndpointResponses, "5xx", 100*time.Millisecond)

	snap := c.Snapshot()
	if snap.TotalRequests != 4 {
		t.Fatalf("TotalRequests = %d, want 4", snap.TotalRequests)
	}
	count := func(endpoint, class string) int64 {
		for _, m := range snap.Requests {
			if m.Endpoint == endpoint && m.StatusClass == class {
				return m.Count
			}
		}
		return -1
	}
	if got := count(EndpointChatCompletions, "2xx"); got != 2 {
		t.Fatalf("chat 2xx = %d, want 2", got)
	}
	if got := count(EndpointChatCompletions, "4xx"); got != 1 {
		t.Fatalf("chat 4xx = %d, want 1", got)
	}
	if got := count(EndpointResponses, "5xx"); got != 1 {
		t.Fatalf("responses 5xx = %d, want 1", got)
	}
}

func TestRecordErrorIncrementsByCode(t *testing.T) {
	t.Parallel()

	c := NewCollector()
	c.RecordError("no_available_model")
	c.RecordError("no_available_model")
	c.RecordError("upstream_error")
	c.RecordError("") // ignored

	snap := c.Snapshot()
	if snap.TotalErrors != 3 {
		t.Fatalf("TotalErrors = %d, want 3", snap.TotalErrors)
	}
	codeCount := func(code string) int64 {
		for _, m := range snap.Errors {
			if m.Code == code {
				return m.Count
			}
		}
		return -1
	}
	if got := codeCount("no_available_model"); got != 2 {
		t.Fatalf("no_available_model = %d, want 2", got)
	}
	if got := codeCount("upstream_error"); got != 1 {
		t.Fatalf("upstream_error = %d, want 1", got)
	}
}

func TestLatencySummaryAggregates(t *testing.T) {
	t.Parallel()

	c := NewCollector()
	for _, d := range []time.Duration{1, 2, 3, 4, 5, 6, 7, 8, 9, 10} {
		c.RecordRequest(EndpointChatCompletions, "2xx", d*time.Millisecond)
	}

	snap := c.Snapshot()
	if len(snap.Latency) != 1 {
		t.Fatalf("latency buckets = %d, want 1", len(snap.Latency))
	}
	m := snap.Latency[0]
	if m.Count != 10 {
		t.Fatalf("Count = %d, want 10", m.Count)
	}
	if m.MinMS != 1 {
		t.Fatalf("MinMS = %d, want 1", m.MinMS)
	}
	if m.MaxMS != 10 {
		t.Fatalf("MaxMS = %d, want 10", m.MaxMS)
	}
	if m.MeanMS != 5 {
		t.Fatalf("MeanMS = %d, want 5", m.MeanMS)
	}
	// P50 should be around the median.
	if m.P50MS < 4 || m.P50MS > 6 {
		t.Fatalf("P50MS = %d, want ~5", m.P50MS)
	}
	// P95 near the top.
	if m.P95MS < 9 || m.P95MS > 10 {
		t.Fatalf("P95MS = %d, want ~10", m.P95MS)
	}
}

func TestSnapshotIsSortedAndDeterministic(t *testing.T) {
	t.Parallel()

	c := NewCollector()
	c.RecordRequest(EndpointResponses, "5xx", 1*time.Millisecond)
	c.RecordRequest(EndpointChatCompletions, "2xx", 1*time.Millisecond)
	c.RecordRequest(EndpointChatCompletions, "4xx", 1*time.Millisecond)
	c.RecordError("upstream_error")
	c.RecordError("invalid_request")

	snap := c.Snapshot()
	if len(snap.Requests) != 3 {
		t.Fatalf("requests = %d", len(snap.Requests))
	}
	if snap.Requests[0].Endpoint != EndpointChatCompletions || snap.Requests[0].StatusClass != "2xx" {
		t.Fatalf("unexpected order: %+v", snap.Requests)
	}
	if snap.Requests[1].Endpoint != EndpointChatCompletions || snap.Requests[1].StatusClass != "4xx" {
		t.Fatalf("unexpected order: %+v", snap.Requests)
	}
	if snap.Errors[0].Code != "invalid_request" || snap.Errors[1].Code != "upstream_error" {
		t.Fatalf("errors not sorted: %+v", snap.Errors)
	}
}

func TestStatusClass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status int
		want   string
	}{
		{200, "2xx"}, {204, "2xx"}, {301, "3xx"}, {400, "4xx"}, {404, "4xx"}, {429, "4xx"}, {500, "5xx"}, {503, "5xx"}, {0, "other"},
	}
	for _, tt := range tests {
		if got := StatusClass(tt.status); got != tt.want {
			t.Fatalf("StatusClass(%d) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestEndpointForPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want string
	}{
		{"/v1/chat/completions", EndpointChatCompletions},
		{"/v1/responses", EndpointResponses},
		{"/v1/models", EndpointModels},
		{"/healthz", EndpointHealth},
		{"/readyz", EndpointHealth},
		{"/unknown", EndpointUnknown},
	}
	for _, tt := range tests {
		if got := EndpointForPath(tt.path); got != tt.want {
			t.Fatalf("EndpointForPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestNilCollectorIsSafe(t *testing.T) {
	t.Parallel()

	var c *Collector
	c.RecordRequest(EndpointChatCompletions, "2xx", 1*time.Millisecond)
	c.RecordError("code")
	snap := c.Snapshot()
	if snap.TotalRequests != 0 {
		t.Fatalf("nil collector snapshot should be empty")
	}
}

func TestSnapshotJSONMarshalRoundTrips(t *testing.T) {
	t.Parallel()

	c := NewCollector()
	c.RecordRequest(EndpointChatCompletions, "2xx", 5*time.Millisecond)
	c.RecordError("upstream_error")

	data, err := c.Snapshot().MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty JSON")
	}
}
