package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load default: %v", err)
	}

	if cfg.Server.Addr != DefaultAddr {
		t.Fatalf("Server.Addr = %q, want %q", cfg.Server.Addr, DefaultAddr)
	}
	if cfg.Server.MaxBodyBytes != DefaultMaxBodyBytes {
		t.Fatalf("Server.MaxBodyBytes = %d, want %d", cfg.Server.MaxBodyBytes, DefaultMaxBodyBytes)
	}
	if cfg.ClientAuth.Mode != ClientAuthModeNone {
		t.Fatalf("ClientAuth.Mode = %q, want %q", cfg.ClientAuth.Mode, ClientAuthModeNone)
	}
	if len(cfg.Providers) != 0 {
		t.Fatalf("Providers length = %d, want 0", len(cfg.Providers))
	}
}

func TestLoadValidConfig(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `{
		"server": {
			"addr": "127.0.0.1:9090",
			"read_timeout_seconds": 15,
			"write_timeout_seconds": 0,
			"idle_timeout_seconds": 60,
			"max_body_bytes": 1048576
		},
		"client_auth": {
			"mode": "static_bearer",
			"tokens_env": "LLM_PROXY_CLIENT_TOKENS"
		},
		"providers": {
			"openai_api": {
				"type": "openai_compatible",
				"base_url": "https://api.openai.com/v1",
				"auth": {
					"type": "bearer_env",
					"env": "OPENAI_API_KEY"
				},
				"models": {
					"gpt-5.2": {
						"modalities": ["text", "image"]
					}
				}
			}
		},
		"synthetic_models": {
			"coding": {
				"strategy": "first_available",
				"expose": true,
				"candidates": ["openai_api:gpt-5.2"]
			}
		},
		"logging": {
			"enabled": true,
			"mode": "metadata_only",
			"path": "./llm-proxy.jsonl"
		}
	}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load valid config: %v", err)
	}

	if cfg.Server.Addr != "127.0.0.1:9090" {
		t.Fatalf("Server.Addr = %q", cfg.Server.Addr)
	}
	if got := cfg.Providers["openai_api"].Models["gpt-5.2"].Modalities; len(got) != 2 || got[0] != "text" || got[1] != "image" {
		t.Fatalf("modalities = %#v", got)
	}
	if !cfg.SyntheticModels["coding"].Expose {
		t.Fatal("synthetic coding model should be exposed")
	}
}

func TestValidateReportsMissingRequiredProviderFields(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Providers["broken"] = ProviderConfig{}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate returned nil, want errors")
	}

	msg := err.Error()
	for _, want := range []string{
		"providers.broken.type is required",
		"providers.broken.base_url is required",
		"providers.broken.auth.type is required",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("validation error %q missing %q", msg, want)
		}
	}
}

func TestValidateReportsInvalidSyntheticCandidateReference(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Providers["openai_api"] = ProviderConfig{
		Type:    ProviderTypeOpenAICompatible,
		BaseURL: "https://api.openai.com/v1",
		Auth: AuthConfig{
			Type: AuthTypeBearerEnv,
			Env:  "OPENAI_API_KEY",
		},
		Models: map[string]ModelConfig{
			"gpt-5.2": {Modalities: []string{"text"}},
		},
	}
	cfg.SyntheticModels["coding"] = SyntheticModelConfig{
		Strategy:   SyntheticStrategyFirstAvailable,
		Candidates: []string{"openai_api:missing-model"},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate returned nil, want invalid candidate error")
	}
	if !strings.Contains(err.Error(), "synthetic_models.coding.candidates references unknown model") {
		t.Fatalf("validation error = %q", err.Error())
	}
}

func TestValidateAllowsDynamicProviderModelsForSyntheticCandidates(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Providers["openai_api"] = ProviderConfig{
		Type:    ProviderTypeOpenAICompatible,
		BaseURL: "https://api.openai.com/v1",
		Auth: AuthConfig{
			Type:  AuthTypeBearerValue,
			Value: "sk-inline-secret",
		},
	}
	cfg.SyntheticModels["coding"] = SyntheticModelConfig{
		Strategy:   SyntheticStrategyFirstAvailable,
		Candidates: []string{"openai_api:gpt-dynamic"},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate dynamic models: %v", err)
	}
}

func TestValidationErrorsDoNotLeakSecretLikeValues(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.ClientAuth.Mode = "super-secret-mode"
	cfg.Providers["openai_api"] = ProviderConfig{
		Type:    "super-secret-provider-type",
		BaseURL: "https://example.invalid/v1/super-secret-url-token",
		Auth: AuthConfig{
			Type: "super-secret-auth-type",
			Env:  "super-secret-env-value",
		},
		Models: map[string]ModelConfig{
			"gpt-test": {Modalities: []string{"super-secret-modality"}},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate returned nil, want errors")
	}

	msg := err.Error()
	for _, secret := range []string{
		"super-secret-mode",
		"super-secret-provider-type",
		"super-secret-url-token",
		"super-secret-auth-type",
		"super-secret-env-value",
		"super-secret-modality",
	} {
		if strings.Contains(msg, secret) {
			t.Fatalf("validation error leaked %q in %q", secret, msg)
		}
	}
}

func TestLoadAcceptsBearerValueAndInvasiveLogging(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `{
		"providers": {
			"inline": {
				"type": "openai_compatible",
				"base_url": "https://example.invalid/v1",
				"auth": { "type": "bearer_val", "value": "sk-inline-secret" }
			}
		},
		"logging": {
			"enabled": true,
			"mode": "invasive",
			"path": "./proxy.jsonl"
		}
	}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load bearer_val config: %v", err)
	}
	if cfg.Providers["inline"].Auth.Type != AuthTypeBearerValue || cfg.Providers["inline"].Auth.Value != "sk-inline-secret" {
		t.Fatalf("inline auth = %#v", cfg.Providers["inline"].Auth)
	}
	if cfg.Logging.Mode != LoggingModeInvasive {
		t.Fatalf("logging mode = %q, want %q", cfg.Logging.Mode, LoggingModeInvasive)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `{
		"server": {
			"addr": "127.0.0.1:9090",
			"unknown_field": true
		}
	}`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load returned nil error for unknown field")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Load error = %q, want unknown field", err.Error())
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}
