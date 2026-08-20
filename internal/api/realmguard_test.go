package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCalculateRealmGuardScoreStars(t *testing.T) {
	stage := realmGuardStageDefinition{Lives: 20}
	balance := realmGuardBalanceDefinition{
		Difficulties:          map[string]realmGuardDifficultyBalance{"normal": {DifficultyBonus: 5000}},
		ClearTimeTargetMS:     900000,
		ClearTimeBonusDivisor: 100,
	}
	tests := []struct {
		lives int
		stars int
	}{{20, 3}, {18, 3}, {17, 2}, {10, 2}, {9, 1}, {1, 1}, {0, 0}}
	for _, test := range tests {
		_, stars, _ := calculateRealmGuardScore(stage, balance, "campaign", "normal", 60000, test.lives, 100, 8, true)
		if stars != test.stars {
			t.Fatalf("lives=%d: got %d stars, want %d", test.lives, stars, test.stars)
		}
	}
	_, stars, _ := calculateRealmGuardScore(stage, balance, "campaign", "normal", 60000, 20, 100, 8, false)
	if stars != 0 {
		t.Fatalf("defeat received %d stars", stars)
	}
}

func TestRealmGuardInitialGoldAndBattleHeroLevel(t *testing.T) {
	stage := realmGuardStageDefinition{StartingGold: 290}
	balance := realmGuardBalanceDefinition{
		Difficulties: map[string]realmGuardDifficultyBalance{
			"casual":  {Gold: 1.18},
			"normal":  {Gold: 1},
			"veteran": {Gold: .9},
		},
		HeroLevelXP: []int64{0, 8, 20, 38, 62, 92, 130, 176, 230, 292},
	}
	if got := realmGuardInitialGold(stage, balance, "casual"); got != 342 {
		t.Fatalf("casual initial gold=%d, want 342", got)
	}
	if got := realmGuardInitialGold(stage, balance, "veteran"); got != 261 {
		t.Fatalf("veteran initial gold=%d, want 261", got)
	}
	for _, test := range []struct{ kills, level int }{{0, 1}, {7, 1}, {8, 2}, {19, 2}, {20, 3}, {291, 9}, {292, 10}, {5000, 10}} {
		if got := realmGuardBattleHeroLevel(test.kills, balance.HeroLevelXP); got != test.level {
			t.Fatalf("kills=%d: battle level=%d, want %d", test.kills, got, test.level)
		}
	}
	balance.MinWaveDurationMS = 5000
	if got := realmGuardMinimumDurationMS(0, false, balance); got != 5000 {
		t.Fatalf("first-wave defeat minimum duration=%d, want 5000", got)
	}
	if got := realmGuardMinimumDurationMS(8, true, balance); got != 40000 {
		t.Fatalf("eight-wave clear minimum duration=%d, want 40000", got)
	}
}

func TestRealmGuardWaveCapacityEndlessAndSummons(t *testing.T) {
	content := realmGuardDecodedContent{
		Waves: []realmGuardWaveDefinition{{ID: "w1", StageID: "endless-rift", Number: 1, Reward: 10, Entries: []realmGuardWaveEntry{
			{Enemy: "shardling", Count: 2, Interval: 1},
			{Enemy: "hollow_king", Count: 1, Interval: 1},
			{Enemy: "timewyrm", Count: 1, Interval: 1},
		}}},
		Enemies: []realmGuardEnemyDefinition{
			{ID: "shardling", Reward: 14, LifeDamage: 1, Traits: []string{"splitting"}},
			{ID: "mireling", Reward: 8, LifeDamage: 1},
			{ID: "veilrunner", Reward: 19, LifeDamage: 1},
			{ID: "glintfox", Reward: 11, LifeDamage: 1},
		},
		Bosses: []realmGuardEnemyDefinition{
			{ID: "hollow_king", Reward: 220, LifeDamage: 10},
			{ID: "timewyrm", Reward: 400, LifeDamage: 15},
		},
	}
	budget := realmGuardWaveCapacity(content, "endless-rift", 2, false)
	if budget.BaseSpawns != 14 || budget.MaxSpawns != 58 || budget.Rewards != 20 {
		t.Fatalf("unexpected endless budget: %+v", budget)
	}
	minimum, maximum := realmGuardRewardBounds(5, budget.RewardCounts)
	if minimum != 40 || maximum != 1820 {
		t.Fatalf("reward bounds=(%d,%d), want (40,1820)", minimum, maximum)
	}
}

func TestValidateRealmGuardCombatOutcome(t *testing.T) {
	budget := realmGuardWaveBudget{BaseSpawns: 10, MaxSpawns: 20, MinLifeDamage: 1, MaxLifeDamage: 3, LifeDamageCounts: map[int]int{1: 20, 3: 20}}
	if err := validateRealmGuardCombatOutcome(false, 20, 20, 0, 0, 0, budget); err == nil || !strings.Contains(err.Error(), "defeat") {
		t.Fatalf("fake no-activity defeat was accepted: %v", err)
	}
	if err := validateRealmGuardCombatOutcome(false, 20, 0, 0, 7, 7, budget); err != nil {
		t.Fatalf("reachable defeat rejected: %v", err)
	}
	if err := validateRealmGuardCombatOutcome(true, 20, 18, 9, 0, 9, budget); err == nil {
		t.Fatal("clear without all base spawns was accepted")
	}
	if err := validateRealmGuardCombatOutcome(true, 20, 18, 10, 2, 12, budget); err != nil {
		t.Fatalf("valid clear rejected: %v", err)
	}
	maximumInt := int(^uint(0) >> 1)
	if err := validateRealmGuardCombatOutcome(false, 20, 0, 0, maximumInt, maximumInt, budget); err == nil {
		t.Fatalf("unbounded hostile counters were accepted: %v", err)
	}
}

func TestRealmGuardSellRefundBound(t *testing.T) {
	towers := []realmGuardTowerDefinition{{Cost: 75}, {Cost: 105}}
	if maximum := realmGuardMaxSoldGold(750, towers, .65); maximum >= 600 || maximum < 488 {
		t.Fatalf("unexpected sell refund bound: %d", maximum)
	}
	if maximum := realmGuardMaxSoldGold(0, towers, .65); maximum != 0 {
		t.Fatalf("zero spending permits %d sold gold", maximum)
	}
}

func TestValidateRealmGuardContent(t *testing.T) {
	t.Run("canonical minimum", func(t *testing.T) {
		if err := validateRealmGuardContent(testRealmGuardContent(t, false, false)); err != nil {
			t.Fatalf("valid content rejected: %v", err)
		}
	})
	t.Run("new stage and enemy", func(t *testing.T) {
		if err := validateRealmGuardContent(testRealmGuardContent(t, true, false)); err != nil {
			t.Fatalf("expanded content rejected: %v", err)
		}
	})
	t.Run("wave reference and order", func(t *testing.T) {
		raw := testRealmGuardContent(t, false, false)
		var document map[string]json.RawMessage
		if err := json.Unmarshal(raw, &document); err != nil {
			t.Fatal(err)
		}
		var waves []map[string]any
		_ = json.Unmarshal(document["waves"], &waves)
		waves[0]["entries"] = []map[string]any{{"enemy": "missing", "count": 1, "interval": 1}}
		document["waves"], _ = json.Marshal(waves)
		broken, _ := json.Marshal(document)
		if err := validateRealmGuardContent(broken); err == nil {
			t.Fatal("unknown enemy reference was accepted")
		}
		raw = testRealmGuardContent(t, false, false)
		_ = json.Unmarshal(raw, &document)
		_ = json.Unmarshal(document["waves"], &waves)
		waves[1]["number"] = 4
		document["waves"], _ = json.Marshal(waves)
		broken, _ = json.Marshal(document)
		if err := validateRealmGuardContent(broken); err == nil {
			t.Fatal("duplicate/non-contiguous wave number was accepted")
		}
	})
	t.Run("wave eager expansion budget", func(t *testing.T) {
		raw := testRealmGuardContent(t, false, false)
		var document map[string]json.RawMessage
		_ = json.Unmarshal(raw, &document)
		var waves []map[string]any
		_ = json.Unmarshal(document["waves"], &waves)
		waves[0]["entries"] = []map[string]any{{"enemy": "mireling", "count": 501, "interval": 1}}
		document["waves"], _ = json.Marshal(waves)
		broken, _ := json.Marshal(document)
		if err := validateRealmGuardContent(broken); err == nil {
			t.Fatal("wave exceeding the eager expansion budget was accepted")
		}
	})
	t.Run("invalid combat number", func(t *testing.T) {
		raw := testRealmGuardContent(t, false, true)
		if err := validateRealmGuardContent(raw); err == nil {
			t.Fatal("zero tower damage was accepted")
		}
	})
	t.Run("overflowing balance", func(t *testing.T) {
		raw := testRealmGuardContent(t, false, false)
		var document map[string]json.RawMessage
		_ = json.Unmarshal(raw, &document)
		var balance map[string]any
		_ = json.Unmarshal(document["balance"], &balance)
		balance["endless_wave_bonus"] = float64(1e15)
		document["balance"], _ = json.Marshal(balance)
		broken, _ := json.Marshal(document)
		if err := validateRealmGuardContent(broken); err == nil {
			t.Fatal("overflowing endless wave bonus was accepted")
		}
	})
	t.Run("telemetry identifiers and payload budget", func(t *testing.T) {
		raw := testRealmGuardContent(t, false, false)
		var document map[string]json.RawMessage
		_ = json.Unmarshal(raw, &document)
		var enemies []map[string]any
		_ = json.Unmarshal(document["enemies"], &enemies)
		enemies[0]["id"] = strings.Repeat("x", 33)
		document["enemies"], _ = json.Marshal(enemies)
		broken, _ := json.Marshal(document)
		if err := validateRealmGuardContent(broken); err == nil {
			t.Fatal("oversized telemetry identifier was accepted")
		}
		ids := map[string]bool{}
		for index := 0; index < 100; index++ {
			ids[strings.Repeat("x", 29)+string(rune('a'+index/26))+string(rune('a'+index%26))] = true
		}
		if realmGuardTelemetryPayloadFits(ids) {
			t.Fatal("oversized ranked telemetry payload was accepted")
		}
	})
	t.Run("extreme enemy and campaign gap", func(t *testing.T) {
		raw := testRealmGuardContent(t, false, false)
		var document map[string]json.RawMessage
		_ = json.Unmarshal(raw, &document)
		var enemies []map[string]any
		_ = json.Unmarshal(document["enemies"], &enemies)
		enemies[0]["life_damage"] = float64(10000)
		document["enemies"], _ = json.Marshal(enemies)
		extreme, _ := json.Marshal(document)
		if err := validateRealmGuardContent(extreme); err == nil {
			t.Fatal("extreme life damage was accepted")
		}
		raw = testRealmGuardContent(t, true, false)
		_ = json.Unmarshal(raw, &document)
		var stages []map[string]any
		_ = json.Unmarshal(document["stages"], &stages)
		for _, stage := range stages {
			if stage["id"] == "stage-11" {
				stage["number"] = float64(13)
			}
		}
		document["stages"], _ = json.Marshal(stages)
		gap, _ := json.Marshal(document)
		if err := validateRealmGuardContent(gap); err == nil {
			t.Fatal("campaign numbering gap was accepted")
		}
	})
	t.Run("parallel paths and bounds", func(t *testing.T) {
		raw := testRealmGuardContent(t, false, false)
		var document map[string]json.RawMessage
		_ = json.Unmarshal(raw, &document)
		var stages, waves []map[string]any
		_ = json.Unmarshal(document["stages"], &stages)
		_ = json.Unmarshal(document["waves"], &waves)
		delete(stages[0], "path")
		stages[0]["paths"] = [][]map[string]any{{{"x": 0, "y": 100}, {"x": 1000, "y": 100}}, {{"x": 0, "y": 200}, {"x": 1000, "y": 200}}}
		entries := waves[0]["entries"].([]any)
		entry := entries[0].(map[string]any)
		entry["path_index"], entry["parallel"] = float64(1), true
		document["stages"], _ = json.Marshal(stages)
		document["waves"], _ = json.Marshal(waves)
		parallel, _ := json.Marshal(document)
		if err := validateRealmGuardContent(parallel); err != nil {
			t.Fatalf("valid parallel path content rejected: %v", err)
		}
		entry["path_index"] = float64(2)
		document["waves"], _ = json.Marshal(waves)
		outOfLane, _ := json.Marshal(document)
		if err := validateRealmGuardContent(outOfLane); err == nil {
			t.Fatal("out-of-range path_index was accepted")
		}
		entry["path_index"] = float64(1)
		paths := stages[0]["paths"].([][]map[string]any)
		paths[0][0]["x"] = float64(1500)
		document["stages"], _ = json.Marshal(stages)
		offscreen, _ := json.Marshal(document)
		if err := validateRealmGuardContent(offscreen); err == nil {
			t.Fatal("offscreen path point was accepted")
		}
	})
}

func TestRealmGuardScoreArithmeticSaturatesInsteadOfWrapping(t *testing.T) {
	const maximum = int64(1<<63 - 1)
	if got := realmGuardScoreProduct(maximum, 2); got != maximum {
		t.Fatalf("overflow product = %d, want %d", got, maximum)
	}
	if got := realmGuardScoreTotal(maximum-2, 10); got != maximum {
		t.Fatalf("overflow total = %d, want %d", got, maximum)
	}
}

func TestRealmGuardConfigPayloadPreservesTopLevelWaveIdentity(t *testing.T) {
	payload, err := realmGuardConfigPayload(realmGuardVersionRecord{RawContent: testRealmGuardContent(t, false, false)})
	if err != nil {
		t.Fatal(err)
	}
	waves := payload["waves"].([]map[string]any)
	if waves[0]["stage_id"] == nil || waves[0]["number"] == nil {
		t.Fatalf("top-level wave identity was removed: %#v", waves[0])
	}
	stages := payload["stages"].([]map[string]any)
	embedded := stages[0]["waves"].([]map[string]any)
	if embedded[0]["stage_id"] != nil || embedded[0]["number"] != nil {
		t.Fatalf("embedded wave unexpectedly retains routing fields: %#v", embedded[0])
	}
}

func TestValidateRealmGuardDraftSectionShape(t *testing.T) {
	if err := validateRealmGuardDraftSection("towers", json.RawMessage(`{}`)); err == nil {
		t.Fatal("object-shaped tower section was accepted")
	}
	if err := validateRealmGuardDraftSection("towers", json.RawMessage(`[{"id":"tower"}]`)); err != nil {
		t.Fatalf("recoverable incomplete array section rejected: %v", err)
	}
	if err := validateRealmGuardDraftSection("balance", json.RawMessage(`[]`)); err == nil {
		t.Fatal("array-shaped balance section was accepted")
	}
}

func TestRealmGuardDraftPreconditionAndManagerTeam(t *testing.T) {
	t.Run("if-match", func(t *testing.T) {
		request := httptest.NewRequest("PUT", "/api/v1/admin/realmguard/drafts/stages", nil)
		response := httptest.NewRecorder()
		if _, ok := realmGuardExpectedChecksum(response, request); ok || response.Code != 428 {
			t.Fatalf("missing If-Match = (%d,%v), want (428,false)", response.Code, ok)
		}
		request = httptest.NewRequest("PUT", "/api/v1/admin/realmguard/drafts/stages", nil)
		request.Header.Set("If-Match", `"`+strings.Repeat("A", 64)+`"`)
		response = httptest.NewRecorder()
		checksum, ok := realmGuardExpectedChecksum(response, request)
		if !ok || checksum != strings.Repeat("a", 64) {
			t.Fatalf("valid If-Match = (%q,%v)", checksum, ok)
		}
	})
	for _, test := range []struct {
		manager string
		creator string
		code    string
	}{{"", "blue", "team_required"}, {"blue", "", "team_required"}, {"blue", "red", "different_team"}, {"blue", "blue", ""}} {
		code, _ := realmGuardManagerReviewTeamError(test.manager, test.creator)
		if code != test.code {
			t.Fatalf("teams (%q,%q): code=%q, want %q", test.manager, test.creator, code, test.code)
		}
	}
}

func TestRequestedRealmGuardVersionID(t *testing.T) {
	if id, err := requestedRealmGuardVersionID(map[string]any{}); err != nil || id != nil {
		t.Fatalf("missing version precondition = (%v,%v), want (nil,nil)", id, err)
	}
	want := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	id, err := requestedRealmGuardVersionID(map[string]any{"realmguard_version_id": want.String()})
	if err != nil || id == nil || *id != want {
		t.Fatalf("valid version precondition = (%v,%v), want %s", id, err, want)
	}
	for _, value := range []any{"", "not-a-uuid", uuid.Nil.String(), 17} {
		if id, err := requestedRealmGuardVersionID(map[string]any{"realmguard_version_id": value}); err == nil || id != nil {
			t.Fatalf("invalid version precondition %v = (%v,%v), want (nil,error)", value, id, err)
		}
	}
}

func TestRealmGuardTelemetryClassLimitsReserveRequiredEvents(t *testing.T) {
	counts := map[string]int{"realmguard.hero.move": realmGuardOptionalTelemetryLimit}
	if !realmGuardTelemetryLimitReached("realmguard.hero.skill", counts) {
		t.Fatal("optional telemetry exceeded its class limit")
	}
	if realmGuardTelemetryLimitReached("realmguard.battle.complete", counts) {
		t.Fatal("optional telemetry consumed the reserved battle.complete slot")
	}
	counts["realmguard.wave.start"] = 1000
	counts["realmguard.wave.complete"] = 999
	if realmGuardTelemetryLimitReached("realmguard.wave.start", counts) || realmGuardTelemetryLimitReached("realmguard.wave.complete", counts) {
		t.Fatal("a normal long endless run exhausted a required milestone class")
	}
	counts["realmguard.battle.complete"] = 1
	if !realmGuardTelemetryLimitReached("realmguard.battle.complete", counts) {
		t.Fatal("a duplicate battle.complete event was accepted")
	}
}

func TestValidateRealmGuardTelemetryAttestation(t *testing.T) {
	started := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	stage := realmGuardStageDefinition{ID: "stage-1", Mode: "campaign", StartingGold: 300, Lives: 20, Version: "1.1.0"}
	content := realmGuardDecodedContent{
		Stages:  []realmGuardStageDefinition{stage},
		Enemies: []realmGuardEnemyDefinition{{ID: "mireling", Reward: 8, LifeDamage: 1}},
		Waves: []realmGuardWaveDefinition{
			{ID: "w1", StageID: stage.ID, Number: 1, Reward: 32, Entries: []realmGuardWaveEntry{{Enemy: "mireling", Count: 1}}},
			{ID: "w2", StageID: stage.ID, Number: 2, Reward: 35, Entries: []realmGuardWaveEntry{{Enemy: "mireling", Count: 1}}},
			{ID: "w3", StageID: stage.ID, Number: 3, Reward: 40, Entries: []realmGuardWaveEntry{{Enemy: "mireling", Count: 21}}},
		},
		Balance: realmGuardBalanceDefinition{
			Difficulties:        map[string]realmGuardDifficultyBalance{"normal": {Gold: 1}},
			TowerUpgradeCost:    []int64{0, 75, 145},
			DurationToleranceMS: 5000,
			MinWaveDurationMS:   5000,
			SellRefundRate:      .65,
		},
	}
	lives, gold := 0, 383
	in := realmGuardResultInput{
		StageID: "stage-1", Mode: "campaign", Difficulty: "normal", DurationMS: 120000,
		RemainingLives: &lives, RemainingGold: &gold, EarnedGold: 83, Kills: 2, Escaped: 20, Spawned: 23, WavesCompleted: 2,
		HeroID: "aerin", HeroLevel: 1, ContentVersion: "0.2.0", BalanceVersion: "2026.08.1", StageVersion: "1.1.0", AssetVersion: "procedural-1",
		DefeatedByEnemy: map[string]int{"mireling": 2}, EscapedByEnemy: map[string]int{"mireling": 20}, SpawnedByEnemy: map[string]int{"mireling": 23},
	}
	version := realmGuardVersionRecord{ContentVersion: in.ContentVersion, BalanceVersion: in.BalanceVersion, AssetVersion: in.AssetVersion}
	record := func(sequence int, offset time.Duration, event string, data any) realmGuardTelemetryRecord {
		raw, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		return realmGuardTelemetryRecord{ID: int64(sequence), Sequence: sequence, ClientEventID: uuid.New(), Event: event, Data: raw, ReceivedAt: started.Add(offset)}
	}
	firstSnapshot := map[string]any{"stage_id": stage.ID, "wave": 1, "lives": 20, "gold": 340, "earned_gold": 40, "spent_gold": 0, "sold_gold": 0, "kills": 1, "escaped": 0, "spawned": 1, "hero_level": 1, "defeated_by_enemy": map[string]int{"mireling": 1}, "escaped_by_enemy": map[string]int{}, "spawned_by_enemy": map[string]int{"mireling": 1}}
	secondSnapshot := map[string]any{"stage_id": stage.ID, "wave": 2, "lives": 20, "gold": 383, "earned_gold": 83, "spent_gold": 0, "sold_gold": 0, "kills": 2, "escaped": 0, "spawned": 2, "hero_level": 1, "defeated_by_enemy": map[string]int{"mireling": 2}, "escaped_by_enemy": map[string]int{}, "spawned_by_enemy": map[string]int{"mireling": 2}}
	complete := map[string]any{"stage_id": stage.ID, "mode": in.Mode, "difficulty": in.Difficulty, "duration_ms": in.DurationMS, "lives": lives, "gold": gold, "earned_gold": in.EarnedGold, "spent_gold": 0, "sold_gold": 0, "kills": in.Kills, "escaped": in.Escaped, "spawned": in.Spawned, "waves": in.WavesCompleted, "waves_completed": in.WavesCompleted, "hero_id": in.HeroID, "hero_level": in.HeroLevel, "content_version": in.ContentVersion, "balance_version": in.BalanceVersion, "stage_version": in.StageVersion, "asset_version": in.AssetVersion, "victory": false, "defeated_by_enemy": in.DefeatedByEnemy, "escaped_by_enemy": in.EscapedByEnemy, "spawned_by_enemy": in.SpawnedByEnemy}
	records := []realmGuardTelemetryRecord{
		record(1, 100*time.Millisecond, "realmguard.battle.ready", map[string]any{"stage_id": stage.ID, "difficulty": in.Difficulty, "hero_id": in.HeroID}),
		record(2, 5*time.Second, "realmguard.wave.start", map[string]any{"stage_id": stage.ID, "wave": 1, "early_call": false, "early_bonus": 0}),
		record(3, 10*time.Second, "realmguard.wave.complete", firstSnapshot),
		record(4, 15*time.Second, "realmguard.wave.start", map[string]any{"stage_id": stage.ID, "wave": 2, "early_call": false, "early_bonus": 0}),
		record(5, 20*time.Second, "realmguard.wave.complete", secondSnapshot),
		record(6, 25*time.Second, "realmguard.wave.start", map[string]any{"stage_id": stage.ID, "wave": 3, "early_call": false, "early_bonus": 0}),
		record(7, 120*time.Second, "realmguard.battle.complete", complete),
	}
	attestation, err := validateRealmGuardTelemetryAttestation(records, started, started.Add(121*time.Second), stage, content, version, in, false)
	if err != nil {
		t.Fatalf("valid telemetry rejected: %v", err)
	}
	if attestation.Method != realmGuardVerificationMethod || attestation.WavesCompleted != 2 || attestation.Digest == "" {
		t.Fatalf("unexpected attestation: %+v", attestation)
	}

	t.Run("sequence gap", func(t *testing.T) {
		broken := append([]realmGuardTelemetryRecord(nil), records...)
		broken[3].Sequence = 9
		if _, err := validateRealmGuardTelemetryAttestation(broken, started, started.Add(121*time.Second), stage, content, version, in, false); err == nil {
			t.Fatal("sequence gap was accepted")
		}
	})
	t.Run("histogram reward tamper", func(t *testing.T) {
		broken := in
		broken.DefeatedByEnemy = map[string]int{"mireling": 1, "unknown": 1}
		if err := validateRealmGuardEnemyHistograms(stage, content, broken, false, 0); err == nil {
			t.Fatal("unknown/reward-tampered histogram was accepted")
		}
	})
	t.Run("batched active wave", func(t *testing.T) {
		broken := append([]realmGuardTelemetryRecord(nil), records...)
		broken[5].ReceivedAt = started.Add(119500 * time.Millisecond)
		if _, err := validateRealmGuardTelemetryAttestation(broken, started, started.Add(121*time.Second), stage, content, version, in, false); err == nil {
			t.Fatal("sub-second active wave milestone was accepted")
		}
	})
	t.Run("final histogram cannot move backwards", func(t *testing.T) {
		brokenInput := in
		brokenGold := 375
		brokenInput.RemainingGold = &brokenGold
		brokenInput.EarnedGold = 75
		brokenInput.Kills = 1
		brokenInput.DefeatedByEnemy = map[string]int{"mireling": 1}
		brokenComplete := map[string]any{
			"stage_id": stage.ID, "mode": brokenInput.Mode, "difficulty": brokenInput.Difficulty,
			"duration_ms": brokenInput.DurationMS, "lives": lives, "gold": brokenGold,
			"earned_gold": brokenInput.EarnedGold, "spent_gold": 0, "sold_gold": 0,
			"kills": brokenInput.Kills, "escaped": brokenInput.Escaped, "spawned": brokenInput.Spawned,
			"waves": brokenInput.WavesCompleted, "waves_completed": brokenInput.WavesCompleted,
			"hero_id": brokenInput.HeroID, "hero_level": brokenInput.HeroLevel,
			"content_version": brokenInput.ContentVersion, "balance_version": brokenInput.BalanceVersion,
			"stage_version": brokenInput.StageVersion, "asset_version": brokenInput.AssetVersion,
			"victory": false, "defeated_by_enemy": brokenInput.DefeatedByEnemy,
			"escaped_by_enemy": brokenInput.EscapedByEnemy, "spawned_by_enemy": brokenInput.SpawnedByEnemy,
		}
		brokenRecords := append([]realmGuardTelemetryRecord(nil), records...)
		brokenRecords[len(brokenRecords)-1] = record(7, 120*time.Second, "realmguard.battle.complete", brokenComplete)
		if _, err := validateRealmGuardTelemetryAttestation(brokenRecords, started, started.Add(121*time.Second), stage, content, version, brokenInput, false); err == nil {
			t.Fatal("final histogram decrease from the last completed wave was accepted")
		}
	})
}

func testRealmGuardContent(t *testing.T, expanded, invalidTower bool) []byte {
	t.Helper()
	points := []map[string]any{{"x": 0, "y": 100}, {"x": 1000, "y": 100}}
	spots := make([]map[string]any, 8)
	for index := range spots {
		spots[index] = map[string]any{"id": "spot-" + string(rune('a'+index)), "x": 100 + index*80, "y": 300}
	}
	stages := []map[string]any{}
	for number := 1; number <= 10; number++ {
		stages = append(stages, map[string]any{"id": "stage-" + itoa(number), "number": number, "name": "Stage", "subtitle": "Defend", "mode": "campaign", "theme": "verdant", "path": points, "tower_spots": spots, "starting_gold": 300, "lives": 20, "version": "1.0.0"})
	}
	if expanded {
		stages = append(stages, map[string]any{"id": "stage-11", "number": 11, "name": "New", "subtitle": "New stage", "mode": "campaign", "theme": "ember", "path": points, "tower_spots": spots, "starting_gold": 320, "lives": 20, "version": "1.11.0"})
	}
	endlessNumber := 11
	if expanded {
		endlessNumber = 12
	}
	stages = append(stages, map[string]any{"id": "endless-rift", "number": endlessNumber, "name": "Endless", "subtitle": "Defend", "mode": "endless", "theme": "void", "path": points, "tower_spots": spots, "starting_gold": 300, "lives": 20, "version": "1.0.0"})
	waves := []map[string]any{}
	for _, stage := range stages {
		for number := 1; number <= 8; number++ {
			waves = append(waves, map[string]any{"id": stage["id"].(string) + "-w" + itoa(number), "stage_id": stage["id"], "number": number, "label": "Wave", "reward": 10, "entries": []map[string]any{{"enemy": "mireling", "count": 2, "interval": 1}}})
		}
	}
	enemyIDs := []string{"mireling", "thornback", "glintfox", "cloudray", "bloomseer", "shardling", "ironroot", "veilrunner", "rammer", "rimeheart"}
	if expanded {
		enemyIDs = append(enemyIDs, "new-enemy")
	}
	enemies := []map[string]any{}
	for index, id := range enemyIDs {
		traits := []string{}
		if id == "shardling" {
			traits = []string{"splitting"}
		}
		enemies = append(enemies, map[string]any{"id": id, "name": "Enemy", "color": 100 + index, "hp": 10, "speed": 10, "armor": .1, "reward": 8, "life_damage": 1, "radius": 10, "traits": traits})
	}
	bosses := []map[string]any{
		{"id": "hollow_king", "name": "Boss", "color": 1, "hp": 100, "speed": 10, "armor": .1, "reward": 100, "life_damage": 10, "radius": 20, "traits": []string{"boss"}},
		{"id": "timewyrm", "name": "Boss", "color": 2, "hp": 100, "speed": 10, "armor": .1, "reward": 100, "life_damage": 15, "radius": 20, "traits": []string{"boss"}},
	}
	towerIDs := []string{"sunspire", "runebloom", "stonepulse", "windward"}
	branchIDs := []string{"dawn_volley", "eagle_oath", "star_lattice", "null_petal", "quake_drum", "ember_core", "shield_line", "skyrider_watch"}
	towers := []map[string]any{}
	for index, id := range towerIDs {
		damage := 10
		if invalidTower && index == 0 {
			damage = 0
		}
		towers = append(towers, map[string]any{"id": id, "name": "Tower", "role": "Defense", "color": 10 + index, "cost": 10, "damage": damage, "range": 100, "fire_rate": 1, "projectile_speed": 100, "damage_type": "physical", "branches": []map[string]any{
			{"id": branchIDs[index*2], "name": "Branch", "description": "Upgrade", "damage_multiplier": 1.2},
			{"id": branchIDs[index*2+1], "name": "Branch", "description": "Upgrade", "range_multiplier": 1.2},
		}})
	}
	heroes := []map[string]any{}
	for index, id := range []string{"aerin", "brann", "nyra"} {
		heroes = append(heroes, map[string]any{"id": id, "name": "Hero", "title": "Guardian", "color": index + 1, "hp": 100, "damage": 10, "range": 100, "speed": 100, "respawn_seconds": 10, "skill1": "One", "skill2": "Two", "ultimate": "Three", "unlock_stage": []int{1, 3, 6}[index]})
	}
	skills := []map[string]any{}
	for index, id := range []string{"meteor", "reinforcement", "freeze"} {
		skills = append(skills, map[string]any{"id": id, "name": "Skill", "description": "Effect", "color": "#aabbcc", "cooldown": 10, "unlock_stage": []int{1, 4, 7}[index]})
	}
	document := map[string]any{
		"schema_version": "0.2.0", "stages": stages, "waves": waves, "enemies": enemies, "bosses": bosses, "towers": towers, "heroes": heroes, "skills": skills,
		"balance": map[string]any{"difficulties": map[string]any{
			"casual":  map[string]any{"enemy_hp": .8, "enemy_speed": .9, "gold": 1.2, "difficulty_bonus": 0},
			"normal":  map[string]any{"enemy_hp": 1, "enemy_speed": 1, "gold": 1, "difficulty_bonus": 5000},
			"veteran": map[string]any{"enemy_hp": 1.3, "enemy_speed": 1.1, "gold": .9, "difficulty_bonus": 10000},
		}, "tower_upgrade_cost": []int{0, 70, 120}, "hero_level_xp": []int{0, 8, 20, 38, 62, 92, 130, 176, 230, 292}, "endless_ramp": .08, "sell_refund_rate": .65, "clear_time_target_ms": 900000, "clear_time_bonus_divisor": 100, "endless_wave_bonus": 1000, "duration_tolerance_ms": 5000, "min_wave_duration_ms": 5000},
	}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	result := ""
	for value > 0 {
		result = string(rune('0'+value%10)) + result
		value /= 10
	}
	return result
}
