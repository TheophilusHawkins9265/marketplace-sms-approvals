package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/example/marketplace-sms-approvals/internal/infrai"
	"github.com/example/marketplace-sms-approvals/internal/marketplace"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	provisioner := marketplace.NewProvisioner(infrai.New(key))

	mux := http.NewServeMux()
	mux.HandleFunc("POST /assets/approve", func(w http.ResponseWriter, r *http.Request) {
		var input marketplace.AssetRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		assets, err := provisioner.Approve(r.Context(), input)
		if err != nil {
			status := http.StatusBadGateway
			var apiErr *infrai.Error
			if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
				status = apiErr.HTTPStatus
			} else if !errors.As(err, &apiErr) {
				status = http.StatusUnprocessableEntity
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, assets)
	})

	address := ":8080"
	log.Printf("SMS asset service listening on %s", address)
	log.Fatal(http.ListenAndServe(address, mux))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write response: %v", err)
	}
}
