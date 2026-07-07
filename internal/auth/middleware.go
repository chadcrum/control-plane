package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

const (
	HeaderForwardedUser              = "X-Forwarded-User"
	HeaderForwardedPreferredUsername = "X-Forwarded-Preferred-Username"
	HeaderProxySecret                = "X-Auth-Proxy-Secret"
)

type ActorResolver interface {
	ResolveActor(ctx context.Context, externalID string) (*ActorInfo, error)
}

type MiddlewareConfig struct {
	ProxySecret string
	Resolver    ActorResolver
	Cache       *ActorCache
	Logger      *slog.Logger
}

func Middleware(cfg MiddlewareConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == MonolithHealthPath {
				next.ServeHTTP(w, r)
				return
			}

			secret := r.Header.Get(HeaderProxySecret)
			if subtle.ConstantTimeCompare([]byte(secret), []byte(cfg.ProxySecret)) != 1 {
				writeAuthError(w, http.StatusUnauthorized, "missing or invalid proxy secret")
				return
			}

			subject := r.Header.Get(HeaderForwardedUser)
			if subject == "" {
				writeAuthError(w, http.StatusUnauthorized, "missing authentication")
				return
			}

			preferredUsername := r.Header.Get(HeaderForwardedPreferredUsername)
			ctx := WithPreferredUsername(r.Context(), preferredUsername)

			if info, ok := cfg.Cache.Get(subject); ok {
				ctx = WithActorInfo(ctx, info)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			info, err := cfg.Resolver.ResolveActor(ctx, subject)
			if err != nil {
				cfg.Logger.Warn("Actor resolution failed", "subject", subject, "error", err)
				writeResolveError(w, err)
				return
			}

			cfg.Cache.Set(subject, *info)
			ctx = WithActorInfo(ctx, *info)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func DisabledMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	logger.Warn("AUTH_DISABLED is set — all requests bypass authentication")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := WithActorInfo(r.Context(), ActorInfo{
				ActorID:   "auth-disabled",
				ActorType: "system",
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

const MonolithHealthPath = "/api/v1alpha1/health"

type authErrorResponse struct {
	Type   string `json:"type"`
	Status int    `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

func writeAuthError(w http.ResponseWriter, status int, detail string) {
	var errType, title string
	switch {
	case status == http.StatusForbidden:
		errType = "PERMISSION_DENIED"
		title = "Forbidden"
	case status >= 500:
		errType = "INTERNAL_ERROR"
		title = "Internal Server Error"
	default:
		errType = "UNAUTHENTICATED"
		title = "Unauthorized"
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(authErrorResponse{
		Type:   errType,
		Status: status,
		Title:  title,
		Detail: detail,
	})
}

func writeResolveError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrUnknownSubject):
		writeAuthError(w, http.StatusForbidden, "unknown subject — actor record does not exist")
	case errors.Is(err, ErrActorSuspended):
		writeAuthError(w, http.StatusForbidden, "account suspended")
	case errors.Is(err, ErrActorDeactivated):
		writeAuthError(w, http.StatusForbidden, "account deactivated")
	default:
		writeAuthError(w, http.StatusInternalServerError, "internal error during authentication")
	}
}
