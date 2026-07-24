//go:build subsystem

package subsystem_test

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var (
	apiURL       string
	keycloakURL  string
	proxySecret  string
	adminSubject string
	dbConnStr    string
	httpClient   = &http.Client{Timeout: 10 * time.Second}
	db           *sql.DB
)

func TestSubsystem(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Auth Subsystem Suite")
}

var _ = BeforeSuite(func() {
	apiURL = envOrDefault("API_URL", "http://localhost:28080/api/v1alpha1")
	keycloakURL = envOrDefault("KEYCLOAK_URL", "http://localhost:28180")
	proxySecret = envOrDefault("AUTH_PROXY_SECRET", "test-proxy-secret")
	adminSubject = envOrDefault("DCM_ADMIN_SUBJECT", "56deb662-4820-5d83-b828-f4beb11a5fa7")
	dbConnStr = envOrDefault("DB_CONN_STR", "postgres://test_user:test_password@localhost:25432/auth_test?sslmode=disable")

	var err error
	db, err = sql.Open("pgx", dbConnStr)
	Expect(err).NotTo(HaveOccurred())
	Expect(db.Ping()).To(Succeed())

	Eventually(func() error {
		resp, err := httpClient.Get(apiURL + "/health")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("health check returned %d", resp.StatusCode)
		}
		return nil
	}).WithTimeout(120 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
})

var _ = AfterSuite(func() {
	if db != nil {
		db.Close()
	}
})

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
