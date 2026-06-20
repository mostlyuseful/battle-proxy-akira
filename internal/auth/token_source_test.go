package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"battle-proxy-akira/internal/config"
)

func TestBearerEnvTokenSourceReturnsToken(t *testing.T) {
	t.Parallel()

	source, err := NewBearerEnvTokenSource("OPENAI_API_KEY")
	if err != nil {
		t.Fatalf("NewBearerEnvTokenSource: %v", err)
	}
	source.lookup = func(name string) (string, bool) {
		if name != "OPENAI_API_KEY" {
			t.Fatalf("lookup name = %q, want OPENAI_API_KEY", name)
		}
		return "sk-test-secret", true
	}

	token, err := source.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if token != "sk-test-secret" {
		t.Fatalf("token = %q, want sk-test-secret", token)
	}
}

func TestBearerEnvTokenSourceMissingEnvVar(t *testing.T) {
	t.Parallel()

	source, err := NewBearerEnvTokenSource("OPENAI_API_KEY")
	if err != nil {
		t.Fatalf("NewBearerEnvTokenSource: %v", err)
	}
	source.lookup = func(string) (string, bool) { return "", false }

	_, err = source.Token(context.Background())
	if err == nil {
		t.Fatal("Token returned nil error, want missing token error")
	}
	if !errors.Is(err, ErrMissingToken) {
		t.Fatalf("Token error = %v, want ErrMissingToken", err)
	}
	if !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Fatalf("Token error = %q, want env var name", err.Error())
	}
}

func TestBearerEnvTokenSourceEmptyEnvVar(t *testing.T) {
	t.Parallel()

	source, err := NewBearerEnvTokenSource("OPENAI_API_KEY")
	if err != nil {
		t.Fatalf("NewBearerEnvTokenSource: %v", err)
	}
	source.lookup = func(string) (string, bool) { return "   ", true }

	_, err = source.Token(context.Background())
	if err == nil {
		t.Fatal("Token returned nil error, want missing token error")
	}
	if !errors.Is(err, ErrMissingToken) {
		t.Fatalf("Token error = %v, want ErrMissingToken", err)
	}
}

func TestBearerEnvTokenSourceErrorsDoNotLeakTokenValues(t *testing.T) {
	t.Parallel()

	source, err := NewBearerEnvTokenSource("OPENAI_API_KEY")
	if err != nil {
		t.Fatalf("NewBearerEnvTokenSource: %v", err)
	}
	source.lookup = func(string) (string, bool) { return "super-secret-token", false }

	_, err = source.Token(context.Background())
	if err == nil {
		t.Fatal("Token returned nil error, want missing token error")
	}
	if strings.Contains(err.Error(), "super-secret-token") {
		t.Fatalf("Token error leaked token value: %q", err.Error())
	}
}

func TestAccessTokenEnvSourceReturnsToken(t *testing.T) {
	t.Parallel()

	source, err := NewAccessTokenEnvSource("CODEX_ACCESS_TOKEN")
	if err != nil {
		t.Fatalf("NewAccessTokenEnvSource: %v", err)
	}
	source.lookup = func(name string) (string, bool) {
		if name != "CODEX_ACCESS_TOKEN" {
			t.Fatalf("lookup name = %q, want CODEX_ACCESS_TOKEN", name)
		}
		return " access-token-secret\n", true
	}

	token, err := source.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if token != "access-token-secret" {
		t.Fatalf("token = %q, want trimmed access token", token)
	}
}

func TestAccessTokenFileSourceReturnsToken(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "token.txt")
	if err := os.WriteFile(path, []byte(" file-token-secret\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	source, err := NewAccessTokenFileSource(path)
	if err != nil {
		t.Fatalf("NewAccessTokenFileSource: %v", err)
	}

	token, err := source.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if token != "file-token-secret" {
		t.Fatalf("token = %q, want file-token-secret", token)
	}
}

func TestAccessTokenFileSourceRejectsMissingOrEmpty(t *testing.T) {
	t.Parallel()

	source, err := NewAccessTokenFileSource(filepath.Join(t.TempDir(), "missing-token"))
	if err != nil {
		t.Fatalf("NewAccessTokenFileSource: %v", err)
	}
	_, err = source.Token(context.Background())
	if !errors.Is(err, ErrMissingToken) {
		t.Fatalf("missing file error = %v, want ErrMissingToken", err)
	}

	emptyPath := filepath.Join(t.TempDir(), "empty-token")
	if err := os.WriteFile(emptyPath, []byte("\n"), 0o600); err != nil {
		t.Fatalf("WriteFile empty: %v", err)
	}
	emptySource, err := NewAccessTokenFileSource(emptyPath)
	if err != nil {
		t.Fatalf("NewAccessTokenFileSource empty: %v", err)
	}
	_, err = emptySource.Token(context.Background())
	if !errors.Is(err, ErrMissingToken) {
		t.Fatalf("empty file error = %v, want ErrMissingToken", err)
	}
}

func TestAccessTokenCommandSourceReturnsTokenAndParsesExpiry(t *testing.T) {
	t.Parallel()

	source, err := NewAccessTokenCommandSource(helperCommand(t, "command-token-secret", time.Now().Add(time.Hour).UTC().Format(time.RFC3339), 0))
	if err != nil {
		t.Fatalf("NewAccessTokenCommandSource: %v", err)
	}

	token, err := source.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if token != "command-token-secret" {
		t.Fatalf("token = %q, want command-token-secret", token)
	}
}

func TestAccessTokenCommandSourceAcceptsMissingExpiry(t *testing.T) {
	t.Parallel()

	source, err := NewAccessTokenCommandSource(helperCommand(t, "command-token-secret", "", 0))
	if err != nil {
		t.Fatalf("NewAccessTokenCommandSource: %v", err)
	}

	token, err := source.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if token != "command-token-secret" {
		t.Fatalf("token = %q, want command-token-secret", token)
	}
}

func TestAccessTokenCommandSourceRejectsMalformedEmptyExpiredAndFailed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command []string
		wantErr error
	}{
		{name: "malformed", command: helperRawCommand(t, `not-json`, 0), wantErr: ErrMalformedToken},
		{name: "empty", command: helperCommand(t, "", "", 0), wantErr: ErrMissingToken},
		{name: "bad expiry", command: helperCommand(t, "secret-value", "not-a-time", 0), wantErr: ErrMalformedToken},
		{name: "expired", command: helperCommand(t, "secret-value", time.Now().Add(-time.Hour).UTC().Format(time.RFC3339), 0), wantErr: ErrMissingToken},
		{name: "failed", command: helperRawCommand(t, `secret-value`, 3), wantErr: ErrMissingToken},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source, err := NewAccessTokenCommandSource(tt.command)
			if err != nil {
				t.Fatalf("NewAccessTokenCommandSource: %v", err)
			}
			_, err = source.Token(context.Background())
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Token error = %v, want %v", err, tt.wantErr)
			}
			if err != nil && strings.Contains(err.Error(), "secret-value") {
				t.Fatalf("Token error leaked token data: %q", err.Error())
			}
		})
	}
}

func TestNewTokenSourceSupportsAccessTokenConfigs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  config.AuthConfig
		want any
	}{
		{name: "env", cfg: config.AuthConfig{Type: config.AuthTypeEnvAccessToken, Env: "CODEX_ACCESS_TOKEN"}, want: &AccessTokenEnvSource{}},
		{name: "file", cfg: config.AuthConfig{Type: config.AuthTypeFileAccessToken, File: "/token"}, want: &AccessTokenFileSource{}},
		{name: "command", cfg: config.AuthConfig{Type: config.AuthTypeCommandAccessToken, Command: []string{"echo", "{}"}}, want: &AccessTokenCommandSource{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source, err := NewTokenSource(tt.cfg)
			if err != nil {
				t.Fatalf("NewTokenSource: %v", err)
			}
			switch tt.want.(type) {
			case *AccessTokenEnvSource:
				if _, ok := source.(*AccessTokenEnvSource); !ok {
					t.Fatalf("source = %T, want *AccessTokenEnvSource", source)
				}
			case *AccessTokenFileSource:
				if _, ok := source.(*AccessTokenFileSource); !ok {
					t.Fatalf("source = %T, want *AccessTokenFileSource", source)
				}
			case *AccessTokenCommandSource:
				if _, ok := source.(*AccessTokenCommandSource); !ok {
					t.Fatalf("source = %T, want *AccessTokenCommandSource", source)
				}
			}
		})
	}
}

func TestNewTokenSourceSupportsBearerEnvConfig(t *testing.T) {
	t.Parallel()

	source, err := NewTokenSource(config.AuthConfig{Type: config.AuthTypeBearerEnv, Env: "OPENAI_API_KEY"})
	if err != nil {
		t.Fatalf("NewTokenSource: %v", err)
	}
	if _, ok := source.(*BearerEnvTokenSource); !ok {
		t.Fatalf("source type = %T, want *BearerEnvTokenSource", source)
	}
}

func TestNewTokenSourceRejectsUnsupportedAuth(t *testing.T) {
	t.Parallel()

	_, err := NewTokenSource(config.AuthConfig{Type: "unknown_auth"})
	if err == nil {
		t.Fatal("NewTokenSource returned nil error, want unsupported auth error")
	}
	if !errors.Is(err, ErrUnsupportedAuthType) {
		t.Fatalf("NewTokenSource error = %v, want ErrUnsupportedAuthType", err)
	}
}

func TestBearerEnvTokenSourceHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	source, err := NewBearerEnvTokenSource("OPENAI_API_KEY")
	if err != nil {
		t.Fatalf("NewBearerEnvTokenSource: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = source.Token(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Token error = %v, want context.Canceled", err)
	}
}

func TestAccessTokenCommandSourceHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	source, err := NewAccessTokenCommandSource(helperCommand(t, "command-token-secret", "", 0))
	if err != nil {
		t.Fatalf("NewAccessTokenCommandSource: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = source.Token(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Token error = %v, want context.Canceled", err)
	}
}

func helperCommand(t *testing.T, token, expiresAt string, exitCode int) []string {
	t.Helper()

	payload := fmt.Sprintf(`{"access_token":%q`, token)
	if expiresAt != "" {
		payload += fmt.Sprintf(`,"expires_at":%q`, expiresAt)
	}
	payload += `}`
	return helperRawCommand(t, payload, exitCode)
}

func helperRawCommand(t *testing.T, stdout string, exitCode int) []string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "token-command.sh")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s' %q\nexit %d\n", stdout, exitCode)
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write helper command: %v", err)
	}
	return []string{path}
}
