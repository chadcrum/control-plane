//go:build subsystem

package subsystem_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// --- Keycloak helpers ---

func getKeycloakAdminToken() string {
	GinkgoHelper()
	tokenURL := keycloakURL + "/realms/master/protocol/openid-connect/token"
	resp, err := httpClient.PostForm(tokenURL, url.Values{
		"grant_type": {"password"},
		"client_id":  {"admin-cli"},
		"username":   {envOrDefault("KEYCLOAK_ADMIN", "admin")},
		"password":   {envOrDefault("KEYCLOAK_ADMIN_PASSWORD", "admin")},
	})
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusOK), "failed to get Keycloak admin token")

	var tokenResp tokenResponse
	Expect(json.NewDecoder(resp.Body).Decode(&tokenResp)).To(Succeed())
	Expect(tokenResp.AccessToken).NotTo(BeEmpty())
	return tokenResp.AccessToken
}

func createKeycloakUser(adminToken, username, password string) string {
	GinkgoHelper()
	usersURL := keycloakURL + "/admin/realms/dcm/users"

	userPayload := fmt.Sprintf(`{
		"username": %q,
		"enabled": true,
		"emailVerified": true,
		"email": %q,
		"firstName": "Test",
		"lastName": "User",
		"credentials": [{"type": "password", "value": %q, "temporary": false}]
	}`, username, username+"@test.local", password)

	req, err := http.NewRequest(http.MethodPost, usersURL, strings.NewReader(userPayload))
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := httpClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusCreated), "failed to create Keycloak user %q", username)

	location := resp.Header.Get("Location")
	Expect(location).NotTo(BeEmpty())
	parts := strings.Split(location, "/")
	return parts[len(parts)-1]
}

func deleteKeycloakUser(adminToken, userID string) {
	GinkgoHelper()
	userURL := keycloakURL + "/admin/realms/dcm/users/" + userID

	req, err := http.NewRequest(http.MethodDelete, userURL, nil)
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := httpClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
}

func getUserToken(username, password string) string {
	GinkgoHelper()
	tokenURL := keycloakURL + "/realms/dcm/protocol/openid-connect/token"
	resp, err := httpClient.PostForm(tokenURL, url.Values{
		"grant_type":    {"password"},
		"client_id":     {"dcm-proxy"},
		"client_secret": {proxySecret},
		"username":      {username},
		"password":      {password},
	})
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusOK), "failed to get user token for %q", username)

	var tokenResp tokenResponse
	Expect(json.NewDecoder(resp.Body).Decode(&tokenResp)).To(Succeed())
	Expect(tokenResp.AccessToken).NotTo(BeEmpty())
	return tokenResp.AccessToken
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
}

// --- HTTP request helpers ---

type requestOption func(*http.Request)

func withBearerToken(token string) requestOption {
	return func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func withProxySecret(secret, subject string) requestOption {
	return func(req *http.Request) {
		req.Header.Set("X-Auth-Proxy-Secret", secret)
		if subject != "" {
			req.Header.Set("X-Forwarded-User", subject)
		}
	}
}

func withPreferredUsername(preferredUsername string) requestOption {
	return func(req *http.Request) {
		req.Header.Set("X-Forwarded-Preferred-Username", preferredUsername)
	}
}

func doRequest(method, path string, opts ...requestOption) *http.Response {
	GinkgoHelper()
	reqURL := apiURL + path
	req, err := http.NewRequest(method, reqURL, nil)
	Expect(err).NotTo(HaveOccurred())
	for _, opt := range opts {
		opt(req)
	}
	resp, err := httpClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	return resp
}

func doRequestWithBody(method, path, body string, opts ...requestOption) *http.Response {
	GinkgoHelper()
	reqURL := apiURL + path
	req, err := http.NewRequest(method, reqURL, strings.NewReader(body))
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")
	for _, opt := range opts {
		opt(req)
	}
	resp, err := httpClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	return resp
}

type problemResponse struct {
	Type   string `json:"type"`
	Status int    `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

func readProblemResponse(resp *http.Response) problemResponse {
	GinkgoHelper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())
	var problem problemResponse
	Expect(json.Unmarshal(body, &problem)).To(Succeed(), "body: %s", string(body))
	return problem
}

// --- DB helpers ---

func updateActorStatus(externalID, status string) {
	GinkgoHelper()
	result, err := db.Exec(
		"UPDATE actors SET status = $1 WHERE id = (SELECT actor_id FROM actor_identities WHERE external_id = $2 AND auth_provider = 'keycloak')",
		status, externalID,
	)
	Expect(err).NotTo(HaveOccurred())
	rows, err := result.RowsAffected()
	Expect(err).NotTo(HaveOccurred())
	Expect(rows).To(Equal(int64(1)), "expected to update exactly 1 actor for external_id %q", externalID)
}

func getActorByExternalID(externalID string) (actorID, username, status string) {
	GinkgoHelper()
	err := db.QueryRow(
		"SELECT a.id, a.username, a.status FROM actors a JOIN actor_identities ai ON a.id = ai.actor_id WHERE ai.external_id = $1 AND ai.auth_provider = 'keycloak'",
		externalID,
	).Scan(&actorID, &username, &status)
	Expect(err).NotTo(HaveOccurred())
	return
}

func countActorsByExternalID(externalID string) int {
	GinkgoHelper()
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM actor_identities WHERE external_id = $1 AND auth_provider = 'keycloak'",
		externalID,
	).Scan(&count)
	Expect(err).NotTo(HaveOccurred())
	return count
}

func extractSubClaim(token string) string {
	GinkgoHelper()
	parts := strings.SplitN(token, ".", 3)
	Expect(parts).To(HaveLen(3))
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	Expect(err).NotTo(HaveOccurred())
	var claims struct {
		Sub string `json:"sub"`
	}
	Expect(json.Unmarshal(decoded, &claims)).To(Succeed())
	Expect(claims.Sub).NotTo(BeEmpty())
	return claims.Sub
}

func getServiceAccountToken() string {
	GinkgoHelper()
	tokenURL := keycloakURL + "/realms/dcm/protocol/openid-connect/token"
	resp, err := httpClient.PostForm(tokenURL, url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"dcm-proxy"},
		"client_secret": {proxySecret},
	})
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusOK), "failed to get service account token")

	var tokenResp tokenResponse
	Expect(json.NewDecoder(resp.Body).Decode(&tokenResp)).To(Succeed())
	Expect(tokenResp.AccessToken).NotTo(BeEmpty())
	return tokenResp.AccessToken
}

// --- Utility ---

func uniqueUsername() string {
	return "test-user-" + uuid.NewString()[:8]
}
