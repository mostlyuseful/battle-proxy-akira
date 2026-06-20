package config

import (
	"strings"
	"testing"
)

func TestStripJSONCommentsNoComments(t *testing.T) {
	t.Parallel()

	in := []byte(`{"a":1,"b":"x"}`)
	out := stripJSONComments(in)
	if string(out) != string(in) {
		t.Fatalf("stripJSONComments(no comments) = %q, want %q", out, in)
	}
}

func TestStripJSONCommentsLineAndBlockComments(t *testing.T) {
	t.Parallel()

	in := []byte(`{
  // line comment
  "addr": "127.0.0.1:8080", /* inline */
  "name": "x" // trailing
}`)
	out := string(stripJSONComments(in))
	if strings.Contains(out, "//") {
		t.Fatalf("output still contains // : %q", out)
	}
	if strings.Contains(out, "/*") || strings.Contains(out, "*/") {
		t.Fatalf("output still contains block comment markers: %q", out)
	}
	if !strings.Contains(out, `"addr"`) || !strings.Contains(out, `"127.0.0.1:8080"`) {
		t.Fatalf("output missing expected fields: %q", out)
	}
}

func TestStripJSONCommentsPreservesCommentLikeContentInsideStrings(t *testing.T) {
	t.Parallel()

	in := []byte(`{"url":"https://example.com/path","note":"/* not a comment */","flag":"// also not"}`)
	out := stripJSONComments(in)
	if string(out) != string(in) {
		t.Fatalf("stripJSONComments altered strings: in=%q out=%q", in, out)
	}
}

func TestStripJSONCommentsHandlesEscapedQuotesInStrings(t *testing.T) {
	t.Parallel()

	in := []byte(`{"escaped":"he said \"// hi\"","value":1}`)
	out := stripJSONComments(in)
	if !strings.Contains(string(out), `"he said \"// hi\""`) {
		t.Fatalf("escaped string content lost: %q", out)
	}
	if strings.Contains(string(out), "/*") {
		t.Fatalf("unexpected block comment markers: %q", out)
	}
}

func TestStripJSONCommentsMultilineBlockComment(t *testing.T) {
	t.Parallel()

	in := []byte(`{"a":1, /* line1
line2
line3 */ "b":2}`)
	out := string(stripJSONComments(in))
	if strings.Contains(out, "line1") || strings.Contains(out, "line2") || strings.Contains(out, "line3") {
		t.Fatalf("block comment content not removed: %q", out)
	}
	if !strings.Contains(out, `"a":1`) || !strings.Contains(out, `"b":2`) {
		t.Fatalf("expected surrounding JSON to remain: %q", out)
	}
}

func TestLoadAcceptsCommentsAndBlockComments(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `{
		// listen on a local port
		"server": {
			"addr": "127.0.0.1:9090", /* override default */
			"max_body_bytes": 1048576
		},
		"providers": {
			"openai_api": {
				"type": "openai_compatible", // api-compatible upstream
				"base_url": "https://api.openai.com/v1",
				"auth": { "type": "bearer_env", "env": "OPENAI_API_KEY" },
				"models": {
					"gpt-5.2": { "modalities": ["text"] }
				}
			}
		}
	}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load with comments failed: %v", err)
	}
	if cfg.Server.Addr != "127.0.0.1:9090" {
		t.Fatalf("Server.Addr = %q, want %q", cfg.Server.Addr, "127.0.0.1:9090")
	}
	if cfg.Providers["openai_api"].Type != ProviderTypeOpenAICompatible {
		t.Fatalf("provider type = %q", cfg.Providers["openai_api"].Type)
	}
}

func TestLoadPreservesURLsContainingCommentMarkers(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `{
		"providers": {
			"commented": {
				"type": "openai_compatible",
				"base_url": "https://example.com/v1/path",
				"auth": { "type": "bearer_env", "env": "X" },
				"models": { "m": { "modalities": ["text"] } }
			}
		}
	}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Providers["commented"].BaseURL != "https://example.com/v1/path" {
		t.Fatalf("base_url = %q", cfg.Providers["commented"].BaseURL)
	}
}
