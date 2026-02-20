package handler

import (
	"database/sql"
	"log/slog"
	"net/http"
	"time"
)

type HealthHandler struct {
	db *sql.DB
}

func NewHealthHandler(db *sql.DB) *HealthHandler {
	return &HealthHandler{db: db}
}

func (h *HealthHandler) Liveness(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"version":   "1.0.0",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	dbStatus := "ok"
	httpStatus := http.StatusOK

	if err := h.db.PingContext(r.Context()); err != nil {
		slog.Warn("readiness check failed: database unreachable", "error", err)
		dbStatus = "down"
		httpStatus = http.StatusServiceUnavailable
	}

	overallStatus := "ok"
	if httpStatus != http.StatusOK {
		overallStatus = "down"
	}

	RespondJSON(w, httpStatus, map[string]any{
		"status":    overallStatus,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"checks": map[string]string{
			"database": dbStatus,
		},
	})
}
