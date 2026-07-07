package app

import (
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/dcm-project/control-plane/internal/auth"
	"github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var testActorInfo = auth.ActorInfo{ActorID: "test-actor", ActorType: "human"}

var _ = Describe("OpenAPI request validation", func() {
	var validators *openAPIValidators

	BeforeEach(func() {
		var err error
		validators, err = newOpenAPIValidators()
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("monolith health", func() {
		It("bypasses domain validators", func() {
			router := chi.NewRouter()
			router.Use(validators.middleware())
			registerMonolithHealth(router)

			req := httptest.NewRequest(http.MethodGet, monolithHealthPath, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
		})
	})

	Describe("policy routes", func() {
		It("rejects malformed JSON on POST /policies", func() {
			expectInvalidJSONRejected(validators, "/api/v1alpha1/policies")
		})

		It("allows valid partial PATCH on /policies/{policyId}", func() {
			router := chi.NewRouter()
			router.Use(validators.middleware())
			router.Patch("/api/v1alpha1/policies/{policyId}", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			body := `{"display_name":"Updated Name","priority":600}`
			req := httptest.NewRequest(http.MethodPatch, "/api/v1alpha1/policies/test-policy-id", strings.NewReader(body))
			req = req.WithContext(auth.WithActorInfo(req.Context(), testActorInfo))
			req.Header.Set("Content-Type", "application/merge-patch+json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
		})
	})

	Describe("catalog routes", func() {
		It("rejects malformed JSON on POST /catalog-items", func() {
			expectInvalidJSONRejected(validators, "/api/v1alpha1/catalog-items")
		})
	})

	Describe("SP provider routes", func() {
		It("rejects malformed JSON on POST /providers", func() {
			expectInvalidJSONRejected(validators, "/api/v1alpha1/providers")
		})
	})

	Describe("SP resource manager routes", func() {
		It("rejects malformed JSON on POST /service-type-instances", func() {
			expectInvalidJSONRejected(validators, "/api/v1alpha1/service-type-instances")
		})
	})
})

func expectInvalidJSONRejected(validators *openAPIValidators, path string) {
	router := chi.NewRouter()
	router.Use(validators.middleware())
	router.Post(path, func(_ http.ResponseWriter, _ *http.Request) {
		Fail("validator passed invalid JSON through to handler for POST " + path)
	})

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("not-json"))
	req = req.WithContext(auth.WithActorInfo(req.Context(), testActorInfo))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	Expect(rec.Code).To(Equal(http.StatusBadRequest), rec.Body.String())
}
