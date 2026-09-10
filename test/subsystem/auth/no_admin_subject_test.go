//go:build subsystem

package subsystem_test

import (
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TC-29: Admin seeding skipped when DCM_ADMIN_SUBJECT is empty.
// Exercised via the dedicated control-plane-configa compose service
// (AUTH_DISABLED=true, DCM_ADMIN_SUBJECT="", its own database with no
// pre-seeded admin actor).
var _ = Describe("Admin seeding skipped when DCM_ADMIN_SUBJECT is empty (TC-29)", Label("configa"), func() {
	BeforeEach(func() {
		waitForConfigAHealthy()
	})

	It("does not create an admin actor", func() {
		var count int
		err := dbNoAdmin.QueryRow("SELECT count(*) FROM actors WHERE username = 'admin'").Scan(&count)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(0))
	})

	It("still serves API requests since auth is disabled", func() {
		resp, err := httpClient.Get(configAURL + "/catalog-items")
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})
})

func waitForConfigAHealthy() {
	GinkgoHelper()
	Eventually(func() error {
		resp, err := httpClient.Get(configAURL + "/health")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("config-a health check returned %d", resp.StatusCode)
		}
		return nil
	}).WithTimeout(90 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
}
