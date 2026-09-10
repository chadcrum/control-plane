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
	configCURL   string
	configAURL   string
	keycloakURL  string
	proxySecret  string
	adminSubject string
	dbConnStr    string
	dbNoAdminStr string
	httpClient   = &http.Client{Timeout: 10 * time.Second}
	db           *sql.DB
	dbNoAdmin    *sql.DB
)

func TestSubsystem(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Auth Subsystem Suite")
}

var _ = BeforeSuite(func() {
	apiURL = envOrDefault("API_URL", "http://localhost:28080/api/v1alpha1")
	configCURL = envOrDefault("CONFIG_C_API_URL", "http://localhost:28081/api/v1alpha1")
	configAURL = envOrDefault("CONFIG_A_API_URL", "http://localhost:28082/api/v1alpha1")
	keycloakURL = envOrDefault("KEYCLOAK_URL", "http://localhost:28180")
	proxySecret = envOrDefault("AUTH_PROXY_SECRET", "test-proxy-secret")
	adminSubject = envOrDefault("DCM_ADMIN_SUBJECT", "56deb662-4820-5d83-b828-f4beb11a5fa7")
	dbConnStr = envOrDefault("DB_CONN_STR", "postgres://test_user:test_password@localhost:25432/auth_test?sslmode=disable")
	dbNoAdminStr = envOrDefault("DB_NOADMIN_CONN_STR", "postgres://test_user:test_password@localhost:25432/auth_test_noadmin?sslmode=disable")

	var err error
	db, err = sql.Open("pgx", dbConnStr)
	Expect(err).NotTo(HaveOccurred())
	Expect(db.Ping()).To(Succeed())

	dbNoAdmin, err = sql.Open("pgx", dbNoAdminStr)
	Expect(err).NotTo(HaveOccurred())
	// Retry: the isolated auth_test_noadmin DB is created by a one-shot
	// db-init-noadmin compose service that may still be running when the
	// suite starts.
	Eventually(func() error {
		return dbNoAdmin.Ping()
	}).WithTimeout(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

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
	if dbNoAdmin != nil {
		dbNoAdmin.Close()
	}
})

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
