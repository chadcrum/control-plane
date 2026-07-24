//go:build subsystem

package subsystem_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("JIT provisioning via proxy-secret path", func() {
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
	})

	AfterEach(func() {
		if kcUserID != "" {
			deleteKeycloakUser(adminToken, kcUserID)
		}
	})

	It("provisions a new actor on first request with a new subject", func() {
		resp := doRequest(http.MethodGet, "/catalog-items",
			withProxySecret(proxySecret, kcUserID),
			withPreferredUsername(username))
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		_, dbUsername, status := getActorByExternalID(kcUserID)
		Expect(dbUsername).To(Equal(username))
		Expect(status).To(Equal("active"))
	})

	It("uses preferred username for the actor username", func() {
		preferred := "my-preferred-" + username
		resp := doRequest(http.MethodGet, "/catalog-items",
			withProxySecret(proxySecret, kcUserID),
			withPreferredUsername(preferred))
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		_, dbUsername, _ := getActorByExternalID(kcUserID)
		Expect(dbUsername).To(Equal(preferred))
	})

	It("falls back to subject as username when preferred username is absent", func() {
		resp := doRequest(http.MethodGet, "/catalog-items",
			withProxySecret(proxySecret, kcUserID))
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		_, dbUsername, _ := getActorByExternalID(kcUserID)
		Expect(dbUsername).To(Equal(kcUserID))
	})

	It("succeeds on subsequent requests for the same subject", func() {
		resp1 := doRequest(http.MethodGet, "/catalog-items",
			withProxySecret(proxySecret, kcUserID),
			withPreferredUsername(username))
		defer resp1.Body.Close()
		Expect(resp1.StatusCode).To(Equal(http.StatusOK))

		resp2 := doRequest(http.MethodGet, "/catalog-items",
			withProxySecret(proxySecret, kcUserID),
			withPreferredUsername(username))
		defer resp2.Body.Close()
		Expect(resp2.StatusCode).To(Equal(http.StatusOK))

		Expect(countActorsByExternalID(kcUserID)).To(Equal(1))
	})
})
