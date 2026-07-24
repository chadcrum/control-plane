//go:build subsystem

package subsystem_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Full OIDC round-trip via JWT", func() {
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

	It("authenticates with a Keycloak JWT and JIT provisions the actor", func() {
		token := getUserToken(username, password)

		resp := doRequest(http.MethodGet, "/catalog-items", withBearerToken(token))
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		_, dbUsername, status := getActorByExternalID(kcUserID)
		Expect(dbUsername).To(Equal(username))
		Expect(status).To(Equal("active"))
	})

	It("validates the dcm-api audience in the JWT", func() {
		token := getUserToken(username, password)

		resp := doRequest(http.MethodGet, "/catalog-items", withBearerToken(token))
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	It("returns 401 for a tampered JWT", func() {
		token := getUserToken(username, password)
		tampered := token[:len(token)-4] + "XXXX"

		resp := doRequest(http.MethodGet, "/catalog-items", withBearerToken(tampered))
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))

		problem := readProblemResponse(resp)
		Expect(problem.Detail).To(Equal("invalid bearer token"))
	})
})

var _ = Describe("Client credentials service account JWT", func() {
	It("authenticates and JIT provisions a service account actor", func() {
		token := getServiceAccountToken()
		sub := extractSubClaim(token)

		resp := doRequest(http.MethodGet, "/catalog-items", withBearerToken(token))
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		_, dbUsername, status := getActorByExternalID(sub)
		Expect(dbUsername).To(HavePrefix("service-account-"))
		Expect(status).To(Equal("active"))
	})
})
