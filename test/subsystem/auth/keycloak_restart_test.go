//go:build subsystem

package subsystem_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TC-42: Keycloak restart (bounce) resilience.
//
// Scope: this exercises a Keycloak *bounce* only. Keycloak persists its
// signing keys to disk by default, so a plain restart does not rotate the
// active JWKS key - this test verifies the OIDC discovery/JWKS round-trip
// survives a Keycloak restart, NOT that the control-plane correctly picks up
// a genuinely rotated key (different `kid`). True key-rotation resilience
// would require swapping the realm's signing key via the Keycloak Admin API
// and is not covered here; track separately if that's a real production risk.
var _ = Describe("Keycloak restart resilience (TC-42)", Label("restart"), func() {
	It("authenticates with a fresh token after Keycloak restarts", func() {
		By("obtaining a token and confirming the API works before restart")
		tokenBefore := getServiceAccountToken()
		resp := doRequest(http.MethodGet, "/catalog-items", withBearerToken(tokenBefore))
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		By("restarting Keycloak")
		container := findComposeContainer("keycloak")
		restartContainer(container)
		waitForKeycloakHealthy()

		By("obtaining a new token after restart and calling the API")
		tokenAfter := getServiceAccountToken()
		respAfter := doRequest(http.MethodGet, "/catalog-items", withBearerToken(tokenAfter))
		defer respAfter.Body.Close()
		Expect(respAfter.StatusCode).To(Equal(http.StatusOK))
	})
})
