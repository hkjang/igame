package database

import (
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hkjang/igame/migrations"
)

func sqlFiles(names ...string) fstest.MapFS {
	files := fstest.MapFS{}
	for _, name := range names {
		files[name] = &fstest.MapFile{Data: []byte("SELECT 1;")}
	}
	return files
}

// The migrations are applied in the order they are numbered, and the tenth one
// is where sorting the names stops agreeing with that: "10_" sorts before
// "002_". Applying a brand new migration second, against a database that is
// already live, is a broken deployment that only happens once the tenth file
// exists.
func TestATenthMigrationIsAppliedAfterTheNinthAndNotSecond(t *testing.T) {
	names, err := migrationNames(sqlFiles("001_initial.sql", "002_extended.sql", "009_receipt_clock.sql", "10_battle_audit.sql", "11_later.sql"))
	if err != nil {
		t.Fatalf("migrationNames: %v", err)
	}
	want := []string{"001_initial.sql", "002_extended.sql", "009_receipt_clock.sql", "10_battle_audit.sql", "11_later.sql"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("migrations run as %v, want %v", names, want)
	}
}

// Two branches that each add "010_" merge cleanly and leave the order of the
// two schema changes up to whatever the file system reports first. Refusing to
// start says so once instead of applying them differently on different hosts.
func TestTwoMigrationsSharingANumberAreRefused(t *testing.T) {
	_, err := migrationNames(sqlFiles("001_initial.sql", "010_ranking.sql", "010_replay.sql"))
	if err == nil {
		t.Fatal("two migrations numbered 010 were accepted; their order is left to the file system")
	}
	if !strings.Contains(err.Error(), "010_ranking.sql") || !strings.Contains(err.Error(), "010_replay.sql") {
		t.Fatalf("the error does not name both files: %v", err)
	}
}

// A file that carries no number has no place in the sequence, so guessing one
// would apply it at an arbitrary point. It is reported instead.
func TestAMigrationWithoutANumberIsRefused(t *testing.T) {
	for _, name := range []string{"initial.sql", "v2_extended.sql", "003 realmguard.sql"} {
		if _, err := migrationNames(sqlFiles("001_initial.sql", name)); err == nil {
			t.Fatalf("%q was accepted as a migration despite not being numbered", name)
		}
	}
}

// Only the .sql files are schema migrations; migrations.go lives beside them.
func TestNonSQLFilesAreNotTreatedAsMigrations(t *testing.T) {
	files := sqlFiles("001_initial.sql")
	files["migrations.go"] = &fstest.MapFile{Data: []byte("package migrations")}
	files["README.md"] = &fstest.MapFile{Data: []byte("# schema")}
	names, err := migrationNames(files)
	if err != nil {
		t.Fatalf("migrationNames: %v", err)
	}
	if len(names) != 1 || names[0] != "001_initial.sql" {
		t.Fatalf("migrations are %v, want only 001_initial.sql", names)
	}
}

// The shipped migrations have to satisfy the convention the loader enforces,
// and their numbers have to run 1, 2, 3 ... with no gap: a gap means a file
// that a deployed database has already applied went missing from the tree.
func TestTheShippedMigrationsAreNumberedFromOneWithoutGaps(t *testing.T) {
	names, err := migrationNames(migrations.Files)
	if err != nil {
		t.Fatalf("the shipped migrations do not follow the naming convention: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("no migrations are embedded; a fresh database would come up with no schema")
	}
	for i, name := range names {
		number, err := strconv.Atoi(migrationFileName.FindStringSubmatch(name)[1])
		if err != nil {
			t.Fatalf("migration %s has an unreadable number: %v", name, err)
		}
		if number != i+1 {
			t.Fatalf("migration %s is numbered %d but sits at position %d; a migration is missing", name, number, i+1)
		}
	}
}
