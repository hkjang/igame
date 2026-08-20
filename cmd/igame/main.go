package main

import (
	"context"
	"errors"
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

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
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
	server := &http.Server{Addr: ":8080", Handler: api.New(db, box, log).Router(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 0, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 1 << 20}
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

// cleanupExpired keeps all background state in PostgreSQL and introduces no
// additional runtime setting.
func cleanupExpired(ctx context.Context, db *pgxpool.Pool, log *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, sessionErr := db.Exec(ctx, `DELETE FROM auth_sessions WHERE expires_at<now()`)
			_, flowErr := db.Exec(ctx, `DELETE FROM oidc_flows WHERE expires_at<now()`)
			_, gameErr := db.Exec(ctx, `UPDATE game_sessions SET status='abandoned',ended_at=now(),duration_ms=GREATEST(0,extract(epoch FROM(now()-started_at))*1000)::bigint WHERE status='active' AND started_at<now()-interval '24 hours'`)
			if sessionErr != nil || flowErr != nil || gameErr != nil {
				log.Warn("expired state cleanup failed", "session_error", sessionErr, "flow_error", flowErr, "game_session_error", gameErr)
			}
		}
	}
}
