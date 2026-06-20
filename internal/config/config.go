// Package config loads and validates the proxy's JSON configuration.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	AuthTypeBearerValue        = "bearer_val"
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
	LoggingModeInvasive                 = "invasive"
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
	Value                string   `json:"value,omitempty"`
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

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	raw = stripJSONComments(raw)

	dec := json.NewDecoder(bytes.NewReader(raw))
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

// LogVerboseDiagnostics emits non-sensitive configuration merge/default messages.
func (c Config) LogVerboseDiagnostics(logger *slog.Logger, path string) {
	if logger == nil {
		return
	}
	if path == "" {
		logger.Info("using default configuration", "config_path", "")
		return
	}
	logger.Info("loaded configuration file", "config_path", path)
	if c.Server.Addr == "" {
		logger.Info("default server address used", "addr", DefaultAddr)
	}
	if c.Server.ReadTimeoutSeconds == 0 {
		logger.Info("default server read timeout used", "read_timeout_seconds", DefaultReadTimeoutSeconds)
	}
	if c.Server.IdleTimeoutSeconds == 0 {
		logger.Info("default server idle timeout used", "idle_timeout_seconds", DefaultIdleTimeoutSeconds)
	}
	if c.Server.MaxBodyBytes == 0 {
		logger.Info("default max body bytes used", "max_body_bytes", DefaultMaxBodyBytes)
	}
	if c.ClientAuth.Mode == "" {
		logger.Info("default client auth mode used", "mode", ClientAuthModeNone)
	}
	if c.Logging.Mode == "" {
		logger.Info("default logging mode used", "mode", LoggingModeOff)
	}
	if len(c.Providers) == 0 {
		logger.Info("no upstream providers configured")
	}
	if len(c.SyntheticModels) == 0 {
		logger.Info("no synthetic models configured")
	}
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
		default:
			problems = append(problems, path+".strategy must be first_available or least_cost_available")
		}
		if len(synthetic.Candidates) == 0 {
			problems = append(problems, path+".candidates must contain at least one candidate")
		}
		for _, candidate := range synthetic.Candidates {
			providerName, providerModel, ok := strings.Cut(candidate, ":")
			if !ok || strings.TrimSpace(providerName) == "" || strings.TrimSpace(providerModel) == "" {
				problems = append(problems, path+".candidates contains invalid provider:model reference "+candidate)
				continue
			}
			providerCfg, ok := c.Providers[providerName]
			if !ok {
				problems = append(problems, path+".candidates references unknown provider "+providerName)
			} else if len(providerCfg.Models) > 0 {
				if _, ok := providerCfg.Models[providerModel]; !ok {
					problems = append(problems, path+".candidates references unknown model "+providerModel+" for provider "+providerName)
				}
			}
		}
	}

	switch c.Logging.Mode {
	case "", LoggingModeOff, LoggingModeMetadataOnly, LoggingModeInvasive, LoggingModeFullTranscript, LoggingModeFullTranscriptPerRequest:
	default:
		problems = append(problems, "logging.mode must be one of: off, metadata_only, invasive, full_transcript, full_transcript_per_request")
	}
	if c.Logging.Enabled && c.Logging.Mode == "" {
		problems = append(problems, "logging.mode is required when logging is enabled")
	}
	if c.Logging.Enabled && c.Logging.Path == "" {
		problems = append(problems, "logging.path is required when logging is enabled")
	}

	if len(problems) > 0 {
		return ValidationError{Problems: problems}
	}
	return nil
}

func ensureMaps(c *Config) {
	if c.Providers == nil {
		c.Providers = map[string]ProviderConfig{}
	}
	if c.SyntheticModels == nil {
		c.SyntheticModels = map[string]SyntheticModelConfig{}
	}
}

func validateProviderAuth(path string, auth AuthConfig) []string {
	var problems []string
	if auth.Type == "" {
		return append(problems, path+".type is required")
	}
	switch auth.Type {
	case AuthTypeBearerEnv:
		if auth.Env == "" {
			problems = append(problems, path+".env is required for bearer_env auth")
		}
	case AuthTypeBearerValue:
		if strings.TrimSpace(auth.Value) == "" {
			problems = append(problems, path+".value is required for bearer_val auth")
		}
	case AuthTypeEnvAccessToken:
		if auth.Env == "" {
			problems = append(problems, path+".env is required for env_access_token auth")
		}
	case AuthTypeFileAccessToken:
		if auth.File == "" {
			problems = append(problems, path+".file is required for file_access_token auth")
		}
	case AuthTypeCommandAccessToken:
		if len(normalizeCommand(auth.Command)) == 0 {
			problems = append(problems, path+".command is required for access_token_command auth")
		}
		if auth.RefreshBeforeSeconds < 0 {
			problems = append(problems, path+".refresh_before_seconds must be non-negative")
		}
	default:
		problems = append(problems, path+".type must be one of: bearer_env, bearer_val, env_access_token, file_access_token, access_token_command")
	}
	return problems
}

func normalizeCommand(command []string) []string {
	out := make([]string, 0, len(command))
	for _, arg := range command {
		arg = strings.TrimSpace(arg)
		if arg != "" {
			out = append(out, arg)
		}
	}
	return out
}

func validHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// ValidationError groups all discovered config validation problems.
type ValidationError struct {
	Problems []string
}

func (e ValidationError) Error() string {
	return "invalid config: " + strings.Join(e.Problems, "; ")
}
