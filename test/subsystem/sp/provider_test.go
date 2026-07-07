//go:build subsystem

package subsystem_test

import (
	"context"
	"net/http"
	"os"

	providerapi "github.com/dcm-project/control-plane/api/sp/v1alpha1/provider"
	providerclient "github.com/dcm-project/control-plane/pkg/sp/client/provider"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Provider API", func() {
	var (
		apiClient *providerclient.ClientWithResponses
		ctx       context.Context
	)

	BeforeEach(func() {
		baseURL := os.Getenv("API_URL")
		if baseURL == "" {
			baseURL = "http://localhost:8080/api/v1alpha1"
		}

		var err error
		authEditor := providerclient.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("X-Forwarded-User", "test-admin-sub")
			return nil
		})
		apiClient, err = providerclient.NewClientWithResponses(baseURL, authEditor)
		Expect(err).NotTo(HaveOccurred())

		ctx = context.Background()
	})

	Describe("Health", func() {
		It("returns healthy status", func() {
			baseURL := os.Getenv("API_URL")
			if baseURL == "" {
				baseURL = "http://localhost:8080/api/v1alpha1"
			}

			resp, err := http.Get(baseURL + "/health")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})

	Describe("Provider CRUD", func() {
		It("creates, reads, updates, and deletes a provider", func() {
			By("creating a new provider")
			createResp, err := apiClient.CreateProviderWithResponse(ctx, nil, providerapi.Provider{
				Name:          "e2e-test-provider",
				Endpoint:      "https://example.com/api",
				ServiceType:   "vm",
				SchemaVersion: "v1alpha1",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(createResp.StatusCode()).To(Equal(http.StatusCreated))
			Expect(createResp.JSON201).NotTo(BeNil())
			Expect(createResp.JSON201.Id).NotTo(BeNil())

			providerID := *createResp.JSON201.Id

			By("getting the provider")
			getResp, err := apiClient.GetProviderWithResponse(ctx, providerID)
			Expect(err).NotTo(HaveOccurred())
			Expect(getResp.StatusCode()).To(Equal(http.StatusOK))
			Expect(getResp.JSON200.Name).To(Equal("e2e-test-provider"))

			By("re-registering without ID (idempotent update)")
			reregResp, err := apiClient.CreateProviderWithResponse(ctx, nil, providerapi.Provider{
				Name:          "e2e-test-provider",
				Endpoint:      "https://updated.example.com/api",
				ServiceType:   "vm",
				SchemaVersion: "v1alpha1",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(reregResp.StatusCode()).To(Equal(http.StatusOK))
			Expect(reregResp.JSON200).NotTo(BeNil())
			Expect(*reregResp.JSON200.Id).To(Equal(providerID))

			By("listing providers")
			listResp, err := apiClient.ListProvidersWithResponse(ctx, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(listResp.StatusCode()).To(Equal(http.StatusOK))
			Expect(listResp.JSON200.Providers).NotTo(BeNil())
			Expect(len(*listResp.JSON200.Providers)).To(BeNumerically(">=", 1))

			By("updating the provider")
			updateResp, err := apiClient.ApplyProviderWithResponse(ctx, providerID, providerapi.Provider{
				Name:          "e2e-test-provider-updated",
				Endpoint:      "https://updated.example.com/api",
				ServiceType:   "vm",
				SchemaVersion: "v1alpha1",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(updateResp.StatusCode()).To(Equal(http.StatusOK))
			Expect(updateResp.JSON200.Name).To(Equal("e2e-test-provider-updated"))

			By("deleting the provider")
			deleteResp, err := apiClient.DeleteProviderWithResponse(ctx, providerID)
			Expect(err).NotTo(HaveOccurred())
			Expect(deleteResp.StatusCode()).To(Equal(http.StatusNoContent))

			By("verifying provider is deleted")
			getDeletedResp, err := apiClient.GetProviderWithResponse(ctx, providerID)
			Expect(err).NotTo(HaveOccurred())
			Expect(getDeletedResp.StatusCode()).To(Equal(http.StatusNotFound))
		})
	})

	Describe("Conflict scenarios", func() {
		var providerID string

		BeforeEach(func() {
			resp, err := apiClient.CreateProviderWithResponse(ctx, nil, providerapi.Provider{
				Name:          "conflict-test-provider",
				Endpoint:      "https://example.com/api",
				ServiceType:   "vm",
				SchemaVersion: "v1alpha1",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusCreated))
			providerID = *resp.JSON201.Id
		})

		AfterEach(func() {
			apiClient.DeleteProviderWithResponse(ctx, providerID)
		})

		It("returns 409 when registering same name with different ID", func() {
			newID := uuid.New().String()
			params := &providerapi.CreateProviderParams{Id: &newID}

			resp, err := apiClient.CreateProviderWithResponse(ctx, params, providerapi.Provider{
				Name:          "conflict-test-provider",
				Endpoint:      "https://other.example.com/api",
				ServiceType:   "vm",
				SchemaVersion: "v1alpha1",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusConflict))
		})
	})
})
