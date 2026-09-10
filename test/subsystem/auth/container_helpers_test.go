//go:build subsystem

package subsystem_test

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// containerEngine returns "docker" or "podman", preferring $CONTAINER_ENGINE
// (set by CI, see .github/workflows/subsystem.yaml) and falling back to
// whichever binary is on PATH. Skips the calling test if neither is found.
func containerEngine() string {
	GinkgoHelper()
	if engine := os.Getenv("CONTAINER_ENGINE"); engine != "" {
		return engine
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker"
	}
	if _, err := exec.LookPath("podman"); err == nil {
		return "podman"
	}
	Skip("neither docker nor podman found on PATH — cannot manage containers for this test")
	return ""
}

// composeProjectName matches COMPOSE_PROJECT_NAME set in make/auth.mk. Scoping
// by project (not just service label) matters because every subsystem suite
// (auth, catalog, policy, sp) names its main service "control-plane" - without
// this, a stale container from another suite running locally could be
// restarted instead of the intended one.
const composeProjectName = "auth-subsystem"

// findComposeContainer resolves the container name for a compose service
// label scoped to this suite's project, regardless of compose project naming
// convention (v1 vs v2) or container engine. Fails loudly if zero or more
// than one container matches, rather than silently picking one.
func findComposeContainer(service string) string {
	GinkgoHelper()
	engine := containerEngine()
	out, err := exec.Command(engine, "ps",
		"--filter", "label=com.docker.compose.service="+service,
		"--filter", "label=com.docker.compose.project="+composeProjectName,
		"--format", "{{.Names}}").CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "listing containers: %s", string(out))

	names := strings.Fields(strings.TrimSpace(string(out)))
	if len(names) == 0 {
		Skip(fmt.Sprintf("no running container found for compose service %q in project %q", service, composeProjectName))
	}
	Expect(names).To(HaveLen(1),
		"expected exactly one container for service %q in project %q, found %v - a stale container from a previous run may be present",
		service, composeProjectName, names)
	return names[0]
}

// restartContainer restarts the named container via the detected engine.
// Idle keep-alive connections in httpClient are closed afterwards so
// subsequent requests can't be routed to a socket left open to the old
// (now-dead) process.
func restartContainer(name string) {
	GinkgoHelper()
	engine := containerEngine()
	out, err := exec.Command(engine, "restart", name).CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "restarting container %q: %s", name, string(out))
	httpClient.CloseIdleConnections()
}

// waitForHealthy polls the control-plane health endpoint until it returns 200
// or the timeout elapses.
func waitForHealthy() {
	GinkgoHelper()
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
	}).WithTimeout(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
}

// waitForKeycloakHealthy polls Keycloak's OIDC discovery endpoint until it
// responds or the timeout elapses.
func waitForKeycloakHealthy() {
	GinkgoHelper()
	Eventually(func() error {
		resp, err := httpClient.Get(keycloakURL + "/realms/dcm/.well-known/openid-configuration")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("OIDC discovery returned %d", resp.StatusCode)
		}
		return nil
	}).WithTimeout(90 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
}
