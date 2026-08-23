package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/hkjang/igame/internal/api"
	"github.com/hkjang/igame/internal/config"
	"github.com/hkjang/igame/internal/database"
	"github.com/hkjang/igame/internal/secretbox"
	"github.com/hkjang/igame/internal/version"
	"github.com/jackc/pgx/v5/pgxpool"
)

const healthcheckURL = "http://127.0.0.1:8080/healthz"

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		if err := checkHealth(ctx, healthcheckURL); err != nil {
			log.Error("healthcheck failed", "error", err)
			os.Exit(1)
		}
		return
	}
	cfg, err := config.Load()
	if err != nil {
		log.Error("configuration error", "error", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	db, err := database.Open(ctx, cfg.PostgresDSN)
	if err == nil {
		err = database.Migrate(ctx, db)
	}
	if err == nil {
		err = database.EnsureBootstrapAdmin(ctx, db, cfg.BootstrapAdmin, cfg.BootstrapPassword)
	}
	cancel()
	if err != nil {
		log.Error("startup failed", "error", err)
		if db != nil {
			db.Close()
		}
		os.Exit(1)
	}
	defer db.Close()
	box, err := secretbox.New(cfg.EncryptionKey)
	if err != nil {
		log.Error("encryption initialization failed", "error", err)
		os.Exit(1)
	}
	service := api.New(db, box, log)
	server := &http.Server{Addr: ":8080", Handler: service.Router(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 0, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 1 << 20}
	// Idle stream clients must not hold a graceful shutdown open for the whole
	// shutdown timeout.
	server.RegisterOnShutdown(service.Drain)
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()
	go cleanupExpired(cleanupCtx, db, log)
	go func() {
		log.Info("igame started", "address", server.Addr, "version", version.Version, "commit", version.Commit)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	cleanupCancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
	}
}

// checkHealth is used by the package-free runtime image. It deliberately runs
// before application configuration and database initialization so Docker can
// probe an already-running container without a shell or utility binary.
func checkHealth(ctx context.Context, endpoint string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create health request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request health endpoint: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("health endpoint returned %s", resp.Status)
	}
	return nil
}

// cleanupExpired keeps all background state in PostgreSQL and introduces no
// additional runtime setting.
func cleanupExpired(ctx context.Context, db *pgxpool.Pool, log *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	// Run once at startup so a restart also clears whatever expired while the
	// service was down, instead of waiting a full hour for the first tick.
	cleanupOnce(ctx, db, log)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanupOnce(ctx, db, log)
		}
	}
}

func cleanupOnce(ctx context.Context, db *pgxpool.Pool, log *slog.Logger) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	_, sessionErr := db.Exec(ctx, `DELETE FROM auth_sessions WHERE expires_at<now()`)
	_, flowErr := db.Exec(ctx, `DELETE FROM oidc_flows WHERE expires_at<now()`)
	_, gameErr := db.Exec(ctx, `UPDATE game_sessions SET status='abandoned',ended_at=now(),duration_ms=GREATEST(0,extract(epoch FROM(now()-started_at))*1000)::bigint WHERE status='active' AND started_at<now()-interval '24 hours'`)
	if sessionErr != nil || flowErr != nil || gameErr != nil {
		log.Warn("expired state cleanup failed", "session_error", sessionErr, "flow_error", flowErr, "game_session_error", gameErr)
	}
}
