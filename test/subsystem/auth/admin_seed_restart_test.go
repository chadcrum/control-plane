//go:build subsystem

package subsystem_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TC-28: Admin seeding is idempotent on restart.
// Restarting the control-plane must not create a duplicate admin actor —
// Seed() checks for an existing admin actor by username before creating one.
var _ = Describe("Admin actor seeding idempotency (TC-28)", Label("restart"), func() {
	It("does not duplicate the admin actor after a control-plane restart", func() {
		By("confirming exactly one admin actor before restart")
		Expect(countActorsByExternalID(adminSubject)).To(Equal(1))

		By("restarting the control-plane container")
		container := findComposeContainer("control-plane")
		restartContainer(container)
		waitForHealthy()

		By("confirming still exactly one admin actor after restart")
		Expect(countActorsByExternalID(adminSubject)).To(Equal(1))

		_, username, status := getActorByExternalID(adminSubject)
		Expect(username).To(Equal("admin"))
		Expect(status).To(Equal("active"))
	})
})
