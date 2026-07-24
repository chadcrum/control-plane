//go:build subsystem

package subsystem_test

import (
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Actor status enforcement", func() {
	var (
		adminToken string
		kcUserID   string
		username   string
		password   = "testpass"
	)

	BeforeEach(func() {
		adminToken = getKeycloakAdminToken()
		username = uniqueUsername()
		kcUserID = createKeycloakUser(adminToken, username, password)

		resp := doRequest(http.MethodGet, "/catalog-items",
			withProxySecret(proxySecret, kcUserID),
			withPreferredUsername(username))
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	AfterEach(func() {
		if kcUserID != "" {
			deleteKeycloakUser(adminToken, kcUserID)
		}
	})

	It("allows access for an active actor", func() {
		resp := doRequest(http.MethodGet, "/catalog-items",
			withProxySecret(proxySecret, kcUserID))
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	It("returns 403 for a suspended actor", func() {
		updateActorStatus(kcUserID, "suspended")
		time.Sleep(3 * time.Second)

		resp := doRequest(http.MethodGet, "/catalog-items",
			withProxySecret(proxySecret, kcUserID))
		Expect(resp.StatusCode).To(Equal(http.StatusForbidden))

		problem := readProblemResponse(resp)
		Expect(problem.Detail).To(Equal("account suspended"))
	})

	It("allows access after reactivating a suspended actor", func() {
		updateActorStatus(kcUserID, "suspended")
		time.Sleep(3 * time.Second)

		resp := doRequest(http.MethodGet, "/catalog-items",
			withProxySecret(proxySecret, kcUserID))
		Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
		problem := readProblemResponse(resp)
		Expect(problem.Detail).To(Equal("account suspended"))

		updateActorStatus(kcUserID, "active")
		time.Sleep(3 * time.Second)

		resp = doRequest(http.MethodGet, "/catalog-items",
			withProxySecret(proxySecret, kcUserID))
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	It("returns 403 for a deactivated actor", func() {
		updateActorStatus(kcUserID, "deactivated")
		time.Sleep(3 * time.Second)

		resp := doRequest(http.MethodGet, "/catalog-items",
			withProxySecret(proxySecret, kcUserID))
		Expect(resp.StatusCode).To(Equal(http.StatusForbidden))

		problem := readProblemResponse(resp)
		Expect(problem.Detail).To(Equal("account deactivated"))
	})
})
