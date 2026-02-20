package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/josh-kwaku/grey-backend-assessment/internal/logging"
)

func main() {
	logging.Init("mock-provider", "info", os.Getenv("APP_ENV"))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			slog.Error("failed to write health response", "error", err)
		}
	})

	slog.Info("mock provider started", "addr", ":8081")
	if err := http.ListenAndServe(":8081", mux); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
