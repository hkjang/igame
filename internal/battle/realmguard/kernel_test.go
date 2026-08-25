package realmguard

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"testing"
)

type vector struct {
	Name             string    `json:"name"`
	StageID          string    `json:"stage_id"`
	Difficulty       string    `json:"difficulty"`
	HeroID           string    `json:"hero_id"`
	AccountHeroLevel int       `json:"account_hero_level"`
	Ticks            int       `json:"ticks"`
	ConfigDigest     string    `json:"config_digest"`
	Config           Config    `json:"config"`
	Commands         []Command `json:"commands"`
	Expected         Outcome   `json:"expected"`
}

func loadVectors(t *testing.T) []vector {
	t.Helper()
	raw, err := os.ReadFile("testdata/vectors.json")
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var vectors []vector
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatalf("decode vectors: %v", err)
	}
	if len(vectors) == 0 {
		t.Fatal("no replay vectors")
	}
	return vectors
}

// The browser generates these vectors from the same rules. If this fails after a
// balance or rules change, regenerate with
// `UPDATE_KERNEL_VECTORS=1 npx vitest run src/games/realmguard/kernel` and make
// sure both sides moved together.
func TestReplayMatchesBrowserVectors(t *testing.T) {
	for _, item := range loadVectors(t) {
		t.Run(item.Name, func(t *testing.T) {
			got := Replay(item.Config, item.Commands, item.Ticks, item.AccountHeroLevel)
			if !reflect.DeepEqual(got, item.Expected) {
				want, _ := json.MarshalIndent(item.Expected, "", "  ")
				have, _ := json.MarshalIndent(got, "", "  ")
				t.Fatalf("outcome diverged from the browser\nwant %s\ngot  %s", want, have)
			}
		})
	}
}

func TestDigestMatchesBrowserProjection(t *testing.T) {
	for _, item := range loadVectors(t) {
		t.Run(item.Name, func(t *testing.T) {
			digest, err := Digest(item.Config)
			if err != nil {
				t.Fatalf("digest: %v", err)
			}
			if digest != item.ConfigDigest {
				t.Fatalf("digest %s does not match the browser digest %s", digest, item.ConfigDigest)
			}
		})
	}
}

func TestReplayIsRepeatable(t *testing.T) {
	for _, item := range loadVectors(t) {
		first := Replay(item.Config, item.Commands, item.Ticks, item.AccountHeroLevel)
		second := Replay(item.Config, item.Commands, item.Ticks, item.AccountHeroLevel)
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("%s replayed differently on a second run", item.Name)
		}
	}
}

func TestReplayRejectsFabricatedOutcomes(t *testing.T) {
	vectors := loadVectors(t)
	var held vector
	for _, item := range vectors {
		if item.Name == "held-line" {
			held = item
		}
	}
	if held.Name == "" {
		t.Fatal("missing held-line vector")
	}
	// Dropping the towers a player claims to have built has to change the
	// outcome, otherwise the ledger is not what decides the score.
	stripped := make([]Command, 0, len(held.Commands))
	for _, command := range held.Commands {
		if command.Op != "build" {
			stripped = append(stripped, command)
		}
	}
	got := Replay(held.Config, stripped, held.Ticks, held.AccountHeroLevel)
	if got.Kills >= held.Expected.Kills || got.Lives >= held.Expected.Lives {
		t.Fatalf("a ledger without towers scored as well as one with them: %+v", got)
	}
}

func TestCanonicalNumberRendersLikeTheBrowser(t *testing.T) {
	cases := []struct {
		value float64
		want  string
	}{
		{0, "0"}, {math.Copysign(0, -1), "0"}, {1, "1"}, {75, "75"}, {0.5, "0.5"},
		{0.1 + 0.2, "0.3"}, {1.0 / 3.0, "0.333333"}, {-1.25, "-1.25"}, {2200, "2200"},
	}
	for _, item := range cases {
		if got := canonicalNumber(item.value); got != item.want {
			t.Errorf("canonicalNumber(%v) = %q, want %q", item.value, got, item.want)
		}
	}
}
