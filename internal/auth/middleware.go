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
	ProxySecret  string
	JWTValidator JWTValidator
	Resolver     ActorResolver
	Cache        *ActorCache
	Logger       *slog.Logger
}

func Middleware(cfg MiddlewareConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == MonolithHealthPath {
				next.ServeHTTP(w, r)
				return
			}

			var subject, preferredUsername string

			// Bearer token takes precedence; an invalid Bearer is a hard 401
			// with no fallback to proxy headers to avoid masking bad tokens.
			if token := extractBearerToken(r); token != "" && cfg.JWTValidator != nil {
				claims, err := cfg.JWTValidator.Validate(r.Context(), token)
				if err != nil {
					writeAuthError(w, http.StatusUnauthorized, "invalid bearer token")
					return
				}
				subject = claims.Subject
				preferredUsername = claims.PreferredUsername
			} else if secret := r.Header.Get(HeaderProxySecret); secret != "" {
				if subtle.ConstantTimeCompare([]byte(secret), []byte(cfg.ProxySecret)) != 1 {
					writeAuthError(w, http.StatusUnauthorized, "invalid proxy secret")
					return
				}
				subject = r.Header.Get(HeaderForwardedUser)
				preferredUsername = r.Header.Get(HeaderForwardedPreferredUsername)
			} else {
				writeAuthError(w, http.StatusUnauthorized, "missing authentication")
				return
			}

			if subject == "" {
				writeAuthError(w, http.StatusUnauthorized, "missing subject identifier")
				return
			}

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
	case status == http.StatusConflict:
		errType = "CONFLICT"
		title = "Conflict"
	case status >= 500:
		errType = "INTERNAL_ERROR"
		title = "Internal Server Error"
	default:
		errType = "UNAUTHENTICATED"
		title = "Unauthorized"
	}
	w.Header().Set("Content-Type", "application/problem+json")
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(authErrorResponse{
		Type:   errType,
		Status: status,
		Title:  title,
		Detail: detail,
	}); err != nil {
		slog.Warn("Failed to write auth error response", "error", err)
	}
}

func writeResolveError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrActorSuspended):
		writeAuthError(w, http.StatusForbidden, "account suspended")
	case errors.Is(err, ErrActorDeactivated):
		writeAuthError(w, http.StatusForbidden, "account deactivated")
	case errors.Is(err, ErrUsernameConflict):
		writeAuthError(w, http.StatusConflict, "username already in use by another account")
	default:
		writeAuthError(w, http.StatusInternalServerError, "internal error during authentication")
	}
}
