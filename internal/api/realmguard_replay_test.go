package api

import (
	"encoding/json"
	"os"
	"testing"

	battle "github.com/hkjang/igame/internal/battle/realmguard"
)

type projectionFixture struct {
	Cases []struct {
		StageID    string `json:"stage_id"`
		Difficulty string `json:"difficulty"`
		HeroID     string `json:"hero_id"`
		Digest     string `json:"digest"`
	} `json:"cases"`
}

// The browser normalizes published content before it plays; this server projects
// the same content before it verifies. If the two ever read a field differently
// every submitted battle would be refused, so the agreement is pinned here.
// Regenerate with `UPDATE_KERNEL_VECTORS=1 npx vitest run src/games/realmguard/kernel`.
func loadProjectionFixture(t *testing.T) (realmGuardDecodedContent, projectionFixture) {
	t.Helper()
	published, err := os.ReadFile("testdata/realmguard_published_config.json")
	if err != nil {
		t.Fatalf("read published config: %v", err)
	}
	content, err := decodeRealmGuardContent(published)
	if err != nil {
		t.Fatalf("decode content: %v", err)
	}
	raw, err := os.ReadFile("testdata/realmguard_projection.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture projectionFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return content, fixture
}

func TestKernelProjectionMatchesBrowser(t *testing.T) {
	content, fixture := loadProjectionFixture(t)
	if len(fixture.Cases) == 0 {
		t.Fatal("no projection cases")
	}
	for _, item := range fixture.Cases {
		t.Run(item.StageID+"/"+item.Difficulty+"/"+item.HeroID, func(t *testing.T) {
			var stage *realmGuardStageDefinition
			for index := range content.Stages {
				if content.Stages[index].ID == item.StageID {
					stage = &content.Stages[index]
					break
				}
			}
			if stage == nil {
				t.Fatalf("stage %s missing from fixture content", item.StageID)
			}
			config, err := realmGuardKernelConfig(content, *stage, item.Difficulty, item.HeroID)
			if err != nil {
				t.Fatalf("project: %v", err)
			}
			digest, err := battle.Digest(config)
			if err != nil {
				t.Fatalf("digest: %v", err)
			}
			if digest != item.Digest {
				t.Fatalf("projection digest %s does not match the browser digest %s", digest, item.Digest)
			}
		})
	}
}

func TestReplayRefusesLedgersItCannotTrust(t *testing.T) {
	content, fixture := loadProjectionFixture(t)
	stage := content.Stages[0]
	good := fixture.Cases[0]

	ledger := func(mutate func(*battle.Ledger)) json.RawMessage {
		value := battle.Ledger{
			RulesVersion: battle.RulesVersion,
			ConfigDigest: good.Digest,
			Ticks:        60,
			Commands:     []battle.Command{{Tick: 1, Op: "build", Spot: stage.TowerSpots[0].ID, Tower: "sunspire"}},
		}
		if mutate != nil {
			mutate(&value)
		}
		encoded, _ := json.Marshal(value)
		return encoded
	}

	for _, item := range []struct {
		name    string
		payload json.RawMessage
		code    string
	}{
		{"absent", nil, "missing_ledger"},
		{"unparsable", json.RawMessage(`{`), "invalid_ledger"},
		{"stale rules", ledger(func(l *battle.Ledger) { l.RulesVersion = "realmguard-kernel-0" }), "ledger_rules_mismatch"},
		{"forged digest", ledger(func(l *battle.Ledger) { l.ConfigDigest = "0000000000000000" }), "content_projection_mismatch"},
		{"absurd length", ledger(func(l *battle.Ledger) { l.Ticks = battle.TickLimit + 1 }), "invalid_ledger"},
		{"command past the end", ledger(func(l *battle.Ledger) { l.Commands[0].Tick = 999 }), "invalid_ledger"},
		{"commands out of order", ledger(func(l *battle.Ledger) {
			l.Commands = append(l.Commands, battle.Command{Tick: 0, Op: "wave"})
		}), "invalid_ledger"},
	} {
		t.Run(item.name, func(t *testing.T) {
			_, _, err := replayRealmGuardBattle(item.payload, content, stage, good.Difficulty, good.HeroID, 1)
			rejection, ok := err.(realmGuardResultError)
			if !ok {
				t.Fatalf("expected a rejection, got %v", err)
			}
			if rejection.Code != item.code {
				t.Fatalf("rejected with %q, want %q", rejection.Code, item.code)
			}
		})
	}
}

func TestReplayDerivesTheOutcomeItself(t *testing.T) {
	content, fixture := loadProjectionFixture(t)
	stage := content.Stages[0]
	good := fixture.Cases[0]
	encoded, _ := json.Marshal(battle.Ledger{
		RulesVersion: battle.RulesVersion,
		ConfigDigest: good.Digest,
		Ticks:        1200,
		Commands: []battle.Command{
			{Tick: 1, Op: "build", Spot: stage.TowerSpots[1].ID, Tower: "sunspire"},
			{Tick: 4, Op: "wave"},
		},
	})
	outcome, attestation, err := replayRealmGuardBattle(encoded, content, stage, good.Difficulty, good.HeroID, 1)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if attestation.Method != realmGuardReplayMethod || attestation.Ticks != 1200 || attestation.Commands != 2 {
		t.Fatalf("unexpected attestation %+v", attestation)
	}
	if outcome.Spawned == 0 || outcome.DurationMS != 1200*battle.TickMS {
		t.Fatalf("replay did not simulate the battle: %+v", outcome)
	}

	// A client that claims a perfect run gets the replayed numbers regardless.
	in := realmGuardResultInput{Kills: 9999, EarnedGold: 9999}
	applied := applyRealmGuardReplay(in, outcome)
	if applied.Kills != outcome.Kills || applied.EarnedGold != outcome.EarnedGold {
		t.Fatalf("client-reported battle numbers survived the replay: %+v", applied)
	}
	if *applied.RemainingLives != outcome.Lives || *applied.Victory != outcome.Victory {
		t.Fatalf("client-reported lives or victory survived the replay: %+v", applied)
	}
}
