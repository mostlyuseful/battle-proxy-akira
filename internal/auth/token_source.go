// Package auth provides upstream provider token sources.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"battle-proxy-akira/internal/config"
)

var (
	// ErrUnsupportedAuthType indicates that no token source exists for the configured auth type.
	ErrUnsupportedAuthType = errors.New("unsupported provider auth type")
	// ErrMissingToken indicates that a configured token source did not produce a token.
	ErrMissingToken = errors.New("provider token is not configured")
	// ErrMalformedToken indicates that a configured source returned malformed token data.
	ErrMalformedToken = errors.New("provider token data is malformed")
)

// TokenSource retrieves an upstream provider bearer token.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// BearerEnvTokenSource reads a provider API key from an environment variable.
type BearerEnvTokenSource struct {
	env    string
	lookup func(string) (string, bool)
	logger *slog.Logger
}

// AccessTokenEnvSource reads an OAuth/access-token style bearer token from an environment variable.
type AccessTokenEnvSource struct {
	env    string
	lookup func(string) (string, bool)
	logger *slog.Logger
}

// AccessTokenFileSource reads an OAuth/access-token style bearer token from a local file.
type AccessTokenFileSource struct {
	file     string
	readFile func(string) ([]byte, error)
	logger   *slog.Logger
}

// AccessTokenCommandSource retrieves an OAuth/access-token style bearer token from a command.
type AccessTokenCommandSource struct {
	command       []string
	refreshBefore time.Duration
	run           func(context.Context, []string) ([]byte, error)
	now           func() time.Time
	mu            sync.Mutex
	cached        cachedAccessToken
	logger        *slog.Logger
}

type cachedAccessToken struct {
	token     string
	expiresAt time.Time
}

// NewBearerEnvTokenSource creates a token source for bearer_env provider auth.
func NewBearerEnvTokenSource(env string) (*BearerEnvTokenSource, error) {
	return NewBearerEnvTokenSourceWithLogger(env, nil)
}

// NewBearerEnvTokenSourceWithLogger creates a token source with optional verbose diagnostics.
func NewBearerEnvTokenSourceWithLogger(env string, logger *slog.Logger) (*BearerEnvTokenSource, error) {
	env = strings.TrimSpace(env)
	if env == "" {
		return nil, fmt.Errorf("bearer_env auth requires env name: %w", ErrMissingToken)
	}
	if logger != nil {
		logger.Info("bearer_env token source configured", "env", env)
	}
	return &BearerEnvTokenSource{env: env, lookup: os.LookupEnv, logger: logger}, nil
}

// NewAccessTokenEnvSource creates a token source for env_access_token provider auth.
func NewAccessTokenEnvSource(env string) (*AccessTokenEnvSource, error) {
	return NewAccessTokenEnvSourceWithLogger(env, nil)
}

// NewAccessTokenEnvSourceWithLogger creates a token source with optional verbose diagnostics.
func NewAccessTokenEnvSourceWithLogger(env string, logger *slog.Logger) (*AccessTokenEnvSource, error) {
	env = strings.TrimSpace(env)
	if env == "" {
		return nil, fmt.Errorf("env_access_token auth requires env name: %w", ErrMissingToken)
	}
	if logger != nil {
		logger.Info("env_access_token source configured", "env", env)
	}
	return &AccessTokenEnvSource{env: env, lookup: os.LookupEnv, logger: logger}, nil
}

// NewAccessTokenFileSource creates a token source for file_access_token provider auth.
func NewAccessTokenFileSource(file string) (*AccessTokenFileSource, error) {
	return NewAccessTokenFileSourceWithLogger(file, nil)
}

// NewAccessTokenFileSourceWithLogger creates a token source with optional verbose diagnostics.
func NewAccessTokenFileSourceWithLogger(file string, logger *slog.Logger) (*AccessTokenFileSource, error) {
	file = strings.TrimSpace(file)
	if file == "" {
		return nil, fmt.Errorf("file_access_token auth requires file path: %w", ErrMissingToken)
	}
	if logger != nil {
		logger.Info("file_access_token source configured", "file", file)
	}
	return &AccessTokenFileSource{file: file, readFile: os.ReadFile, logger: logger}, nil
}

// NewAccessTokenCommandSource creates a token source for access_token_command provider auth.
func NewAccessTokenCommandSource(command []string) (*AccessTokenCommandSource, error) {
	return NewAccessTokenCommandSourceWithRefreshAndLogger(command, 0, nil)
}

// NewAccessTokenCommandSourceWithRefresh creates a command source with an expiry refresh window.
func NewAccessTokenCommandSourceWithRefresh(command []string, refreshBefore time.Duration) (*AccessTokenCommandSource, error) {
	return NewAccessTokenCommandSourceWithRefreshAndLogger(command, refreshBefore, nil)
}

// NewAccessTokenCommandSourceWithRefreshAndLogger creates a command source with optional verbose diagnostics.
func NewAccessTokenCommandSourceWithRefreshAndLogger(command []string, refreshBefore time.Duration, logger *slog.Logger) (*AccessTokenCommandSource, error) {
	clean := normalizeCommand(command)
	if len(clean) == 0 {
		return nil, fmt.Errorf("access_token_command auth requires command: %w", ErrMissingToken)
	}
	if refreshBefore < 0 {
		refreshBefore = 0
	}
	if logger != nil {
		logger.Info("access_token_command source configured", "command", clean, "refresh_before_seconds", refreshBefore.Seconds())
	}
	return &AccessTokenCommandSource{command: clean, refreshBefore: refreshBefore, run: runCommand, now: time.Now, logger: logger}, nil
}

// NewTokenSource creates a TokenSource for the supported provider auth config.
func NewTokenSource(auth config.AuthConfig) (TokenSource, error) {
	return NewTokenSourceWithLogger(auth, nil)
}

// NewTokenSourceWithLogger creates a TokenSource with optional verbose diagnostics.
func NewTokenSourceWithLogger(auth config.AuthConfig, logger *slog.Logger) (TokenSource, error) {
	switch auth.Type {
	case config.AuthTypeBearerEnv:
		return NewBearerEnvTokenSourceWithLogger(auth.Env, logger)
	case config.AuthTypeEnvAccessToken:
		return NewAccessTokenEnvSourceWithLogger(auth.Env, logger)
	case config.AuthTypeFileAccessToken:
		return NewAccessTokenFileSourceWithLogger(auth.File, logger)
	case config.AuthTypeCommandAccessToken:
		return NewAccessTokenCommandSourceWithRefreshAndLogger(auth.Command, time.Duration(auth.RefreshBeforeSeconds)*time.Second, logger)
	default:
		return nil, fmt.Errorf("%s: %w", auth.Type, ErrUnsupportedAuthType)
	}
}

// Token returns the current bearer token, or an error that identifies only the env var name.
func (s *BearerEnvTokenSource) Token(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s == nil || s.env == "" {
		return "", fmt.Errorf("bearer_env token source is not configured: %w", ErrMissingToken)
	}
	if s.logger != nil {
		s.logger.Info("reading provider token from environment", "env", s.env)
	}
	return tokenFromEnv(s.env, s.lookup, "provider token")
}

// Token returns the current access token from an environment variable.
func (s *AccessTokenEnvSource) Token(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s == nil || s.env == "" {
		return "", fmt.Errorf("env_access_token token source is not configured: %w", ErrMissingToken)
	}
	if s.logger != nil {
		s.logger.Info("reading provider access token from environment", "env", s.env)
	}
	return tokenFromEnv(s.env, s.lookup, "provider access token")
}

// Token returns the current access token from a local file.
func (s *AccessTokenFileSource) Token(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s == nil || s.file == "" {
		return "", fmt.Errorf("file_access_token token source is not configured: %w", ErrMissingToken)
	}
	if s.logger != nil {
		s.logger.Info("reading provider access token file", "file", s.file)
	}
	data, err := s.readFile(s.file)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("provider access token file read failed", "file", s.file, "error", err)
		}
		return "", fmt.Errorf("read provider access token file %q: %w", s.file, ErrMissingToken)
	}
	return nonEmptyToken(string(data), "provider access token file")
}

// Token invokes the configured command and returns its access_token JSON field.
func (s *AccessTokenCommandSource) Token(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s == nil || len(s.command) == 0 {
		return "", fmt.Errorf("access_token_command token source is not configured: %w", ErrMissingToken)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now
	if s.now != nil {
		now = s.now
	}
	if s.cached.token != "" && s.cached.expiresAt.After(now().Add(s.refreshBefore)) {
		if s.logger != nil {
			s.logger.Info("using cached provider access token", "expires_at", s.cached.expiresAt)
		}
		return s.cached.token, nil
	}

	if s.logger != nil {
		s.logger.Info("running provider access token command", "command", s.command)
	}
	out, err := s.run(ctx, s.command)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("provider access token command failed", "command", s.command, "error", err)
		}
		return "", fmt.Errorf("provider access token command failed: %w", ErrMissingToken)
	}
	result, err := parseCommandToken(out, now())
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("provider access token command output invalid", "command", s.command, "error", err)
		}
		return "", err
	}
	if !result.expiresAt.IsZero() {
		s.cached = result
		if s.logger != nil {
			s.logger.Info("provider access token command returned expiring token", "expires_at", result.expiresAt)
		}
	} else {
		// Without expiry metadata there is no safe refresh window, so command
		// output is treated as one-shot and is not cached.
		s.cached = cachedAccessToken{}
		if s.logger != nil {
			s.logger.Info("provider access token command returned non-expiring token")
		}
	}
	return result.token, nil
}

func parseCommandToken(out []byte, now time.Time) (cachedAccessToken, error) {
	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresAt   string `json:"expires_at"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return cachedAccessToken{}, fmt.Errorf("provider access token command output: %w", ErrMalformedToken)
	}
	token, err := nonEmptyToken(payload.AccessToken, "provider access token command")
	if err != nil {
		return cachedAccessToken{}, err
	}
	result := cachedAccessToken{token: token}
	if payload.ExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339, payload.ExpiresAt)
		if err != nil {
			return cachedAccessToken{}, fmt.Errorf("provider access token command expires_at: %w", ErrMalformedToken)
		}
		if !expiresAt.After(now) {
			return cachedAccessToken{}, fmt.Errorf("provider access token command returned expired token: %w", ErrMissingToken)
		}
		result.expiresAt = expiresAt
	}
	return result, nil
}

func tokenFromEnv(env string, lookup func(string) (string, bool), label string) (string, error) {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	token, ok := lookup(env)
	if !ok {
		return "", fmt.Errorf("%s env %q is unset or empty: %w", label, env, ErrMissingToken)
	}
	return nonEmptyToken(token, label+" env "+env)
}

func nonEmptyToken(raw string, source string) (string, error) {
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", fmt.Errorf("%s is empty: %w", source, ErrMissingToken)
	}
	return token, nil
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

func runCommand(ctx context.Context, command []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	return cmd.Output()
}
