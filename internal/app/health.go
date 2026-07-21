package app

import (
	"encoding/json"
	"net/http"

	"github.com/dcm-project/control-plane/internal/auth"
	"github.com/go-chi/chi/v5"
)

type healthResponse struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}

func registerMonolithHealth(router chi.Router) {
	router.Get(auth.MonolithHealthPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(healthResponse{
			Status: "ok",
			Path:   auth.MonolithHealthPath,
		})
	})
}
