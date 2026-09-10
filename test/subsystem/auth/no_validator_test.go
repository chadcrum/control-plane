//go:build subsystem

package subsystem_test

import (
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TC-13: No JWTValidator falls through to proxy path.
// When AUTH_ISSUER_URL is not set (Config C, exercised by the dedicated
// control-plane-configc compose service), Bearer tokens are ignored and the
// proxy-header path is used instead.
var _ = Describe("No JWTValidator falls through to proxy path (TC-13)", Label("configc"), func() {
	BeforeEach(func() {
		waitForConfigCHealthy()
	})

	It("ignores an invalid Bearer token and authenticates via proxy headers", func() {
		req, err := http.NewRequest(http.MethodGet, configCURL+"/catalog-items", nil)
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Authorization", "Bearer not-a-real-jwt")
		req.Header.Set("X-Auth-Proxy-Secret", proxySecret)
		req.Header.Set("X-Forwarded-User", adminSubject)

		resp, err := httpClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	It("still rejects requests with no valid auth at all", func() {
		req, err := http.NewRequest(http.MethodGet, configCURL+"/catalog-items", nil)
		Expect(err).NotTo(HaveOccurred())

		resp, err := httpClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		resp.Body.Close()
	})
})

func waitForConfigCHealthy() {
	GinkgoHelper()
	Eventually(func() error {
		resp, err := httpClient.Get(configCURL + "/health")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("config-c health check returned %d", resp.StatusCode)
		}
		return nil
	}).WithTimeout(90 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
}
