package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/dcm-project/control-plane/internal/auth"

	catalogapi "github.com/dcm-project/control-plane/api/catalog/v1alpha1"
	policyapi "github.com/dcm-project/control-plane/api/policy/v1alpha1"
	spproviderapi "github.com/dcm-project/control-plane/api/sp/v1alpha1/provider"
	sprmapi "github.com/dcm-project/control-plane/api/sp/v1alpha1/resource_manager"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
)

const apiV1Alpha1Prefix = "/api/v1alpha1"

type openAPIValidators struct {
	catalog  func(http.Handler) http.Handler
	policy   func(http.Handler) http.Handler
	provider func(http.Handler) http.Handler
	rm       func(http.Handler) http.Handler
}

func newOpenAPIValidators() (*openAPIValidators, error) {
	catalogSpec, err := catalogapi.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load catalog OpenAPI spec: %w", err)
	}

	policySpec, err := policyapi.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load policy OpenAPI spec: %w", err)
	}

	providerSpec, err := spproviderapi.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load service provider OpenAPI spec: %w", err)
	}

	rmSpec, err := sprmapi.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load resource manager OpenAPI spec: %w", err)
	}

	return &openAPIValidators{
		catalog:  oapiRequestValidator(catalogSpec),
		policy:   oapiRequestValidator(policySpec),
		provider: oapiRequestValidator(providerSpec),
		rm:       oapiRequestValidator(rmSpec),
	}, nil
}

func oapiRequestValidator(spec *openapi3.T) func(http.Handler) http.Handler {
	return nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		Options: openapi3filter.Options{
			AuthenticationFunc: verifyActorContext,
			// kin-openapi rewrites validated bodies when schema defaults are applied,
			// but only registers encoders for application/json. PATCH merge bodies
			// use application/merge-patch+json and must stay partial (RFC 7396).
			SkipSettingDefaults: true,
		},
		SilenceServersWarning: true,
	})
}

func verifyActorContext(ctx context.Context, input *openapi3filter.AuthenticationInput) error {
	if _, ok := auth.ActorInfoFromContext(ctx); !ok {
		return fmt.Errorf("unauthenticated: actor context not populated")
	}
	return nil
}

func (v *openAPIValidators) middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if !strings.HasPrefix(path, apiV1Alpha1Prefix) {
				next.ServeHTTP(w, r)
				return
			}
			if path == monolithHealthPath {
				next.ServeHTTP(w, r)
				return
			}

			switch {
			case strings.HasPrefix(path, apiV1Alpha1Prefix+"/service-type-instances"):
				v.rm(next).ServeHTTP(w, r)
			case strings.HasPrefix(path, apiV1Alpha1Prefix+"/providers"):
				v.provider(next).ServeHTTP(w, r)
			case strings.HasPrefix(path, apiV1Alpha1Prefix+"/policies"):
				v.policy(next).ServeHTTP(w, r)
			default:
				v.catalog(next).ServeHTTP(w, r)
			}
		})
	}
}
