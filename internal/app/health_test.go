package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/dcm-project/control-plane/internal/auth"
	"github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Monolith health", func() {
	It("returns ok at the canonical path", func() {
		router := chi.NewRouter()
		registerMonolithHealth(router)

		req := httptest.NewRequest(http.MethodGet, auth.MonolithHealthPath, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))

		var body healthResponse
		Expect(json.NewDecoder(rec.Body).Decode(&body)).To(Succeed())
		Expect(body.Status).To(Equal("ok"))
		Expect(body.Path).To(Equal(auth.MonolithHealthPath))
	})
})
