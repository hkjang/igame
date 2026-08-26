package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func defenseTestValidOfficeContent(t *testing.T) (defenseDecodedContent, []byte) {
	t.Helper()
	content := defenseDecodedContent{}
	stageIDs := make([]string, 0, 8)
	for number := 1; number <= 8; number++ {
		id := fmt.Sprintf("stage-%d", number)
		stageIDs = append(stageIDs, id)
		content.Stages = append(content.Stages, defenseStageDefinition{ID: id, Number: number, Name: fmt.Sprintf("Stage %d", number), Mode: "campaign", StartingHealth: 20, StartingResource: 300, Version: "1.0", Theme: "verdant", Path: []defensePoint{{X: 0, Y: 0}, {X: 100, Y: 100}}, TowerSpots: []defensePoint{{ID: id + "-spot-1", X: 10, Y: 10}, {ID: id + "-spot-2", X: 20, Y: 20}, {ID: id + "-spot-3", X: 30, Y: 30}, {ID: id + "-spot-4", X: 40, Y: 40}}})
		content.Waves = append(content.Waves, defenseWaveDefinition{ID: id + "-wave-1", StageID: id, Number: 1, Reward: 10, Entries: []defenseWaveEntry{{Enemy: "enemy-1", Count: 1, Interval: 1}}})
	}
	for number := 1; number <= 6; number++ {
		id := fmt.Sprintf("tower-%d", number)
		content.Towers = append(content.Towers, defenseUnitDefinition{ID: id, Name: id, Role: "defense", Color: number, Cost: 10, Damage: 10, Range: 100, FireRate: 1, ProjectileSpeed: 100, DamageType: "physical", EffectiveAgainst: []string{"threat"}, EffectiveMultiplier: 1.5, Branches: []defenseTowerBranch{{ID: "precision", Name: "Precision", Description: "Precision branch", DamageMultiplier: defenseFloat64Pointer(1.5)}, {ID: "network", Name: "Network", Description: "Network branch", RateMultiplier: defenseFloat64Pointer(0.8)}}})
	}
	for number := 1; number <= 10; number++ {
		id := fmt.Sprintf("enemy-%d", number)
		content.Enemies = append(content.Enemies, defenseUnitDefinition{ID: id, Name: id, HP: 100, Speed: 30, Radius: 10, Reward: 10, HealthDamage: 1, ThreatType: "threat"})
	}
	for number := 1; number <= 2; number++ {
		id := fmt.Sprintf("boss-%d", number)
		content.Bosses = append(content.Bosses, defenseUnitDefinition{ID: id, Name: id, HP: 1000, Speed: 20, Radius: 30, Reward: 100, HealthDamage: 5, ThreatType: "threat"})
	}
	for number := 1; number <= 3; number++ {
		id := fmt.Sprintf("hero-%d", number)
		content.Heroes = append(content.Heroes, defenseUnitDefinition{ID: id, Name: id, Title: "Guardian", Role: "hero", Color: number, HP: 500, Damage: 20, Range: 100, Speed: 100, RespawnSeconds: 10, Skill1: "one", Skill2: "two", Ultimate: "ultimate", UnlockStage: number})
	}
	content.Skills = []defenseSkillDefinition{{ID: "area", Name: "Area", Description: "Area attack", Cooldown: 10, Color: "#ff0000", Effect: "area_damage"}, {ID: "reinforce", Name: "Reinforce", Description: "Reinforce", Cooldown: 10, Color: "#00ff00", Effect: "reinforcement"}, {ID: "freeze", Name: "Freeze", Description: "Freeze", Cooldown: 10, Color: "#0000ff", Effect: "freeze"}}
	content.Balance = defenseBalanceDefinition{Difficulties: map[string]defenseDifficulty{"casual": {EnemyHP: 0.8, EnemySpeed: 0.9, Gold: 1.1, Score: 0.8}, "normal": {EnemyHP: 1, EnemySpeed: 1, Gold: 1, Score: 1}, "veteran": {DifficultyBonus: 100, EnemyHP: 1.2, EnemySpeed: 1.1, Gold: 0.9, Score: 1.2}}, HealthScoreFactor: 100, ResourceScoreFactor: 10, WaveScoreFactor: 10, ClearTimeTargetMS: 60_000, ClearTimeBonusDivisor: 100, MinWaveDurationMS: 1000, DurationToleranceMS: 5000, TowerUpgradeCost: []int64{0, 20, 40}, SellRefundRate: 0.5, ResourceStateLimits: map[string]int64{}, AIResourceScoreFactors: map[string]int64{}}
	content.Campaigns = []defenseCampaignDefinition{{ID: "campaign", Name: "Campaign", StageIDs: stageIDs}}
	sections := map[string]any{"stages": content.Stages, "waves": content.Waves, "towers": content.Towers, "enemies": content.Enemies, "bosses": content.Bosses, "heroes": content.Heroes, "skills": content.Skills, "events": content.Events, "education": content.Education, "balance": content.Balance, "campaigns": content.Campaigns, "resource_rules": content.ResourceRules, "model_profiles": content.ModelProfiles}
	raw, err := json.Marshal(sections)
	if err != nil {
		t.Fatal(err)
	}
	return content, raw
}

func defenseFloat64Pointer(value float64) *float64 { return &value }

func TestDefenseDraftLifecyclePreservesSchemaMajorMinor(t *testing.T) {
	for _, test := range []struct {
		name   string
		source defenseVersionRecord
		want   string
	}{
		{name: "0.4 schema overrides stale metadata", source: defenseVersionRecord{ContentVersion: "0.3.9-r2", RawContent: json.RawMessage(`{"schema_version":"0.4.0"}`)}, want: "0.4"},
		{name: "0.4 metadata fallback", source: defenseVersionRecord{ContentVersion: "0.4.6-r3", RawContent: json.RawMessage(`{}`)}, want: "0.4"},
		{name: "0.3 custom schema", source: defenseVersionRecord{ContentVersion: "tenant-release", RawContent: json.RawMessage(`{"schema_version":"0.3.7"}`)}, want: "0.3"},
		{name: "legacy custom fallback", source: defenseVersionRecord{ContentVersion: "tenant-release", RawContent: json.RawMessage(`{}`)}, want: "0.3"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := defenseDraftLifecycle(test.source); got != test.want {
				t.Fatalf("defenseDraftLifecycle() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDefenseAuthoritativeInputsAcceptSDKGameID(t *testing.T) {
	gameID := uuid.New()
	for name, destination := range map[string]any{
		"education answer": &defenseAnswerInput{},
		"result":           &defenseResultInput{},
	} {
		t.Run(name, func(t *testing.T) {
			decoder := json.NewDecoder(strings.NewReader(fmt.Sprintf(`{"game_id":%q}`, gameID.String())))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(destination); err != nil {
				t.Fatalf("SDK game_id was rejected: %v", err)
			}
		})
	}

	const slug = "cyber-fortress"
	if !defenseGameClaimMatches("", slug, gameID) ||
		!defenseGameClaimMatches(slug, slug, gameID) ||
		!defenseGameClaimMatches(gameID.String(), slug, gameID) {
		t.Fatal("a valid optional Defense game claim was rejected")
	}
	if defenseGameClaimMatches(uuid.New().String(), slug, gameID) ||
		defenseGameClaimMatches("office-guardians", slug, gameID) {
		t.Fatal("a mismatched Defense game claim was accepted")
	}

	base := defenseResultInput{StageID: "stage-1", Difficulty: "normal"}
	base.GameID = gameID.String()
	byID := defenseResultRequestChecksum(base)
	base.GameID = slug
	bySlug := defenseResultRequestChecksum(base)
	base.GameID = ""
	withoutClaim := defenseResultRequestChecksum(base)
	if byID != bySlug || bySlug != withoutClaim {
		t.Fatal("equivalent optional Defense game claims changed the idempotency checksum")
	}
}

func TestDefenseContentRuntimeSafetyValidation(t *testing.T) {
	content, raw := defenseTestValidOfficeContent(t)
	if err := validateDefenseContent("office-guardians", raw); err != nil {
		t.Fatalf("valid test content rejected: %v", err)
	}

	content.Towers[0].Damage = 1e308
	sections, _ := json.Marshal(map[string]any{"stages": content.Stages, "waves": content.Waves, "towers": content.Towers, "enemies": content.Enemies, "bosses": content.Bosses, "heroes": content.Heroes, "skills": content.Skills, "events": content.Events, "education": content.Education, "balance": content.Balance, "campaigns": content.Campaigns, "resource_rules": content.ResourceRules, "model_profiles": content.ModelProfiles})
	if err := validateDefenseContent("office-guardians", sections); err == nil {
		t.Fatal("unsafe tower numeric magnitude was accepted")
	}

	content, _ = defenseTestValidOfficeContent(t)
	content.Events = []defenseEventDefinition{{ID: "event-1", StageID: "stage-1", Trigger: "wave-1", EducationID: "question-1", Reward: json.RawMessage(`{"resource":1}`), Penalty: json.RawMessage(`{"resource":1}`)}, {ID: "event-2", StageID: "stage-1", Trigger: "wave_1", EducationID: "question-1", Reward: json.RawMessage(`{"resource":1}`), Penalty: json.RawMessage(`{"resource":1}`)}}
	content.Education = []defenseEducationDefinition{{ID: "question-1", Topic: "topic", Question: "question", Answers: []defenseAnswerDefinition{{ID: "A", Text: "one"}, {ID: "B", Text: "two"}}, CorrectAnswerID: "A", Score: 100}}
	sections, _ = json.Marshal(map[string]any{"stages": content.Stages, "waves": content.Waves, "towers": content.Towers, "enemies": content.Enemies, "bosses": content.Bosses, "heroes": content.Heroes, "skills": content.Skills, "events": content.Events, "education": content.Education, "balance": content.Balance, "campaigns": content.Campaigns, "resource_rules": content.ResourceRules, "model_profiles": content.ModelProfiles})
	if err := validateDefenseContent("office-guardians", sections); err == nil {
		t.Fatal("duplicate normalized stage education trigger was accepted")
	}
}

func TestDefenseContentAcceptsMultiLaneWaveGeometry(t *testing.T) {
	content, _ := defenseTestValidOfficeContent(t)
	content.Stages[0].Paths = [][]defensePoint{
		{{X: 0, Y: 0}, {X: 100, Y: 100}},
		{{X: 0, Y: 100}, {X: 100, Y: 0}},
	}
	firstPath, secondPath := 0, 1
	content.Waves[0].Entries = []defenseWaveEntry{
		{Enemy: "enemy-1", Count: 2, Interval: 0.5, Delay: 0.75, PathIndex: &firstPath, Modifiers: []string{"armored", "stealth"}},
		{Enemy: "enemy-2", Count: 3, Interval: 0.25, Delay: 1.5, PathIndex: &secondPath, Parallel: true, Modifiers: []string{"swift", "flying", "magic_resist", "berserk", "immune_stun"}},
	}

	raw := defenseTestContentJSON(t, content)
	if err := validateDefenseContent("office-guardians", raw); err != nil {
		t.Fatalf("valid multi-lane wave geometry rejected: %v", err)
	}
}

func TestDefenseContentRejectsInvalidWaveGeometry(t *testing.T) {
	intPointer := func(value int) *int { return &value }
	tests := []struct {
		name   string
		mutate func(*defenseWaveEntry)
	}{
		{name: "negative path index", mutate: func(entry *defenseWaveEntry) { entry.PathIndex = intPointer(-1) }},
		{name: "out of range path index", mutate: func(entry *defenseWaveEntry) { entry.PathIndex = intPointer(2) }},
		{name: "negative delay", mutate: func(entry *defenseWaveEntry) { entry.Delay = -0.01 }},
		{name: "excessive delay", mutate: func(entry *defenseWaveEntry) { entry.Delay = 3600.01 }},
		{name: "duplicate modifier", mutate: func(entry *defenseWaveEntry) { entry.Modifiers = []string{"armored", "armored"} }},
		{name: "unknown modifier", mutate: func(entry *defenseWaveEntry) { entry.Modifiers = []string{"regenerating"} }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content, _ := defenseTestValidOfficeContent(t)
			content.Stages[0].Paths = [][]defensePoint{
				{{X: 0, Y: 0}, {X: 100, Y: 100}},
				{{X: 0, Y: 100}, {X: 100, Y: 0}},
			}
			content.Waves[0].Entries[0].PathIndex = intPointer(0)
			test.mutate(&content.Waves[0].Entries[0])

			if err := validateDefenseContent("office-guardians", defenseTestContentJSON(t, content)); err == nil {
				t.Fatalf("invalid wave geometry was accepted: %+v", content.Waves[0].Entries[0])
			}
		})
	}
}

func TestDefenseContentRejectsExplicitZeroBranchMultiplier(t *testing.T) {
	_, raw := defenseTestValidOfficeContent(t)
	var sections map[string]any
	if err := json.Unmarshal(raw, &sections); err != nil {
		t.Fatal(err)
	}
	towers := sections["towers"].([]any)
	branches := towers[0].(map[string]any)["branches"].([]any)
	branches[0].(map[string]any)["damage_multiplier"] = float64(0)
	mutated, err := json.Marshal(sections)
	if err != nil {
		t.Fatal(err)
	}
	if err = validateDefenseContent("office-guardians", mutated); err == nil {
		t.Fatal("an explicitly zero branch multiplier must be rejected")
	}
}

func TestDefenseContentBoundsHeroCardinalityAndDisplayStrings(t *testing.T) {
	content, _ := defenseTestValidOfficeContent(t)
	for number := len(content.Heroes) + 1; number <= 101; number++ {
		content.Heroes = append(content.Heroes, defenseUnitDefinition{ID: fmt.Sprintf("hero-%d", number), Name: "Hero", Title: "Guardian", HP: 100, Damage: 10, Range: 10, Speed: 10, RespawnSeconds: 1, Skill1: "one", Skill2: "two", Ultimate: "three", UnlockStage: 1})
	}
	raw := defenseTestJSON(t, map[string]any{"stages": content.Stages, "waves": content.Waves, "towers": content.Towers, "enemies": content.Enemies, "bosses": content.Bosses, "heroes": content.Heroes, "skills": content.Skills, "events": content.Events, "education": content.Education, "balance": content.Balance, "campaigns": content.Campaigns, "resource_rules": content.ResourceRules, "model_profiles": content.ModelProfiles})
	if err := validateDefenseContent("office-guardians", raw); err == nil || !strings.Contains(err.Error(), "limits") {
		t.Fatalf("excessive hero cardinality was not rejected: %v", err)
	}

	for name, mutate := range map[string]func(*defenseDecodedContent){
		"tower role":    func(value *defenseDecodedContent) { value.Towers[0].Role = strings.Repeat("r", 121) },
		"hero title":    func(value *defenseDecodedContent) { value.Heroes[0].Title = strings.Repeat("t", 121) },
		"campaign name": func(value *defenseDecodedContent) { value.Campaigns[0].Name = strings.Repeat("c", 121) },
	} {
		t.Run(name, func(t *testing.T) {
			candidate, _ := defenseTestValidOfficeContent(t)
			mutate(&candidate)
			raw := defenseTestJSON(t, map[string]any{"stages": candidate.Stages, "waves": candidate.Waves, "towers": candidate.Towers, "enemies": candidate.Enemies, "bosses": candidate.Bosses, "heroes": candidate.Heroes, "skills": candidate.Skills, "events": candidate.Events, "education": candidate.Education, "balance": candidate.Balance, "campaigns": candidate.Campaigns, "resource_rules": candidate.ResourceRules, "model_profiles": candidate.ModelProfiles})
			if err := validateDefenseContent("office-guardians", raw); err == nil {
				t.Fatalf("oversized %s was accepted", name)
			}
		})
	}
}

func TestDefenseContentRejectsTelemetrySnapshotsOver4KiB(t *testing.T) {
	content, _ := defenseTestValidOfficeContent(t)
	content.Waves = slices.DeleteFunc(content.Waves, func(wave defenseWaveDefinition) bool { return wave.StageID == "stage-1" })
	for index := 0; index < 64; index++ {
		id := fmt.Sprintf("threat_%024d", index)
		content.Enemies = append(content.Enemies, defenseUnitDefinition{ID: id, Name: id, HP: 100, Speed: 30, Radius: 10, Reward: 1, HealthDamage: 1, ThreatType: "threat"})
	}
	for wave := 1; wave <= 8; wave++ {
		entries := make([]defenseWaveEntry, 0, 8)
		for offset := 0; offset < 8; offset++ {
			entries = append(entries, defenseWaveEntry{Enemy: fmt.Sprintf("threat_%024d", (wave-1)*8+offset), Count: 1, Interval: 1})
		}
		content.Waves = append(content.Waves, defenseWaveDefinition{ID: fmt.Sprintf("stage-1-wave-%d", wave), StageID: "stage-1", Number: wave, Reward: 1, Entries: entries})
	}
	raw, _ := json.Marshal(map[string]any{"stages": content.Stages, "waves": content.Waves, "towers": content.Towers, "enemies": content.Enemies, "bosses": content.Bosses, "heroes": content.Heroes, "skills": content.Skills, "events": content.Events, "education": content.Education, "balance": content.Balance, "campaigns": content.Campaigns, "resource_rules": content.ResourceRules, "model_profiles": content.ModelProfiles})
	if err := validateDefenseContent("office-guardians", raw); err == nil || !strings.Contains(err.Error(), "4 KiB") {
		t.Fatalf("oversized cumulative telemetry snapshot was not rejected: %v", err)
	}
}

func defenseTestJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func defenseTestContentJSON(t *testing.T, content defenseDecodedContent) json.RawMessage {
	t.Helper()
	return defenseTestJSON(t, map[string]any{"stages": content.Stages, "waves": content.Waves, "towers": content.Towers, "enemies": content.Enemies, "bosses": content.Bosses, "heroes": content.Heroes, "skills": content.Skills, "events": content.Events, "education": content.Education, "balance": content.Balance, "campaigns": content.Campaigns, "resource_rules": content.ResourceRules, "model_profiles": content.ModelProfiles})
}

func defenseTestRecord(t *testing.T, id int64, at time.Time, event string, data any) defenseTelemetryRecord {
	t.Helper()
	return defenseTelemetryRecord{ID: id, Event: event, Data: defenseTestJSON(t, data), ReceivedAt: at, ClientEventID: uuid.New(), Sequence: int(id)}
}

func defenseTestNonAIContent() (defenseStageDefinition, defenseDecodedContent, defenseVersionRecord) {
	stage := defenseStageDefinition{ID: "stage-1", Number: 1, StartingHealth: 20, StartingResource: 300}
	content := defenseDecodedContent{
		Waves:   []defenseWaveDefinition{{ID: "wave-1", StageID: stage.ID, Number: 1, Reward: 30, Entries: []defenseWaveEntry{{Enemy: "threat", Count: 1, Interval: 1}}}},
		Enemies: []defenseUnitDefinition{{ID: "threat", Reward: 10, HealthDamage: 1}},
		Balance: defenseBalanceDefinition{
			Difficulties:      map[string]defenseDifficulty{"normal": {Gold: 1, Score: 1}},
			MinWaveDurationMS: 1000, DurationToleranceMS: 5000,
		},
	}
	version := defenseVersionRecord{ContentVersion: "0.3.0", PolicyVersion: "policy-1"}
	return stage, content, version
}

func TestDefenseTelemetryAttestsEarlyWaveBonus(t *testing.T) {
	stage, content, version := defenseTestNonAIContent()
	started := time.Date(2026, 8, 21, 1, 0, 0, 0, time.UTC)
	result := defenseResultInput{
		StageID: "stage-1", Difficulty: "normal", DurationMS: 5000,
		RemainingHealth: 20, RemainingResource: 370, Kills: 1, Spawned: 1, WavesCompleted: 1, Victory: true,
		Battle:          defenseBattleInput{EarnedResource: 70, HeroID: "hero", HeroLevel: 1},
		DefeatedByEnemy: map[string]int64{"threat": 1}, EscapedByEnemy: map[string]int64{}, SpawnedByEnemy: map[string]int64{"threat": 1},
	}
	snapshot := map[string]any{
		"stage_id": "stage-1", "wave": 1, "health": 20, "resource": 370, "earned_resource": 70, "spent_resource": 0, "sold_resource": 0,
		"kills": 1, "escaped": 0, "spawned": 1, "defeated_by_enemy": map[string]int64{"threat": 1}, "escaped_by_enemy": map[string]int64{}, "spawned_by_enemy": map[string]int64{"threat": 1},
	}
	complete := map[string]any{}
	for key, value := range snapshot {
		complete[key] = value
	}
	complete["difficulty"], complete["duration_ms"], complete["waves_completed"], complete["victory"] = "normal", int64(5000), 1, true
	complete["hero_id"], complete["hero_level"], complete["content_version"], complete["policy_version"] = "hero", 1, version.ContentVersion, version.PolicyVersion
	records := []defenseTelemetryRecord{
		defenseTestRecord(t, 1, started, "defense.battle.ready", map[string]any{"stage_id": stage.ID, "difficulty": "normal", "hero_id": "hero", "content_version": version.ContentVersion, "policy_version": version.PolicyVersion}),
		defenseTestRecord(t, 2, started.Add(time.Second), "defense.wave.start", map[string]any{"stage_id": stage.ID, "wave": 1, "early_call": true, "early_bonus": 30}),
		defenseTestRecord(t, 3, started.Add(5*time.Second), "defense.wave.complete", snapshot),
		defenseTestRecord(t, 4, started.Add(6*time.Second), "defense.battle.complete", complete),
	}
	attestation, err := validateDefenseTelemetryAttestation(records, "office-guardians", started, started.Add(6*time.Second), stage, content, version, result, map[string]map[string]int64{}, 0, 0)
	if err != nil {
		t.Fatalf("valid early bonus ledger rejected: %v", err)
	}
	if attestation.EarlyBonus != 30 {
		t.Fatalf("early bonus = %d, want 30", attestation.EarlyBonus)
	}

	records[1] = defenseTestRecord(t, 2, started.Add(time.Second), "defense.wave.start", map[string]any{"stage_id": stage.ID, "wave": 1, "early_call": true, "early_bonus": 31})
	if _, err = validateDefenseTelemetryAttestation(records, "office-guardians", started, started.Add(6*time.Second), stage, content, version, result, map[string]map[string]int64{}, 0, 0); err == nil {
		t.Fatal("non-canonical early bonus was accepted")
	}
}

func TestDefenseTelemetryAllowsAttestedZeroWaveAIResourceDefeat(t *testing.T) {
	stage, content, version := defenseTestNonAIContent()
	content.ResourceRules = defenseResourceRules{ComputeStart: 100, TokenStart: 100, TrustStart: 10, LatencyMax: 10}
	content.Balance.ResourceStateLimits = map[string]int64{"compute": 100, "token": 100, "trust": 10, "latency": 10}
	content.Events = []defenseEventDefinition{{ID: "event-1", StageID: stage.ID, Trigger: "battle-start", EducationID: "question-1"}}
	started := time.Date(2026, 8, 21, 2, 0, 0, 0, time.UTC)
	initial := initialDefenseAIResourceState(content)
	depleted := initialDefenseAIResourceState(content)
	applyDefenseAIEducationEffect(depleted, map[string]int64{"trust_delta": -10})
	result := defenseResultInput{
		StageID: stage.ID, Difficulty: "normal", DurationMS: 1000, RemainingHealth: 20, RemainingResource: 300, Victory: false,
		Battle: defenseBattleInput{HeroID: "hero", HeroLevel: 1}, ResourceState: depleted,
		DefeatedByEnemy: map[string]int64{}, EscapedByEnemy: map[string]int64{}, SpawnedByEnemy: map[string]int64{},
	}
	complete := map[string]any{
		"stage_id": stage.ID, "difficulty": "normal", "duration_ms": int64(1000), "health": 20, "resource": 300, "earned_resource": 0, "spent_resource": 0, "sold_resource": 0,
		"kills": 0, "escaped": 0, "spawned": 0, "waves_completed": 0, "victory": false, "hero_id": "hero", "hero_level": 1,
		"content_version": version.ContentVersion, "policy_version": version.PolicyVersion, "resource_state": depleted,
		"defeated_by_enemy": map[string]int64{}, "escaped_by_enemy": map[string]int64{}, "spawned_by_enemy": map[string]int64{},
	}
	records := []defenseTelemetryRecord{
		defenseTestRecord(t, 1, started, "defense.battle.ready", map[string]any{"stage_id": stage.ID, "difficulty": "normal", "hero_id": "hero", "content_version": version.ContentVersion, "policy_version": version.PolicyVersion, "resource_state": initial}),
		defenseTestRecord(t, 2, started.Add(500*time.Millisecond), "defense.education.apply", map[string]any{"event_id": "event-1", "resource_delta": 0, "trust_delta": -10, "latency_headroom_delta": 0, "resource_state": depleted}),
		defenseTestRecord(t, 3, started.Add(time.Second), "defense.battle.complete", complete),
	}
	effects := map[string]map[string]int64{"event-1": normalizedDefenseEducationEffect(0, -10, 0)}
	if _, err := validateDefenseTelemetryAttestation(records, "ai-nexus-defense", started, started.Add(time.Second), stage, content, version, result, effects, 0, 0); err != nil {
		t.Fatalf("attested zero-wave AI resource defeat rejected: %v", err)
	}

	forged := records[:1]
	forged = append(forged, defenseTestRecord(t, 2, started.Add(time.Second), "defense.battle.complete", complete))
	if _, err := validateDefenseTelemetryAttestation(forged, "ai-nexus-defense", started, started.Add(time.Second), stage, content, version, result, map[string]map[string]int64{}, 0, 0); err == nil {
		t.Fatal("zero-wave AI defeat without a resource-depleting ledger action was accepted")
	}
}

func cloneDefenseResourceState(state map[string]defenseResourceMetric) map[string]defenseResourceMetric {
	cloned := make(map[string]defenseResourceMetric, len(state))
	for key, value := range state {
		cloned[key] = value
	}
	return cloned
}

func defenseTestAIContent(profileCost, waveCost int64) (defenseStageDefinition, defenseDecodedContent, defenseVersionRecord) {
	stage, content, version := defenseTestNonAIContent()
	stage.TowerSpots = []defensePoint{{ID: "spot-1", X: 100, Y: 100}}
	content.ResourceRules = defenseResourceRules{ComputeStart: 100, TokenStart: 100, TrustStart: 10, LatencyMax: 10, WaveComputeCost: waveCost}
	content.Balance.ResourceStateLimits = map[string]int64{"compute": 100, "token": 100, "trust": 10, "latency": 10}
	content.Towers = []defenseUnitDefinition{{ID: "model-router", Cost: 10}}
	content.ModelProfiles = []defenseModelProfile{{ID: "small", TowerID: "model-router", ComputeCost: profileCost, TokenCost: 1}}
	return stage, content, version
}

func TestDefenseTelemetryAllowsBetweenWaveAIProfileDepletion(t *testing.T) {
	stage, content, version := defenseTestAIContent(90, 10)
	started := time.Date(2026, 8, 21, 2, 30, 0, 0, time.UTC)
	initial := initialDefenseAIResourceState(content)
	afterWave := cloneDefenseResourceState(initial)
	if err := applyDefenseAIResourceCost(afterWave, map[string]int64{"compute": 10}); err != nil {
		t.Fatal(err)
	}
	depleted := cloneDefenseResourceState(afterWave)
	if err := applyDefenseAIResourceCost(depleted, map[string]int64{"compute": 90, "token": 1}); err != nil {
		t.Fatal(err)
	}
	result := defenseResultInput{
		StageID: stage.ID, Difficulty: "normal", DurationMS: 5000, RemainingHealth: 20, RemainingResource: 330, Kills: 1, Spawned: 1, WavesCompleted: 1, Victory: false,
		Battle: defenseBattleInput{EarnedResource: 40, SpentResource: 10, HeroID: "hero", HeroLevel: 1}, ResourceState: depleted,
		DefeatedByEnemy: map[string]int64{"threat": 1}, EscapedByEnemy: map[string]int64{}, SpawnedByEnemy: map[string]int64{"threat": 1},
	}
	waveSnapshot := map[string]any{
		"stage_id": stage.ID, "wave": 1, "health": 20, "resource": 340, "earned_resource": 40, "spent_resource": 0, "sold_resource": 0,
		"kills": 1, "escaped": 0, "spawned": 1, "resource_state": afterWave,
		"defeated_by_enemy": map[string]int64{"threat": 1}, "escaped_by_enemy": map[string]int64{}, "spawned_by_enemy": map[string]int64{"threat": 1},
	}
	complete := map[string]any{
		"stage_id": stage.ID, "difficulty": "normal", "duration_ms": int64(5000), "health": 20, "resource": 330, "earned_resource": 40, "spent_resource": 10, "sold_resource": 0,
		"kills": 1, "escaped": 0, "spawned": 1, "waves_completed": 1, "victory": false, "hero_id": "hero", "hero_level": 1,
		"content_version": version.ContentVersion, "policy_version": version.PolicyVersion, "resource_state": depleted,
		"defeated_by_enemy": map[string]int64{"threat": 1}, "escaped_by_enemy": map[string]int64{}, "spawned_by_enemy": map[string]int64{"threat": 1},
	}
	records := []defenseTelemetryRecord{
		defenseTestRecord(t, 1, started, "defense.battle.ready", map[string]any{"stage_id": stage.ID, "difficulty": "normal", "hero_id": "hero", "content_version": version.ContentVersion, "policy_version": version.PolicyVersion, "resource_state": initial}),
		defenseTestRecord(t, 2, started.Add(time.Second), "defense.wave.start", map[string]any{"stage_id": stage.ID, "wave": 1, "early_call": false, "early_bonus": 0, "resource_state": afterWave}),
		defenseTestRecord(t, 3, started.Add(5*time.Second), "defense.wave.complete", waveSnapshot),
		defenseTestRecord(t, 4, started.Add(5500*time.Millisecond), "defense.tower.build", map[string]any{"tower": "model-router", "spot": "spot-1", "profile_id": "small", "resource_state": depleted}),
		defenseTestRecord(t, 5, started.Add(6*time.Second), "defense.battle.complete", complete),
	}
	if _, err := validateDefenseTelemetryAttestation(records, "ai-nexus-defense", started, started.Add(6*time.Second), stage, content, version, result, map[string]map[string]int64{}, 0, 0); err != nil {
		t.Fatalf("between-wave profile depletion rejected: %v", err)
	}
}

func TestDefenseTelemetryAllowsImmediateWaveStartDepletion(t *testing.T) {
	stage, content, version := defenseTestAIContent(80, 20)
	started := time.Date(2026, 8, 21, 2, 45, 0, 0, time.UTC)
	initial := initialDefenseAIResourceState(content)
	afterBuild := cloneDefenseResourceState(initial)
	_ = applyDefenseAIResourceCost(afterBuild, map[string]int64{"compute": 80, "token": 1})
	depleted := cloneDefenseResourceState(afterBuild)
	_ = applyDefenseAIResourceCost(depleted, map[string]int64{"compute": 20})
	result := defenseResultInput{
		StageID: stage.ID, Difficulty: "normal", DurationMS: 300, RemainingHealth: 20, RemainingResource: 290, WavesCompleted: 0, Victory: false,
		Battle: defenseBattleInput{SpentResource: 10, HeroID: "hero", HeroLevel: 1}, ResourceState: depleted,
		DefeatedByEnemy: map[string]int64{}, EscapedByEnemy: map[string]int64{}, SpawnedByEnemy: map[string]int64{},
	}
	complete := map[string]any{
		"stage_id": stage.ID, "difficulty": "normal", "duration_ms": int64(300), "health": 20, "resource": 290, "earned_resource": 0, "spent_resource": 10, "sold_resource": 0,
		"kills": 0, "escaped": 0, "spawned": 0, "waves_completed": 0, "victory": false, "hero_id": "hero", "hero_level": 1,
		"content_version": version.ContentVersion, "policy_version": version.PolicyVersion, "resource_state": depleted,
		"defeated_by_enemy": map[string]int64{}, "escaped_by_enemy": map[string]int64{}, "spawned_by_enemy": map[string]int64{},
	}
	records := []defenseTelemetryRecord{
		defenseTestRecord(t, 1, started, "defense.battle.ready", map[string]any{"stage_id": stage.ID, "difficulty": "normal", "hero_id": "hero", "content_version": version.ContentVersion, "policy_version": version.PolicyVersion, "resource_state": initial}),
		defenseTestRecord(t, 2, started.Add(100*time.Millisecond), "defense.tower.build", map[string]any{"tower": "model-router", "spot": "spot-1", "profile_id": "small", "resource_state": afterBuild}),
		defenseTestRecord(t, 3, started.Add(200*time.Millisecond), "defense.wave.start", map[string]any{"stage_id": stage.ID, "wave": 1, "early_call": false, "early_bonus": 0, "resource_state": depleted}),
		defenseTestRecord(t, 4, started.Add(300*time.Millisecond), "defense.battle.complete", complete),
	}
	if _, err := validateDefenseTelemetryAttestation(records, "ai-nexus-defense", started, started.Add(300*time.Millisecond), stage, content, version, result, map[string]map[string]int64{}, 0, 0); err != nil {
		t.Fatalf("immediate wave-start depletion rejected: %v", err)
	}
}

func TestDefenseEducationRequiredSetUsesReachedWaves(t *testing.T) {
	content := defenseDecodedContent{Events: []defenseEventDefinition{
		{ID: "start", StageID: "stage-1", Trigger: "battle_start"},
		{ID: "wave-1", StageID: "stage-1", Trigger: "wave-1"},
		{ID: "wave-2", StageID: "stage-1", Trigger: "wave_2"},
		{ID: "other", StageID: "stage-2", Trigger: "battle-start"},
	}}
	required := defenseRequiredEducationEvents(content, "stage-1", 1, 0)
	if len(required) != 2 || !required["start"] || !required["wave-1"] || required["wave-2"] || required["other"] {
		t.Fatalf("unexpected reached education set: %#v", required)
	}
	terminal := defenseRequiredEducationEvents(content, "stage-1", 1, 1)
	if len(terminal) != 1 || !terminal["start"] {
		t.Fatalf("terminal depleted wave event must be excluded: %#v", terminal)
	}
}

func TestDefenseHeroAvailabilityUsesPinnedStageNumber(t *testing.T) {
	content := defenseDecodedContent{Heroes: []defenseUnitDefinition{{ID: "starter", UnlockStage: 1}, {ID: "advanced", UnlockStage: 3}}}
	if !defenseHeroAvailable(content, "starter", 1) || defenseHeroAvailable(content, "advanced", 2) || !defenseHeroAvailable(content, "advanced", 3) || defenseHeroAvailable(content, "missing", 10) {
		t.Fatal("hero unlock_stage was not enforced against the pinned stage number")
	}
}

func TestDefenseEducationEnabledIsContentDriven(t *testing.T) {
	content := defenseDecodedContent{}
	if defenseEducationEnabled(content) {
		t.Fatal("empty content unexpectedly enabled education")
	}
	content.Events = []defenseEventDefinition{{ID: "event-1"}}
	if defenseEducationEnabled(content) {
		t.Fatal("events without education questions unexpectedly enabled education")
	}
	content.Education = []defenseEducationDefinition{{ID: "question-1"}}
	if !defenseEducationEnabled(content) {
		t.Fatal("content with events and education questions did not enable education")
	}
}

func TestDefenseRankingMCPExposesRESTPeriods(t *testing.T) {
	t.Helper()
	for _, tool := range mcpTools() {
		if tool["name"] != "defense_rankings_get" {
			continue
		}
		schema, ok := tool["inputSchema"].(map[string]any)
		if !ok {
			t.Fatal("defense_rankings_get input schema is missing")
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatal("defense_rankings_get properties are missing")
		}
		period, ok := properties["period"].(map[string]any)
		if !ok {
			t.Fatal("defense_rankings_get period schema is missing")
		}
		values, ok := period["enum"].([]string)
		if !ok {
			t.Fatal("defense_rankings_get period enum is missing")
		}
		for _, expected := range []string{"daily", "weekly", "monthly", "season", "all_time"} {
			if !slices.Contains(values, expected) {
				t.Fatalf("defense_rankings_get period enum is missing %q: %v", expected, values)
			}
		}
		return
	}
	t.Fatal("defense_rankings_get MCP tool is missing")
}

func TestDefenseAIResourceScoreFactorsRequireExactKeys(t *testing.T) {
	valid := map[string]int64{"compute": 1, "token": 2, "trust": 3, "latency": 4}
	if !validDefenseAIResourceScoreFactors(valid) {
		t.Fatal("canonical AI resource score factors should be valid")
	}
	withExtra := map[string]int64{"compute": 1, "token": 2, "trust": 3, "latency": 4, "unexpected": 0}
	if validDefenseAIResourceScoreFactors(withExtra) {
		t.Fatal("an extra AI resource score factor must be rejected")
	}
	missing := map[string]int64{"compute": 1, "token": 2, "trust": 3}
	if validDefenseAIResourceScoreFactors(missing) {
		t.Fatal("a missing AI resource score factor must be rejected")
	}
}

func TestDefenseModelProfileRequiresDisplayName(t *testing.T) {
	rules := defenseResourceRules{ComputeStart: 100, TokenStart: 100, LatencyMax: 100}
	profile := defenseModelProfile{ID: "small", Name: "Small Model", ComputeCost: 10, TokenCost: 5, LatencyCost: 1, Accuracy: 70, DamageMultiplier: 1}
	if !validDefenseModelProfileRuntime(profile, rules) {
		t.Fatal("canonical model profile should be valid")
	}
	profile.Name = "  "
	if validDefenseModelProfileRuntime(profile, rules) {
		t.Fatal("a model profile without a display name must be rejected")
	}
}

func TestSanitizeDefenseContentRecursivelyRedactsAnswerMaterial(t *testing.T) {
	raw := []byte(`{"education":[{"id":"q","correct_answer_id":"B","correctAnswerId":"B","correctAnswers":["B"],"answerKey":"B","explanation":"secret","answers":[{"id":"A","text":"choice","isCorrect":true}]}],"nested":{"correct":true,"solution":"secret","policy_reference":"private","policyReference":"private","rationale":"secret"}}`)
	sanitized, err := sanitizeDefenseContent(raw)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(sanitized)
	for _, forbidden := range []string{"correct_answer_id", "correctAnswerId", "correctAnswers", "answerKey", "isCorrect", "explanation", "rationale", "policy_reference", "policyReference", "solution", `"correct"`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("public content leaked %q: %s", forbidden, encoded)
		}
	}
}

func defenseTestFindForbiddenKey(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if defenseContentKeyIsSensitive(key) {
				return key
			}
			if found := defenseTestFindForbiddenKey(child); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := defenseTestFindForbiddenKey(child); found != "" {
				return found
			}
		}
	}
	return ""
}

func TestDefensePublishedSeedContract(t *testing.T) {
	dsn := os.Getenv("DEFENSE_TEST_DSN")
	if dsn == "" {
		t.Skip("DEFENSE_TEST_DSN is not set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)

	expected := map[string][7]int{
		"office-guardians": {8, 6, 3, 10, 2, 0, 0},
		"cyber-fortress":   {10, 8, 3, 15, 3, 30, 50},
		"ai-nexus-defense": {10, 10, 5, 15, 4, 30, 50},
	}
	rows, err := conn.Query(ctx, `SELECT g.slug,v.content FROM defense_content_versions v JOIN games g ON g.id=v.game_id WHERE v.status='published' AND g.slug=ANY($1::text[]) ORDER BY g.slug`, defenseGameSlugs)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var slug string
		var raw []byte
		if err := rows.Scan(&slug, &raw); err != nil {
			t.Fatal(err)
		}
		seen++
		if err := validateDefenseContent(slug, raw); err != nil {
			t.Fatalf("%s published seed is invalid: %v", slug, err)
		}
		content, err := decodeDefenseContent(raw)
		if err != nil {
			t.Fatal(err)
		}
		counts := [7]int{len(content.Stages), len(content.Towers), len(content.Heroes), len(content.Enemies), len(content.Bosses), len(content.Events), len(content.Education)}
		if counts != expected[slug] {
			t.Fatalf("%s content counts = %v, want %v", slug, counts, expected[slug])
		}
		for _, tower := range content.Towers {
			if tower.DamageType != "physical" && tower.DamageType != "magic" && tower.DamageType != "true" {
				t.Fatalf("%s has non-canonical damage type %q", slug, tower.DamageType)
			}
		}
		for _, question := range content.Education {
			if question.CorrectAnswerID != "A" && question.CorrectAnswerID != "B" && question.CorrectAnswerID != "C" {
				t.Fatalf("%s question uses semantic answer id %q", slug, question.CorrectAnswerID)
			}
		}
		public, err := sanitizeDefenseContent(raw)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(public)
		var publicValue any
		if err := json.Unmarshal(encoded, &publicValue); err != nil {
			t.Fatal(err)
		}
		if forbidden := defenseTestFindForbiddenKey(publicValue); forbidden != "" {
			t.Fatalf("%s sanitized public seed leaked %s", slug, forbidden)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != len(defenseGameSlugs) {
		t.Fatalf("found %d published Defense packs, want %d", seen, len(defenseGameSlugs))
	}
	var progressPrimaryKey string
	if err := conn.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='defense_user_progress'::regclass AND contype='p'`).Scan(&progressPrimaryKey); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(progressPrimaryKey, "content_version_id") {
		t.Fatalf("progress primary key is not content-version isolated: %s", progressPrimaryKey)
	}
	var sourceColumn bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='defense_content_versions' AND column_name='source_version_id')`).Scan(&sourceColumn); err != nil || !sourceColumn {
		t.Fatalf("rollback source lineage column missing: exists=%v err=%v", sourceColumn, err)
	}
}

func TestDefenseDraftCreatePreservesPublishedSourceLifecycle(t *testing.T) {
	dsn := os.Getenv("DEFENSE_TEST_DSN")
	if dsn == "" {
		t.Skip("DEFENSE_TEST_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	const slug = "office-guardians"
	var sourceID, userID uuid.UUID
	var sourceContentVersion string
	if err = pool.QueryRow(ctx, `SELECT v.id,v.content_version FROM defense_content_versions v JOIN games g ON g.id=v.game_id WHERE g.slug=$1 AND v.status='published'`, slug).Scan(&sourceID, &sourceContentVersion); err != nil {
		t.Fatal(err)
	}
	if lifecycle, ok := defenseLifecycleMajorMinor(sourceContentVersion); !ok || lifecycle != "0.4" {
		t.Fatalf("published source content_version = %q, want the 0.4 lifecycle", sourceContentVersion)
	}
	username := "defense-lifecycle-" + uuid.NewString()
	if err = pool.QueryRow(ctx, `INSERT INTO users(username,display_name,role,status) VALUES($1,'Defense lifecycle test','admin','active') RETURNING id`, username).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID) }()

	body := fmt.Sprintf(`{"source_version_id":%q}`, sourceID.String())
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/defense/"+slug+"/versions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	route := chi.NewRouteContext()
	route.URLParams.Add("slug", slug)
	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, route))
	request = request.WithContext(context.WithValue(request.Context(), principalKey, Principal{UserID: userID, Role: "admin"}))
	response := httptest.NewRecorder()
	New(pool, nil, slog.Default()).createDefenseVersion(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", response.Code, response.Body.String())
	}

	var payload struct {
		Version struct {
			ID              uuid.UUID  `json:"id"`
			VersionNo       int        `json:"version_no"`
			Label           string     `json:"label"`
			ContentVersion  string     `json:"content_version"`
			SourceVersionID *uuid.UUID `json:"source_version_id"`
		} `json:"version"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM audit_logs WHERE resource_type='defense_content_version' AND resource_id=$1`, payload.Version.ID.String())
		_, _ = pool.Exec(ctx, `DELETE FROM defense_content_versions WHERE id=$1`, payload.Version.ID)
	}()
	wantContentVersion := fmt.Sprintf("0.4.%d", payload.Version.VersionNo-1)
	if payload.Version.ContentVersion != wantContentVersion || payload.Version.Label != "v"+wantContentVersion {
		t.Fatalf("created label/content_version = %q/%q, want %q/%q", payload.Version.Label, payload.Version.ContentVersion, "v"+wantContentVersion, wantContentVersion)
	}
	if payload.Version.SourceVersionID == nil || *payload.Version.SourceVersionID != sourceID {
		t.Fatalf("created source_version_id = %v, want %s", payload.Version.SourceVersionID, sourceID)
	}

	var persistedLabel, persistedContentVersion string
	var persistedSourceID *uuid.UUID
	if err = pool.QueryRow(ctx, `SELECT label,content_version,source_version_id FROM defense_content_versions WHERE id=$1`, payload.Version.ID).Scan(&persistedLabel, &persistedContentVersion, &persistedSourceID); err != nil {
		t.Fatal(err)
	}
	if persistedLabel != payload.Version.Label || persistedContentVersion != payload.Version.ContentVersion || persistedSourceID == nil || *persistedSourceID != sourceID {
		t.Fatalf("persisted lifecycle metadata differs from response: label=%q content_version=%q source=%v", persistedLabel, persistedContentVersion, persistedSourceID)
	}
}

func TestDefenseDraftPUTChecksumMatchesPersistedJSONB(t *testing.T) {
	dsn := os.Getenv("DEFENSE_TEST_DSN")
	if dsn == "" {
		t.Skip("DEFENSE_TEST_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	const slug = "office-guardians"
	var gameID, sourceID uuid.UUID
	var sourceContent []byte
	var policyVersion, assetVersion string
	if err = pool.QueryRow(ctx, `SELECT g.id,v.id,v.policy_version,v.asset_version,v.content FROM games g JOIN defense_content_versions v ON v.game_id=g.id AND v.status='published' WHERE g.slug=$1`, slug).Scan(&gameID, &sourceID, &policyVersion, &assetVersion, &sourceContent); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text,0))`, gameID); err != nil {
		t.Fatal(err)
	}
	var versionNo int
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(version_no),0)+1 FROM defense_content_versions WHERE game_id=$1`, gameID).Scan(&versionNo); err != nil {
		t.Fatal(err)
	}
	versionID := uuid.New()
	label := "checksum-regression-" + uuid.NewString()
	contentVersion := fmt.Sprintf("checksum-regression-%d", versionNo)
	legacyChecksum := strings.Repeat("0", 64)
	if _, err = tx.Exec(ctx, `INSERT INTO defense_content_versions(id,game_id,version_no,label,status,content_version,policy_version,asset_version,checksum,notes,content,source_version_id) VALUES($1,$2,$3,$4,'draft',$5,$6,$7,$8,'',$9,$10)`, versionID, gameID, versionNo, label, contentVersion, policyVersion, assetVersion, legacyChecksum, sourceContent, sourceID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM audit_logs WHERE resource_type='defense_content_version' AND resource_id=$1`, versionID.String())
		_, _ = pool.Exec(ctx, `DELETE FROM defense_content_versions WHERE id=$1`, versionID)
	}()

	server := New(pool, nil, slog.Default())
	staleVersion, err := scanDefenseVersion(pool.QueryRow(ctx, `SELECT `+defenseVersionColumns+` FROM defense_content_versions WHERE id=$1`, versionID))
	if err != nil {
		t.Fatal(err)
	}
	type putResponse struct {
		Version struct {
			Checksum string `json:"checksum"`
		} `json:"version"`
	}
	performPUT := func(checksum, body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/defense/"+slug+"/drafts/balance?version_id="+versionID.String(), strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("If-Match", `"`+checksum+`"`)
		route := chi.NewRouteContext()
		route.URLParams.Add("slug", slug)
		route.URLParams.Add("section", "balance")
		request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, route))
		response := httptest.NewRecorder()
		server.putDefenseDraftSection(response, request)
		return response
	}
	put := func(checksum, body string) putResponse {
		t.Helper()
		response := performPUT(checksum, body)
		if response.Code != http.StatusOK {
			t.Fatalf("PUT status = %d, want 200: %s", response.Code, response.Body.String())
		}
		var payload putResponse
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if response.Header().Get("ETag") != `"`+payload.Version.Checksum+`"` {
			t.Fatalf("ETag %q does not match response checksum %q", response.Header().Get("ETag"), payload.Version.Checksum)
		}
		return payload
	}

	first := put(legacyChecksum, `{"data":{"z":1, "nested":{"z":2,"a":3}, "a":4}}`)
	loaded, err := server.loadDefenseVersion(ctx, versionID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Version.Checksum != loaded.Checksum {
		t.Fatalf("PUT checksum %q differs from immediate load checksum %q", first.Version.Checksum, loaded.Checksum)
	}
	// Simulate a checksum repair from a GET snapshot that was taken before the
	// PUT acquired its row lock. It must not clobber the newer content checksum.
	_ = server.normalizeDefenseChecksum(ctx, staleVersion)
	var persistedContent []byte
	var persistedChecksum string
	if err = pool.QueryRow(ctx, `SELECT content,checksum FROM defense_content_versions WHERE id=$1`, versionID).Scan(&persistedContent, &persistedChecksum); err != nil {
		t.Fatal(err)
	}
	if first.Version.Checksum != persistedChecksum || persistedChecksum != defenseChecksum(persistedContent) {
		t.Fatalf("PUT checksum %q, persisted checksum %q, persisted content checksum %q", first.Version.Checksum, persistedChecksum, defenseChecksum(persistedContent))
	}
	staleResponse := performPUT(legacyChecksum, `{"data":{"stale":true}}`)
	if staleResponse.Code != http.StatusConflict || !strings.Contains(staleResponse.Body.String(), `"stale_version"`) {
		t.Fatalf("reused checksum status = %d, want stale_version 409: %s", staleResponse.Code, staleResponse.Body.String())
	}

	second := put(first.Version.Checksum, `{"data":{"nested":{"b":5,"a":4}, "second":true}}`)
	loaded, err = server.loadDefenseVersion(ctx, versionID)
	if err != nil {
		t.Fatal(err)
	}
	if second.Version.Checksum != loaded.Checksum {
		t.Fatalf("second PUT checksum %q differs from immediate load checksum %q", second.Version.Checksum, loaded.Checksum)
	}
}

// A game without resource metrics omits the field entirely, and a nil map is
// written to a NOT NULL jsonb column as SQL NULL rather than as the column's
// default. Two of the three Defense games shipped like that and returned 500 on
// every submitted battle.
func TestDefenseResultOmittingResourceStateIsStoredAsAnEmptyObject(t *testing.T) {
	var in defenseResultInput
	if err := json.Unmarshal([]byte(`{"stage_id":"stage-1","difficulty":"normal"}`), &in); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if in.ResourceState != nil {
		t.Fatal("a payload without resource_state should decode to a nil map; the guard below is what fixes it")
	}
	stored := defenseResourceStateForStorage(in.ResourceState)
	if stored == nil {
		t.Fatal("a nil map reaches Postgres as NULL and violates the NOT NULL column")
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if string(encoded) != "{}" {
		t.Fatalf("stored resource_state is %s, want {}", encoded)
	}
}

func TestDefenseResourceStateForStorageKeepsWhatTheAIGameSends(t *testing.T) {
	sent := map[string]defenseResourceMetric{"compute": {Start: 1000, Spent: 40, Remaining: 960}}
	stored := defenseResourceStateForStorage(sent)
	if len(stored) != 1 || stored["compute"].Remaining != 960 {
		t.Fatalf("the AI game's metrics did not survive normalisation: %+v", stored)
	}
}
