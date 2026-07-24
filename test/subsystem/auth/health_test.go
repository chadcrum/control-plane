//go:build subsystem

package subsystem_test

import (
	"io"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Health endpoint bypass", func() {
	It("returns 200 with no auth headers", func() {
		resp := doRequest(http.MethodGet, "/health")
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(ContainSubstring(`"status":"ok"`))
	})

	It("returns 200 even with an invalid Bearer token", func() {
		resp := doRequest(http.MethodGet, "/health", withBearerToken("garbage-token"))
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	It("returns 200 even with a wrong proxy secret", func() {
		resp := doRequest(http.MethodGet, "/health", withProxySecret("wrong-secret", "any-subject"))
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})
})
