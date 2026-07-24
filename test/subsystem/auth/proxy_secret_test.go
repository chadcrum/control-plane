//go:build subsystem

package subsystem_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Proxy secret validation", func() {
	It("returns 401 with no auth credentials", func() {
		resp := doRequest(http.MethodGet, "/catalog-items")
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		Expect(resp.Header.Get("WWW-Authenticate")).To(Equal("Bearer"))

		problem := readProblemResponse(resp)
		Expect(problem.Detail).To(Equal("missing authentication"))
	})

	It("returns 401 with wrong proxy secret", func() {
		resp := doRequest(http.MethodGet, "/catalog-items",
			withProxySecret("wrong-secret", adminSubject))
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))

		problem := readProblemResponse(resp)
		Expect(problem.Detail).To(Equal("invalid proxy secret"))
	})

	It("succeeds with correct proxy secret and valid subject", func() {
		resp := doRequest(http.MethodGet, "/catalog-items",
			withProxySecret(proxySecret, adminSubject))
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	It("returns 401 with correct proxy secret but empty subject", func() {
		resp := doRequest(http.MethodGet, "/catalog-items",
			withProxySecret(proxySecret, ""))
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))

		problem := readProblemResponse(resp)
		Expect(problem.Detail).To(Equal("missing subject identifier"))
	})

	It("uses JWT identity when both valid Bearer and valid proxy secret are present", func() {
		adminToken := getKeycloakAdminToken()
		username := uniqueUsername()
		kcUserID := createKeycloakUser(adminToken, username, "testpass")
		defer deleteKeycloakUser(adminToken, kcUserID)

		token := getUserToken(username, "testpass")

		resp := doRequest(http.MethodGet, "/catalog-items",
			withBearerToken(token),
			withProxySecret(proxySecret, adminSubject))
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		_, dbUsername, status := getActorByExternalID(kcUserID)
		Expect(dbUsername).To(Equal(username))
		Expect(status).To(Equal("active"))
	})

	It("returns 401 for invalid Bearer even when valid proxy secret is present", func() {
		resp := doRequest(http.MethodGet, "/catalog-items",
			withBearerToken("garbage-token"),
			withProxySecret(proxySecret, adminSubject))
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))

		problem := readProblemResponse(resp)
		Expect(problem.Detail).To(Equal("invalid bearer token"))
	})
})
