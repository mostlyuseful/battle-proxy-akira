package api

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"

	"battle-proxy-akira/internal/config"
)

// NewClientAuthMiddleware builds reusable client bearer authentication middleware.
func NewClientAuthMiddleware(cfg config.ClientAuthConfig) (Middleware, error) {
	mode := cfg.Mode
	if mode == "" {
		mode = config.ClientAuthModeNone
	}

	switch mode {
	case config.ClientAuthModeNone:
		return identityMiddleware, nil
	case config.ClientAuthModeStaticBearer, config.ClientAuthModeBearerTokens:
		tokens := parseBearerTokens(os.Getenv(cfg.TokensEnv))
		if len(tokens) == 0 {
			return nil, NewProxyError(ErrorInvalidRequest, "client bearer auth token environment variable is unset or empty", "client_auth.tokens_env")
		}
		return StaticBearerAuth(tokens), nil
	default:
		return nil, NewProxyError(ErrorInvalidRequest, "unsupported client auth mode", "client_auth.mode")
	}
}

// StaticBearerAuth returns middleware that accepts any configured bearer token.
func StaticBearerAuth(tokens []string) Middleware {
	cleanTokens := normalizeTokens(tokens)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(cleanTokens) == 0 {
				WriteOpenAIError(w, NewProxyError(ErrorInvalidRequest, "client bearer auth is not configured", "authorization"))
				return
			}

			token, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok || !containsToken(cleanTokens, token) {
				w.Header().Set("WWW-Authenticate", `Bearer realm="llm-proxy"`)
				WriteOpenAIError(w, NewProxyError(ErrorPolicyDenied, "missing or invalid bearer token", "authorization"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func parseBearerTokens(raw string) []string {
	if raw == "" {
		return nil
	}
	return normalizeTokens(strings.Split(raw, ","))
}

func normalizeTokens(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token != "" {
			out = append(out, token)
		}
	}
	return out
}

func bearerToken(header string) (string, bool) {
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	if token == "" || strings.Contains(token, " ") {
		return "", false
	}
	return token, true
}

func containsToken(tokens []string, got string) bool {
	matched := 0
	for _, token := range tokens {
		if subtle.ConstantTimeCompare([]byte(token), []byte(got)) == 1 {
			matched = 1
		}
	}
	return matched == 1
}
