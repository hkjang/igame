package database

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/igame/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Each test owns a schema, never the API fixture's default schema. pgcrypto
// must already exist at database scope so schema cleanup cannot remove it.
func migrationPool(t *testing.T) (context.Context, *pgxpool.Pool, string) {
	t.Helper()
	dsn := os.Getenv("IGAME_TEST_DSN")
	if dsn == "" {
		t.Skip("IGAME_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	var extensionSchema string
	if err := admin.QueryRow(ctx, `SELECT n.nspname FROM pg_extension e JOIN pg_namespace n ON n.oid=e.extnamespace WHERE e.extname='pgcrypto'`).Scan(&extensionSchema); err != nil {
		t.Fatal(err)
	}
	if extensionSchema == "public" {
		t.Fatal("prepare pgcrypto in a dedicated extension schema, not public (see README)")
	}
	schema := "migrate_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, `DROP SCHEMA `+quoted+` CASCADE`); err != nil {
			t.Errorf("drop owned schema: %v", err)
		}
	})
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	// Startup parameters apply to every connection, including replacements.
	// Omitting public also prevents an absent local table resolving to API data.
	searchPath := quoted + "," + pgx.Identifier{extensionSchema}.Sanitize()
	config.ConnConfig.RuntimeParams["search_path"] = searchPath
	config.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close) // LIFO: close before dropping the schema.
	// Hold both connections to verify the actual session setting on each one.
	for range 2 {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Release()
		var current, path string
		if err := conn.QueryRow(ctx, `SELECT current_schema(), current_setting('search_path')`).Scan(&current, &path); err != nil {
			t.Fatal(err)
		}
		if current != schema || path != searchPath {
			t.Fatalf("unexpected schema/search_path: %q / %q", current, path)
		}
	}
	return ctx, pool, schema
}

type migrationRecord struct {
	Checksum  string
	AppliedAt time.Time
}

func migrationHistory(t *testing.T, ctx context.Context, pool *pgxpool.Pool) map[string]migrationRecord {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT name, checksum, applied_at FROM schema_migrations ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	records := make(map[string]migrationRecord)
	for rows.Next() {
		var name string
		var record migrationRecord
		if err := rows.Scan(&name, &record.Checksum, &record.AppliedAt); err != nil {
			t.Fatal(err)
		}
		records[name] = record
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return records
}

func embeddedMigrationNames(t *testing.T) []string {
	t.Helper()
	names, err := migrationNames(migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("no embedded migrations")
	}
	return names
}

func assertMigrationHistory(t *testing.T, history map[string]migrationRecord, names []string) {
	t.Helper()
	if len(history) != len(names) {
		t.Fatalf("history has %d rows, want %d: %v", len(history), len(names), history)
	}
	for _, name := range names {
		body, err := migrations.Files.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		record, ok := history[name]
		if !ok || record.Checksum != fmt.Sprintf("%x", sha256.Sum256(body)) || record.AppliedAt.IsZero() {
			t.Fatalf("invalid history for %s: %+v", name, record)
		}
	}
}

// Compare the whole seed row, including its generated ID and timestamps.
func migrationSeed(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var seed string
	if err := pool.QueryRow(ctx, `SELECT to_jsonb(g)::text FROM games g WHERE slug='snake'`).Scan(&seed); err != nil {
		t.Fatal(err)
	}
	return seed
}

func TestMigrateAppliesEmbeddedFilesAndIsIdempotent(t *testing.T) {
	ctx, pool, _ := migrationPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	before := migrationHistory(t, ctx, pool)
	assertMigrationHistory(t, before, embeddedMigrationNames(t))
	seed := migrationSeed(t, ctx, pool)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if after := migrationHistory(t, ctx, pool); !reflect.DeepEqual(before, after) {
		t.Fatalf("rerun changed history: before=%v after=%v", before, after)
	}
	if after := migrationSeed(t, ctx, pool); after != seed {
		t.Fatalf("rerun changed seed: before=%s after=%s", seed, after)
	}
}

func TestMigrateRejectsChangedChecksumAndRecovers(t *testing.T) {
	ctx, pool, _ := migrationPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	names := embeddedMigrationNames(t)
	before := migrationHistory(t, ctx, pool)
	seed := migrationSeed(t, ctx, pool)
	name := names[0]
	const corrupted = "deliberately-corrupted-checksum"
	if _, err := pool.Exec(ctx, `UPDATE schema_migrations SET checksum=$1 WHERE name=$2`, corrupted, name); err != nil {
		t.Fatal(err)
	}
	tampered := migrationHistory(t, ctx, pool)
	err := Migrate(ctx, pool)
	if err == nil || !strings.Contains(err.Error(), name) || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("want checksum error naming %s, got %v", name, err)
	}
	if after := migrationHistory(t, ctx, pool); !reflect.DeepEqual(tampered, after) {
		t.Fatalf("checksum refusal changed history: %v", after)
	}
	if _, err := pool.Exec(ctx, `UPDATE schema_migrations SET checksum=$1 WHERE name=$2`, before[name].Checksum, name); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if after := migrationHistory(t, ctx, pool); !reflect.DeepEqual(before, after) {
		t.Fatalf("recovery changed history: %v", after)
	}
	if after := migrationSeed(t, ctx, pool); after != seed {
		t.Fatal("checksum refusal/recovery changed seed")
	}
}

func TestMigrateRollsBackDDLAndHistoryThenRetries(t *testing.T) {
	ctx, pool, schema := migrationPool(t)
	const target = "010_silent_sso.sql"
	names := embeddedMigrationNames(t)
	index := slices.Index(names, target)
	if index < 1 {
		t.Fatalf("expected migrations preceding %s", target)
	}
	// Fail the real history INSERT after 010 has executed its ALTER TABLE.
	// The trigger also verifies that the DDL was visible inside that transaction.
	_, err := pool.Exec(ctx, `
 CREATE TABLE schema_migrations (
  name text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now());
 CREATE FUNCTION reject_migration_history() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN
  IF NEW.name = '010_silent_sso.sql' THEN
   IF NOT EXISTS (SELECT 1 FROM information_schema.columns
      WHERE table_schema = TG_TABLE_SCHEMA AND table_name = 'oidc_flows' AND column_name = 'silent') THEN
    RAISE EXCEPTION '010 DDL was not executed';
   END IF;
   RAISE EXCEPTION 'injected migration history failure' USING ERRCODE = 'P0001';
  END IF;
  RETURN NEW;
 END $$;
 CREATE TRIGGER reject_migration_history BEFORE INSERT ON schema_migrations
 FOR EACH ROW EXECUTE FUNCTION reject_migration_history();`)
	if err != nil {
		t.Fatal(err)
	}
	err = Migrate(ctx, pool)
	var pgErr *pgconn.PgError
	if err == nil || !strings.Contains(err.Error(), target) || !errors.As(err, &pgErr) || pgErr.Code != "P0001" || pgErr.Message != "injected migration history failure" {
		t.Fatalf("want injected INSERT error for %s, got %v", target, err)
	}
	before := migrationHistory(t, ctx, pool)
	assertMigrationHistory(t, before, names[:index])
	seed := migrationSeed(t, ctx, pool)
	assertSilentColumn := func(want bool) {
		t.Helper()
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns
   WHERE table_schema=$1 AND table_name='oidc_flows' AND column_name='silent')`, schema).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists != want {
			t.Fatalf("silent column exists=%v, want %v", exists, want)
		}
	}
	assertSilentColumn(false)
	if _, err := pool.Exec(ctx, `DROP TRIGGER reject_migration_history ON schema_migrations; DROP FUNCTION reject_migration_history()`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	after := migrationHistory(t, ctx, pool)
	assertMigrationHistory(t, after, names)
	assertSilentColumn(true)
	for name, record := range before {
		if after[name] != record {
			t.Fatalf("retry rewrote completed migration %s", name)
		}
	}
	if after := migrationSeed(t, ctx, pool); after != seed {
		t.Fatal("retry changed previously committed seed")
	}
}

// A start-up can be cancelled while a migration is open: a container
// healthcheck gives up, the orchestrator sends SIGTERM, the DSN deadline
// expires. The three tests above only ever fail a migration through the
// server, so none of them says what that leaves behind. This one stops 010
// inside its own transaction, cancels the context there, and then asks a
// living context what survived and whether the same pool can still finish.
func TestMigrateCancelledMidMigrationLeavesNothingAndRetries(t *testing.T) {
	ctx, pool, schema := migrationPool(t)
	const target = "010_silent_sso.sql"
	names := embeddedMigrationNames(t)
	index := slices.Index(names, target)
	if index < 1 {
		t.Fatalf("expected migrations preceding %s", target)
	}
	// The lock holder stays outside the pool: the pool's two connections are
	// for the migration and for observing it, and cancelling the first must
	// not cost us the second. Advisory locks are shared by the whole database,
	// so the key is drawn per run; keeping it inside 32 bits also keeps it
	// readable in pg_locks.objid with a zero classid.
	holder, err := pgx.Connect(ctx, os.Getenv("IGAME_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer closeCancel()
		if err := holder.Close(closeCtx); err != nil {
			t.Errorf("close lock holder: %v", err)
		}
	})
	key := int64(rand.Uint32()) + 1
	if _, err := holder.Exec(ctx, `SELECT pg_advisory_lock($1)`, key); err != nil {
		t.Fatal(err)
	}
	// Same shape as the rollback test, except the trigger blocks where that one
	// raises: 010's transaction stays open, holding the ALTER TABLE it has
	// already run, until this test decides to cancel it.
	_, err = pool.Exec(ctx, fmt.Sprintf(`
 CREATE TABLE schema_migrations (
  name text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now());
 CREATE FUNCTION hold_migration_history() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN
  IF NEW.name = '010_silent_sso.sql' THEN
   IF NOT EXISTS (SELECT 1 FROM information_schema.columns
      WHERE table_schema = TG_TABLE_SCHEMA AND table_name = 'oidc_flows' AND column_name = 'silent') THEN
    RAISE EXCEPTION '010 DDL was not executed';
   END IF;
   PERFORM pg_advisory_lock(TG_ARGV[0]::bigint);
  END IF;
  RETURN NEW;
 END $$;
 CREATE TRIGGER hold_migration_history BEFORE INSERT ON schema_migrations
 FOR EACH ROW EXECUTE FUNCTION hold_migration_history('%d');`, key))
	if err != nil {
		t.Fatal(err)
	}
	mctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Migrate(mctx, pool) }()
	// Cancel on state rather than on a sleep: the waiter appears only once
	// 010's trigger has reached the advisory lock, which is after its ALTER
	// TABLE ran in the same, still-open transaction.
	var blocked int32
	for deadline := time.Now().Add(time.Minute); ; {
		err := holder.QueryRow(ctx, `SELECT pid FROM pg_locks WHERE locktype='advisory'
   AND classid=0 AND objid::bigint=$1 AND objsubid=1 AND NOT granted`, key).Scan(&blocked)
		if err == nil {
			break
		}
		if err != pgx.ErrNoRows {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			t.Fatalf("Migrate finished without stopping inside %s: %v", target, err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s never reached the advisory lock", target)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	err = <-done
	if err == nil || !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), target) {
		t.Fatalf("want a context.Canceled error naming %s, got %v", target, err)
	}
	assertSilentColumn := func(want bool) {
		t.Helper()
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns
   WHERE table_schema=$1 AND table_name='oidc_flows' AND column_name='silent')`, schema).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists != want {
			t.Fatalf("silent column exists=%v, want %v", exists, want)
		}
	}
	before := migrationHistory(t, ctx, pool)
	assertMigrationHistory(t, before, names[:index])
	seed := migrationSeed(t, ctx, pool)
	assertSilentColumn(false)
	// A cancelled backend holds 010's locks until it actually ends, so wait for
	// that instead of racing the retry against someone else's rollback.
	for deadline := time.Now().Add(time.Minute); ; {
		var alive bool
		if err := holder.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid=$1)`, blocked).Scan(&alive); err != nil {
			t.Fatal(err)
		}
		if !alive {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("backend %d still running after the cancellation", blocked)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := holder.Exec(ctx, `SELECT pg_advisory_unlock($1)`, key); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER hold_migration_history ON schema_migrations; DROP FUNCTION hold_migration_history()`); err != nil {
		t.Fatal(err)
	}
	// The same pool, so a connection killed by the cancellation cannot be what
	// the next start-up would be handed.
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	after := migrationHistory(t, ctx, pool)
	assertMigrationHistory(t, after, names)
	assertSilentColumn(true)
	for name, record := range before {
		if after[name] != record {
			t.Fatalf("retry rewrote completed migration %s", name)
		}
	}
	if after := migrationSeed(t, ctx, pool); after != seed {
		t.Fatal("retry changed previously committed seed")
	}
}
