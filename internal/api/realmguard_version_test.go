package api

import "testing"

// A draft used to take its content version from the table's row count as
// `0.2.<n>`, ignoring what was actually published. A draft cut from published
// 0.3.1 came out as 0.2.5, and publishing it moved the live content version
// backwards past two releases that had already shipped.
func TestRealmGuardNextContentVersionAdvancesPastEverythingOnFile(t *testing.T) {
	for _, item := range []struct {
		name     string
		existing []string
		want     string
	}{
		{"the newest line, not the oldest", []string{"0.3.1", "0.2.0", "0.3.0"}, "0.3.2"},
		{"a saved draft's revision suffix is not a new line", []string{"0.3.1", "0.3.2-r7"}, "0.3.3"},
		{"a second open draft advances again", []string{"0.3.1", "0.3.2"}, "0.3.3"},
		{"double digits compare as numbers, not as text", []string{"0.9.9", "0.10.1"}, "0.10.2"},
		{"a single version is enough", []string{"1.0.0"}, "1.0.1"},
	} {
		t.Run(item.name, func(t *testing.T) {
			if got := realmGuardNextContentVersion(item.existing); got != item.want {
				t.Fatalf("next content version is %s, want %s", got, item.want)
			}
		})
	}
}

func TestRealmGuardNextContentVersionLeavesUnparseableVersionsAlone(t *testing.T) {
	// Inventing a number would claim an ordering the content does not have.
	if got := realmGuardNextContentVersion([]string{"nightly", "nightly-r3"}); got != "nightly" {
		t.Fatalf("got %s, want the version unchanged", got)
	}
	if got := realmGuardNextContentVersion(nil); got != "0.0.1" {
		t.Fatalf("got %s, want 0.0.1 for an empty table", got)
	}
}
