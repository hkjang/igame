package database

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
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

// migrationFileName is the convention every schema migration follows: a
// zero-padded number, an underscore, a lower-case name, then ".sql".
var migrationFileName = regexp.MustCompile(`^([0-9]+)_[a-z0-9_]+\.sql$`)

// migrationNames lists the schema migrations in the order they must be applied.
//
// The files are ordered by their number rather than by their name. Sorting the
// names agrees with the numbers only while every number is padded to the same
// width, so the moment a "10_x.sql" joined the existing "001_".."009_" it would
// sort second and try to alter tables that do not exist yet — against a live
// database, halfway through a deployment. Sorting on the parsed number instead
// keeps the order the numbers describe no matter how they are written.
//
// A name that does not follow the convention, or a number used twice, is
// refused rather than applied at a guessed position: the files are compiled in,
// so this is a mistake in the repository that every start-up should report the
// same way instead of one that depends on the file system.
func migrationNames(fsys fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	type migration struct {
		number int
		name   string
	}
	found := make([]migration, 0, len(entries))
	seen := make(map[int]string, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		match := migrationFileName.FindStringSubmatch(name)
		if match == nil {
			return nil, fmt.Errorf("migration %s is not named <number>_<name>.sql", name)
		}
		number, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, fmt.Errorf("migration %s has an unreadable number: %w", name, err)
		}
		if other, duplicate := seen[number]; duplicate {
			return nil, fmt.Errorf("migrations %s and %s share the number %d", other, name, number)
		}
		seen[number] = name
		found = append(found, migration{number: number, name: name})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].number < found[j].number })
	names := make([]string, 0, len(found))
	for _, entry := range found {
		names = append(names, entry.name)
	}
	return names, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		name text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("create schema migrations table: %w", err)
	}
	names, err := migrationNames(migrations.Files)
	if err != nil {
		return err
	}
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
