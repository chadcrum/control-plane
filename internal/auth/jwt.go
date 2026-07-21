package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// JWTValidator validates a raw JWT string and returns the extracted claims.
type JWTValidator interface {
	Validate(ctx context.Context, rawToken string) (*JWTClaims, error)
}

// JWTClaims holds the identity claims extracted from a validated JWT.
type JWTClaims struct {
	Subject           string
	PreferredUsername string
}

// OIDCValidator validates JWTs against an OIDC provider's JWKS endpoint.
type OIDCValidator struct {
	verifier *oidc.IDTokenVerifier
}

// NewOIDCValidator creates a validator that verifies tokens issued by the
// given OIDC issuer. If audience is non-empty, tokens must contain it in
// their aud claim.
func NewOIDCValidator(ctx context.Context, issuerURL, audience string) (*OIDCValidator, error) {
	discoverCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	provider, err := oidc.NewProvider(discoverCtx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}

	cfg := &oidc.Config{}
	if audience != "" {
		cfg.ClientID = audience
	} else {
		cfg.SkipClientIDCheck = true
	}

	return &OIDCValidator{
		verifier: provider.Verifier(cfg),
	}, nil
}

// Validate verifies the token signature, expiry, and issuer against the
// OIDC provider's JWKS, then extracts the subject and preferred username.
func (v *OIDCValidator) Validate(ctx context.Context, rawToken string) (*JWTClaims, error) {
	verifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	idToken, err := v.verifier.Verify(verifyCtx, rawToken)
	if err != nil {
		return nil, fmt.Errorf("token verification failed: %w", err)
	}

	var claims struct {
		PreferredUsername string `json:"preferred_username"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse token claims: %w", err)
	}

	if idToken.Subject == "" {
		return nil, fmt.Errorf("token missing sub claim")
	}

	return &JWTClaims{
		Subject:           idToken.Subject,
		PreferredUsername: claims.PreferredUsername,
	}, nil
}

// extractBearerToken returns the token from an Authorization: Bearer header,
// or an empty string if the header is absent or malformed.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && strings.EqualFold(auth[:7], "bearer ") {
		return auth[7:]
	}
	return ""
}
