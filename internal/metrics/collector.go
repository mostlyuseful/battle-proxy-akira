// Package metrics provides a small, dependency-free runtime metrics collector
// for request counts, error counts, and latency summaries. It uses stdlib
// synchronization primitives and is safe for concurrent use.
package metrics

import (
	"encoding/json"
	"math"
	"sort"
	"sync"
	"time"
)

// Endpoint label constants for known proxy endpoints.
const (
	EndpointChatCompletions = "chat_completions"
	EndpointResponses       = "responses"
	EndpointModels          = "models"
	EndpointHealth          = "health"
	EndpointUnknown         = "unknown"
)

// EndpointForPath maps a request path to a stable metrics endpoint label.
func EndpointForPath(path string) string {
	switch path {
	case "/v1/chat/completions":
		return EndpointChatCompletions
	case "/v1/responses":
		return EndpointResponses
	case "/v1/models":
		return EndpointModels
	case "/healthz", "/readyz":
		return EndpointHealth
	default:
		return EndpointUnknown
	}
}

// StatusClass maps an HTTP status code to a coarse class label.
func StatusClass(status int) string {
	switch {
	case status >= 200 && status < 300:
		return "2xx"
	case status >= 300 && status < 400:
		return "3xx"
	case status >= 400 && status < 500:
		return "4xx"
	case status >= 500:
		return "5xx"
	default:
		return "other"
	}
}

// requestKey identifies one endpoint/status-class bucket.
type requestKey struct {
	endpoint    string
	statusClass string
}

// latencySummary is an aggregate latency summary for one bucket.
type latencySummary struct {
	count int64
	sum   time.Duration
	min   time.Duration
	max   time.Duration
}

// recentRing is a fixed-capacity ring buffer of recent latencies for percentile
// estimation. It trades a little memory for O(1) inserts.
type recentRing struct {
	samples []time.Duration
	size    int
	next    int
	full    bool
}

func newRecentRing(size int) *recentRing {
	if size <= 0 {
		size = 128
	}
	return &recentRing{samples: make([]time.Duration, size), size: size}
}

func (r *recentRing) add(d time.Duration) {
	r.samples[r.next] = d
	r.next = (r.next + 1) % r.size
	if r.next == 0 {
		r.full = true
	}
}

func (r *recentRing) values() []time.Duration {
	n := r.next
	if r.full {
		n = r.size
	}
	out := make([]time.Duration, n)
	copy(out, r.samples[:n])
	return out
}

// Collector records runtime counters and latency summaries.
type Collector struct {
	mu              sync.Mutex
	requests        map[requestKey]int64
	errors          map[string]int64
	latencyByBucket map[requestKey]*latencySummary
	recentByBucket  map[requestKey]*recentRing
	startedAt       time.Time
}

// NewCollector creates an empty collector.
func NewCollector() *Collector {
	return &Collector{
		requests:        map[requestKey]int64{},
		errors:          map[string]int64{},
		latencyByBucket: map[requestKey]*latencySummary{},
		recentByBucket:  map[requestKey]*recentRing{},
		startedAt:       time.Now(),
	}
}

// RecordRequest increments the request counter for one endpoint/status-class
// bucket and updates its latency summary.
func (c *Collector) RecordRequest(endpoint, statusClass string, latency time.Duration) {
	if c == nil {
		return
	}
	key := requestKey{endpoint: endpoint, statusClass: statusClass}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests[key]++
	summary := c.latencyByBucket[key]
	if summary == nil {
		summary = &latencySummary{min: math.MaxInt64}
		c.latencyByBucket[key] = summary
	}
	summary.count++
	summary.sum += latency
	if latency < summary.min {
		summary.min = latency
	}
	if latency > summary.max {
		summary.max = latency
	}
	ring := c.recentByBucket[key]
	if ring == nil {
		ring = newRecentRing(128)
		c.recentByBucket[key] = ring
	}
	ring.add(latency)
}

// RecordError increments the error counter for one internal error code.
func (c *Collector) RecordError(code string) {
	if c == nil || code == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errors[code]++
}

// Snapshot is a point-in-time view of collected metrics for JSON exposition.
type Snapshot struct {
	StartedAt       time.Time         `json:"started_at"`
	GeneratedAt     time.Time         `json:"generated_at"`
	Requests        []RequestMetric   `json:"requests"`
	Errors          []ErrorMetric     `json:"errors,omitempty"`
	Latency         []LatencyMetric   `json:"latency"`
	TotalRequests   int64             `json:"total_requests"`
	TotalErrors     int64             `json:"total_errors"`
}

// RequestMetric is one endpoint/status-class request count bucket.
type RequestMetric struct {
	Endpoint    string `json:"endpoint"`
	StatusClass string `json:"status_class"`
	Count       int64  `json:"count"`
}

// ErrorMetric is one internal error code count.
type ErrorMetric struct {
	Code  string `json:"code"`
	Count int64  `json:"count"`
}

// LatencyMetric is the aggregate latency summary for one bucket.
type LatencyMetric struct {
	Endpoint    string `json:"endpoint"`
	StatusClass string `json:"status_class"`
	Count       int64  `json:"count"`
	SumMS       int64  `json:"sum_ms"`
	MinMS       int64  `json:"min_ms"`
	MaxMS       int64  `json:"max_ms"`
	MeanMS      int64  `json:"mean_ms"`
	P50MS       int64  `json:"p50_ms"`
	P95MS       int64  `json:"p95_ms"`
	P99MS       int64  `json:"p99_ms"`
}

// Snapshot returns a point-in-time copy of all collected metrics.
func (c *Collector) Snapshot() Snapshot {
	if c == nil {
		return Snapshot{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	snap := Snapshot{
		StartedAt:   c.startedAt,
		GeneratedAt: time.Now(),
	}

	var totalReqs int64
	for key, count := range c.requests {
		snap.Requests = append(snap.Requests, RequestMetric{
			Endpoint:    key.endpoint,
			StatusClass: key.statusClass,
			Count:       count,
		})
		totalReqs += count
	}
	snap.TotalRequests = totalReqs

	var totalErrs int64
	for code, count := range c.errors {
		snap.Errors = append(snap.Errors, ErrorMetric{Code: code, Count: count})
		totalErrs += count
	}
	snap.TotalErrors = totalErrs

	for key, summary := range c.latencyByBucket {
		m := LatencyMetric{
			Endpoint:    key.endpoint,
			StatusClass: key.statusClass,
			Count:       summary.count,
			SumMS:       summary.sum.Milliseconds(),
		}
		if summary.min != math.MaxInt64 {
			m.MinMS = summary.min.Milliseconds()
		}
		m.MaxMS = summary.max.Milliseconds()
		if summary.count > 0 {
			m.MeanMS = (summary.sum / time.Duration(summary.count)).Milliseconds()
		}
		if ring := c.recentByBucket[key]; ring != nil {
			values := ring.values()
			if len(values) > 0 {
				m.P50MS = percentile(values, 0.50).Milliseconds()
				m.P95MS = percentile(values, 0.95).Milliseconds()
				m.P99MS = percentile(values, 0.99).Milliseconds()
			}
		}
		snap.Latency = append(snap.Latency, m)
	}

	sort.Slice(snap.Requests, func(i, j int) bool {
		if snap.Requests[i].Endpoint == snap.Requests[j].Endpoint {
			return snap.Requests[i].StatusClass < snap.Requests[j].StatusClass
		}
		return snap.Requests[i].Endpoint < snap.Requests[j].Endpoint
	})
	sort.Slice(snap.Errors, func(i, j int) bool { return snap.Errors[i].Code < snap.Errors[j].Code })
	sort.Slice(snap.Latency, func(i, j int) bool {
		if snap.Latency[i].Endpoint == snap.Latency[j].Endpoint {
			return snap.Latency[i].StatusClass < snap.Latency[j].StatusClass
		}
		return snap.Latency[i].Endpoint < snap.Latency[j].Endpoint
	})
	return snap
}

// MarshalJSON renders the snapshot as JSON.
func (s Snapshot) MarshalJSON() ([]byte, error) {
	type alias Snapshot
	return json.Marshal(alias(s))
}

// percentile returns the p-th percentile of a sample of durations using nearest-rank.
func percentile(values []time.Duration, p float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	// Nearest-rank method.
	rank := int(math.Ceil(p * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}
