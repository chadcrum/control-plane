//go:build subsystem

package subsystem_test

import (
	"net/http"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Concurrent first-login race condition", func() {
	var (
		adminToken string
	)

	BeforeEach(func() {
		adminToken = getKeycloakAdminToken()
	})

	It("handles concurrent JIT provisioning for the same subject", func() {
		username := uniqueUsername()
		kcUserID := createKeycloakUser(adminToken, username, "testpass")
		defer deleteKeycloakUser(adminToken, kcUserID)

		const concurrency = 10
		var wg sync.WaitGroup
		statuses := make([]int, concurrency)

		wg.Add(concurrency)
		for i := 0; i < concurrency; i++ {
			go func(idx int) {
				defer GinkgoRecover()
				defer wg.Done()
				resp := doRequest(http.MethodGet, "/catalog-items",
					withProxySecret(proxySecret, kcUserID),
					withPreferredUsername(username))
				defer resp.Body.Close()
				statuses[idx] = resp.StatusCode
			}(i)
		}
		wg.Wait()

		for i, status := range statuses {
			Expect(status).To(Equal(http.StatusOK), "request %d returned %d", i, status)
		}
		Expect(countActorsByExternalID(kcUserID)).To(Equal(1))
	})

	It("returns 409 on username collision between different subjects", func() {
		collisionName := "collision-" + uniqueUsername()

		user1 := uniqueUsername()
		kcUser1ID := createKeycloakUser(adminToken, user1, "testpass")
		defer deleteKeycloakUser(adminToken, kcUser1ID)

		resp1 := doRequest(http.MethodGet, "/catalog-items",
			withProxySecret(proxySecret, kcUser1ID),
			withPreferredUsername(collisionName))
		defer resp1.Body.Close()
		Expect(resp1.StatusCode).To(Equal(http.StatusOK))

		user2 := uniqueUsername()
		kcUser2ID := createKeycloakUser(adminToken, user2, "testpass")
		defer deleteKeycloakUser(adminToken, kcUser2ID)

		resp2 := doRequest(http.MethodGet, "/catalog-items",
			withProxySecret(proxySecret, kcUser2ID),
			withPreferredUsername(collisionName))
		Expect(resp2.StatusCode).To(Equal(http.StatusConflict))

		problem := readProblemResponse(resp2)
		Expect(problem.Detail).To(Equal("username already in use by another account"))
	})
})
