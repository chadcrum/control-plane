//go:build subsystem

package subsystem_test

import (
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RFC 7807 error response format", func() {
	It("returns well-formed problem+json on 401", func() {
		resp := doRequest(http.MethodGet, "/catalog-items")
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
		Expect(resp.Header.Get("WWW-Authenticate")).To(Equal("Bearer"))

		problem := readProblemResponse(resp)
		Expect(problem.Type).To(Equal("UNAUTHENTICATED"))
		Expect(problem.Status).To(Equal(401))
		Expect(problem.Title).To(Equal("Unauthorized"))
		Expect(problem.Detail).NotTo(BeEmpty())
	})

	It("returns well-formed problem+json on 403", func() {
		adminToken := getKeycloakAdminToken()
		username := uniqueUsername()
		kcUserID := createKeycloakUser(adminToken, username, "testpass")
		defer deleteKeycloakUser(adminToken, kcUserID)

		resp := doRequest(http.MethodGet, "/catalog-items",
			withProxySecret(proxySecret, kcUserID),
			withPreferredUsername(username))
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		updateActorStatus(kcUserID, "suspended")
		time.Sleep(3 * time.Second)

		resp2 := doRequest(http.MethodGet, "/catalog-items",
			withProxySecret(proxySecret, kcUserID))
		Expect(resp2.StatusCode).To(Equal(http.StatusForbidden))
		Expect(resp2.Header.Get("Content-Type")).To(Equal("application/problem+json"))

		problem := readProblemResponse(resp2)
		Expect(problem.Type).To(Equal("PERMISSION_DENIED"))
		Expect(problem.Status).To(Equal(403))
		Expect(problem.Title).To(Equal("Forbidden"))
		Expect(problem.Detail).To(Equal("account suspended"))
	})
})
