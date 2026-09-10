//go:build subsystem

package subsystem_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TC-14 gap: Wrong-audience JWT rejected.
// A token that is validly signed by the trusted issuer but carries an
// audience other than "dcm-api" must be rejected. This proves the
// control-plane checks `aud`, not just the signature. Stock clients
// (dcm-proxy, dcm-cli) always map aud=dcm-api, so a dedicated client
// without that mapper is created for this test.
var _ = Describe("Wrong-audience JWT rejected (TC-14 gap)", Label("audience"), func() {
	var (
		adminToken   string
		clientUUID   string
		wrongAudCID  string
		wrongAudSecr = "wrong-audience-secret"
	)

	BeforeEach(func() {
		adminToken = getKeycloakAdminToken()
		wrongAudCID = "wrong-audience-client-" + uuid.NewString()[:8]
		clientUUID = createConfidentialClient(adminToken, wrongAudCID, wrongAudSecr)
	})

	AfterEach(func() {
		if clientUUID != "" {
			deleteKeycloakClient(adminToken, clientUUID)
		}
	})

	It("rejects a validly-signed token whose audience is not dcm-api", func() {
		token := getClientCredentialsToken(wrongAudCID, wrongAudSecr)

		resp := doRequest(http.MethodGet, "/catalog-items", withBearerToken(token))
		problem := readProblemResponse(resp)
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		Expect(problem.Detail).To(Equal("invalid bearer token"))
	})
})

// createConfidentialClient creates a confidential client with
// serviceAccountsEnabled but WITHOUT a dcm-api audience mapper, so tokens
// it issues carry only the default audience (the client ID itself).
func createConfidentialClient(adminToken, clientID, secret string) string {
	GinkgoHelper()
	clientsURL := keycloakURL + "/admin/realms/dcm/clients"

	payload := fmt.Sprintf(`{
		"clientId": %q,
		"enabled": true,
		"protocol": "openid-connect",
		"publicClient": false,
		"secret": %q,
		"directAccessGrantsEnabled": false,
		"serviceAccountsEnabled": true,
		"standardFlowEnabled": false
	}`, clientID, secret)

	req, err := http.NewRequest(http.MethodPost, clientsURL, strings.NewReader(payload))
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := httpClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusCreated), "failed to create client %q", clientID)

	location := resp.Header.Get("Location")
	Expect(location).NotTo(BeEmpty())
	parts := strings.Split(location, "/")
	return parts[len(parts)-1]
}

func deleteKeycloakClient(adminToken, clientUUID string) {
	GinkgoHelper()
	clientURL := keycloakURL + "/admin/realms/dcm/clients/" + clientUUID

	req, err := http.NewRequest(http.MethodDelete, clientURL, nil)
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := httpClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
}

func getClientCredentialsToken(clientID, secret string) string {
	GinkgoHelper()
	tokenURL := keycloakURL + "/realms/dcm/protocol/openid-connect/token"
	resp, err := httpClient.PostForm(tokenURL, url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {secret},
	})
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusOK), "failed to get token for client %q", clientID)

	var tokenResp tokenResponse
	Expect(json.NewDecoder(resp.Body).Decode(&tokenResp)).To(Succeed())
	Expect(tokenResp.AccessToken).NotTo(BeEmpty())
	return tokenResp.AccessToken
}
