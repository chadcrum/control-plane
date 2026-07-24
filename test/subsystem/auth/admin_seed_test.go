//go:build subsystem

package subsystem_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Admin actor seeding", func() {
	It("seeds an active admin actor on startup", func() {
		resp := doRequest(http.MethodGet, "/catalog-items",
			withProxySecret(proxySecret, adminSubject))
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		_, username, status := getActorByExternalID(adminSubject)
		Expect(username).To(Equal("admin"))
		Expect(status).To(Equal("active"))
	})

	It("binds the admin identity to the configured subject", func() {
		count := countActorsByExternalID(adminSubject)
		Expect(count).To(Equal(1))
	})
})
