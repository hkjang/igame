package api

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigratedPoolIsolation(t *testing.T) {
	dsn := os.Getenv("IGAME_TEST_DSN")
	if dsn == "" {
		t.Skip("IGAME_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	var extensionOID, namespaceOID uint32
	var extensionSchema string
	if err := admin.QueryRow(ctx, `SELECT e.oid, n.oid, n.nspname FROM pg_extension e JOIN pg_namespace n ON n.oid=e.extnamespace WHERE e.extname='pgcrypto'`).Scan(&extensionOID, &namespaceOID, &extensionSchema); err != nil {
		t.Fatal(err)
	}
	sibling := migratedPool(t)
	schemaOf := func(t *testing.T, pool *pgxpool.Pool) string {
		t.Helper()
		var schema string
		if err := pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
			t.Fatal(err)
		}
		return schema
	}
	siblingSchema := schemaOf(t, sibling)
	checkConnections := func(t *testing.T, pool *pgxpool.Pool, schema string) {
		t.Helper()
		if !strings.HasPrefix(schema, "api_test_") {
			t.Fatalf("schema %q lacks api_test_ prefix", schema)
		}
		if _, err := uuid.Parse(strings.TrimPrefix(schema, "api_test_")); err != nil {
			t.Fatalf("schema UUID: %v", err)
		}
		wantPath := pgx.Identifier{schema}.Sanitize() + "," + pgx.Identifier{extensionSchema}.Sanitize()
		// Keep both acquired until checked, forcing two distinct backend sessions.
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
			if current != schema || path != wantPath {
				t.Fatalf("schema/search_path = %q / %q, want %q / %q", current, path, schema, wantPath)
			}
		}
	}
	writeSetting := func(t *testing.T, pool *pgxpool.Pool, value string) {
		t.Helper()
		tag, err := pool.Exec(ctx, `UPDATE system_settings SET value=jsonb_build_object('fixture', $1::text) WHERE key='service'`, value)
		if err != nil {
			t.Fatal(err)
		}
		if tag.RowsAffected() != 1 {
			t.Fatal("missing migrated service seed")
		}
	}
	assertSetting := func(t *testing.T, pool *pgxpool.Pool, want string) {
		t.Helper()
		var got string
		if err := pool.QueryRow(ctx, `SELECT value->>'fixture' FROM system_settings WHERE key='service'`).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("setting = %q, want %q", got, want)
		}
	}
	var childSchema string
	if !t.Run("child", func(t *testing.T) {
		child := migratedPool(t)
		childSchema = schemaOf(t, child)
		if childSchema == siblingSchema {
			t.Fatalf("fixtures share schema %q", childSchema)
		}
		checkConnections(t, sibling, siblingSchema)
		checkConnections(t, child, childSchema)
		writeSetting(t, sibling, "sibling")
		writeSetting(t, child, "child")
		assertSetting(t, sibling, "sibling")
		assertSetting(t, child, "child")
	}) {
		return
	}
	var exists bool
	if err := admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname=$1)`, childSchema).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatalf("child schema %q survived cleanup", childSchema)
	}
	if err := admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname=$1)`, siblingSchema).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("sibling schema removed by child cleanup")
	}
	assertSetting(t, sibling, "sibling")
	var gotExtensionOID, gotNamespaceOID uint32
	var gotSchema string
	if err := admin.QueryRow(ctx, `SELECT e.oid, n.oid, n.nspname FROM pg_extension e JOIN pg_namespace n ON n.oid=e.extnamespace WHERE e.extname='pgcrypto'`).Scan(&gotExtensionOID, &gotNamespaceOID, &gotSchema); err != nil {
		t.Fatal(err)
	}
	if gotExtensionOID != extensionOID || gotNamespaceOID != namespaceOID || gotSchema != extensionSchema {
		t.Fatal("pgcrypto identity or namespace changed during cleanup")
	}
}
