// Package config loads and validates the proxy's JSON configuration.
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
)

const (
	DefaultAddr               = "127.0.0.1:8080"
	DefaultReadTimeoutSeconds = 30
	DefaultIdleTimeoutSeconds = 120
	DefaultMaxBodyBytes       = 20 * 1024 * 1024
)

const (
	ClientAuthModeNone         = "none"
	ClientAuthModeStaticBearer = "static_bearer"
	ClientAuthModeBearerTokens = "bearer_tokens"
)

const (
	ProviderTypeOpenAICompatible = "openai_compatible"
)

const (
	AuthTypeBearerEnv          = "bearer_env"
	AuthTypeEnvAccessToken     = "env_access_token"
	AuthTypeFileAccessToken    = "file_access_token"
	AuthTypeCommandAccessToken = "access_token_command"
)

const (
	SyntheticStrategyFirstAvailable     = "first_available"
	SyntheticStrategyLeastCostAvailable = "least_cost_available"
)

const (
	LoggingModeOff                      = "off"
	LoggingModeMetadataOnly             = "metadata_only"
	LoggingModeFullTranscript           = "full_transcript"
	LoggingModeFullTranscriptPerRequest = "full_transcript_per_request"
)

// Config is the root JSON configuration for the proxy.
type Config struct {
	Server          ServerConfig                    `json:"server"`
	ClientAuth      ClientAuthConfig                `json:"client_auth"`
	Providers       map[string]ProviderConfig       `json:"providers"`
	SyntheticModels map[string]SyntheticModelConfig `json:"synthetic_models"`
	Logging         LoggingConfig                   `json:"logging"`
}

// ServerConfig controls the local HTTP server.
type ServerConfig struct {
	Addr                string `json:"addr"`
	ReadTimeoutSeconds  int    `json:"read_timeout_seconds"`
	WriteTimeoutSeconds int    `json:"write_timeout_seconds"`
	IdleTimeoutSeconds  int    `json:"idle_timeout_seconds"`
	MaxBodyBytes        int64  `json:"max_body_bytes"`
}

// ClientAuthConfig controls authentication for clients calling the proxy.
type ClientAuthConfig struct {
	Mode      string `json:"mode"`
	TokensEnv string `json:"tokens_env"`
}

// ProviderConfig describes one upstream provider.
type ProviderConfig struct {
	Type    string                 `json:"type"`
	BaseURL string                 `json:"base_url"`
	Auth    AuthConfig             `json:"auth"`
	Models  map[string]ModelConfig `json:"models"`
}

// AuthConfig describes how to retrieve an upstream provider token.
type AuthConfig struct {
	Type                 string   `json:"type"`
	Env                  string   `json:"env,omitempty"`
	File                 string   `json:"file,omitempty"`
	Command              []string `json:"command,omitempty"`
	RefreshBeforeSeconds int      `json:"refresh_before_seconds,omitempty"`
}

// ModelConfig describes configured model capabilities.
type ModelConfig struct {
	Modalities []string `json:"modalities"`
}

// SyntheticModelConfig describes an exposed model alias and fallback pool.
type SyntheticModelConfig struct {
	Strategy   string   `json:"strategy"`
	Expose     bool     `json:"expose"`
	Candidates []string `json:"candidates"`
}

// LoggingConfig controls request logging.
type LoggingConfig struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"`
	Path    string `json:"path"`
}

// Default returns a local-development configuration that can run health endpoints
// without requiring any upstream providers.
func Default() Config {
	return Config{
		Server: ServerConfig{
			Addr:                DefaultAddr,
			ReadTimeoutSeconds:  DefaultReadTimeoutSeconds,
			WriteTimeoutSeconds: 0,
			IdleTimeoutSeconds:  DefaultIdleTimeoutSeconds,
			MaxBodyBytes:        DefaultMaxBodyBytes,
		},
		ClientAuth: ClientAuthConfig{
			Mode: ClientAuthModeNone,
		},
		Providers:       map[string]ProviderConfig{},
		SyntheticModels: map[string]SyntheticModelConfig{},
		Logging: LoggingConfig{
			Enabled: false,
			Mode:    LoggingModeOff,
		},
	}
}

// Load reads a JSON config file from path. An empty path returns validated local defaults.
func Load(path string) (*Config, error) {
	cfg := Default()
	if path == "" {
		return &cfg, cfg.Validate()
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode config JSON: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode config JSON: multiple JSON values are not allowed")
		}
		return nil, fmt.Errorf("decode config JSON: %w", err)
	}

	ensureMaps(&cfg)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Validate returns all known configuration problems without including secret values.
func (c Config) Validate() error {
	var problems []string

	if c.Server.Addr == "" {
		problems = append(problems, "server.addr is required")
	}
	if c.Server.ReadTimeoutSeconds < 0 {
		problems = append(problems, "server.read_timeout_seconds must be non-negative")
	}
	if c.Server.WriteTimeoutSeconds < 0 {
		problems = append(problems, "server.write_timeout_seconds must be non-negative")
	}
	if c.Server.IdleTimeoutSeconds < 0 {
		problems = append(problems, "server.idle_timeout_seconds must be non-negative")
	}
	if c.Server.MaxBodyBytes <= 0 {
		problems = append(problems, "server.max_body_bytes must be positive")
	}

	mode := c.ClientAuth.Mode
	if mode == "" {
		mode = ClientAuthModeNone
	}
	switch mode {
	case ClientAuthModeNone:
	case ClientAuthModeStaticBearer, ClientAuthModeBearerTokens:
		if c.ClientAuth.TokensEnv == "" {
			problems = append(problems, "client_auth.tokens_env is required for bearer client auth")
		}
	default:
		problems = append(problems, "client_auth.mode must be one of: none, static_bearer, bearer_tokens")
	}

	for providerName, provider := range c.Providers {
		path := "providers." + providerName
		if strings.TrimSpace(providerName) == "" {
			problems = append(problems, "provider names must not be empty")
		}
		if provider.Type == "" {
			problems = append(problems, path+".type is required")
		} else if provider.Type != ProviderTypeOpenAICompatible {
			problems = append(problems, path+".type must be openai_compatible")
		}
		if provider.BaseURL == "" {
			problems = append(problems, path+".base_url is required")
		} else if !validHTTPURL(provider.BaseURL) {
			problems = append(problems, path+".base_url must be an absolute http(s) URL")
		}
		problems = append(problems, validateProviderAuth(path+".auth", provider.Auth)...)
		if len(provider.Models) == 0 {
			problems = append(problems, path+".models must contain at least one model")
		}
		for modelName, model := range provider.Models {
			modelPath := path + ".models." + modelName
			if strings.TrimSpace(modelName) == "" {
				problems = append(problems, path+".models contains an empty model name")
			}
			if len(model.Modalities) == 0 {
				problems = append(problems, modelPath+".modalities must contain at least one modality")
			}
			for _, modality := range model.Modalities {
				if modality != "text" && modality != "image" {
					problems = append(problems, modelPath+".modalities may only contain text or image")
				}
			}
		}
	}

	for alias, synthetic := range c.SyntheticModels {
		path := "synthetic_models." + alias
		if strings.TrimSpace(alias) == "" {
			problems = append(problems, "synthetic model names must not be empty")
		}
		switch synthetic.Strategy {
		case SyntheticStrategyFirstAvailable, SyntheticStrategyLeastCostAvailable:
		case "":
			problems = append(problems, path+".strategy is required")
		default:
			problems = append(problems, path+".strategy must be one of: first_available, least_cost_available")
		}
		if len(synthetic.Candidates) == 0 {
			problems = append(problems, path+".candidates must contain at least one provider:model reference")
		}
		for i, candidate := range synthetic.Candidates {
			if !c.validCandidate(candidate) {
				problems = append(problems, fmt.Sprintf("%s.candidates[%d] must reference an existing provider:model", path, i))
			}
		}
	}

	if c.Logging.Mode == "" {
		if c.Logging.Enabled {
			problems = append(problems, "logging.mode is required when logging is enabled")
		}
	} else {
		switch c.Logging.Mode {
		case LoggingModeOff, LoggingModeMetadataOnly, LoggingModeFullTranscript, LoggingModeFullTranscriptPerRequest:
		default:
			problems = append(problems, "logging.mode must be one of: off, metadata_only, full_transcript, full_transcript_per_request")
		}
	}
	if c.Logging.Enabled && c.Logging.Mode != LoggingModeOff && c.Logging.Path == "" {
		problems = append(problems, "logging.path is required when logging is enabled")
	}

	if len(problems) > 0 {
		return ValidationError{Problems: problems}
	}
	return nil
}

// ValidationError groups all discovered config validation problems.
type ValidationError struct {
	Problems []string
}

func (e ValidationError) Error() string {
	return "invalid config: " + strings.Join(e.Problems, "; ")
}

func ensureMaps(cfg *Config) {
	if cfg.Providers == nil {
		cfg.Providers = map[string]ProviderConfig{}
	}
	if cfg.SyntheticModels == nil {
		cfg.SyntheticModels = map[string]SyntheticModelConfig{}
	}
}

func validateProviderAuth(path string, auth AuthConfig) []string {
	var problems []string
	if auth.Type == "" {
		return append(problems, path+".type is required")
	}
	switch auth.Type {
	case AuthTypeBearerEnv, AuthTypeEnvAccessToken:
		if auth.Env == "" {
			problems = append(problems, path+".env is required for env-based auth")
		}
	case AuthTypeFileAccessToken:
		if auth.File == "" {
			problems = append(problems, path+".file is required for file access-token auth")
		}
	case AuthTypeCommandAccessToken:
		if len(auth.Command) == 0 {
			problems = append(problems, path+".command is required for command access-token auth")
		}
		if auth.RefreshBeforeSeconds < 0 {
			problems = append(problems, path+".refresh_before_seconds must be non-negative")
		}
	default:
		problems = append(problems, path+".type must be a supported provider auth type")
	}
	return problems
}

func validHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return u.IsAbs() && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https")
}

func (c Config) validCandidate(candidate string) bool {
	providerName, modelName, ok := strings.Cut(candidate, ":")
	if !ok || providerName == "" || modelName == "" {
		return false
	}
	provider, ok := c.Providers[providerName]
	if !ok {
		return false
	}
	_, ok = provider.Models[modelName]
	return ok
}
