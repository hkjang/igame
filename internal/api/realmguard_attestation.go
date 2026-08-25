package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const realmGuardVerificationMethod = "server_received_telemetry_v1"

const (
	realmGuardOptionalTelemetryLimit = 128
	realmGuardTowerLedgerLimit       = 10000
	realmGuardMaxEndlessWaves        = 10000
)

type realmGuardTelemetryRecord struct {
	ID            int64
	Event         string
	Data          json.RawMessage
	ReceivedAt    time.Time
	ClientEventID uuid.UUID
	Sequence      int
}

type realmGuardTelemetryAttestation struct {
	Method             string         `json:"method"`
	Digest             string         `json:"digest"`
	EventCount         int            `json:"event_count"`
	FirstReceivedAt    time.Time      `json:"first_received_at"`
	LastReceivedAt     time.Time      `json:"last_received_at"`
	ObservedDurationMS int64          `json:"observed_duration_ms"`
	WavesStarted       int            `json:"waves_started"`
	WavesCompleted     int            `json:"waves_completed"`
	EarlyBonus         int            `json:"early_bonus"`
	TowerBuilds        int            `json:"tower_builds"`
	TowerUpgrades      int            `json:"tower_upgrades"`
	TowerSales         int            `json:"tower_sales"`
	LedgerSpentGold    int            `json:"ledger_spent_gold"`
	LedgerSoldGold     int            `json:"ledger_sold_gold"`
	DefeatedByEnemy    map[string]int `json:"defeated_by_enemy"`
	EscapedByEnemy     map[string]int `json:"escaped_by_enemy"`
	SpawnedByEnemy     map[string]int `json:"spawned_by_enemy"`
}

var realmGuardTelemetryEvents = map[string]bool{
	"game.pause":                     true,
	"game.resume":                    true,
	"realmguard.barracks.block":      true,
	"realmguard.battle.complete":     true,
	"realmguard.battle.ready":        true,
	"realmguard.boss.phase":          true,
	"realmguard.enemy.defeated":      true,
	"realmguard.enemy.escape":        true,
	"realmguard.enemy.siege_disrupt": true,
	"realmguard.hero.defeated":       true,
	"realmguard.hero.level_up":       true,
	"realmguard.hero.move":           true,
	"realmguard.hero.move_armed":     true,
	"realmguard.hero.respawn":        true,
	"realmguard.hero.skill":          true,
	"realmguard.hero.ultimate":       true,
	"realmguard.skill.cast":          true,
	"realmguard.stage.gimmick":       true,
	"realmguard.tower.build":         true,
	"realmguard.tower.sell":          true,
	"realmguard.tower.targeting":     true,
	"realmguard.tower.upgrade":       true,
	"realmguard.wave.complete":       true,
	"realmguard.wave.start":          true,
}

var realmGuardRequiredTelemetryEvents = map[string]bool{
	"realmguard.battle.ready":    true,
	"realmguard.wave.start":      true,
	"realmguard.wave.complete":   true,
	"realmguard.tower.build":     true,
	"realmguard.tower.upgrade":   true,
	"realmguard.tower.sell":      true,
	"realmguard.battle.complete": true,
}

func validRealmGuardTelemetryEvent(event string) bool {
	return realmGuardTelemetryEvents[event]
}

func realmGuardTelemetryLimitReached(event string, counts map[string]int) bool {
	if !realmGuardRequiredTelemetryEvents[event] {
		optional := 0
		for recorded, count := range counts {
			if !realmGuardRequiredTelemetryEvents[recorded] {
				optional += count
			}
		}
		return optional >= realmGuardOptionalTelemetryLimit
	}
	switch event {
	case "realmguard.battle.ready", "realmguard.battle.complete":
		return counts[event] >= 1
	case "realmguard.wave.start":
		// A defeated run can start one wave beyond its completed count.
		return counts[event] >= realmGuardMaxEndlessWaves+1
	case "realmguard.wave.complete":
		return counts[event] >= realmGuardMaxEndlessWaves
	case "realmguard.tower.build", "realmguard.tower.upgrade", "realmguard.tower.sell":
		return counts["realmguard.tower.build"]+counts["realmguard.tower.upgrade"]+counts["realmguard.tower.sell"] >= realmGuardTowerLedgerLimit
	default:
		return true
	}
}

func (s *Server) loadRealmGuardTelemetryRecords(ctx context.Context, tx pgx.Tx, sessionID uuid.UUID) ([]realmGuardTelemetryRecord, error) {
	load := func() ([]realmGuardTelemetryRecord, bool, error) {
		rows, err := tx.Query(ctx, `SELECT id,event,data,received_at,client_event_id,sequence_no FROM game_telemetry WHERE session_id=$1 ORDER BY sequence_no`, sessionID)
		if err != nil {
			return nil, false, err
		}
		defer rows.Close()
		records := make([]realmGuardTelemetryRecord, 0)
		complete := false
		for rows.Next() {
			var record realmGuardTelemetryRecord
			if err := rows.Scan(&record.ID, &record.Event, &record.Data, &record.ReceivedAt, &record.ClientEventID, &record.Sequence); err != nil {
				return nil, false, err
			}
			complete = complete || record.Event == "realmguard.battle.complete"
			records = append(records, record)
		}
		return records, complete, rows.Err()
	}

	// The browser emits battle.complete immediately before the authoritative
	// result. Those independent HTTP requests can arrive in either order, so a
	// short bounded wait removes a harmless race without accepting missing data.
	deadline := time.Now().Add(1200 * time.Millisecond)
	for {
		records, complete, err := load()
		if err != nil || complete || time.Now().After(deadline) {
			return records, err
		}
		timer := time.NewTimer(40 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

type realmGuardReadyTelemetry struct {
	StageID    string `json:"stage_id"`
	Difficulty string `json:"difficulty"`
	HeroID     string `json:"hero_id"`
}

type realmGuardWaveStartTelemetry struct {
	StageID    string `json:"stage_id"`
	Wave       int    `json:"wave"`
	EarlyCall  bool   `json:"early_call"`
	EarlyBonus int    `json:"early_bonus"`
}

type realmGuardWaveSnapshot struct {
	StageID         string         `json:"stage_id"`
	Wave            int            `json:"wave"`
	Lives           *int           `json:"lives"`
	Gold            *int           `json:"gold"`
	EarnedGold      *int           `json:"earned_gold"`
	SpentGold       *int           `json:"spent_gold"`
	SoldGold        *int           `json:"sold_gold"`
	Kills           *int           `json:"kills"`
	Escaped         *int           `json:"escaped"`
	Spawned         *int           `json:"spawned"`
	HeroLevel       *int           `json:"hero_level"`
	DefeatedByEnemy map[string]int `json:"defeated_by_enemy"`
	EscapedByEnemy  map[string]int `json:"escaped_by_enemy"`
	SpawnedByEnemy  map[string]int `json:"spawned_by_enemy"`
}

type realmGuardCompleteTelemetry struct {
	StageID         string         `json:"stage_id"`
	Mode            string         `json:"mode"`
	Difficulty      string         `json:"difficulty"`
	DurationMS      *int64         `json:"duration_ms"`
	Lives           *int           `json:"lives"`
	Gold            *int           `json:"gold"`
	EarnedGold      *int           `json:"earned_gold"`
	SpentGold       *int           `json:"spent_gold"`
	SoldGold        *int           `json:"sold_gold"`
	Kills           *int           `json:"kills"`
	Escaped         *int           `json:"escaped"`
	Spawned         *int           `json:"spawned"`
	Waves           *int           `json:"waves"`
	WavesCompleted  *int           `json:"waves_completed"`
	HeroID          string         `json:"hero_id"`
	HeroLevel       *int           `json:"hero_level"`
	ContentVersion  string         `json:"content_version"`
	BalanceVersion  string         `json:"balance_version"`
	StageVersion    string         `json:"stage_version"`
	AssetVersion    string         `json:"asset_version"`
	Victory         *bool          `json:"victory"`
	DefeatedByEnemy map[string]int `json:"defeated_by_enemy"`
	EscapedByEnemy  map[string]int `json:"escaped_by_enemy"`
	SpawnedByEnemy  map[string]int `json:"spawned_by_enemy"`
}

type realmGuardTowerLedgerState struct {
	TowerID  string
	Level    int
	Invested int
}

func telemetryDecode(raw json.RawMessage, dst any) error {
	if len(raw) == 0 || !json.Valid(raw) || json.Unmarshal(raw, dst) != nil {
		return rejectRealmGuardResult(422, "telemetry_attestation_failed", "a required RealmGuard telemetry event has invalid data")
	}
	return nil
}

func telemetrySnapshotValues(snapshot realmGuardWaveSnapshot) ([]int, bool) {
	values := []*int{snapshot.Lives, snapshot.Gold, snapshot.EarnedGold, snapshot.SpentGold, snapshot.SoldGold, snapshot.Kills, snapshot.Escaped, snapshot.Spawned, snapshot.HeroLevel}
	result := make([]int, len(values))
	for index, value := range values {
		if value == nil || *value < 0 {
			return nil, false
		}
		result[index] = *value
	}
	return result, true
}

func realmGuardHistogramTotal(values map[string]int) (int, bool) {
	total := 0
	for id, count := range values {
		if id == "" || count < 0 {
			return 0, false
		}
		total += count
		if total < 0 {
			return 0, false
		}
	}
	return total, true
}

func realmGuardHistogramEqual(left, right map[string]int) bool {
	if len(left) != len(right) {
		return false
	}
	for id, count := range left {
		if right[id] != count {
			return false
		}
	}
	return true
}

func realmGuardHistogramMonotonic(previous, current map[string]int) bool {
	for id, count := range previous {
		if current[id] < count {
			return false
		}
	}
	return true
}

func validateRealmGuardEnemyHistograms(stage realmGuardStageDefinition, content realmGuardDecodedContent, in realmGuardResultInput, cleared bool, earlyBonus int) error {
	if in.DefeatedByEnemy == nil || in.EscapedByEnemy == nil || in.SpawnedByEnemy == nil {
		return rejectRealmGuardResult(422, "telemetry_attestation_failed", "enemy histograms are required for ranked RealmGuard results")
	}
	defeated, validDefeated := realmGuardHistogramTotal(in.DefeatedByEnemy)
	escaped, validEscaped := realmGuardHistogramTotal(in.EscapedByEnemy)
	spawned, validSpawned := realmGuardHistogramTotal(in.SpawnedByEnemy)
	if !validDefeated || !validEscaped || !validSpawned || defeated != in.Kills || escaped != in.Escaped || spawned != in.Spawned {
		return rejectRealmGuardResult(422, "telemetry_attestation_failed", "enemy histogram totals do not match result counters")
	}
	definitions := map[string]realmGuardEnemyDefinition{}
	for _, enemy := range content.Enemies {
		definitions[enemy.ID] = enemy
	}
	for _, boss := range content.Bosses {
		definitions[boss.ID] = boss
	}
	budget := realmGuardWaveCapacity(content, stage.ID, in.WavesCompleted, !cleared)
	killReward, escapedDamage := 0, 0
	for id, count := range in.SpawnedByEnemy {
		if _, ok := definitions[id]; !ok || count > budget.EnemyCounts[id] || in.DefeatedByEnemy[id]+in.EscapedByEnemy[id] > count || cleared && in.DefeatedByEnemy[id]+in.EscapedByEnemy[id] != count {
			return rejectRealmGuardResult(422, "telemetry_attestation_failed", "enemy histogram exceeds the pinned wave spawn budget")
		}
	}
	for id, count := range in.DefeatedByEnemy {
		definition, ok := definitions[id]
		if !ok || count > in.SpawnedByEnemy[id] {
			return rejectRealmGuardResult(422, "telemetry_attestation_failed", "defeated enemy histogram is invalid")
		}
		killReward += count * definition.Reward
	}
	for id, count := range in.EscapedByEnemy {
		definition, ok := definitions[id]
		if !ok || count > in.SpawnedByEnemy[id] {
			return rejectRealmGuardResult(422, "telemetry_attestation_failed", "escaped enemy histogram is invalid")
		}
		escapedDamage += count * definition.LifeDamage
	}
	if max(0, stage.Lives-escapedDamage) != *in.RemainingLives {
		return rejectRealmGuardResult(422, "telemetry_attestation_failed", "escaped enemy life damage does not match remaining lives")
	}
	if killReward+budget.Rewards+earlyBonus != in.EarnedGold {
		return rejectRealmGuardResult(422, "telemetry_attestation_failed", "enemy rewards, wave rewards, and early bonuses do not match earned gold")
	}
	return nil
}

func validateRealmGuardTelemetryAttestation(records []realmGuardTelemetryRecord, started, serverNow time.Time, stage realmGuardStageDefinition, content realmGuardDecodedContent, version realmGuardVersionRecord, in realmGuardResultInput, cleared bool) (realmGuardTelemetryAttestation, error) {
	attestation := realmGuardTelemetryAttestation{Method: realmGuardVerificationMethod, EventCount: len(records)}
	if len(records) == 0 {
		return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "ranked RealmGuard results require server-received battle telemetry")
	}
	hasher := sha256.New()
	for index, record := range records {
		if record.Sequence != index+1 || record.ClientEventID == uuid.Nil {
			return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "RealmGuard telemetry sequence is not contiguous")
		}
		_, _ = fmt.Fprintf(hasher, "%d\n%s\n%s\n%s\n%s\n", record.Sequence, record.ClientEventID, record.Event, record.Data, record.ReceivedAt.UTC().Format(time.RFC3339Nano))
	}
	attestation.Digest = hex.EncodeToString(hasher.Sum(nil))

	var ready *realmGuardTelemetryRecord
	var complete *realmGuardTelemetryRecord
	starts := map[int]realmGuardTelemetryRecord{}
	completes := map[int]struct {
		record   realmGuardTelemetryRecord
		snapshot realmGuardWaveSnapshot
	}{}
	towers := make(map[string]realmGuardTowerDefinition, len(content.Towers))
	for _, tower := range content.Towers {
		towers[tower.ID] = tower
	}
	spots := make(map[string]bool, len(stage.TowerSpots))
	for _, spot := range stage.TowerSpots {
		spots[spot.ID] = true
	}
	activeTowers := map[string]realmGuardTowerLedgerState{}

	for index := range records {
		record := records[index]
		switch record.Event {
		case "realmguard.battle.ready":
			var data realmGuardReadyTelemetry
			if err := telemetryDecode(record.Data, &data); err != nil {
				return attestation, err
			}
			if data.StageID != stage.ID || data.Difficulty != in.Difficulty || data.HeroID != in.HeroID {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "battle.ready does not match the submitted result")
			}
			if ready == nil {
				copy := record
				ready = &copy
			}
		case "realmguard.wave.start":
			var data realmGuardWaveStartTelemetry
			if err := telemetryDecode(record.Data, &data); err != nil {
				return attestation, err
			}
			if data.StageID != stage.ID || data.Wave < 1 || starts[data.Wave].ID != 0 || data.EarlyBonus < 0 || data.EarlyBonus > 30 || data.EarlyBonus%3 != 0 || (!data.EarlyCall && data.EarlyBonus != 0) {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "wave.start sequence or early-call bonus is invalid")
			}
			starts[data.Wave] = record
			attestation.EarlyBonus += data.EarlyBonus
		case "realmguard.wave.complete":
			var data realmGuardWaveSnapshot
			if err := telemetryDecode(record.Data, &data); err != nil {
				return attestation, err
			}
			values, valid := telemetrySnapshotValues(data)
			if !valid || data.StageID != stage.ID || data.Wave < 1 || completes[data.Wave].record.ID != 0 || values[0] > stage.Lives || values[8] < 1 || values[8] > 10 || values[5]+values[6] != values[7] {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "wave.complete snapshot is invalid")
			}
			completes[data.Wave] = struct {
				record   realmGuardTelemetryRecord
				snapshot realmGuardWaveSnapshot
			}{record: record, snapshot: data}
		case "realmguard.tower.build":
			var data struct {
				Tower string `json:"tower"`
				Spot  string `json:"spot"`
			}
			if err := telemetryDecode(record.Data, &data); err != nil {
				return attestation, err
			}
			tower, exists := towers[data.Tower]
			if !exists || !spots[data.Spot] || activeTowers[data.Spot].TowerID != "" {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "tower.build ledger is invalid")
			}
			activeTowers[data.Spot] = realmGuardTowerLedgerState{TowerID: data.Tower, Level: 1, Invested: tower.Cost}
			attestation.TowerBuilds++
			attestation.LedgerSpentGold += tower.Cost
		case "realmguard.tower.upgrade":
			var data struct {
				Tower  string `json:"tower"`
				Spot   string `json:"spot"`
				Level  int    `json:"level"`
				Branch string `json:"branch"`
			}
			if err := telemetryDecode(record.Data, &data); err != nil {
				return attestation, err
			}
			tower, exists := towers[data.Tower]
			if !exists || data.Level < 2 || data.Level > 3 || data.Level-1 >= len(content.Balance.TowerUpgradeCost) {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "tower.upgrade ledger is invalid")
			}
			if data.Level == 3 {
				branchValid := false
				for _, branch := range tower.Branches {
					branchValid = branchValid || branch.ID == data.Branch
				}
				if !branchValid {
					return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "tower.upgrade branch is invalid")
				}
			}
			state, updated := activeTowers[data.Spot]
			if !updated || state.TowerID != data.Tower || state.Level != data.Level-1 {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "tower.upgrade has no matching built tower")
			}
			cost := int(content.Balance.TowerUpgradeCost[data.Level-1])
			state.Level, state.Invested = data.Level, state.Invested+cost
			activeTowers[data.Spot] = state
			attestation.TowerUpgrades++
			attestation.LedgerSpentGold += cost
		case "realmguard.tower.sell":
			var data struct {
				Spot string `json:"spot"`
			}
			if err := telemetryDecode(record.Data, &data); err != nil {
				return attestation, err
			}
			state, exists := activeTowers[data.Spot]
			if !exists {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "tower.sell has no matching built tower")
			}
			refund := int(math.Round(float64(state.Invested) * content.Balance.SellRefundRate))
			attestation.TowerSales++
			attestation.LedgerSoldGold += refund
			delete(activeTowers, data.Spot)
		case "realmguard.battle.complete":
			var data realmGuardCompleteTelemetry
			if err := telemetryDecode(record.Data, &data); err != nil {
				return attestation, err
			}
			if !realmGuardCompletionMatches(data, in, stage, version, cleared) {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "battle.complete does not match the submitted result")
			}
			copy := record
			complete = &copy
		}
	}

	if ready == nil || complete == nil || complete.ID <= ready.ID {
		return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "battle.ready and battle.complete telemetry are required in order")
	}
	if ready.ReceivedAt.Before(started.Add(-time.Second)) || ready.ReceivedAt.After(started.Add(time.Minute)) {
		return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "battle.ready was not received near session start")
	}
	if complete.ReceivedAt.After(serverNow.Add(time.Second)) || serverNow.Sub(complete.ReceivedAt) > 30*time.Second {
		return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "battle.complete is stale or has an invalid server receipt time")
	}
	attestation.FirstReceivedAt = ready.ReceivedAt.UTC()
	attestation.LastReceivedAt = complete.ReceivedAt.UTC()
	attestation.ObservedDurationMS = complete.ReceivedAt.Sub(ready.ReceivedAt).Milliseconds()
	// Battle time may run ahead of wall time because the player can double the
	// battle speed, so the observed window is compared against the fastest a
	// replay of this length could legitimately have been played.
	if (attestation.ObservedDurationMS+content.Balance.DurationToleranceMS+2000)*realmGuardMaxSpeedup < in.DurationMS {
		return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "replayed battle duration exceeds server-observed battle telemetry time")
	}

	requiredStarts := in.WavesCompleted
	if !cleared {
		requiredStarts++
	}
	for wave := range starts {
		if wave > requiredStarts {
			return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "wave.start exceeds the submitted battle progress")
		}
	}
	for wave := range completes {
		if wave > in.WavesCompleted {
			return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "wave.complete exceeds the submitted battle progress")
		}
	}
	var previousCompleteID int64 = ready.ID
	previous := []int{stage.Lives, 0, 0, 0, 0, 0, 0, 0, 1}
	previousDefeated := map[string]int{}
	previousEscaped := map[string]int{}
	previousSpawned := map[string]int{}
	minimumMilestoneMS := min(int64(1000), max(int64(250), content.Balance.MinWaveDurationMS/5))
	for wave := 1; wave <= requiredStarts; wave++ {
		start, ok := starts[wave]
		if !ok || start.ID <= previousCompleteID {
			return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "wave.start milestones are missing or out of order")
		}
		if wave <= in.WavesCompleted {
			completed, ok := completes[wave]
			if !ok || completed.record.ID <= start.ID || completed.record.ReceivedAt.Sub(start.ReceivedAt).Milliseconds() < minimumMilestoneMS {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "wave.complete milestones are missing, too fast, or out of order")
			}
			values, _ := telemetrySnapshotValues(completed.snapshot)
			if values[0] > previous[0] || values[1] < 0 || values[2] < previous[2] || values[3] < previous[3] || values[4] < previous[4] || values[5] < previous[5] || values[6] < previous[6] || values[7] < previous[7] || values[8] < previous[8] {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "wave.complete cumulative counters moved backwards")
			}
			defeated, defeatedOK := realmGuardHistogramTotal(completed.snapshot.DefeatedByEnemy)
			escaped, escapedOK := realmGuardHistogramTotal(completed.snapshot.EscapedByEnemy)
			spawned, spawnedOK := realmGuardHistogramTotal(completed.snapshot.SpawnedByEnemy)
			if completed.snapshot.DefeatedByEnemy == nil || completed.snapshot.EscapedByEnemy == nil || completed.snapshot.SpawnedByEnemy == nil || !defeatedOK || !escapedOK || !spawnedOK || defeated != values[5] || escaped != values[6] || spawned != values[7] || !realmGuardHistogramMonotonic(previousDefeated, completed.snapshot.DefeatedByEnemy) || !realmGuardHistogramMonotonic(previousEscaped, completed.snapshot.EscapedByEnemy) || !realmGuardHistogramMonotonic(previousSpawned, completed.snapshot.SpawnedByEnemy) {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "wave.complete enemy histograms are missing or moved backwards")
			}
			previous = values
			previousDefeated = completed.snapshot.DefeatedByEnemy
			previousEscaped = completed.snapshot.EscapedByEnemy
			previousSpawned = completed.snapshot.SpawnedByEnemy
			previousCompleteID = completed.record.ID
		} else if complete.ReceivedAt.Sub(start.ReceivedAt).Milliseconds() < minimumMilestoneMS {
			return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "the active defeat wave was reported too quickly")
		}
	}
	attestation.WavesStarted = len(starts)
	attestation.WavesCompleted = len(completes)
	if cleared {
		final := previous
		if final[0] != *in.RemainingLives || final[1] != *in.RemainingGold || final[2] != in.EarnedGold || final[3] != in.SpentGold || final[4] != in.SoldGold || final[5] != in.Kills || final[6] != in.Escaped || final[7] != in.Spawned || final[8] != in.HeroLevel {
			return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "the final wave snapshot does not match the clear result")
		}
	}
	if attestation.LedgerSpentGold != in.SpentGold || attestation.LedgerSoldGold != in.SoldGold {
		return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "tower action ledger does not match spent or sold gold")
	}
	if !realmGuardHistogramMonotonic(previousDefeated, in.DefeatedByEnemy) || !realmGuardHistogramMonotonic(previousEscaped, in.EscapedByEnemy) || !realmGuardHistogramMonotonic(previousSpawned, in.SpawnedByEnemy) {
		return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "final enemy histograms moved backwards from the last completed wave")
	}
	if err := validateRealmGuardEnemyHistograms(stage, content, in, cleared, attestation.EarlyBonus); err != nil {
		return attestation, err
	}
	attestation.DefeatedByEnemy = in.DefeatedByEnemy
	attestation.EscapedByEnemy = in.EscapedByEnemy
	attestation.SpawnedByEnemy = in.SpawnedByEnemy
	return attestation, nil
}

func realmGuardCompletionMatches(data realmGuardCompleteTelemetry, in realmGuardResultInput, stage realmGuardStageDefinition, version realmGuardVersionRecord, cleared bool) bool {
	if data.DurationMS == nil || data.Lives == nil || data.Gold == nil || data.EarnedGold == nil || data.SpentGold == nil || data.SoldGold == nil || data.Kills == nil || data.Escaped == nil || data.Spawned == nil || data.Waves == nil || data.WavesCompleted == nil || data.HeroLevel == nil || data.Victory == nil || data.DefeatedByEnemy == nil || data.EscapedByEnemy == nil || data.SpawnedByEnemy == nil {
		return false
	}
	return data.StageID == stage.ID && data.Mode == in.Mode && data.Difficulty == in.Difficulty &&
		*data.DurationMS == in.DurationMS && *data.Lives == *in.RemainingLives && *data.Gold == *in.RemainingGold &&
		*data.EarnedGold == in.EarnedGold && *data.SpentGold == in.SpentGold && *data.SoldGold == in.SoldGold &&
		*data.Kills == in.Kills && *data.Escaped == in.Escaped && *data.Spawned == in.Spawned &&
		*data.Waves == in.WavesCompleted && *data.WavesCompleted == in.WavesCompleted && data.HeroID == in.HeroID && *data.HeroLevel == in.HeroLevel &&
		data.ContentVersion == version.ContentVersion && data.BalanceVersion == version.BalanceVersion && data.StageVersion == stage.Version && data.AssetVersion == version.AssetVersion && *data.Victory == cleared &&
		realmGuardHistogramEqual(data.DefeatedByEnemy, in.DefeatedByEnemy) && realmGuardHistogramEqual(data.EscapedByEnemy, in.EscapedByEnemy) && realmGuardHistogramEqual(data.SpawnedByEnemy, in.SpawnedByEnemy)
}
