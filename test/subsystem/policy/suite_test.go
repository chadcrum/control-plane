//go:build subsystem

package subsystem_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/dcm-project/control-plane/pkg/policy/client"
)

var (
	apiClient *client.ClientWithResponses
	ctx       context.Context
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Suite")
}

var _ = BeforeSuite(func() {
	ctx = context.Background()

	// Get API URL from environment or use default
	apiURL := getEnvOrDefault("API_URL", "http://localhost:8080/api/v1alpha1")

	// Initialize API client
	var err error
	apiClient, err = client.NewClientWithResponses(apiURL,
		client.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("X-Forwarded-User", "test-admin-sub")
			return nil
		}),
	)
	Expect(err).NotTo(HaveOccurred(), "Failed to create API client")

	// Verify health endpoint
	Eventually(func() error {
		resp, err := http.Get(apiURL + "/health")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("health check returned %d", resp.StatusCode)
		}
		return nil
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "Service should be healthy")
})

// Helper functions

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func ptr[T any](v T) *T {
	return &v
}
