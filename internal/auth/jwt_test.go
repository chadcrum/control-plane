package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{name: "valid", header: "Bearer eyJhbGciOi.test.token", want: "eyJhbGciOi.test.token"},
		{name: "lowercase", header: "bearer eyJhbGciOi.test.token", want: "eyJhbGciOi.test.token"},
		{name: "mixed case", header: "BEARER eyJhbGciOi.test.token", want: "eyJhbGciOi.test.token"},
		{name: "empty", header: "", want: ""},
		{name: "no scheme", header: "eyJhbGciOi.test.token", want: ""},
		{name: "wrong scheme", header: "Basic dXNlcjpwYXNz", want: ""},
		{name: "bearer only", header: "Bearer ", want: ""},
		{name: "bearer no space", header: "Bearertoken", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				r.Header.Set("Authorization", tt.header)
			}
			got := extractBearerToken(r)
			if got != tt.want {
				t.Errorf("extractBearerToken() = %q, want %q", got, tt.want)
			}
		})
	}
}
