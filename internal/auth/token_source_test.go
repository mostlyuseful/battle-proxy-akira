package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

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

	_, err := NewTokenSource(config.AuthConfig{Type: config.AuthTypeCommandAccessToken})
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
