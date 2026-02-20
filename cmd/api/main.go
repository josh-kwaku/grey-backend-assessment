package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/josh-kwaku/grey-backend-assessment/internal/config"
	"github.com/josh-kwaku/grey-backend-assessment/internal/fx"
	"github.com/josh-kwaku/grey-backend-assessment/internal/handler"
	"github.com/josh-kwaku/grey-backend-assessment/internal/logging"
	"github.com/josh-kwaku/grey-backend-assessment/internal/middleware"
	"github.com/josh-kwaku/grey-backend-assessment/internal/repository"
	"github.com/josh-kwaku/grey-backend-assessment/internal/service"
	"github.com/josh-kwaku/grey-backend-assessment/internal/service/payment"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logging.Init("grey-api", cfg.LogLevel, cfg.AppEnv)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := repository.NewPostgresDB(ctx, cfg.DatabaseURL, repository.PoolConfig{
		MaxOpenConns:     cfg.DBMaxOpenConns,
		MaxIdleConns:     cfg.DBMaxIdleConns,
		ConnMaxLifetimeS: cfg.DBConnMaxLifetimeS,
		ConnMaxIdleTimeS: cfg.DBConnMaxIdleTimeS,
	})
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	userRepo := repository.NewUserRepository(db)
	accountRepo := repository.NewAccountRepository(db)
	paymentRepo := repository.NewPaymentRepository(db)
	ledgerRepo := repository.NewLedgerRepository(db)
	paymentEventRepo := repository.NewPaymentEventRepository(db)

	fxSvc := fx.NewRateService(cfg.FXSpreadPct)

	accountSvc := service.NewAccountService(accountRepo, userRepo)
	paymentSvc := payment.NewService(paymentRepo, accountRepo, ledgerRepo, paymentEventRepo, userRepo, fxSvc, db, cfg)

	authHandler := handler.NewAuthHandler(userRepo, cfg.JWTSecret, 24*time.Hour)
	userHandler := handler.NewUserHandler(userRepo)
	accountHandler := handler.NewAccountHandler(accountSvc)
	paymentHandler := handler.NewPaymentHandler(paymentSvc)
	fxHandler := handler.NewFXHandler(fxSvc)

	authMW := middleware.Auth(cfg.JWTSecret)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /health/ready", handleHealth)
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)

	mux.Handle("GET /api/v1/users/{id}", authMW(http.HandlerFunc(userHandler.GetByID)))
	mux.Handle("POST /api/v1/users/{id}/accounts", authMW(http.HandlerFunc(accountHandler.Create)))
	mux.Handle("GET /api/v1/users/{id}/accounts", authMW(http.HandlerFunc(accountHandler.List)))

	mux.Handle("POST /api/v1/payments", authMW(http.HandlerFunc(paymentHandler.Create)))
	mux.Handle("GET /api/v1/payments/{id}", authMW(http.HandlerFunc(paymentHandler.Get)))

	mux.Handle("GET /api/v1/fx/rates", authMW(http.HandlerFunc(fxHandler.GetRate)))

	stack := middleware.Tracing(middleware.Logging(middleware.Recovery(mux)))

	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           stack,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("server started", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		slog.Error("failed to write health response", "error", err)
	}
}
