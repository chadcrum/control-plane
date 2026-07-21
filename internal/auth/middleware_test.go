package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockResolver implements ActorResolver for testing.
type mockResolver struct {
	info *ActorInfo
	err  error
}

func (m *mockResolver) ResolveActor(_ context.Context, _ string) (*ActorInfo, error) {
	return m.info, m.err
}

// callCountResolver tracks how many times ResolveActor is called.
type callCountResolver struct {
	info  *ActorInfo
	err   error
	calls int
}

func (r *callCountResolver) ResolveActor(_ context.Context, _ string) (*ActorInfo, error) {
	r.calls++
	return r.info, r.err
}

// mockJWTValidator implements JWTValidator for testing.
type mockJWTValidator struct {
	claims *JWTClaims
	err    error
}

func (m *mockJWTValidator) Validate(_ context.Context, _ string) (*JWTClaims, error) {
	return m.claims, m.err
}

// echoHandler writes 200 OK — used to verify the request reached the handler.
var echoHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func newMiddlewareConfig(resolver ActorResolver, proxySecret string) MiddlewareConfig {
	return MiddlewareConfig{
		ProxySecret: proxySecret,
		Resolver:    resolver,
		Cache:       NewActorCache(time.Minute),
		Logger:      slog.Default(),
	}
}

func TestMiddleware_HealthBypassesAuth(t *testing.T) {
	cfg := newMiddlewareConfig(&mockResolver{err: errors.New("should not be called")}, "secret")
	handler := Middleware(cfg)(echoHandler)

	req := httptest.NewRequest(http.MethodGet, MonolithHealthPath, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("health endpoint: got status %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestMiddleware_MissingProxySecretHeaderReturns401(t *testing.T) {
	info := &ActorInfo{ActorID: "user-1", ActorType: "user"}
	cfg := newMiddlewareConfig(&mockResolver{info: info}, "secret")
	handler := Middleware(cfg)(echoHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/providers", nil)
	req.Header.Set(HeaderForwardedUser, "alice")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusUnauthorized, "UNAUTHENTICATED")
}

func TestMiddleware_InvalidProxySecretReturns401(t *testing.T) {
	cfg := newMiddlewareConfig(&mockResolver{}, "correct-secret")
	handler := Middleware(cfg)(echoHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/providers", nil)
	req.Header.Set(HeaderProxySecret, "wrong-secret")
	req.Header.Set(HeaderForwardedUser, "alice")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusUnauthorized, "UNAUTHENTICATED")
}

func TestMiddleware_MissingForwardedUserReturns401(t *testing.T) {
	cfg := newMiddlewareConfig(&mockResolver{}, "secret")
	handler := Middleware(cfg)(echoHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/providers", nil)
	req.Header.Set(HeaderProxySecret, "secret")
	// No X-Forwarded-User header.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusUnauthorized, "UNAUTHENTICATED")
}

func TestMiddleware_CacheHitSkipsResolver(t *testing.T) {
	resolver := &callCountResolver{
		info: &ActorInfo{ActorID: "user-1", ActorType: "user"},
	}
	cfg := newMiddlewareConfig(resolver, "secret")

	// Pre-populate the cache.
	cfg.Cache.Set("alice", ActorInfo{ActorID: "cached-user", ActorType: "user"})
	handler := Middleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := ActorInfoFromContext(r.Context())
		if !ok {
			t.Fatal("expected ActorInfo in context on cache hit")
		}
		if info.ActorID != "cached-user" {
			t.Fatalf("expected cached ActorID, got %q", info.ActorID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/providers", nil)
	req.Header.Set(HeaderProxySecret, "secret")
	req.Header.Set(HeaderForwardedUser, "alice")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("cache hit: got status %d, want %d", rec.Code, http.StatusOK)
	}
	if resolver.calls != 0 {
		t.Fatalf("expected resolver not called on cache hit, got %d calls", resolver.calls)
	}
}

func TestMiddleware_CacheMissCallsResolverAndCaches(t *testing.T) {
	resolver := &callCountResolver{
		info: &ActorInfo{ActorID: "resolved-user", ActorType: "user"},
	}
	cfg := newMiddlewareConfig(resolver, "secret")
	handler := Middleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := ActorInfoFromContext(r.Context())
		if !ok {
			t.Fatal("expected ActorInfo in context after resolve")
		}
		if info.ActorID != "resolved-user" {
			t.Fatalf("expected resolved ActorID, got %q", info.ActorID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/providers", nil)
	req.Header.Set(HeaderProxySecret, "secret")
	req.Header.Set(HeaderForwardedUser, "alice")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("cache miss: got status %d, want %d", rec.Code, http.StatusOK)
	}
	if resolver.calls != 1 {
		t.Fatalf("expected resolver called once, got %d", resolver.calls)
	}

	// Verify the result was cached.
	cached, ok := cfg.Cache.Get("alice")
	if !ok {
		t.Fatal("expected entry to be cached after resolve")
	}
	if cached.ActorID != "resolved-user" {
		t.Fatalf("cached ActorID = %q, want %q", cached.ActorID, "resolved-user")
	}
}

func TestMiddleware_ResolverErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantType   string
		wantDetail string
	}{
		{
			name:       "ErrActorSuspended returns 403",
			err:        ErrActorSuspended,
			wantStatus: http.StatusForbidden,
			wantType:   "PERMISSION_DENIED",
			wantDetail: "account suspended",
		},
		{
			name:       "ErrActorDeactivated returns 403",
			err:        ErrActorDeactivated,
			wantStatus: http.StatusForbidden,
			wantType:   "PERMISSION_DENIED",
			wantDetail: "account deactivated",
		},
		{
			name:       "ErrUsernameConflict returns 409",
			err:        ErrUsernameConflict,
			wantStatus: http.StatusConflict,
			wantType:   "CONFLICT",
			wantDetail: "username already in use by another account",
		},
		{
			name:       "generic error returns 500",
			err:        errors.New("database timeout"),
			wantStatus: http.StatusInternalServerError,
			wantType:   "INTERNAL_ERROR",
			wantDetail: "internal error during authentication",
		},
		{
			name:       "wrapped ErrActorSuspended returns 403",
			err:        fmt.Errorf("db: %w", ErrActorSuspended),
			wantStatus: http.StatusForbidden,
			wantType:   "PERMISSION_DENIED",
			wantDetail: "account suspended",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newMiddlewareConfig(&mockResolver{err: tt.err}, "secret")
			handler := Middleware(cfg)(echoHandler)

			req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/providers", nil)
			req.Header.Set(HeaderProxySecret, "secret")
			req.Header.Set(HeaderForwardedUser, "alice")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			resp := assertErrorResponse(t, rec, tt.wantStatus, tt.wantType)
			if resp.Detail != tt.wantDetail {
				t.Fatalf("detail = %q, want %q", resp.Detail, tt.wantDetail)
			}
		})
	}
}

func TestMiddleware_NoAuthHeadersReturns401(t *testing.T) {
	cfg := newMiddlewareConfig(&mockResolver{}, "secret")
	handler := Middleware(cfg)(echoHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/providers", nil)
	req.Header.Set(HeaderForwardedUser, "alice")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusUnauthorized, "UNAUTHENTICATED")
}

func TestMiddleware_PropagatesPreferredUsername(t *testing.T) {
	resolver := &callCountResolver{
		info: &ActorInfo{ActorID: "user-1", ActorType: "user"},
	}
	cfg := newMiddlewareConfig(resolver, "secret")
	handler := Middleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := PreferredUsernameFromContext(r.Context())
		if got != "alice" {
			t.Errorf("PreferredUsernameFromContext() = %q, want %q", got, "alice")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/providers", nil)
	req.Header.Set(HeaderProxySecret, "secret")
	req.Header.Set(HeaderForwardedUser, "ext-123")
	req.Header.Set(HeaderForwardedPreferredUsername, "alice")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestMiddleware_EmptyForwardedUserReturns401(t *testing.T) {
	cfg := newMiddlewareConfig(&mockResolver{}, "secret")
	handler := Middleware(cfg)(echoHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/providers", nil)
	req.Header.Set(HeaderProxySecret, "secret")
	req.Header.Set(HeaderForwardedUser, "")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusUnauthorized, "UNAUTHENTICATED")
}

func TestDisabledMiddleware_IgnoresAuthHeaders(t *testing.T) {
	handler := DisabledMiddleware(slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := ActorInfoFromContext(r.Context())
		if !ok {
			t.Fatal("expected ActorInfo in context from DisabledMiddleware")
		}
		if info.ActorID != "auth-disabled" {
			t.Fatalf("ActorID = %q, want %q", info.ActorID, "auth-disabled")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/providers", nil)
	req.Header.Set(HeaderProxySecret, "garbage-secret")
	req.Header.Set(HeaderForwardedUser, "garbage-user")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("DisabledMiddleware with garbage headers: got status %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestDisabledMiddleware_SetsSystemActor(t *testing.T) {
	handler := DisabledMiddleware(slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := ActorInfoFromContext(r.Context())
		if !ok {
			t.Fatal("expected ActorInfo in context from DisabledMiddleware")
		}
		if info.ActorID != "auth-disabled" {
			t.Fatalf("ActorID = %q, want %q", info.ActorID, "auth-disabled")
		}
		if info.ActorType != "system" {
			t.Fatalf("ActorType = %q, want %q", info.ActorType, "system")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/providers", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("DisabledMiddleware: got status %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestMiddleware_BearerTokenHappyPath(t *testing.T) {
	resolver := &callCountResolver{
		info: &ActorInfo{ActorID: "jwt-user-1", ActorType: "human"},
	}
	jwt := &mockJWTValidator{
		claims: &JWTClaims{Subject: "ext-uuid-123", PreferredUsername: "alice"},
	}
	cfg := newMiddlewareConfig(resolver, "secret")
	cfg.JWTValidator = jwt
	handler := Middleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := ActorInfoFromContext(r.Context())
		if !ok {
			t.Fatal("expected ActorInfo in context from JWT path")
		}
		if info.ActorID != "jwt-user-1" {
			t.Fatalf("ActorID = %q, want %q", info.ActorID, "jwt-user-1")
		}
		if got := PreferredUsernameFromContext(r.Context()); got != "alice" {
			t.Fatalf("PreferredUsername = %q, want %q", got, "alice")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/providers", nil)
	req.Header.Set("Authorization", "Bearer valid.jwt.token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("JWT happy path: got status %d, want %d", rec.Code, http.StatusOK)
	}
	if resolver.calls != 1 {
		t.Fatalf("expected resolver called once for JWT, got %d", resolver.calls)
	}
}

func TestMiddleware_BearerTokenInvalidReturns401(t *testing.T) {
	jwt := &mockJWTValidator{err: fmt.Errorf("token expired")}
	cfg := newMiddlewareConfig(&mockResolver{}, "secret")
	cfg.JWTValidator = jwt
	handler := Middleware(cfg)(echoHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/providers", nil)
	req.Header.Set("Authorization", "Bearer expired.jwt.token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusUnauthorized, "UNAUTHENTICATED")
}

func TestMiddleware_BearerTokenNoValidatorFallsToProxy(t *testing.T) {
	resolver := &callCountResolver{
		info: &ActorInfo{ActorID: "proxy-user", ActorType: "human"},
	}
	cfg := newMiddlewareConfig(resolver, "secret")
	// JWTValidator is nil — Bearer token should be ignored, proxy path used.
	handler := Middleware(cfg)(echoHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/providers", nil)
	req.Header.Set("Authorization", "Bearer some.jwt.token")
	req.Header.Set(HeaderProxySecret, "secret")
	req.Header.Set(HeaderForwardedUser, "bob")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("fallback to proxy: got status %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestMiddleware_BearerTokenTakesPrecedenceOverProxy(t *testing.T) {
	jwtResolver := &callCountResolver{
		info: &ActorInfo{ActorID: "jwt-user", ActorType: "human"},
	}
	jwt := &mockJWTValidator{
		claims: &JWTClaims{Subject: "jwt-sub", PreferredUsername: "jwt-alice"},
	}
	cfg := newMiddlewareConfig(jwtResolver, "secret")
	cfg.JWTValidator = jwt
	handler := Middleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := PreferredUsernameFromContext(r.Context()); got != "jwt-alice" {
			t.Errorf("PreferredUsername = %q, want %q (JWT should win)", got, "jwt-alice")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/providers", nil)
	req.Header.Set("Authorization", "Bearer valid.jwt.token")
	req.Header.Set(HeaderProxySecret, "secret")
	req.Header.Set(HeaderForwardedUser, "proxy-subject")
	req.Header.Set(HeaderForwardedPreferredUsername, "proxy-alice")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("JWT precedence: got status %d, want %d", rec.Code, http.StatusOK)
	}
}

// assertErrorResponse validates the JSON error body structure and status.
func assertErrorResponse(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantType string) authErrorResponse {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d", rec.Code, wantStatus)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want %q", ct, "application/problem+json")
	}
	var resp authErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp.Type != wantType {
		t.Fatalf("error type = %q, want %q", resp.Type, wantType)
	}
	if resp.Status != wantStatus {
		t.Fatalf("error status field = %d, want %d", resp.Status, wantStatus)
	}
	return resp
}
