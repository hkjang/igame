package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const defenseVerificationMethod = "server_received_telemetry_v1"

const (
	defenseOptionalTelemetryLimit = 128
	defenseWaveTelemetryLimit     = 100
	defenseTowerLedgerLimit       = 10000
)

var defenseTelemetryEvents = map[string]bool{
	"game.pause": true, "game.resume": true,
	"defense.battle.ready": true, "defense.wave.start": true, "defense.wave.complete": true,
	"defense.tower.build": true, "defense.tower.upgrade": true, "defense.tower.sell": true,
	"defense.battle.complete": true, "defense.skill.cast": true, "defense.hero.move": true,
	"defense.education.prompt": true,
	"defense.education.apply":  true,
}

var defenseRequiredTelemetryEvents = map[string]bool{
	"defense.battle.ready": true, "defense.wave.start": true, "defense.wave.complete": true,
	"defense.tower.build": true, "defense.tower.upgrade": true, "defense.tower.sell": true,
	"defense.battle.complete": true,
	"defense.education.apply": true,
}

func validDefenseTelemetryEvent(event string) bool { return defenseTelemetryEvents[event] }

func defenseTelemetryLimitReached(event string, counts map[string]int) bool {
	if !defenseRequiredTelemetryEvents[event] {
		optional := 0
		for recorded, count := range counts {
			if !defenseRequiredTelemetryEvents[recorded] {
				optional += count
			}
		}
		return optional >= defenseOptionalTelemetryLimit
	}
	switch event {
	case "defense.battle.ready", "defense.battle.complete":
		return counts[event] >= 1
	case "defense.wave.start":
		return counts[event] >= defenseWaveTelemetryLimit+1
	case "defense.wave.complete":
		return counts[event] >= defenseWaveTelemetryLimit
	case "defense.education.apply":
		return counts[event] >= 500
	case "defense.tower.build", "defense.tower.upgrade", "defense.tower.sell":
		return counts["defense.tower.build"]+counts["defense.tower.upgrade"]+counts["defense.tower.sell"] >= defenseTowerLedgerLimit
	default:
		return true
	}
}

func (s *Server) insertDefenseTelemetry(w http.ResponseWriter, r *http.Request, p Principal, in telemetryInput, gameID uuid.UUID, occurred time.Time, tokenHash []byte) {
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var status string
	if err = tx.QueryRow(r.Context(), `SELECT status FROM game_sessions WHERE id=$1 AND user_id=$2 AND game_id=$3 AND session_token_hash=$4 AND defense_content_version_id IS NOT NULL FOR UPDATE`, in.SessionID, p.UserID, gameID, tokenHash).Scan(&status); err != nil {
		writeError(w, 403, "invalid_session", "session or token is invalid")
		return
	}
	if status != "active" {
		writeError(w, 409, "session_finished", "Defense Series telemetry is only accepted while the battle session is active")
		return
	}
	var existingEvent string
	var existingSequence int
	var sameData bool
	err = tx.QueryRow(r.Context(), `SELECT event,sequence_no,data=$3::jsonb FROM game_telemetry WHERE session_id=$1 AND client_event_id=$2`, in.SessionID, *in.ClientEventID, in.Data).Scan(&existingEvent, &existingSequence, &sameData)
	if err == nil {
		if existingEvent != in.Event || existingSequence != *in.Sequence || !sameData {
			writeError(w, 409, "telemetry_event_conflict", "client_event_id was already used for different telemetry")
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			s.dbError(w, r, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "duplicate": true, "client_event_id": in.ClientEventID, "sequence": in.Sequence})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		s.dbError(w, r, err)
		return
	}
	var lastSequence int
	if err = tx.QueryRow(r.Context(), `SELECT COALESCE(max(sequence_no),0) FROM game_telemetry WHERE session_id=$1`, in.SessionID).Scan(&lastSequence); err != nil {
		s.dbError(w, r, err)
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT event,count(*) FROM game_telemetry WHERE session_id=$1 GROUP BY event`, in.SessionID)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	counts := map[string]int{}
	for rows.Next() {
		var event string
		var count int
		if err = rows.Scan(&event, &count); err != nil {
			rows.Close()
			s.dbError(w, r, err)
			return
		}
		counts[event] = count
	}
	if err := rows.Err(); err != nil {
		s.dbError(w, r, err)
		return
	}
	rows.Close()
	if defenseTelemetryLimitReached(in.Event, counts) {
		writeError(w, 429, "telemetry_limit", "Defense Series telemetry class limit reached")
		return
	}
	if *in.Sequence != lastSequence+1 {
		writeError(w, 409, "telemetry_sequence_conflict", fmt.Sprintf("expected Defense Series telemetry sequence %d", lastSequence+1))
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO game_telemetry(session_id,user_id,game_id,event,data,occurred_at,client_event_id,sequence_no) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, in.SessionID, p.UserID, gameID, in.Event, in.Data, occurred, in.ClientEventID, in.Sequence)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.dbError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "duplicate": false, "client_event_id": in.ClientEventID, "sequence": in.Sequence})
}

type defenseTelemetryRecord struct {
	ID            int64
	Event         string
	Data          json.RawMessage
	ReceivedAt    time.Time
	ClientEventID uuid.UUID
	Sequence      int
}

func loadDefenseTelemetryRecords(ctx context.Context, tx pgx.Tx, sessionID uuid.UUID, waitForComplete bool) ([]defenseTelemetryRecord, error) {
	load := func() ([]defenseTelemetryRecord, bool, error) {
		rows, err := tx.Query(ctx, `SELECT id,event,data,received_at,client_event_id,sequence_no FROM game_telemetry WHERE session_id=$1 ORDER BY sequence_no`, sessionID)
		if err != nil {
			return nil, false, err
		}
		defer rows.Close()
		records := []defenseTelemetryRecord{}
		complete := false
		for rows.Next() {
			var record defenseTelemetryRecord
			if err := rows.Scan(&record.ID, &record.Event, &record.Data, &record.ReceivedAt, &record.ClientEventID, &record.Sequence); err != nil {
				return nil, false, err
			}
			complete = complete || record.Event == "defense.battle.complete"
			records = append(records, record)
		}
		return records, complete, rows.Err()
	}
	deadline := time.Now().Add(1200 * time.Millisecond)
	for {
		records, complete, err := load()
		if err != nil || !waitForComplete || complete || time.Now().After(deadline) {
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

type defenseReadyTelemetry struct {
	StageID        string                           `json:"stage_id"`
	Difficulty     string                           `json:"difficulty"`
	HeroID         string                           `json:"hero_id"`
	ContentVersion string                           `json:"content_version"`
	PolicyVersion  string                           `json:"policy_version"`
	ResourceState  map[string]defenseResourceMetric `json:"resource_state,omitempty"`
}

type defenseWaveStartTelemetry struct {
	StageID       string                           `json:"stage_id"`
	Wave          int                              `json:"wave"`
	EarlyCall     bool                             `json:"early_call"`
	EarlyBonus    int64                            `json:"early_bonus"`
	ResourceState map[string]defenseResourceMetric `json:"resource_state,omitempty"`
}

type defenseSnapshotTelemetry struct {
	StageID         string                           `json:"stage_id"`
	Wave            int                              `json:"wave"`
	Difficulty      string                           `json:"difficulty,omitempty"`
	DurationMS      *int64                           `json:"duration_ms,omitempty"`
	Health          *int64                           `json:"health,omitempty"`
	Lives           *int64                           `json:"lives,omitempty"`
	Resource        *int64                           `json:"resource,omitempty"`
	Gold            *int64                           `json:"gold,omitempty"`
	EarnedResource  *int64                           `json:"earned_resource,omitempty"`
	EarnedGold      *int64                           `json:"earned_gold,omitempty"`
	SpentResource   *int64                           `json:"spent_resource,omitempty"`
	SpentGold       *int64                           `json:"spent_gold,omitempty"`
	SoldResource    *int64                           `json:"sold_resource,omitempty"`
	SoldGold        *int64                           `json:"sold_gold,omitempty"`
	Kills           *int64                           `json:"kills"`
	Escaped         *int64                           `json:"escaped"`
	Spawned         *int64                           `json:"spawned"`
	WavesCompleted  *int                             `json:"waves_completed,omitempty"`
	Victory         *bool                            `json:"victory,omitempty"`
	HeroID          string                           `json:"hero_id,omitempty"`
	HeroLevel       *int                             `json:"hero_level,omitempty"`
	ContentVersion  string                           `json:"content_version,omitempty"`
	PolicyVersion   string                           `json:"policy_version,omitempty"`
	DefeatedByEnemy map[string]int64                 `json:"defeated_by_enemy"`
	EscapedByEnemy  map[string]int64                 `json:"escaped_by_enemy"`
	SpawnedByEnemy  map[string]int64                 `json:"spawned_by_enemy"`
	ResourceState   map[string]defenseResourceMetric `json:"resource_state,omitempty"`
}

func (snapshot defenseSnapshotTelemetry) normalized() (health, resource, earned, spent, sold int64, ok bool) {
	choose := func(primary, legacy *int64) (*int64, bool) {
		if primary != nil && legacy != nil && *primary != *legacy {
			return nil, false
		}
		if primary != nil {
			return primary, true
		}
		return legacy, legacy != nil
	}
	values := [][2]*int64{{snapshot.Health, snapshot.Lives}, {snapshot.Resource, snapshot.Gold}, {snapshot.EarnedResource, snapshot.EarnedGold}, {snapshot.SpentResource, snapshot.SpentGold}, {snapshot.SoldResource, snapshot.SoldGold}}
	result := make([]int64, 5)
	for index, pair := range values {
		value, exists := choose(pair[0], pair[1])
		if !exists || *value < 0 {
			return 0, 0, 0, 0, 0, false
		}
		result[index] = *value
	}
	return result[0], result[1], result[2], result[3], result[4], true
}

type defenseResourceMetric struct {
	Start     int64 `json:"start"`
	Spent     int64 `json:"spent"`
	Remaining int64 `json:"remaining"`
}

type defenseAttestation struct {
	Method                  string                           `json:"method"`
	Digest                  string                           `json:"digest"`
	EventCount              int                              `json:"event_count"`
	FirstReceivedAt         time.Time                        `json:"first_received_at"`
	LastReceivedAt          time.Time                        `json:"last_received_at"`
	ObservedDurationMS      int64                            `json:"observed_duration_ms"`
	WavesStarted            int                              `json:"waves_started"`
	WavesCompleted          int                              `json:"waves_completed"`
	EarlyBonus              int64                            `json:"early_bonus"`
	TowerBuilds             int                              `json:"tower_builds"`
	TowerUpgrades           int                              `json:"tower_upgrades"`
	TowerSales              int                              `json:"tower_sales"`
	ModelProfileBuilds      int                              `json:"model_profile_builds"`
	EducationApplies        int                              `json:"education_applies"`
	LedgerSpentResource     int64                            `json:"ledger_spent_resource"`
	LedgerSoldResource      int64                            `json:"ledger_sold_resource"`
	EducationEarnedResource int64                            `json:"education_earned_resource"`
	EducationSpentResource  int64                            `json:"education_spent_resource"`
	ResourceState           map[string]defenseResourceMetric `json:"resource_state,omitempty"`
	DefeatedByEnemy         map[string]int64                 `json:"defeated_by_enemy"`
	EscapedByEnemy          map[string]int64                 `json:"escaped_by_enemy"`
	SpawnedByEnemy          map[string]int64                 `json:"spawned_by_enemy"`
}

func decodeDefenseTelemetry(raw json.RawMessage, dst any) error {
	if len(raw) == 0 || !json.Valid(raw) || json.Unmarshal(raw, dst) != nil {
		return rejectRealmGuardResult(422, "telemetry_attestation_failed", "a required Defense Series telemetry event has invalid data")
	}
	return nil
}

func defenseHistogramTotal(values map[string]int64) (int64, bool) {
	var total int64
	for id, count := range values {
		if !validRealmGuardIdentifier(id) || count < 0 || total > math.MaxInt64-count {
			return 0, false
		}
		total += count
	}
	return total, true
}

func defenseHistogramEqual(left, right map[string]int64) bool {
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

func defenseHistogramMonotonic(previous, current map[string]int64) bool {
	for id, count := range previous {
		if current[id] < count {
			return false
		}
	}
	return true
}

func mapsCloneInt64(source map[string]int64) map[string]int64 {
	clone := make(map[string]int64, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func defenseWaveDefinitions(content defenseDecodedContent, stageID string) []defenseWaveDefinition {
	waves := []defenseWaveDefinition{}
	for _, wave := range content.Waves {
		if wave.StageID == stageID {
			waves = append(waves, wave)
		}
	}
	slices.SortFunc(waves, func(a, b defenseWaveDefinition) int { return a.Number - b.Number })
	return waves
}

func defenseExpectedHistogram(waves []defenseWaveDefinition, completed int) map[string]int64 {
	result := map[string]int64{}
	for index := 0; index < min(completed, len(waves)); index++ {
		for _, entry := range waves[index].Entries {
			result[entry.Enemy] += entry.Count
		}
	}
	return result
}

func defenseHistWithin(actual, maximum map[string]int64) bool {
	for id, count := range actual {
		if count < 0 || count > maximum[id] {
			return false
		}
	}
	return true
}

func validateDefenseResourceState(slug string, limits map[string]int64, state map[string]defenseResourceMetric) error {
	if slug != "ai-nexus-defense" {
		if len(state) > 0 {
			return rejectRealmGuardResult(422, "invalid_resource_state", "resource_state is only supported by AI Nexus Defense")
		}
		return nil
	}
	for _, key := range []string{"compute", "token", "trust", "latency"} {
		metric, ok := state[key]
		if !ok || metric.Start != limits[key] || metric.Start < 0 || metric.Spent < 0 || metric.Remaining < 0 || metric.Spent > metric.Start || metric.Remaining != metric.Start-metric.Spent {
			return rejectRealmGuardResult(422, "invalid_resource_state", "AI resource_state must match pinned start, spent, and remaining bounds")
		}
	}
	if len(state) != 4 {
		return rejectRealmGuardResult(422, "invalid_resource_state", "AI resource_state contains unsupported metrics")
	}
	return nil
}

func initialDefenseAIResourceState(content defenseDecodedContent) map[string]defenseResourceMetric {
	starts := map[string]int64{
		"compute": content.ResourceRules.ComputeStart,
		"token":   content.ResourceRules.TokenStart,
		"trust":   content.ResourceRules.TrustStart,
		"latency": content.ResourceRules.LatencyMax,
	}
	state := make(map[string]defenseResourceMetric, len(starts))
	for key, start := range starts {
		state[key] = defenseResourceMetric{Start: start, Remaining: start}
	}
	return state
}

func applyDefenseAIResourceCost(state map[string]defenseResourceMetric, costs map[string]int64) error {
	for key, cost := range costs {
		metric, ok := state[key]
		if !ok || cost < 0 {
			return rejectRealmGuardResult(422, "invalid_resource_state", "AI resource ledger contains an unsupported metric or cost")
		}
		metric.Remaining = max(int64(0), metric.Remaining-cost)
		metric.Spent = metric.Start - metric.Remaining
		state[key] = metric
	}
	return nil
}

func applyDefenseAIEducationEffect(state map[string]defenseResourceMetric, effect map[string]int64) {
	deltas := map[string]int64{
		"trust":   effect["trust_delta"],
		"latency": effect["latency_headroom_delta"],
	}
	for key, delta := range deltas {
		metric := state[key]
		if delta > 0 && metric.Remaining > math.MaxInt64-delta {
			metric.Remaining = metric.Start
		} else {
			metric.Remaining = min(metric.Start, max(int64(0), metric.Remaining+delta))
		}
		metric.Spent = metric.Start - metric.Remaining
		state[key] = metric
	}
}

func normalizedDefenseEducationEffect(resource, trust, latency int64) map[string]int64 {
	return map[string]int64{"resource_delta": resource, "trust_delta": trust, "latency_headroom_delta": latency}
}

func defenseEffectEqual(left, right map[string]int64) bool {
	for _, key := range []string{"resource_delta", "trust_delta", "latency_headroom_delta"} {
		if left[key] != right[key] {
			return false
		}
	}
	for key, value := range left {
		if !slices.Contains([]string{"resource_delta", "trust_delta", "latency_headroom_delta"}, key) && value != 0 {
			return false
		}
	}
	for key, value := range right {
		if !slices.Contains([]string{"resource_delta", "trust_delta", "latency_headroom_delta"}, key) && value != 0 {
			return false
		}
	}
	return true
}

func defenseEducationTriggerReachedBefore(records map[int]defenseTelemetryRecord, ready *defenseTelemetryRecord, event defenseEventDefinition, beforeID int64) bool {
	trigger := strings.ReplaceAll(event.Trigger, "_", "-")
	if trigger == "battle-start" {
		return ready != nil && ready.ID < beforeID
	}
	if strings.HasPrefix(trigger, "wave-") {
		wave, err := strconv.Atoi(strings.TrimPrefix(trigger, "wave-"))
		record, ok := records[wave]
		return err == nil && ok && record.ID < beforeID
	}
	return false
}

func defenseEventReached(records []defenseTelemetryRecord, content defenseDecodedContent, event defenseEventDefinition) bool {
	stageID := ""
	started := map[int]bool{}
	for _, record := range records {
		switch record.Event {
		case "defense.battle.ready":
			var ready defenseReadyTelemetry
			if decodeDefenseTelemetry(record.Data, &ready) == nil {
				stageID = ready.StageID
			}
		case "defense.wave.start":
			var wave defenseWaveStartTelemetry
			if decodeDefenseTelemetry(record.Data, &wave) == nil && wave.StageID == stageID {
				started[wave.Wave] = true
			}
		}
	}
	if stageID == "" || event.StageID != stageID {
		return false
	}
	trigger := strings.ReplaceAll(event.Trigger, "_", "-")
	if trigger == "battle-start" {
		return true
	}
	if strings.HasPrefix(trigger, "wave-") {
		wave, err := strconv.Atoi(strings.TrimPrefix(trigger, "wave-"))
		return err == nil && started[wave]
	}
	return false
}

func defenseEventTriggeredByDepletedAIStart(records []defenseTelemetryRecord, event defenseEventDefinition) bool {
	trigger := strings.ReplaceAll(event.Trigger, "_", "-")
	if !strings.HasPrefix(trigger, "wave-") {
		return false
	}
	wave, err := strconv.Atoi(strings.TrimPrefix(trigger, "wave-"))
	if err != nil {
		return false
	}
	for _, record := range records {
		if record.Event != "defense.wave.start" {
			continue
		}
		var started defenseWaveStartTelemetry
		if decodeDefenseTelemetry(record.Data, &started) == nil && started.StageID == event.StageID && started.Wave == wave {
			return defenseResourceStateDepleted(started.ResourceState)
		}
	}
	return false
}

func validateDefenseTelemetryAttestation(records []defenseTelemetryRecord, slug string, started, serverNow time.Time, stage defenseStageDefinition, content defenseDecodedContent, version defenseVersionRecord, in defenseResultInput, expectedEducationEffects map[string]map[string]int64, educationEarned, educationSpent int64) (defenseAttestation, error) {
	attestation := defenseAttestation{Method: defenseVerificationMethod, EventCount: len(records)}
	startingResource := defenseStartingResource(stage, content.Balance, in.Difficulty)
	if len(records) == 0 {
		return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "ranked Defense Series results require server-received battle telemetry")
	}
	hasher := sha256.New()
	for index, record := range records {
		if record.Sequence != index+1 || record.ClientEventID == uuid.Nil || !validDefenseTelemetryEvent(record.Event) {
			return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "Defense Series telemetry is unsupported or not contiguous")
		}
		_, _ = fmt.Fprintf(hasher, "%d\n%s\n%s\n%s\n%s\n", record.Sequence, record.ClientEventID, record.Event, record.Data, record.ReceivedAt.UTC().Format(time.RFC3339Nano))
	}
	attestation.Digest = hex.EncodeToString(hasher.Sum(nil))

	var ready, complete *defenseTelemetryRecord
	starts := map[int]defenseTelemetryRecord{}
	earlyBonuses := map[int]int64{}
	waveStartStates := map[int]map[string]defenseResourceMetric{}
	completes := map[int]struct {
		record   defenseTelemetryRecord
		snapshot defenseSnapshotTelemetry
	}{}
	towers := map[string]defenseUnitDefinition{}
	for _, tower := range content.Towers {
		towers[tower.ID] = tower
	}
	modelProfiles := map[string]defenseModelProfile{}
	profilesByTower := map[string]bool{}
	for _, profile := range content.ModelProfiles {
		modelProfiles[profile.ID] = profile
		profilesByTower[profile.TowerID] = true
	}
	educationEvents := map[string]defenseEventDefinition{}
	for _, event := range content.Events {
		educationEvents[event.ID] = event
	}
	spots := map[string]bool{}
	for _, spot := range stage.TowerSpots {
		spots[spot.ID] = true
	}
	type towerState struct {
		Tower    string
		Level    int
		Invested int64
	}
	activeTowers := map[string]towerState{}
	var final defenseSnapshotTelemetry
	educationApplyRecords := map[string]defenseTelemetryRecord{}
	var observedEducationEarned, observedEducationSpent int64
	ledgerSpentAtWave := map[int]int64{}
	ledgerSoldAtWave := map[int]int64{}
	educationEarnedAtWaveRecord := map[int]int64{}
	educationSpentAtWaveRecord := map[int]int64{}
	aiState := map[string]defenseResourceMetric{}
	previousAIEscaped := map[string]int64{}
	aiDepletedSequence := 0
	if slug == "ai-nexus-defense" {
		aiState = initialDefenseAIResourceState(content)
	}
	enemiesForResource := map[string]defenseUnitDefinition{}
	for _, enemy := range append(append([]defenseUnitDefinition{}, content.Enemies...), content.Bosses...) {
		enemiesForResource[enemy.ID] = enemy
	}

	for index := range records {
		record := records[index]
		if aiDepletedSequence > 0 && record.Sequence > aiDepletedSequence && defenseRequiredTelemetryEvents[record.Event] && record.Event != "defense.battle.complete" {
			return attestation, rejectRealmGuardResult(422, "invalid_resource_state", "AI battle ledger continued after a terminal resource depletion")
		}
		switch record.Event {
		case "defense.battle.ready":
			var data defenseReadyTelemetry
			if err := decodeDefenseTelemetry(record.Data, &data); err != nil {
				return attestation, err
			}
			if ready != nil || data.StageID != stage.ID || data.Difficulty != in.Difficulty || data.HeroID != in.Battle.HeroID || data.ContentVersion != version.ContentVersion || data.PolicyVersion != version.PolicyVersion || (slug == "ai-nexus-defense" && !resourceStatesEqual(data.ResourceState, aiState)) || (slug != "ai-nexus-defense" && len(data.ResourceState) != 0) {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "battle.ready does not match the pinned result")
			}
			copy := record
			ready = &copy
		case "defense.wave.start":
			var data defenseWaveStartTelemetry
			if err := decodeDefenseTelemetry(record.Data, &data); err != nil {
				return attestation, err
			}
			if data.StageID != stage.ID || data.Wave < 1 || data.Wave > defenseWaveTelemetryLimit+1 || starts[data.Wave].ID != 0 || data.EarlyBonus < 0 || data.EarlyBonus > 30 || data.EarlyBonus%3 != 0 || (!data.EarlyCall && data.EarlyBonus != 0) {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "wave.start is invalid or duplicated")
			}
			if slug == "ai-nexus-defense" {
				if err := applyDefenseAIResourceCost(aiState, map[string]int64{"compute": content.ResourceRules.WaveComputeCost, "token": content.ResourceRules.WaveTokenCost}); err != nil || !resourceStatesEqual(data.ResourceState, aiState) {
					return attestation, rejectRealmGuardResult(422, "invalid_resource_state", "wave.start AI resource_state does not match pinned wave costs")
				}
			} else if len(data.ResourceState) != 0 {
				return attestation, rejectRealmGuardResult(422, "invalid_resource_state", "resource_state is only supported by AI Nexus Defense")
			}
			starts[data.Wave] = record
			earlyBonuses[data.Wave] = data.EarlyBonus
			waveStartStates[data.Wave] = data.ResourceState
			attestation.EarlyBonus = defenseSaturatingTotal(attestation.EarlyBonus, data.EarlyBonus)
		case "defense.wave.complete":
			var data defenseSnapshotTelemetry
			if err := decodeDefenseTelemetry(record.Data, &data); err != nil {
				return attestation, err
			}
			health, resource, earned, spent, sold, valid := data.normalized()
			if !valid || data.StageID != stage.ID || data.Wave < 1 || data.Kills == nil || data.Escaped == nil || data.Spawned == nil || *data.Kills < 0 || *data.Escaped < 0 || *data.Spawned < 0 || health > stage.StartingHealth || resource > math.MaxInt64-earned || spent > math.MaxInt64-sold || completes[data.Wave].record.ID != 0 {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "wave.complete snapshot is invalid")
			}
			if slug == "ai-nexus-defense" {
				costs := map[string]int64{}
				var newlyEscaped int64
				for enemyID, total := range data.EscapedByEnemy {
					previous := previousAIEscaped[enemyID]
					definition, known := enemiesForResource[enemyID]
					if !known || total < previous {
						return attestation, rejectRealmGuardResult(422, "invalid_resource_state", "wave.complete escaped resource ledger is invalid")
					}
					newlyEscaped = defenseSaturatingTotal(newlyEscaped, total-previous)
					for metric, cost := range definition.ResourceEffect {
						costs[metric] = defenseSaturatingTotal(costs[metric], defenseSaturatingProduct(total-previous, cost))
					}
				}
				costs["trust"] = defenseSaturatingTotal(costs["trust"], defenseSaturatingProduct(newlyEscaped, content.ResourceRules.EscapedTrustCost))
				costs["latency"] = defenseSaturatingTotal(costs["latency"], defenseSaturatingProduct(newlyEscaped, content.ResourceRules.EscapedLatencyCost))
				if err := applyDefenseAIResourceCost(aiState, costs); err != nil || !resourceStatesEqual(data.ResourceState, aiState) {
					return attestation, rejectRealmGuardResult(422, "invalid_resource_state", "wave.complete AI resource_state does not match escaped threat effects")
				}
				previousAIEscaped = mapsCloneInt64(data.EscapedByEnemy)
			} else if len(data.ResourceState) != 0 {
				return attestation, rejectRealmGuardResult(422, "invalid_resource_state", "resource_state is only supported by AI Nexus Defense")
			}
			completes[data.Wave] = struct {
				record   defenseTelemetryRecord
				snapshot defenseSnapshotTelemetry
			}{record, data}
			ledgerSpentAtWave[data.Wave] = attestation.LedgerSpentResource
			ledgerSoldAtWave[data.Wave] = attestation.LedgerSoldResource
			educationEarnedAtWaveRecord[data.Wave] = observedEducationEarned
			educationSpentAtWaveRecord[data.Wave] = observedEducationSpent
		case "defense.tower.build":
			var data struct {
				Tower         string                           `json:"tower"`
				Spot          string                           `json:"spot"`
				ProfileID     string                           `json:"profile_id"`
				ResourceState map[string]defenseResourceMetric `json:"resource_state,omitempty"`
			}
			if err := decodeDefenseTelemetry(record.Data, &data); err != nil {
				return attestation, err
			}
			tower, exists := towers[data.Tower]
			if !exists || !spots[data.Spot] || activeTowers[data.Spot].Tower != "" {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "tower.build ledger is invalid")
			}
			if slug == "ai-nexus-defense" {
				profile, hasProfile := modelProfiles[data.ProfileID]
				if profilesByTower[data.Tower] {
					if !hasProfile || profile.TowerID != data.Tower {
						return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "AI model tower build requires its selected pinned profile_id")
					}
					if err := applyDefenseAIResourceCost(aiState, map[string]int64{"compute": profile.ComputeCost, "token": profile.TokenCost, "latency": profile.LatencyCost}); err != nil {
						return attestation, err
					}
					attestation.ModelProfileBuilds++
				} else if data.ProfileID != "" {
					return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "profile_id is not supported by this tower")
				}
				if !resourceStatesEqual(data.ResourceState, aiState) {
					return attestation, rejectRealmGuardResult(422, "invalid_resource_state", "tower.build AI resource_state does not match the selected model profile")
				}
			} else if data.ProfileID != "" || len(data.ResourceState) != 0 {
				return attestation, rejectRealmGuardResult(422, "invalid_resource_state", "model profiles and resource_state are only supported by AI Nexus Defense")
			}
			activeTowers[data.Spot] = towerState{data.Tower, 1, tower.Cost}
			attestation.TowerBuilds++
			attestation.LedgerSpentResource += tower.Cost
		case "defense.tower.upgrade":
			var data struct {
				Tower, Spot, Branch string
				Level               int
			}
			if err := decodeDefenseTelemetry(record.Data, &data); err != nil {
				return attestation, err
			}
			tower, exists := towers[data.Tower]
			state := activeTowers[data.Spot]
			if !exists || state.Tower != data.Tower || data.Level != state.Level+1 || data.Level < 2 || data.Level > 3 || data.Level-1 >= len(content.Balance.TowerUpgradeCost) {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "tower.upgrade ledger is invalid")
			}
			if data.Level == 3 && !slices.ContainsFunc(tower.Branches, func(branch defenseTowerBranch) bool { return branch.ID == data.Branch }) {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "tower.upgrade branch is invalid")
			}
			cost := content.Balance.TowerUpgradeCost[data.Level-1]
			state.Level, state.Invested = data.Level, state.Invested+cost
			activeTowers[data.Spot] = state
			attestation.TowerUpgrades++
			attestation.LedgerSpentResource += cost
		case "defense.tower.sell":
			var data struct{ Spot string }
			if err := decodeDefenseTelemetry(record.Data, &data); err != nil {
				return attestation, err
			}
			state, exists := activeTowers[data.Spot]
			if !exists {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "tower.sell ledger is invalid")
			}
			refund := int64(math.Round(float64(state.Invested) * content.Balance.SellRefundRate))
			attestation.TowerSales++
			attestation.LedgerSoldResource += refund
			delete(activeTowers, data.Spot)
		case "defense.education.apply":
			var data struct {
				EventID              string                           `json:"event_id"`
				ResourceDelta        int64                            `json:"resource_delta"`
				TrustDelta           int64                            `json:"trust_delta"`
				LatencyHeadroomDelta int64                            `json:"latency_headroom_delta"`
				Effect               map[string]int64                 `json:"effect,omitempty"`
				ResourceState        map[string]defenseResourceMetric `json:"resource_state,omitempty"`
			}
			if err := decodeDefenseTelemetry(record.Data, &data); err != nil {
				return attestation, err
			}
			observedEffect := normalizedDefenseEducationEffect(data.ResourceDelta, data.TrustDelta, data.LatencyHeadroomDelta)
			if len(data.Effect) != 0 {
				if data.ResourceDelta != 0 || data.TrustDelta != 0 || data.LatencyHeadroomDelta != 0 {
					return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "education.apply must use one canonical effect representation")
				}
				observedEffect = normalizedDefenseEducationEffect(data.Effect["resource_delta"], data.Effect["trust_delta"], data.Effect["latency_headroom_delta"])
			}
			expectedEffect, answered := expectedEducationEffects[data.EventID]
			event, exists := educationEvents[data.EventID]
			if !answered || !exists || educationApplyRecords[data.EventID].ID != 0 || event.StageID != stage.ID || !defenseEffectEqual(observedEffect, expectedEffect) || !defenseEducationTriggerReachedBefore(starts, ready, event, record.ID) {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "education.apply does not match a reached server-validated answer")
			}
			if observedEffect["resource_delta"] >= 0 {
				observedEducationEarned = defenseSaturatingTotal(observedEducationEarned, observedEffect["resource_delta"])
			} else {
				observedEducationSpent = defenseSaturatingTotal(observedEducationSpent, -observedEffect["resource_delta"])
			}
			if slug == "ai-nexus-defense" {
				applyDefenseAIEducationEffect(aiState, observedEffect)
				if !resourceStatesEqual(data.ResourceState, aiState) {
					return attestation, rejectRealmGuardResult(422, "invalid_resource_state", "education.apply AI resource_state does not match the server answer effect")
				}
			} else if len(data.ResourceState) != 0 {
				return attestation, rejectRealmGuardResult(422, "invalid_resource_state", "resource_state is only supported by AI Nexus Defense")
			}
			educationApplyRecords[data.EventID] = record
			attestation.EducationApplies++
		case "defense.battle.complete":
			if complete != nil {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "battle.complete is duplicated")
			}
			if err := decodeDefenseTelemetry(record.Data, &final); err != nil {
				return attestation, err
			}
			if slug == "ai-nexus-defense" {
				costs := map[string]int64{}
				var newlyEscaped int64
				for enemyID, total := range final.EscapedByEnemy {
					previous := previousAIEscaped[enemyID]
					definition, known := enemiesForResource[enemyID]
					if !known || total < previous {
						return attestation, rejectRealmGuardResult(422, "invalid_resource_state", "battle.complete escaped resource ledger is invalid")
					}
					newlyEscaped = defenseSaturatingTotal(newlyEscaped, total-previous)
					for metric, cost := range definition.ResourceEffect {
						costs[metric] = defenseSaturatingTotal(costs[metric], defenseSaturatingProduct(total-previous, cost))
					}
				}
				costs["trust"] = defenseSaturatingTotal(costs["trust"], defenseSaturatingProduct(newlyEscaped, content.ResourceRules.EscapedTrustCost))
				costs["latency"] = defenseSaturatingTotal(costs["latency"], defenseSaturatingProduct(newlyEscaped, content.ResourceRules.EscapedLatencyCost))
				if err := applyDefenseAIResourceCost(aiState, costs); err != nil || !resourceStatesEqual(final.ResourceState, aiState) {
					return attestation, rejectRealmGuardResult(422, "invalid_resource_state", "battle.complete AI resource_state does not match the pinned ledger")
				}
			} else if len(final.ResourceState) != 0 {
				return attestation, rejectRealmGuardResult(422, "invalid_resource_state", "resource_state is only supported by AI Nexus Defense")
			}
			copy := record
			complete = &copy
		}
		if slug == "ai-nexus-defense" && aiDepletedSequence == 0 && defenseResourceStateDepleted(aiState) {
			aiDepletedSequence = record.Sequence
		}
	}
	if ready == nil || complete == nil || complete.ID <= ready.ID || complete.Sequence != len(records) {
		return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "battle.ready and battle.complete are required in order")
	}
	if len(educationApplyRecords) != len(expectedEducationEffects) || observedEducationEarned != educationEarned || observedEducationSpent != educationSpent {
		return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "every server-validated education answer requires one exact education.apply ledger event")
	}
	for eventID := range expectedEducationEffects {
		if educationApplyRecords[eventID].ID == 0 {
			return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "an education.apply ledger event is missing")
		}
	}
	if ready.ReceivedAt.Before(started.Add(-time.Second)) || ready.ReceivedAt.After(started.Add(time.Minute)) || complete.ReceivedAt.After(serverNow.Add(time.Second)) || serverNow.Sub(complete.ReceivedAt) > 30*time.Second {
		return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "telemetry server receipt time is invalid")
	}
	attestation.FirstReceivedAt, attestation.LastReceivedAt = ready.ReceivedAt.UTC(), complete.ReceivedAt.UTC()
	attestation.ObservedDurationMS = complete.ReceivedAt.Sub(ready.ReceivedAt).Milliseconds()
	if attestation.ObservedDurationMS+content.Balance.DurationToleranceMS+2000 < in.DurationMS {
		return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "submitted duration exceeds server-observed telemetry time")
	}

	requiredStarts := in.WavesCompleted
	aiResourceDefeat := slug == "ai-nexus-defense" && !in.Victory && defenseResourceStateDepleted(in.ResourceState)
	if !in.Victory && !aiResourceDefeat {
		requiredStarts++
	} else if aiResourceDefeat {
		if len(starts) < in.WavesCompleted || len(starts) > in.WavesCompleted+1 {
			return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "AI resource defeat wave milestones do not match the pinned ledger")
		}
		requiredStarts = len(starts)
	}
	if requiredStarts < 1 && (!aiResourceDefeat || attestation.EducationApplies == 0 && attestation.ModelProfileBuilds == 0) {
		return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "a zero-wave defeat requires exact AI resource depletion from a server-validated ledger action")
	}
	waves := defenseWaveDefinitions(content, stage.ID)
	previousDefeated, previousEscaped, previousSpawned := map[string]int64{}, map[string]int64{}, map[string]int64{}
	var previousCompleteID int64 = ready.ID
	minimumMilestoneMS := min(int64(1000), max(int64(250), content.Balance.MinWaveDurationMS/5))
	var cumulativeEarlyBonus int64
	enemyDefinitions := map[string]defenseUnitDefinition{}
	for _, enemy := range append(append([]defenseUnitDefinition{}, content.Enemies...), content.Bosses...) {
		enemyDefinitions[enemy.ID] = enemy
	}
	for wave := 1; wave <= requiredStarts; wave++ {
		startRecord, exists := starts[wave]
		if !exists || startRecord.ID <= previousCompleteID {
			return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "wave.start milestones are missing or out of order")
		}
		cumulativeEarlyBonus = defenseSaturatingTotal(cumulativeEarlyBonus, earlyBonuses[wave])
		if wave <= in.WavesCompleted {
			completed, exists := completes[wave]
			if !exists || completed.record.ID <= startRecord.ID || completed.record.ReceivedAt.Sub(startRecord.ReceivedAt).Milliseconds() < minimumMilestoneMS {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "wave.complete milestones are missing, too fast, or out of order")
			}
			snapshot := completed.snapshot
			health, resource, earned, spent, sold, valid := snapshot.normalized()
			defeated, okD := defenseHistogramTotal(snapshot.DefeatedByEnemy)
			escaped, okE := defenseHistogramTotal(snapshot.EscapedByEnemy)
			spawned, okS := defenseHistogramTotal(snapshot.SpawnedByEnemy)
			expected := defenseExpectedHistogram(waves, wave)
			if !valid || snapshot.Kills == nil || snapshot.Escaped == nil || snapshot.Spawned == nil || !okD || !okE || !okS || defeated != *snapshot.Kills || escaped != *snapshot.Escaped || spawned != *snapshot.Spawned || defeated+escaped != spawned || !defenseHistogramEqual(snapshot.SpawnedByEnemy, expected) || !defenseHistogramMonotonic(previousDefeated, snapshot.DefeatedByEnemy) || !defenseHistogramMonotonic(previousEscaped, snapshot.EscapedByEnemy) || !defenseHistogramMonotonic(previousSpawned, snapshot.SpawnedByEnemy) {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "wave.complete cumulative histograms do not match pinned waves")
			}
			var damage, killReward, waveReward int64
			for id, count := range snapshot.EscapedByEnemy {
				damage += defenseSaturatingProduct(count, enemyDefinitions[id].HealthDamage)
			}
			for id, count := range snapshot.DefeatedByEnemy {
				killReward += defenseSaturatingProduct(count, enemyDefinitions[id].Reward)
			}
			for index := 0; index < wave; index++ {
				waveReward += waves[index].Reward
			}
			educationEarnedAtWave := educationEarnedAtWaveRecord[wave]
			educationSpentAtWave := educationSpentAtWaveRecord[wave]
			expectedResource := max(int64(0), startingResource+earned+sold-spent)
			if health != max(int64(0), stage.StartingHealth-damage) || earned != defenseSaturatingTotal(defenseSaturatingTotal(killReward, waveReward), defenseSaturatingTotal(cumulativeEarlyBonus, educationEarnedAtWave)) || spent != defenseSaturatingTotal(ledgerSpentAtWave[wave], educationSpentAtWave) || sold != ledgerSoldAtWave[wave] || resource != expectedResource {
				return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "wave.complete health or economy ledger is invalid")
			}
			previousDefeated, previousEscaped, previousSpawned = snapshot.DefeatedByEnemy, snapshot.EscapedByEnemy, snapshot.SpawnedByEnemy
			previousCompleteID = completed.record.ID
		} else if complete.ReceivedAt.Sub(startRecord.ReceivedAt).Milliseconds() < minimumMilestoneMS && !(aiResourceDefeat && defenseResourceStateDepleted(waveStartStates[wave])) {
			return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "the active defeat wave was reported too quickly")
		}
	}
	if len(starts) != requiredStarts || len(completes) != in.WavesCompleted {
		return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "wave milestone count does not match the result")
	}
	health, resource, earned, spent, sold, valid := final.normalized()
	if !valid || final.DurationMS == nil || final.Kills == nil || final.Escaped == nil || final.Spawned == nil || final.WavesCompleted == nil || final.Victory == nil || final.HeroLevel == nil || final.StageID != stage.ID || final.Difficulty != in.Difficulty || *final.DurationMS != in.DurationMS || health != in.RemainingHealth || resource != in.RemainingResource || earned != in.Battle.EarnedResource || spent != in.Battle.SpentResource || sold != in.Battle.RecoveredResource || *final.Kills != in.Kills || *final.Escaped != in.Escaped || *final.Spawned != in.Spawned || *final.WavesCompleted != in.WavesCompleted || *final.Victory != in.Victory || final.HeroID != in.Battle.HeroID || *final.HeroLevel != in.Battle.HeroLevel || final.ContentVersion != version.ContentVersion || final.PolicyVersion != version.PolicyVersion || !defenseHistogramEqual(final.DefeatedByEnemy, in.DefeatedByEnemy) || !defenseHistogramEqual(final.EscapedByEnemy, in.EscapedByEnemy) || !defenseHistogramEqual(final.SpawnedByEnemy, in.SpawnedByEnemy) {
		return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "battle.complete does not match the submitted result")
	}
	defeated, okD := defenseHistogramTotal(in.DefeatedByEnemy)
	escaped, okE := defenseHistogramTotal(in.EscapedByEnemy)
	spawned, okS := defenseHistogramTotal(in.SpawnedByEnemy)
	if !okD || !okE || !okS || defeated != in.Kills || escaped != in.Escaped || spawned != in.Spawned || !defenseHistogramMonotonic(previousDefeated, in.DefeatedByEnemy) || !defenseHistogramMonotonic(previousEscaped, in.EscapedByEnemy) || !defenseHistogramMonotonic(previousSpawned, in.SpawnedByEnemy) {
		return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "final enemy histograms are invalid")
	}
	maximum := defenseExpectedHistogram(waves, requiredStarts)
	if !defenseHistWithin(in.SpawnedByEnemy, maximum) || !defenseHistWithin(in.DefeatedByEnemy, in.SpawnedByEnemy) || !defenseHistWithin(in.EscapedByEnemy, in.SpawnedByEnemy) {
		return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "final enemy histogram exceeds the pinned wave budget")
	}
	var damage, killReward, waveReward int64
	for id, count := range in.EscapedByEnemy {
		definition, ok := enemyDefinitions[id]
		if !ok {
			return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "unknown enemy in escaped histogram")
		}
		damage += defenseSaturatingProduct(count, definition.HealthDamage)
	}
	for id, count := range in.DefeatedByEnemy {
		definition, ok := enemyDefinitions[id]
		if !ok {
			return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "unknown enemy in defeated histogram")
		}
		killReward += defenseSaturatingProduct(count, definition.Reward)
	}
	for index := 0; index < in.WavesCompleted; index++ {
		waveReward += waves[index].Reward
	}
	expectedRemainingResource := max(int64(0), startingResource+in.Battle.EarnedResource+in.Battle.RecoveredResource-in.Battle.SpentResource)
	if max(int64(0), stage.StartingHealth-damage) != in.RemainingHealth || defenseSaturatingTotal(defenseSaturatingTotal(killReward, waveReward), defenseSaturatingTotal(attestation.EarlyBonus, educationEarned)) != in.Battle.EarnedResource || expectedRemainingResource != in.RemainingResource || defenseSaturatingTotal(attestation.LedgerSpentResource, educationSpent) != in.Battle.SpentResource || attestation.LedgerSoldResource != in.Battle.RecoveredResource {
		return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "final health or tower economy ledger is invalid")
	}
	if err := validateDefenseResourceState(slug, content.Balance.ResourceStateLimits, in.ResourceState); err != nil {
		return attestation, err
	}
	if !resourceStatesEqual(final.ResourceState, in.ResourceState) {
		return attestation, rejectRealmGuardResult(422, "telemetry_attestation_failed", "battle.complete resource_state does not match result")
	}
	attestation.WavesStarted = len(starts)
	attestation.WavesCompleted = len(completes)
	attestation.DefeatedByEnemy = in.DefeatedByEnemy
	attestation.EscapedByEnemy = in.EscapedByEnemy
	attestation.SpawnedByEnemy = in.SpawnedByEnemy
	attestation.EducationEarnedResource = educationEarned
	attestation.EducationSpentResource = educationSpent
	attestation.ResourceState = in.ResourceState
	return attestation, nil
}

func resourceStatesEqual(left, right map[string]defenseResourceMetric) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func defenseResourceStateDepleted(state map[string]defenseResourceMetric) bool {
	for _, metric := range state {
		if metric.Remaining == 0 {
			return true
		}
	}
	return false
}
