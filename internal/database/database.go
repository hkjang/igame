package database

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/hkjang/igame/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres DSN: %w", err)
	}
	config.MaxConns = 20
	config.MinConns = 2
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		name text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("create schema migrations table: %w", err)
	}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		body, err := migrations.Files.ReadFile(name)
		if err != nil {
			return err
		}
		checksum := fmt.Sprintf("%x", sha256.Sum256(body))
		var existing string
		err = pool.QueryRow(ctx, `SELECT checksum FROM schema_migrations WHERE name=$1`, name).Scan(&existing)
		if err == nil {
			if existing != checksum {
				return fmt.Errorf("migration %s checksum changed", name)
			}
			continue
		}
		if err != pgx.ErrNoRows {
			return fmt.Errorf("read migration %s state: %w", name, err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, string(body)); err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO schema_migrations(name,checksum) VALUES($1,$2)`, name, checksum)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}
	return nil
}

// EnsureBootstrapAdmin creates the first local administrator. It intentionally
// does not overwrite a password later changed through the administrator UI.
func EnsureBootstrapAdmin(ctx context.Context, pool *pgxpool.Pool, username, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `INSERT INTO users(username,display_name,password_hash,role,status)
		VALUES($1,$1,$2,'admin','active') ON CONFLICT(username) DO UPDATE SET
		password_hash=CASE WHEN users.password_hash='' THEN excluded.password_hash ELSE users.password_hash END,
		role='admin',status='active',updated_at=now()`, username, string(hash))
	if err != nil {
		return fmt.Errorf("ensure bootstrap administrator: %w", err)
	}
	return nil
}
