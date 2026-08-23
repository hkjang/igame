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
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var defenseGameSlugs = []string{"office-guardians", "cyber-fortress", "ai-nexus-defense"}

var defenseSections = []string{"stages", "waves", "towers", "enemies", "bosses", "heroes", "skills", "events", "education", "balance", "campaigns", "resource_rules", "model_profiles"}

type defenseVersionRecord struct {
	ID              uuid.UUID
	GameID          uuid.UUID
	VersionNo       int
	Label           string
	Status          string
	ContentVersion  string
	PolicyVersion   string
	AssetVersion    string
	Checksum        string
	Notes           string
	RawContent      json.RawMessage
	SourceVersionID *uuid.UUID
	CreatedBy       *uuid.UUID
	ApprovedBy      *uuid.UUID
	CreatedAt       time.Time
	TestedAt        *time.Time
	RequestedAt     *time.Time
	ApprovedAt      *time.Time
	ReviewComment   string
	ReviewedAt      *time.Time
	PublishedAt     *time.Time
	UpdatedAt       time.Time
}

const defenseVersionColumns = `id,game_id,version_no,label,status,content_version,policy_version,asset_version,checksum,notes,content,source_version_id,created_by,approved_by,created_at,tested_at,approval_requested_at,approved_at,review_comment,reviewed_at,published_at,updated_at`

func scanDefenseVersion(row rowScanner) (defenseVersionRecord, error) {
	var version defenseVersionRecord
	err := row.Scan(&version.ID, &version.GameID, &version.VersionNo, &version.Label, &version.Status, &version.ContentVersion, &version.PolicyVersion, &version.AssetVersion, &version.Checksum, &version.Notes, &version.RawContent, &version.SourceVersionID, &version.CreatedBy, &version.ApprovedBy, &version.CreatedAt, &version.TestedAt, &version.RequestedAt, &version.ApprovedAt, &version.ReviewComment, &version.ReviewedAt, &version.PublishedAt, &version.UpdatedAt)
	return version, err
}

type defenseStageDefinition struct {
	ID               string           `json:"id"`
	Number           int              `json:"number"`
	Name             string           `json:"name"`
	Mode             string           `json:"mode"`
	StartingHealth   int64            `json:"starting_health"`
	StartingResource int64            `json:"starting_resource"`
	Version          string           `json:"version"`
	Theme            string           `json:"theme"`
	Gimmick          string           `json:"gimmick,omitempty"`
	Path             []defensePoint   `json:"path"`
	Paths            [][]defensePoint `json:"paths,omitempty"`
	TowerSpots       []defensePoint   `json:"tower_spots"`
}

type defensePoint struct {
	ID string  `json:"id,omitempty"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
}

type defenseWaveEntry struct {
	Enemy    string  `json:"enemy"`
	Count    int64   `json:"count"`
	Interval float64 `json:"interval"`
}

type defenseWaveDefinition struct {
	ID      string             `json:"id"`
	StageID string             `json:"stage_id"`
	Number  int                `json:"number"`
	Reward  int64              `json:"reward"`
	Entries []defenseWaveEntry `json:"entries"`
}

type defenseUnitDefinition struct {
	ID                  string               `json:"id"`
	Name                string               `json:"name"`
	Cost                int64                `json:"cost,omitempty"`
	Damage              float64              `json:"damage,omitempty"`
	Range               float64              `json:"range,omitempty"`
	FireRate            float64              `json:"fire_rate,omitempty"`
	HP                  float64              `json:"hp,omitempty"`
	Speed               float64              `json:"speed,omitempty"`
	Radius              float64              `json:"radius,omitempty"`
	Armor               float64              `json:"armor,omitempty"`
	Reward              int64                `json:"reward,omitempty"`
	HealthDamage        int64                `json:"health_damage,omitempty"`
	UnlockStage         int                  `json:"unlock_stage,omitempty"`
	Traits              []string             `json:"traits,omitempty"`
	Role                string               `json:"role,omitempty"`
	Color               int                  `json:"color,omitempty"`
	ProjectileSpeed     float64              `json:"projectile_speed,omitempty"`
	DamageType          string               `json:"damage_type,omitempty"`
	Title               string               `json:"title,omitempty"`
	RespawnSeconds      float64              `json:"respawn_seconds,omitempty"`
	Skill1              string               `json:"skill1,omitempty"`
	Skill2              string               `json:"skill2,omitempty"`
	Ultimate            string               `json:"ultimate,omitempty"`
	Branches            []defenseTowerBranch `json:"branches,omitempty"`
	ThreatType          string               `json:"threat_type,omitempty"`
	EffectiveAgainst    []string             `json:"effective_against,omitempty"`
	EffectiveMultiplier float64              `json:"effective_multiplier,omitempty"`
	ResourceEffect      map[string]int64     `json:"resource_effect,omitempty"`
}

type defenseTowerBranch struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	DamageMultiplier *float64 `json:"damage_multiplier,omitempty"`
	RangeMultiplier  *float64 `json:"range_multiplier,omitempty"`
	RateMultiplier   *float64 `json:"rate_multiplier,omitempty"`
	Splash           float64  `json:"splash,omitempty"`
	Slow             float64  `json:"slow,omitempty"`
	Pierce           float64  `json:"pierce,omitempty"`
}

type defenseSkillDefinition struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Cooldown    float64 `json:"cooldown"`
	Color       string  `json:"color,omitempty"`
	Effect      string  `json:"effect"`
}

type defenseEventDefinition struct {
	ID          string          `json:"id"`
	StageID     string          `json:"stage_id,omitempty"`
	Trigger     string          `json:"trigger"`
	EducationID string          `json:"education_id,omitempty"`
	Reward      json.RawMessage `json:"reward,omitempty"`
	Penalty     json.RawMessage `json:"penalty,omitempty"`
}

type defenseAnswerDefinition struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type defenseEducationDefinition struct {
	ID              string                    `json:"id"`
	Topic           string                    `json:"topic"`
	Question        string                    `json:"question"`
	Answers         []defenseAnswerDefinition `json:"answers"`
	CorrectAnswerID string                    `json:"correct_answer_id"`
	Score           int                       `json:"score"`
	Explanation     string                    `json:"explanation,omitempty"`
	PolicyReference string                    `json:"policy_reference,omitempty"`
}

type defenseDifficulty struct {
	DifficultyBonus int64   `json:"difficulty_bonus"`
	EnemyHP         float64 `json:"enemy_hp"`
	EnemySpeed      float64 `json:"enemy_speed"`
	Gold            float64 `json:"gold"`
	Score           float64 `json:"score"`
}

type defenseBalanceDefinition struct {
	Difficulties           map[string]defenseDifficulty `json:"difficulties"`
	HealthScoreFactor      int64                        `json:"health_score_factor"`
	ResourceScoreFactor    int64                        `json:"resource_score_factor"`
	WaveScoreFactor        int64                        `json:"wave_score_factor"`
	ClearTimeTargetMS      int64                        `json:"clear_time_target_ms"`
	ClearTimeBonusDivisor  int64                        `json:"clear_time_bonus_divisor"`
	MinWaveDurationMS      int64                        `json:"min_wave_duration_ms"`
	DurationToleranceMS    int64                        `json:"duration_tolerance_ms"`
	TowerUpgradeCost       []int64                      `json:"tower_upgrade_cost"`
	SellRefundRate         float64                      `json:"sell_refund_rate"`
	ResourceStateLimits    map[string]int64             `json:"resource_state_limits,omitempty"`
	AIResourceScoreFactors map[string]int64             `json:"ai_resource_score_factors,omitempty"`
}

type defenseCampaignDefinition struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	StageIDs              []string `json:"stage_ids"`
	RequiredLearningScore int      `json:"required_learning_score"`
}

type defenseResourceRules struct {
	ComputeStart       int64 `json:"compute_start"`
	TokenStart         int64 `json:"token_start"`
	TrustStart         int64 `json:"trust_start"`
	LatencyMax         int64 `json:"latency_max"`
	WaveComputeCost    int64 `json:"wave_compute_cost"`
	WaveTokenCost      int64 `json:"wave_token_cost"`
	EscapedTrustCost   int64 `json:"escaped_trust_cost"`
	EscapedLatencyCost int64 `json:"escaped_latency_cost"`
}

type defenseModelProfile struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	TowerID          string  `json:"tower_id"`
	ComputeCost      int64   `json:"compute_cost"`
	TokenCost        int64   `json:"token_cost"`
	LatencyCost      int64   `json:"latency_cost"`
	Accuracy         int     `json:"accuracy"`
	DamageMultiplier float64 `json:"damage_multiplier"`
}

type defenseDecodedContent struct {
	Sections      map[string]json.RawMessage
	Stages        []defenseStageDefinition
	Waves         []defenseWaveDefinition
	Towers        []defenseUnitDefinition
	Enemies       []defenseUnitDefinition
	Bosses        []defenseUnitDefinition
	Heroes        []defenseUnitDefinition
	Skills        []defenseSkillDefinition
	Events        []defenseEventDefinition
	Education     []defenseEducationDefinition
	Balance       defenseBalanceDefinition
	Campaigns     []defenseCampaignDefinition
	ResourceRules defenseResourceRules
	ModelProfiles []defenseModelProfile
}

func isDefenseGameSlug(slug string) bool { return slices.Contains(defenseGameSlugs, slug) }

func validDefenseHexColor(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 3
}

func validDefenseAnswerIdentifier(value string) bool {
	return slices.Contains([]string{"A", "B", "C", "D", "E", "F"}, value) || validRealmGuardIdentifier(value)
}

func validDefenseFiniteRange(value, minimum, maximum float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= minimum && value <= maximum
}

func defenseEducationEnabled(content defenseDecodedContent) bool {
	return len(content.Events) > 0 && len(content.Education) > 0
}

func defenseRequiredEducationEvents(content defenseDecodedContent, stageID string, wavesStarted, terminalDepletedWave int) map[string]bool {
	required := map[string]bool{}
	for _, event := range content.Events {
		if event.StageID != stageID {
			continue
		}
		trigger := strings.ReplaceAll(event.Trigger, "_", "-")
		if trigger == "battle-start" {
			required[event.ID] = true
			continue
		}
		wave, err := strconv.Atoi(strings.TrimPrefix(trigger, "wave-"))
		if err == nil && strings.HasPrefix(trigger, "wave-") && wave >= 1 && wave <= wavesStarted && wave != terminalDepletedWave {
			required[event.ID] = true
		}
	}
	return required
}

func defenseHeroAvailable(content defenseDecodedContent, heroID string, stageNumber int) bool {
	for _, hero := range content.Heroes {
		if hero.ID == heroID && hero.UnlockStage <= stageNumber {
			return true
		}
	}
	return false
}

func isProtectedAuthoritativeGameSlug(slug string) bool {
	return slug == realmGuardSlug || isDefenseGameSlug(slug)
}

func defenseSlugParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	slug := strings.TrimSpace(chi.URLParam(r, "slug"))
	if !isDefenseGameSlug(slug) {
		writeError(w, http.StatusNotFound, "defense_game_not_found", "unknown Defense Series game")
		return "", false
	}
	return slug, true
}

func defenseChecksum(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *Server) defenseGame(ctx context.Context, slug string) (uuid.UUID, string, error) {
	var id uuid.UUID
	var name string
	err := s.DB.QueryRow(ctx, `SELECT id,name FROM games WHERE slug=$1 AND status='active'`, slug).Scan(&id, &name)
	return id, name, err
}

func (s *Server) loadDefensePublished(ctx context.Context, slug string) (defenseVersionRecord, error) {
	version, err := scanDefenseVersion(s.DB.QueryRow(ctx, `SELECT `+defenseVersionColumns+` FROM defense_content_versions WHERE game_id=(SELECT id FROM games WHERE slug=$1) AND status='published'`, slug))
	if err != nil {
		return version, err
	}
	return s.normalizeDefenseChecksum(ctx, version), nil
}

func (s *Server) loadDefenseVersion(ctx context.Context, id uuid.UUID) (defenseVersionRecord, error) {
	version, err := scanDefenseVersion(s.DB.QueryRow(ctx, `SELECT `+defenseVersionColumns+` FROM defense_content_versions WHERE id=$1`, id))
	if err != nil {
		return version, err
	}
	return s.normalizeDefenseChecksum(ctx, version), nil
}

func (s *Server) normalizeDefenseChecksum(ctx context.Context, version defenseVersionRecord) defenseVersionRecord {
	checksum := defenseChecksum(version.RawContent)
	if version.Checksum != checksum {
		storedChecksum := version.Checksum
		version.Checksum = checksum
		// A read may have captured an old content/checksum pair before a writer
		// committed. Only repair that exact pair so a delayed normalization can
		// never overwrite the checksum produced by a newer content mutation.
		_, _ = s.DB.Exec(ctx, `UPDATE defense_content_versions SET checksum=$2 WHERE id=$1 AND checksum=$3 AND content=$4`, version.ID, checksum, storedChecksum, version.RawContent)
	}
	return version
}

func defenseVersionJSON(version defenseVersionRecord) map[string]any {
	return map[string]any{
		"id": version.ID, "game_id": version.GameID, "version_no": version.VersionNo, "label": version.Label, "status": version.Status,
		"content_version": version.ContentVersion, "policy_version": version.PolicyVersion, "asset_version": version.AssetVersion,
		"checksum": version.Checksum, "notes": version.Notes, "source_version_id": version.SourceVersionID, "created_by": version.CreatedBy, "approved_by": version.ApprovedBy,
		"created_at": version.CreatedAt, "tested_at": version.TestedAt, "approval_requested_at": version.RequestedAt,
		"approved_at": version.ApprovedAt, "review_comment": version.ReviewComment, "reviewed_at": version.ReviewedAt,
		"published_at": version.PublishedAt, "updated_at": version.UpdatedAt,
	}
}

func validateDefenseVersionMetadata(version defenseVersionRecord) error {
	for name, value := range map[string]string{"label": version.Label, "content_version": version.ContentVersion, "policy_version": version.PolicyVersion, "asset_version": version.AssetVersion} {
		if strings.TrimSpace(value) == "" || len(value) > 100 || strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("%s is required and must be at most 100 safe characters", name)
		}
	}
	if len(version.Notes) > 2000 {
		return fmt.Errorf("notes must be at most 2000 characters")
	}
	return nil
}

func decodeDefenseContent(raw []byte) (defenseDecodedContent, error) {
	var content defenseDecodedContent
	if len(raw) > maxJSONBody {
		return content, fmt.Errorf("content pack exceeds 2 MiB")
	}
	if err := json.Unmarshal(raw, &content.Sections); err != nil || content.Sections == nil {
		return content, fmt.Errorf("content must be a JSON object")
	}
	decode := func(section string, target any) error {
		rawSection, ok := content.Sections[section]
		if !ok {
			return fmt.Errorf("missing %s section", section)
		}
		if err := json.Unmarshal(rawSection, target); err != nil {
			return fmt.Errorf("invalid %s section", section)
		}
		return nil
	}
	for section, target := range map[string]any{
		"stages": &content.Stages, "waves": &content.Waves, "towers": &content.Towers, "enemies": &content.Enemies,
		"bosses": &content.Bosses, "heroes": &content.Heroes, "skills": &content.Skills, "events": &content.Events,
		"education": &content.Education, "balance": &content.Balance, "campaigns": &content.Campaigns,
		"resource_rules": &content.ResourceRules, "model_profiles": &content.ModelProfiles,
	} {
		if err := decode(section, target); err != nil {
			return content, err
		}
	}
	return content, nil
}

func validateDefenseContent(slug string, raw []byte) error {
	content, err := decodeDefenseContent(raw)
	if err != nil {
		return err
	}
	minimum := map[string][7]int{
		"office-guardians": {8, 6, 10, 2, 3, 0, 0},
		"cyber-fortress":   {10, 8, 15, 3, 3, 30, 50},
		"ai-nexus-defense": {10, 10, 15, 4, 5, 30, 50},
	}[slug]
	counts := [7]int{len(content.Stages), len(content.Towers), len(content.Enemies), len(content.Bosses), len(content.Heroes), len(content.Events), len(content.Education)}
	for index, name := range []string{"stages", "towers", "enemies", "bosses", "heroes", "events", "education"} {
		if counts[index] < minimum[index] {
			return fmt.Errorf("%s requires at least %d items", name, minimum[index])
		}
	}
	if (len(content.Events) == 0) != (len(content.Education) == 0) {
		return fmt.Errorf("events and education must either both be empty or both be configured")
	}
	if len(content.Stages) > 100 || len(content.Waves) > 10000 || len(content.Towers) > 100 || len(content.Enemies)+len(content.Bosses) > 200 || len(content.Heroes) > 100 || len(content.Events) > 500 || len(content.Education) > 1000 {
		return fmt.Errorf("one or more content section limits were exceeded")
	}
	stageIDs := map[string]defenseStageDefinition{}
	stageNumbers := map[int]bool{}
	for _, stage := range content.Stages {
		if !validRealmGuardIdentifier(stage.ID) || strings.TrimSpace(stage.Name) == "" || len(stage.Name) > 120 || strings.TrimSpace(stage.Version) == "" || len(stage.Version) > 100 || stage.Number < 1 || stageNumbers[stage.Number] || stageIDs[stage.ID].ID != "" || stage.Mode != "campaign" || stage.StartingHealth < 1 || stage.StartingHealth > 1_000_000 || stage.StartingResource < 0 || stage.StartingResource > 1_000_000_000 || !slices.Contains([]string{"verdant", "ember", "frost", "void"}, stage.Theme) || !slices.Contains([]string{"", "time_surge", "ember_vents", "winter_blessing"}, stage.Gimmick) {
			return fmt.Errorf("invalid or duplicate stage %q", stage.ID)
		}
		paths := stage.Paths
		if len(paths) == 0 {
			paths = [][]defensePoint{stage.Path}
		}
		if len(paths) < 1 || len(paths) > 4 || len(stage.TowerSpots) < 4 || len(stage.TowerSpots) > 32 {
			return fmt.Errorf("stage %s requires one to four paths and four to 32 tower spots", stage.ID)
		}
		for _, path := range paths {
			if len(path) < 2 || len(path) > 64 {
				return fmt.Errorf("stage %s path length is invalid", stage.ID)
			}
			for _, point := range path {
				if math.IsNaN(point.X) || math.IsNaN(point.Y) || math.IsInf(point.X, 0) || math.IsInf(point.Y, 0) || point.X < -100 || point.X > 1380 || point.Y < -100 || point.Y > 820 {
					return fmt.Errorf("stage %s path coordinate is invalid", stage.ID)
				}
			}
		}
		spotIDs := map[string]bool{}
		for _, spot := range stage.TowerSpots {
			if !validRealmGuardIdentifier(spot.ID) || spotIDs[spot.ID] || math.IsNaN(spot.X) || math.IsNaN(spot.Y) || spot.X < 0 || spot.X > 1280 || spot.Y < 0 || spot.Y > 720 {
				return fmt.Errorf("stage %s tower spot is invalid", stage.ID)
			}
			spotIDs[spot.ID] = true
		}
		stageIDs[stage.ID], stageNumbers[stage.Number] = stage, true
	}
	for number := 1; number <= len(content.Stages); number++ {
		if !stageNumbers[number] {
			return fmt.Errorf("stage numbers must be contiguous from 1")
		}
	}
	unitIDs := map[string]bool{}
	validateUnits := func(section string, units []defenseUnitDefinition) error {
		for _, unit := range units {
			if !validRealmGuardIdentifier(unit.ID) || strings.TrimSpace(unit.Name) == "" || len(unit.Name) > 120 || unitIDs[unit.ID] {
				return fmt.Errorf("invalid or duplicate %s item %q", section, unit.ID)
			}
			unitIDs[unit.ID] = true
		}
		return nil
	}
	for section, units := range map[string][]defenseUnitDefinition{"towers": content.Towers, "enemies": content.Enemies, "bosses": content.Bosses, "heroes": content.Heroes} {
		if err := validateUnits(section, units); err != nil {
			return err
		}
	}
	if len(content.Skills) != 3 {
		return fmt.Errorf("skills requires exactly three active skills")
	}
	skillIDs, skillEffects := map[string]bool{}, map[string]bool{}
	for _, skill := range content.Skills {
		if !validRealmGuardIdentifier(skill.ID) || skillIDs[skill.ID] || strings.TrimSpace(skill.Name) == "" || len(skill.Name) > 120 || strings.TrimSpace(skill.Description) == "" || len(skill.Description) > 1000 || skill.Cooldown <= 0 || skill.Cooldown > 3600 || math.IsNaN(skill.Cooldown) || math.IsInf(skill.Cooldown, 0) || !slices.Contains([]string{"area_damage", "reinforcement", "freeze"}, skill.Effect) || skillEffects[skill.Effect] || (skill.Color != "" && (len(skill.Color) != 7 || skill.Color[0] != '#' || !validDefenseHexColor(skill.Color[1:]))) {
			return fmt.Errorf("invalid active skill %q", skill.ID)
		}
		skillIDs[skill.ID], skillEffects[skill.Effect] = true, true
	}
	if len(skillEffects) != 3 {
		return fmt.Errorf("skills must define area_damage, reinforcement, and freeze exactly once")
	}
	for _, tower := range content.Towers {
		if strings.TrimSpace(tower.Role) == "" || len(tower.Role) > 120 || tower.Color < 0 || tower.Color > 0xffffff || tower.Cost < 1 || tower.Cost > 1_000_000_000 || !validDefenseFiniteRange(tower.Damage, 0.01, 1_000_000_000) || !validDefenseFiniteRange(tower.Range, 1, 5000) || !validDefenseFiniteRange(tower.FireRate, 0.01, 3600) || !validDefenseFiniteRange(tower.ProjectileSpeed, 1, 100_000) || !slices.Contains([]string{"physical", "magic", "true"}, tower.DamageType) || len(tower.Branches) != 2 || len(tower.EffectiveAgainst) == 0 || !validDefenseFiniteRange(tower.EffectiveMultiplier, 1.000001, 10) {
			return fmt.Errorf("tower %s is missing required runtime statistics", tower.ID)
		}
		branches := map[string]bool{}
		for _, branch := range tower.Branches {
			validOptionalMultiplier := func(value *float64) bool { return value == nil || validDefenseFiniteRange(*value, 0.01, 10) }
			if !validRealmGuardIdentifier(branch.ID) || branches[branch.ID] || strings.TrimSpace(branch.Name) == "" || len(branch.Name) > 120 || strings.TrimSpace(branch.Description) == "" || len(branch.Description) > 1000 || !validOptionalMultiplier(branch.DamageMultiplier) || !validOptionalMultiplier(branch.RangeMultiplier) || !validOptionalMultiplier(branch.RateMultiplier) || !validDefenseFiniteRange(branch.Splash, 0, 2000) || !validDefenseFiniteRange(branch.Slow, 0, 1) || !validDefenseFiniteRange(branch.Pierce, 0, 1000) {
				return fmt.Errorf("tower %s has an invalid upgrade branch", tower.ID)
			}
			branches[branch.ID] = true
		}
	}
	for _, hero := range content.Heroes {
		if strings.TrimSpace(hero.Title) == "" || len(hero.Title) > 120 || hero.Color < 0 || hero.Color > 0xffffff || !validDefenseFiniteRange(hero.HP, 1, 1_000_000_000_000) || !validDefenseFiniteRange(hero.Damage, 0.01, 1_000_000_000) || !validDefenseFiniteRange(hero.Range, 1, 5000) || !validDefenseFiniteRange(hero.Speed, 0.01, 10_000) || !validDefenseFiniteRange(hero.RespawnSeconds, 0.01, 3600) || strings.TrimSpace(hero.Skill1) == "" || len(hero.Skill1) > 120 || strings.TrimSpace(hero.Skill2) == "" || len(hero.Skill2) > 120 || strings.TrimSpace(hero.Ultimate) == "" || len(hero.Ultimate) > 120 || hero.UnlockStage < 1 || hero.UnlockStage > len(content.Stages) {
			return fmt.Errorf("hero %s is missing required runtime statistics", hero.ID)
		}
	}
	enemyIDs := map[string]bool{}
	threatTypes := map[string]bool{}
	for _, enemy := range append(append([]defenseUnitDefinition{}, content.Enemies...), content.Bosses...) {
		if enemy.HP <= 0 || enemy.HP > 1e12 || enemy.Speed <= 0 || enemy.Speed > 10000 || enemy.Armor < 0 || enemy.Armor > 1 || enemy.Reward < 0 || enemy.Reward > 1_000_000_000 || enemy.HealthDamage < 1 || enemy.HealthDamage > 1_000_000 || enemy.Radius <= 0 || enemy.Radius > 200 || math.IsNaN(enemy.HP) || math.IsInf(enemy.HP, 0) || !validRealmGuardIdentifier(enemy.ThreatType) {
			return fmt.Errorf("invalid enemy statistics for %s", enemy.ID)
		}
		enemyIDs[enemy.ID] = true
		threatTypes[enemy.ThreatType] = true
		if slug == "ai-nexus-defense" {
			if len(enemy.ResourceEffect) == 0 {
				return fmt.Errorf("AI enemy %s requires a resource_effect", enemy.ID)
			}
			for metric, cost := range enemy.ResourceEffect {
				if !slices.Contains([]string{"compute", "token", "trust", "latency"}, metric) || cost < 0 || cost > 1_000_000 {
					return fmt.Errorf("AI enemy %s has an invalid resource_effect", enemy.ID)
				}
			}
		} else if len(enemy.ResourceEffect) != 0 {
			return fmt.Errorf("resource_effect is only supported by AI Nexus Defense")
		}
	}
	for _, tower := range content.Towers {
		seenTargets := map[string]bool{}
		for _, target := range tower.EffectiveAgainst {
			if !validRealmGuardIdentifier(target) || !threatTypes[target] || seenTargets[target] {
				return fmt.Errorf("tower %s references an invalid threat type", tower.ID)
			}
			seenTargets[target] = true
		}
	}
	wavesByStage := map[string]map[int]bool{}
	stageEnemyRefs := map[string]map[string]bool{}
	waveIDs := map[string]bool{}
	for _, wave := range content.Waves {
		if !validRealmGuardIdentifier(wave.ID) || waveIDs[wave.ID] || stageIDs[wave.StageID].ID == "" || wave.Number < 1 || len(wave.Entries) < 1 || len(wave.Entries) > 8 || wave.Reward < 0 || wave.Reward > 1_000_000_000 {
			return fmt.Errorf("invalid wave %q", wave.ID)
		}
		if wavesByStage[wave.StageID] == nil {
			wavesByStage[wave.StageID] = map[int]bool{}
		}
		if stageEnemyRefs[wave.StageID] == nil {
			stageEnemyRefs[wave.StageID] = map[string]bool{}
		}
		if wavesByStage[wave.StageID][wave.Number] {
			return fmt.Errorf("duplicate wave number for %s", wave.StageID)
		}
		wavesByStage[wave.StageID][wave.Number], waveIDs[wave.ID] = true, true
		var waveCount int64
		for _, entry := range wave.Entries {
			if !enemyIDs[entry.Enemy] || entry.Count < 1 || entry.Count > 500 || entry.Interval < 0.05 || entry.Interval > 3600 || math.IsNaN(entry.Interval) || math.IsInf(entry.Interval, 0) {
				return fmt.Errorf("invalid wave entry in %s", wave.ID)
			}
			stageEnemyRefs[wave.StageID][entry.Enemy] = true
			waveCount += entry.Count
		}
		if waveCount > 2000 {
			return fmt.Errorf("wave %s exceeds the spawn limit", wave.ID)
		}
	}
	for stageID := range stageIDs {
		if len(wavesByStage[stageID]) == 0 || len(wavesByStage[stageID]) > defenseWaveTelemetryLimit {
			return fmt.Errorf("stage %s requires between one and %d waves", stageID, defenseWaveTelemetryLimit)
		}
		for number := 1; number <= len(wavesByStage[stageID]); number++ {
			if !wavesByStage[stageID][number] {
				return fmt.Errorf("stage %s wave numbers must be contiguous from 1", stageID)
			}
		}
		maximumCounter, maximumWave, victory := int64(math.MaxInt32), defenseWaveTelemetryLimit, false
		histogram := make(map[string]int64, len(stageEnemyRefs[stageID]))
		for enemyID := range stageEnemyRefs[stageID] {
			histogram[enemyID] = maximumCounter
		}
		resourceState := map[string]defenseResourceMetric{}
		if slug == "ai-nexus-defense" {
			for _, metric := range []string{"compute", "token", "trust", "latency"} {
				resourceState[metric] = defenseResourceMetric{Start: 1_000_000, Spent: 1_000_000, Remaining: 0}
			}
		}
		sample, _ := json.Marshal(defenseSnapshotTelemetry{StageID: stageID, Wave: maximumWave, Difficulty: "veteran", DurationMS: &maximumCounter, Health: &maximumCounter, Resource: &maximumCounter, EarnedResource: &maximumCounter, SpentResource: &maximumCounter, SoldResource: &maximumCounter, Kills: &maximumCounter, Escaped: &maximumCounter, Spawned: &maximumCounter, WavesCompleted: &maximumWave, Victory: &victory, HeroID: strings.Repeat("h", 32), HeroLevel: &maximumWave, ContentVersion: strings.Repeat("v", 100), PolicyVersion: strings.Repeat("p", 100), DefeatedByEnemy: histogram, EscapedByEnemy: histogram, SpawnedByEnemy: histogram, ResourceState: resourceState})
		// The shared runtime also includes local score plus balance/stage/asset
		// version fields that are ignored by the authoritative decoder. Reserve
		// their bounded transport footprint in addition to the canonical snapshot.
		if len(sample)+512 > 4<<10 {
			return fmt.Errorf("stage %s cumulative telemetry snapshot exceeds the 4 KiB transport limit", stageID)
		}
	}
	questions := map[string]defenseEducationDefinition{}
	for _, question := range content.Education {
		if !validRealmGuardIdentifier(question.ID) || strings.TrimSpace(question.Topic) == "" || len(question.Topic) > 80 || strings.TrimSpace(question.Question) == "" || len(question.Question) > 2000 || len(question.Answers) < 2 || len(question.Answers) > 6 || question.Score < 0 || question.Score > 100 || questions[question.ID].ID != "" {
			return fmt.Errorf("invalid education question %q", question.ID)
		}
		answerIDs, correct := map[string]bool{}, false
		for _, answer := range question.Answers {
			if !validDefenseAnswerIdentifier(answer.ID) || strings.TrimSpace(answer.Text) == "" || len(answer.Text) > 1000 || answerIDs[answer.ID] {
				return fmt.Errorf("invalid answer in question %s", question.ID)
			}
			answerIDs[answer.ID] = true
			correct = correct || answer.ID == question.CorrectAnswerID
		}
		if !correct {
			return fmt.Errorf("question %s has no valid correct answer", question.ID)
		}
		questions[question.ID] = question
	}
	eventIDs, eventTriggers := map[string]bool{}, map[string]bool{}
	for _, event := range content.Events {
		if !validRealmGuardIdentifier(event.ID) || eventIDs[event.ID] || stageIDs[event.StageID].ID == "" || strings.TrimSpace(event.Trigger) == "" || questions[event.EducationID].ID == "" {
			return fmt.Errorf("invalid education event %q", event.ID)
		}
		trigger := strings.ReplaceAll(event.Trigger, "_", "-")
		triggerKey := event.StageID + "\x00" + trigger
		if eventTriggers[triggerKey] {
			return fmt.Errorf("stage %s has multiple education events for %s", event.StageID, trigger)
		}
		if trigger != "battle-start" {
			wave, parseErr := strconv.Atoi(strings.TrimPrefix(trigger, "wave-"))
			if !strings.HasPrefix(trigger, "wave-") || parseErr != nil || !wavesByStage[event.StageID][wave] {
				return fmt.Errorf("education event %s has an invalid trigger", event.ID)
			}
		}
		for _, rawEffect := range []json.RawMessage{event.Reward, event.Penalty} {
			var effect map[string]int64
			if len(rawEffect) == 0 || json.Unmarshal(rawEffect, &effect) != nil {
				return fmt.Errorf("education event %s has an invalid effect", event.ID)
			}
			for key, value := range effect {
				if !slices.Contains([]string{"resource", "trust", "latency_headroom"}, key) || value < 0 || value > 1_000_000 {
					return fmt.Errorf("education event %s has an unsupported effect", event.ID)
				}
				if slug != "ai-nexus-defense" && key != "resource" {
					return fmt.Errorf("education event %s uses an AI-only effect", event.ID)
				}
			}
			if _, ok := effect["resource"]; !ok {
				return fmt.Errorf("education event %s requires a resource effect", event.ID)
			}
		}
		eventIDs[event.ID], eventTriggers[triggerKey] = true, true
	}
	if len(content.Campaigns) < 1 || len(content.Campaigns) > 50 {
		return fmt.Errorf("content requires between one and 50 campaigns")
	}
	campaignIDs := map[string]bool{}
	for _, campaign := range content.Campaigns {
		if !validRealmGuardIdentifier(campaign.ID) || campaignIDs[campaign.ID] || strings.TrimSpace(campaign.Name) == "" || len(campaign.Name) > 120 || len(campaign.StageIDs) < 1 || len(campaign.StageIDs) > len(content.Stages) || campaign.RequiredLearningScore < 0 || campaign.RequiredLearningScore > 100 {
			return fmt.Errorf("invalid campaign %q", campaign.ID)
		}
		seen := map[string]bool{}
		for _, stageID := range campaign.StageIDs {
			if stageIDs[stageID].ID == "" || seen[stageID] {
				return fmt.Errorf("campaign %s references an invalid stage", campaign.ID)
			}
			seen[stageID] = true
		}
		campaignIDs[campaign.ID] = true
	}
	if content.Balance.HealthScoreFactor < 0 || content.Balance.HealthScoreFactor > 1_000_000_000 || content.Balance.ResourceScoreFactor < 0 || content.Balance.ResourceScoreFactor > 1_000_000_000 || content.Balance.WaveScoreFactor < 0 || content.Balance.WaveScoreFactor > 1_000_000_000 || content.Balance.ClearTimeTargetMS < 1 || content.Balance.ClearTimeTargetMS > 86_400_000 || content.Balance.ClearTimeBonusDivisor < 1 || content.Balance.ClearTimeBonusDivisor > 1_000_000_000 || content.Balance.MinWaveDurationMS < 100 || content.Balance.MinWaveDurationMS > 3_600_000 || content.Balance.DurationToleranceMS < 0 || content.Balance.DurationToleranceMS > 60_000 || len(content.Balance.TowerUpgradeCost) < 3 || len(content.Balance.TowerUpgradeCost) > 10 || content.Balance.SellRefundRate < 0 || content.Balance.SellRefundRate > 1 || math.IsNaN(content.Balance.SellRefundRate) || math.IsInf(content.Balance.SellRefundRate, 0) {
		return fmt.Errorf("invalid balance limits")
	}
	for index, cost := range content.Balance.TowerUpgradeCost {
		if cost < 0 || cost > 1_000_000_000 || (index > 0 && cost == 0) {
			return fmt.Errorf("tower upgrade costs must be positive and bounded")
		}
	}
	for _, difficulty := range []string{"casual", "normal", "veteran"} {
		value, ok := content.Balance.Difficulties[difficulty]
		if !ok {
			return fmt.Errorf("balance requires %s difficulty", difficulty)
		}
		if value.DifficultyBonus < 0 || value.DifficultyBonus > 1_000_000_000_000 || value.EnemyHP <= 0 || value.EnemyHP > 100 || value.EnemySpeed <= 0 || value.EnemySpeed > 100 || value.Gold <= 0 || value.Gold > 100 || value.Score <= 0 || value.Score > 100 || math.IsNaN(value.EnemyHP) || math.IsInf(value.EnemyHP, 0) || math.IsNaN(value.EnemySpeed) || math.IsInf(value.EnemySpeed, 0) || math.IsNaN(value.Gold) || math.IsInf(value.Gold, 0) || math.IsNaN(value.Score) || math.IsInf(value.Score, 0) {
			return fmt.Errorf("balance has invalid %s difficulty factors", difficulty)
		}
	}
	if slug == "ai-nexus-defense" {
		rules := content.ResourceRules
		if rules.ComputeStart < 1 || rules.ComputeStart > 1_000_000 || rules.TokenStart < 1 || rules.TokenStart > 1_000_000 || rules.TrustStart < 1 || rules.TrustStart > 1_000_000 || rules.LatencyMax < 1 || rules.LatencyMax > 1_000_000 || rules.WaveComputeCost < 0 || rules.WaveComputeCost >= rules.ComputeStart || rules.WaveTokenCost < 0 || rules.WaveTokenCost >= rules.TokenStart || rules.EscapedTrustCost < 0 || rules.EscapedTrustCost > rules.TrustStart || rules.EscapedLatencyCost < 0 || rules.EscapedLatencyCost > rules.LatencyMax || len(content.ModelProfiles) != 5 {
			return fmt.Errorf("AI Nexus Defense requires pinned resource rules and five model profiles")
		}
		expectedLimits := map[string]int64{"compute": rules.ComputeStart, "token": rules.TokenStart, "trust": rules.TrustStart, "latency": rules.LatencyMax}
		if len(content.Balance.ResourceStateLimits) != len(expectedLimits) {
			return fmt.Errorf("AI resource_state_limits must exactly match resource_rules")
		}
		for metric, expected := range expectedLimits {
			if content.Balance.ResourceStateLimits[metric] != expected {
				return fmt.Errorf("AI resource_state_limits must exactly match resource_rules")
			}
		}
		profileIDs := map[string]bool{}
		towerIDs := map[string]bool{}
		for _, tower := range content.Towers {
			towerIDs[tower.ID] = true
		}
		for _, profile := range content.ModelProfiles {
			if !validDefenseModelProfileRuntime(profile, rules) || profileIDs[profile.ID] || !towerIDs[profile.TowerID] {
				return fmt.Errorf("invalid AI model profile %q", profile.ID)
			}
			profileIDs[profile.ID] = true
		}
		if !validDefenseAIResourceScoreFactors(content.Balance.AIResourceScoreFactors) {
			return fmt.Errorf("AI Nexus Defense requires exactly four bounded resource score factors")
		}
	} else if len(content.ModelProfiles) != 0 || len(content.Balance.ResourceStateLimits) != 0 || len(content.Balance.AIResourceScoreFactors) != 0 || content.ResourceRules != (defenseResourceRules{}) {
		return fmt.Errorf("AI resource settings are only supported by AI Nexus Defense")
	}
	return nil
}

func validDefenseModelProfileRuntime(profile defenseModelProfile, rules defenseResourceRules) bool {
	return slices.Contains([]string{"small", "medium", "large", "reasoning", "vision"}, profile.ID) &&
		strings.TrimSpace(profile.Name) != "" && len(profile.Name) <= 120 &&
		profile.ComputeCost > 0 && profile.ComputeCost <= rules.ComputeStart &&
		profile.TokenCost > 0 && profile.TokenCost <= rules.TokenStart &&
		profile.LatencyCost >= 0 && profile.LatencyCost <= rules.LatencyMax &&
		profile.Accuracy >= 1 && profile.Accuracy <= 100 &&
		validDefenseFiniteRange(profile.DamageMultiplier, 0.01, 10)
}

func validDefenseAIResourceScoreFactors(factors map[string]int64) bool {
	if len(factors) != 4 {
		return false
	}
	for _, metric := range []string{"compute", "token", "trust", "latency"} {
		factor, ok := factors[metric]
		if !ok || factor < 0 || factor > 1_000_000 {
			return false
		}
	}
	return true
}

func defenseContentKeyIsSensitive(key string) bool {
	normalized := strings.Map(func(character rune) rune {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return unicode.ToLower(character)
		}
		return -1
	}, key)
	return normalized == "correct" || normalized == "iscorrect" ||
		strings.HasPrefix(normalized, "correctanswer") ||
		strings.HasPrefix(normalized, "expectedanswer") ||
		strings.HasPrefix(normalized, "rightanswer") ||
		strings.HasPrefix(normalized, "answerkey") ||
		strings.HasPrefix(normalized, "explanation") ||
		strings.HasPrefix(normalized, "rationale") ||
		strings.HasPrefix(normalized, "solution") ||
		strings.HasPrefix(normalized, "policyreference")
}

func sanitizeDefenseContent(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	var redact func(any)
	redact = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if defenseContentKeyIsSensitive(key) {
					delete(typed, key)
					continue
				}
				redact(child)
			}
		case []any:
			for _, child := range typed {
				redact(child)
			}
		}
	}
	redact(value)
	root, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("content must be an object")
	}
	document := make(map[string]json.RawMessage, len(root))
	for key, child := range root {
		document[key], _ = json.Marshal(child)
	}
	return document, nil
}

func (s *Server) defenseConfig(w http.ResponseWriter, r *http.Request) {
	slug, ok := defenseSlugParam(w, r)
	if !ok {
		return
	}
	gameID, name, err := s.defenseGame(r.Context(), slug)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	version, err := s.loadDefensePublished(r.Context(), slug)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	decoded, err := decodeDefenseContent(version.RawContent)
	if err != nil {
		writeError(w, 500, "invalid_published_content", "published Defense Series content is invalid")
		return
	}
	content, err := sanitizeDefenseContent(version.RawContent)
	if err != nil {
		writeError(w, 500, "invalid_published_content", "published Defense Series content is invalid")
		return
	}
	w.Header().Set("ETag", `"`+version.Checksum+`"`)
	writeJSON(w, 200, map[string]any{"game": map[string]any{"id": gameID, "slug": slug, "name": name, "education_enabled": defenseEducationEnabled(decoded)}, "version": defenseVersionJSON(version), "content": content})
}

func (s *Server) defenseVersion(w http.ResponseWriter, r *http.Request) {
	slug, ok := defenseSlugParam(w, r)
	if !ok {
		return
	}
	version, err := s.loadDefensePublished(r.Context(), slug)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"version": defenseVersionJSON(version)})
}

func (s *Server) ensureDefenseProgress(ctx context.Context, userID, gameID uuid.UUID, version defenseVersionRecord, content defenseDecodedContent) error {
	return seedDefenseProgress(ctx, s.DB, userID, gameID, version.ID, content)
}

// seedDefenseProgress creates the stage/difficulty rows a player is missing. It
// runs on every progress read and again inside the result transaction, so the
// rows are expanded in one statement rather than three inserts per stage.
func seedDefenseProgress(ctx context.Context, db execer, userID, gameID, versionID uuid.UUID, content defenseDecodedContent) error {
	stageIDs := make([]string, 0, len(content.Stages))
	unlocked := make([]bool, 0, len(content.Stages))
	for _, stage := range content.Stages {
		stageIDs = append(stageIDs, stage.ID)
		unlocked = append(unlocked, stage.Number == 1)
	}
	_, err := db.Exec(ctx, `INSERT INTO defense_user_progress(user_id,game_id,stage_id,difficulty,unlocked,content_version_id)
		SELECT $1,$2,stage.id,d.difficulty,stage.unlocked,$3
		FROM unnest($4::text[],$5::boolean[]) AS stage(id,unlocked)
		CROSS JOIN unnest(ARRAY['casual','normal','veteran']) AS d(difficulty)
		ON CONFLICT(user_id,game_id,content_version_id,stage_id,difficulty) DO NOTHING`,
		userID, gameID, versionID, stageIDs, unlocked)
	return err
}

func (s *Server) defenseProgressData(ctx context.Context, userID, gameID uuid.UUID, version defenseVersionRecord, content defenseDecodedContent) (map[string]any, error) {
	if err := s.ensureDefenseProgress(ctx, userID, gameID, version, content); err != nil {
		return nil, err
	}
	stageIDs := make([]string, 0, len(content.Stages))
	for _, stage := range content.Stages {
		stageIDs = append(stageIDs, stage.ID)
	}
	rows, err := s.DB.Query(ctx, `SELECT stage_id,difficulty,unlocked,completed,stars,best_score,best_learning_score,attempts,completions,total_playtime_ms,updated_at FROM defense_user_progress WHERE user_id=$1 AND game_id=$2 AND content_version_id=$3 AND stage_id=ANY($4) ORDER BY stage_id,difficulty`, userID, gameID, version.ID, stageIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	completedStages := map[string]bool{}
	var totalStars int
	var totalPlaytime int64
	for rows.Next() {
		var stageID, difficulty string
		var unlocked, completed bool
		var stars, learning, attempts, completions int
		var score, playtime int64
		var updated time.Time
		if err := rows.Scan(&stageID, &difficulty, &unlocked, &completed, &stars, &score, &learning, &attempts, &completions, &playtime, &updated); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"stage_id": stageID, "difficulty": difficulty, "unlocked": unlocked, "completed": completed, "stars": stars, "best_score": score, "best_learning_score": learning, "attempts": attempts, "completions": completions, "total_playtime_ms": playtime, "updated_at": updated})
		if completed {
			completedStages[stageID] = true
		}
		totalStars += stars
		totalPlaytime += playtime
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"version": defenseVersionJSON(version), "items": items, "summary": map[string]any{"completed_stages": len(completedStages), "total_stars": totalStars, "total_playtime_ms": totalPlaytime, "campaign_complete": len(completedStages) == len(content.Stages)}}, nil
}

func (s *Server) defenseProgress(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	slug, ok := defenseSlugParam(w, r)
	if !ok {
		return
	}
	gameID, _, err := s.defenseGame(r.Context(), slug)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	version, err := s.loadDefensePublished(r.Context(), slug)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	content, err := decodeDefenseContent(version.RawContent)
	if err != nil {
		writeError(w, 500, "invalid_published_content", err.Error())
		return
	}
	data, err := s.defenseProgressData(r.Context(), p.UserID, gameID, version, content)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	writeJSON(w, 200, data)
}

type defenseAnswerInput struct {
	GameID       string    `json:"game_id,omitempty"`
	SessionID    uuid.UUID `json:"session_id"`
	SessionToken string    `json:"session_token"`
	AnswerID     string    `json:"answer_id"`
}

type defenseSubmittedAnswer struct {
	EventID  string `json:"event_id"`
	AnswerID string `json:"answer_id"`
}

func defenseQuestionForEvent(content defenseDecodedContent, eventID string) (defenseEventDefinition, defenseEducationDefinition, bool) {
	var event defenseEventDefinition
	for _, candidate := range content.Events {
		if candidate.ID == eventID {
			event = candidate
			break
		}
	}
	if event.ID == "" {
		return event, defenseEducationDefinition{}, false
	}
	for _, question := range content.Education {
		if question.ID == event.EducationID {
			return event, question, true
		}
	}
	return event, defenseEducationDefinition{}, false
}

func defenseGameClaimMatches(claimed, slug string, gameID uuid.UUID) bool {
	return claimed == "" || claimed == slug || claimed == gameID.String()
}

func defenseAnswerEffect(event defenseEventDefinition, correct bool) map[string]int64 {
	raw := event.Penalty
	if correct {
		raw = event.Reward
	}
	var configured map[string]int64
	_ = json.Unmarshal(raw, &configured)
	effect := map[string]int64{"resource_delta": 0, "trust_delta": 0, "latency_headroom_delta": 0}
	if correct {
		effect["resource_delta"] = max(int64(0), configured["resource"])
	} else {
		effect["resource_delta"] = -max(int64(0), configured["resource"])
	}
	if configured["trust"] != 0 {
		effect["trust_delta"] = configured["trust"]
		if !correct {
			effect["trust_delta"] = -absInt64(configured["trust"])
		}
	}
	if configured["latency_headroom"] != 0 {
		effect["latency_headroom_delta"] = configured["latency_headroom"]
		if !correct {
			effect["latency_headroom_delta"] = -absInt64(configured["latency_headroom"])
		}
	}
	return effect
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func recordDefenseAnswer(ctx context.Context, tx pgx.Tx, userID, gameID, sessionID, versionID uuid.UUID, policyVersion string, content defenseDecodedContent, eventID, answerID string) (map[string]any, bool, error) {
	event, question, ok := defenseQuestionForEvent(content, eventID)
	if !ok {
		return nil, false, fmt.Errorf("unknown education event")
	}
	answerExists := false
	for _, answer := range question.Answers {
		answerExists = answerExists || answer.ID == answerID
	}
	if !answerExists {
		return nil, false, fmt.Errorf("unknown answer")
	}
	hash := defenseChecksum([]byte(event.ID + "\x00" + answerID))
	var storedAnswer, storedHash, topic string
	var correct bool
	var score int
	err := tx.QueryRow(ctx, `SELECT answer_id,request_hash,topic,correct,score FROM defense_event_answers WHERE session_id=$1 AND event_id=$2 FOR UPDATE`, sessionID, eventID).Scan(&storedAnswer, &storedHash, &topic, &correct, &score)
	if err == nil {
		if storedHash != hash || storedAnswer != answerID {
			return nil, false, errors.New("answer_conflict")
		}
		return map[string]any{"event_id": eventID, "question_id": question.ID, "answer_id": storedAnswer, "correct": correct, "topic": topic, "score": score, "explanation": question.Explanation, "policy_reference": question.PolicyReference, "effect": defenseAnswerEffect(event, correct)}, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, err
	}
	correct = answerID == question.CorrectAnswerID
	if correct {
		score = question.Score
	}
	_, err = tx.Exec(ctx, `INSERT INTO defense_event_answers(session_id,user_id,game_id,content_version_id,event_id,question_id,answer_id,topic,correct,score,policy_version,request_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, sessionID, userID, gameID, versionID, eventID, question.ID, answerID, question.Topic, correct, score, policyVersion, hash)
	if err != nil {
		return nil, false, err
	}
	return map[string]any{"event_id": eventID, "question_id": question.ID, "answer_id": answerID, "correct": correct, "topic": question.Topic, "score": score, "explanation": question.Explanation, "policy_reference": question.PolicyReference, "effect": defenseAnswerEffect(event, correct)}, false, nil
}

func (s *Server) answerDefenseEducationEvent(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	slug, ok := defenseSlugParam(w, r)
	if !ok {
		return
	}
	eventID := strings.TrimSpace(chi.URLParam(r, "eventID"))
	var in defenseAnswerInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.SessionID == uuid.Nil || in.SessionToken == "" || !validRealmGuardIdentifier(eventID) || !validDefenseAnswerIdentifier(in.AnswerID) {
		writeError(w, 400, "invalid_answer", "session, event, and answer identifiers are required")
		return
	}
	hash := sha256.Sum256([]byte(in.SessionToken))
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var gameID, versionID uuid.UUID
	var status string
	var raw json.RawMessage
	var policyVersion string
	err = tx.QueryRow(r.Context(), `SELECT gs.game_id,gs.defense_content_version_id,gs.status,v.policy_version,v.content FROM game_sessions gs JOIN games g ON g.id=gs.game_id JOIN defense_content_versions v ON v.id=gs.defense_content_version_id WHERE gs.id=$1 AND gs.user_id=$2 AND gs.session_token_hash=$3 AND g.slug=$4 FOR UPDATE OF gs`, in.SessionID, p.UserID, hash[:], slug).Scan(&gameID, &versionID, &status, &policyVersion, &raw)
	if err != nil || status != "active" {
		writeError(w, 409, "invalid_session", "an active pinned Defense Series session is required")
		return
	}
	if !defenseGameClaimMatches(in.GameID, slug, gameID) {
		writeError(w, 409, "game_mismatch", "game_id does not identify the Defense Series game pinned to this session")
		return
	}
	content, err := decodeDefenseContent(raw)
	if err != nil {
		writeError(w, 500, "invalid_pinned_content", err.Error())
		return
	}
	if !defenseEducationEnabled(content) {
		writeError(w, 409, "education_not_enabled", "this Defense Series content version has no education events")
		return
	}
	records, err := loadDefenseTelemetryRecords(r.Context(), tx, in.SessionID, false)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	event, _, exists := defenseQuestionForEvent(content, eventID)
	if !exists || !defenseEventReached(records, content, event) {
		writeError(w, 409, "event_not_triggered", "the education event has not been reached in the server-received battle ledger")
		return
	}
	if slug == "ai-nexus-defense" && defenseEventTriggeredByDepletedAIStart(records, event) {
		writeError(w, 409, "battle_terminal", "education answers are not accepted after the triggering AI resource was depleted")
		return
	}
	answer, duplicate, err := recordDefenseAnswer(r.Context(), tx, p.UserID, gameID, in.SessionID, versionID, policyVersion, content, eventID, in.AnswerID)
	if err != nil {
		if err.Error() == "answer_conflict" {
			writeError(w, 409, "answer_conflict", "the event was already answered differently")
		} else {
			writeError(w, 422, "invalid_answer", err.Error())
		}
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		s.dbError(w, r, err)
		return
	}
	s.audit(r, "defense.education.answer", "defense_session", in.SessionID.String(), map[string]any{"game": slug, "event_id": eventID, "correct": answer["correct"]})
	writeJSON(w, 200, map[string]any{"answer": answer, "duplicate": duplicate})
}

type defenseResultInput struct {
	GameID              string                           `json:"game_id,omitempty"`
	SessionID           uuid.UUID                        `json:"session_id"`
	SessionToken        string                           `json:"session_token"`
	StageID             string                           `json:"stage_id"`
	Difficulty          string                           `json:"difficulty"`
	DurationMS          int64                            `json:"duration_ms"`
	RemainingHealth     int64                            `json:"remaining_health"`
	RemainingResource   int64                            `json:"remaining_resource"`
	Kills               int64                            `json:"kills"`
	Escaped             int64                            `json:"escaped"`
	Spawned             int64                            `json:"spawned"`
	WavesCompleted      int                              `json:"waves_completed"`
	Victory             bool                             `json:"victory"`
	ContentVersion      string                           `json:"content_version"`
	PolicyVersion       string                           `json:"policy_version"`
	Answers             []defenseSubmittedAnswer         `json:"answers,omitempty"`
	Battle              defenseBattleInput               `json:"battle"`
	ResourceState       map[string]defenseResourceMetric `json:"resource_state,omitempty"`
	AIResources         map[string]int64                 `json:"ai_resources,omitempty"`
	DefeatedByEnemy     map[string]int64                 `json:"defeated_by_enemy"`
	EscapedByEnemy      map[string]int64                 `json:"escaped_by_enemy"`
	SpawnedByEnemy      map[string]int64                 `json:"spawned_by_enemy"`
	ClientScore         *int64                           `json:"score,omitempty"`
	ClientStars         *int                             `json:"stars,omitempty"`
	ClientLearningScore *int                             `json:"learning_score,omitempty"`
}

func defenseResultRequestChecksum(in defenseResultInput) string {
	in.GameID = ""
	in.ClientScore, in.ClientStars, in.ClientLearningScore, in.AIResources = nil, nil, nil, nil
	canonical, _ := json.Marshal(in)
	return defenseChecksum(canonical)
}

type defenseBattleInput struct {
	EarnedResource    int64  `json:"earned_resource"`
	SpentResource     int64  `json:"spent_resource"`
	RecoveredResource int64  `json:"recovered_resource"`
	HeroID            string `json:"hero_id"`
	HeroLevel         int    `json:"hero_level"`
}

func defenseSaturatingProduct(left, right int64) int64 {
	if left <= 0 || right <= 0 {
		return 0
	}
	if left > math.MaxInt64/right {
		return math.MaxInt64
	}
	return left * right
}

func defenseSaturatingTotal(values ...int64) int64 {
	var total int64
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if total > math.MaxInt64-value {
			return math.MaxInt64
		}
		total += value
	}
	return total
}

func defenseSaturatingScale(value int64, factor float64) int64 {
	if value <= 0 || factor <= 0 {
		return 0
	}
	if float64(value) > float64(math.MaxInt64)/factor {
		return math.MaxInt64
	}
	return int64(math.Round(float64(value) * factor))
}

func defenseBattleBudget(content defenseDecodedContent, stageID string, wavesCompleted int, includePartial bool) (waves, spawns, rewards, maxEnemyReward int64) {
	stageWaves := []defenseWaveDefinition{}
	for _, wave := range content.Waves {
		if wave.StageID == stageID {
			stageWaves = append(stageWaves, wave)
		}
	}
	slices.SortFunc(stageWaves, func(a, b defenseWaveDefinition) int { return a.Number - b.Number })
	waves = int64(len(stageWaves))
	limit := wavesCompleted
	if includePartial && limit < len(stageWaves) {
		limit++
	}
	limit = min(limit, len(stageWaves))
	for index := 0; index < limit; index++ {
		for _, entry := range stageWaves[index].Entries {
			spawns += entry.Count
		}
		if index < wavesCompleted {
			rewards += stageWaves[index].Reward
		}
	}
	for _, enemy := range append(append([]defenseUnitDefinition{}, content.Enemies...), content.Bosses...) {
		maxEnemyReward = max(maxEnemyReward, enemy.Reward)
	}
	return
}

func defenseStartingResource(stage defenseStageDefinition, balance defenseBalanceDefinition, difficulty string) int64 {
	return int64(math.Round(float64(stage.StartingResource) * balance.Difficulties[difficulty].Gold))
}

func defenseScore(stage defenseStageDefinition, balance defenseBalanceDefinition, in defenseResultInput) (int64, int, map[string]any) {
	health := defenseSaturatingProduct(in.RemainingHealth, balance.HealthScoreFactor)
	resource := defenseSaturatingProduct(in.RemainingResource, balance.ResourceScoreFactor)
	waves := defenseSaturatingProduct(int64(in.WavesCompleted), balance.WaveScoreFactor)
	timeBonus := int64(0)
	if in.Victory && balance.ClearTimeBonusDivisor > 0 && in.DurationMS < balance.ClearTimeTargetMS {
		timeBonus = (balance.ClearTimeTargetMS - in.DurationMS) / balance.ClearTimeBonusDivisor
	}
	difficulty := balance.Difficulties[in.Difficulty].DifficultyBonus
	aiResourceBonus := int64(0)
	for name, metric := range in.ResourceState {
		factor := balance.AIResourceScoreFactors[name]
		aiResourceBonus = defenseSaturatingTotal(aiResourceBonus, defenseSaturatingProduct(metric.Remaining, factor))
	}
	baseTotal := defenseSaturatingTotal(health, resource, waves, timeBonus, difficulty, aiResourceBonus)
	scoreMultiplier := balance.Difficulties[in.Difficulty].Score
	total := defenseSaturatingScale(baseTotal, scoreMultiplier)
	stars := 0
	if in.Victory {
		ratio := float64(in.RemainingHealth) / float64(stage.StartingHealth)
		switch {
		case ratio >= .9:
			stars = 3
		case ratio >= .5:
			stars = 2
		default:
			stars = 1
		}
	}
	return total, stars, map[string]any{"health_bonus": health, "resource_bonus": resource, "ai_resource_bonus": aiResourceBonus, "wave_bonus": waves, "clear_time_bonus": timeBonus, "difficulty_bonus": difficulty, "base_total": baseTotal, "difficulty_score_multiplier": scoreMultiplier, "total": total}
}

func defenseLearningBreakdown(ctx context.Context, tx pgx.Tx, sessionID uuid.UUID) (int, map[string]any, []map[string]any, error) {
	rows, err := tx.Query(ctx, `SELECT event_id,question_id,answer_id,topic,correct,score FROM defense_event_answers WHERE session_id=$1 ORDER BY answered_at,event_id`, sessionID)
	if err != nil {
		return 0, nil, nil, err
	}
	defer rows.Close()
	type topicStats struct{ Correct, Total, Points int }
	topics := map[string]*topicStats{}
	answers := []map[string]any{}
	correctTotal := 0
	for rows.Next() {
		var eventID, questionID, answerID, topic string
		var correct bool
		var score int
		if err := rows.Scan(&eventID, &questionID, &answerID, &topic, &correct, &score); err != nil {
			return 0, nil, nil, err
		}
		stats := topics[topic]
		if stats == nil {
			stats = &topicStats{}
			topics[topic] = stats
		}
		stats.Total++
		stats.Points += score
		if correct {
			stats.Correct++
			correctTotal++
		}
		answers = append(answers, map[string]any{"event_id": eventID, "question_id": questionID, "answer_id": answerID, "topic": topic, "correct": correct, "score": score})
	}
	if err := rows.Err(); err != nil {
		return 0, nil, nil, err
	}
	overall := 0
	if len(answers) > 0 {
		overall = int(math.Round(float64(correctTotal) * 100 / float64(len(answers))))
	}
	breakdown := map[string]any{}
	for topic, stats := range topics {
		breakdown[topic] = map[string]any{"correct": stats.Correct, "total": stats.Total, "score": int(math.Round(float64(stats.Correct) * 100 / float64(stats.Total)))}
	}
	return overall, breakdown, answers, nil
}

func defenseStoredAnswerEffects(ctx context.Context, tx pgx.Tx, sessionID uuid.UUID, content defenseDecodedContent) (map[string]map[string]int64, int64, int64, error) {
	rows, err := tx.Query(ctx, `SELECT event_id,correct FROM defense_event_answers WHERE session_id=$1 ORDER BY answered_at,event_id`, sessionID)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()
	effects := map[string]map[string]int64{}
	var earned, spent int64
	for rows.Next() {
		var eventID string
		var correct bool
		if err := rows.Scan(&eventID, &correct); err != nil {
			return nil, 0, 0, err
		}
		event, _, ok := defenseQuestionForEvent(content, eventID)
		if !ok {
			return nil, 0, 0, fmt.Errorf("stored answer references an unknown pinned event")
		}
		effect := defenseAnswerEffect(event, correct)
		effects[eventID] = effect
		delta := effect["resource_delta"]
		if delta >= 0 {
			earned = defenseSaturatingTotal(earned, delta)
		} else {
			spent = defenseSaturatingTotal(spent, -delta)
		}
	}
	return effects, earned, spent, rows.Err()
}

func defenseCurrentCampaignComplete(ctx context.Context, tx pgx.Tx, userID uuid.UUID, slug string) (bool, error) {
	var complete bool
	err := tx.QueryRow(ctx, `SELECT COALESCE(bool_and(EXISTS(
		SELECT 1 FROM defense_campaign_progress p
		WHERE p.user_id=$1 AND p.game_id=v.game_id AND p.content_version_id=v.id AND p.campaign_id=campaign->>'id' AND p.completed
	)),false)
		FROM defense_content_versions v
		CROSS JOIN LATERAL jsonb_array_elements(v.content->'campaigns') campaign
		JOIN games g ON g.id=v.game_id
		WHERE g.slug=$2 AND v.status='published'`, userID, slug).Scan(&complete)
	return complete, err
}

func unlockDefenseAchievements(ctx context.Context, tx pgx.Tx, userID uuid.UUID, slug string) error {
	codes := []string{slug + "-first-defense", "defender"}
	var gamesPlayed int
	if err := tx.QueryRow(ctx, `SELECT count(DISTINCT g.slug) FROM defense_results r JOIN games g ON g.id=r.game_id WHERE r.user_id=$1 AND r.verified AND g.slug=ANY($2::text[])`, userID, defenseGameSlugs).Scan(&gamesPlayed); err != nil {
		return err
	}
	if gamesPlayed == len(defenseGameSlugs) {
		codes = append(codes, "triple-guardian")
	}
	securityComplete, err := defenseCurrentCampaignComplete(ctx, tx, userID, "cyber-fortress")
	if err != nil {
		return err
	}
	if securityComplete {
		codes = append(codes, "security-guardian")
	}
	aiComplete, err := defenseCurrentCampaignComplete(ctx, tx, userID, "ai-nexus-defense")
	if err != nil {
		return err
	}
	if aiComplete {
		codes = append(codes, "ai-guardian")
	}
	allComplete := true
	for _, gameSlug := range defenseGameSlugs {
		complete, err := defenseCurrentCampaignComplete(ctx, tx, userID, gameSlug)
		if err != nil {
			return err
		}
		allComplete = allComplete && complete
	}
	if allComplete {
		codes = append(codes, "defense-master")
	}
	_, err = tx.Exec(ctx, `INSERT INTO user_achievements(user_id,achievement_id)
		SELECT $1,id FROM achievements WHERE active AND code=ANY($2::text[])
		ON CONFLICT DO NOTHING`, userID, codes)
	return err
}

// rejectDefenseResult refuses an authoritative Defense Series result and leaves
// an audit trail, so repeated forged submissions are visible to the anomaly
// detection the attestation boundary depends on. The session transaction is
// released first: the audit write must outlive this rollback and must not hold
// a second pooled connection behind the session row lock.
func (s *Server) rejectDefenseResult(w http.ResponseWriter, r *http.Request, tx pgx.Tx, slug string, sessionID uuid.UUID, status int, code, message string) {
	_ = tx.Rollback(r.Context())
	s.audit(r, "defense.result.reject", "game_session", sessionID.String(), map[string]any{"game": slug, "code": code, "reason": message})
	writeError(w, status, code, message)
}

func (s *Server) submitDefenseResult(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	slug, ok := defenseSlugParam(w, r)
	if !ok {
		return
	}
	var in defenseResultInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.SessionID == uuid.Nil || in.SessionToken == "" || !validRealmGuardIdentifier(in.StageID) || !slices.Contains([]string{"casual", "normal", "veteran"}, in.Difficulty) || in.DurationMS < 0 || in.RemainingHealth < 0 || in.RemainingResource < 0 || in.Kills < 0 || in.Escaped < 0 || in.Spawned < 0 || in.Kills > math.MaxInt32 || in.Escaped > math.MaxInt32 || in.Spawned > math.MaxInt32 || in.WavesCompleted < 0 || len(in.Answers) > 500 || in.Battle.EarnedResource < 0 || in.Battle.SpentResource < 0 || in.Battle.RecoveredResource < 0 || !validRealmGuardIdentifier(in.Battle.HeroID) || in.Battle.HeroLevel < 1 || in.Battle.HeroLevel > 100 {
		writeError(w, 400, "invalid_result", "result fields are missing or outside supported limits")
		return
	}
	if len(in.AIResources) > 0 && len(in.ResourceState) == 0 {
		writeError(w, 400, "resource_state_required", "AI results require start, spent, and remaining resource_state metrics")
		return
	}
	requestHash := defenseResultRequestChecksum(in)
	tokenHash := sha256.Sum256([]byte(in.SessionToken))
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var gameID, versionID uuid.UUID
	var started time.Time
	var status, contentVersion, policyVersion string
	var raw json.RawMessage
	err = tx.QueryRow(r.Context(), `SELECT gs.game_id,gs.defense_content_version_id,gs.started_at,gs.status,v.content_version,v.policy_version,v.content FROM game_sessions gs JOIN games g ON g.id=gs.game_id JOIN defense_content_versions v ON v.id=gs.defense_content_version_id WHERE gs.id=$1 AND gs.user_id=$2 AND gs.session_token_hash=$3 AND g.slug=$4 FOR UPDATE OF gs`, in.SessionID, p.UserID, tokenHash[:], slug).Scan(&gameID, &versionID, &started, &status, &contentVersion, &policyVersion, &raw)
	if err != nil {
		s.rejectDefenseResult(w, r, tx, slug, in.SessionID, 409, "invalid_session", "session, token, game, or pinned content is invalid")
		return
	}
	if !defenseGameClaimMatches(in.GameID, slug, gameID) {
		s.rejectDefenseResult(w, r, tx, slug, in.SessionID, 409, "game_mismatch", "game_id does not identify the Defense Series game pinned to this session")
		return
	}
	var existingID uuid.UUID
	var existingHash string
	err = tx.QueryRow(r.Context(), `SELECT id,request_hash FROM defense_results WHERE session_id=$1`, in.SessionID).Scan(&existingID, &existingHash)
	if err == nil {
		if existingHash != requestHash {
			s.rejectDefenseResult(w, r, tx, slug, in.SessionID, 409, "idempotency_conflict", "this session already has a different authoritative result")
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			s.dbError(w, r, err)
			return
		}
		result, err := s.defenseResultByID(r.Context(), existingID)
		if err != nil {
			s.dbError(w, r, err)
			return
		}
		version, _ := s.loadDefenseVersion(r.Context(), versionID)
		content, _ := decodeDefenseContent(raw)
		progress, _ := s.defenseProgressData(r.Context(), p.UserID, gameID, version, content)
		writeJSON(w, 200, map[string]any{"result": result, "progress": progress, "duplicate": true, "idempotent": true})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		s.dbError(w, r, err)
		return
	}
	if status != "active" {
		s.rejectDefenseResult(w, r, tx, slug, in.SessionID, 409, "invalid_session", "session cannot accept an authoritative result")
		return
	}
	if in.ContentVersion == "" || in.PolicyVersion == "" || in.ContentVersion != contentVersion || in.PolicyVersion != policyVersion {
		s.rejectDefenseResult(w, r, tx, slug, in.SessionID, 409, "defense_config_mismatch", "result versions do not match the pinned Defense Series content and policy")
		return
	}
	content, err := decodeDefenseContent(raw)
	if err != nil {
		writeError(w, 500, "invalid_pinned_content", err.Error())
		return
	}
	var stage defenseStageDefinition
	for _, candidate := range content.Stages {
		if candidate.ID == in.StageID {
			stage = candidate
			break
		}
	}
	if stage.ID == "" {
		s.rejectDefenseResult(w, r, tx, slug, in.SessionID, 422, "invalid_stage", "stage is not present in the pinned content")
		return
	}
	if !defenseHeroAvailable(content, in.Battle.HeroID, stage.Number) {
		s.rejectDefenseResult(w, r, tx, slug, in.SessionID, 422, "hero_locked", "hero is not unlocked for the selected pinned stage")
		return
	}
	if err = seedDefenseProgress(r.Context(), tx, p.UserID, gameID, versionID, content); err != nil {
		s.dbError(w, r, err)
		return
	}
	var stageUnlocked bool
	if err = tx.QueryRow(r.Context(), `SELECT bool_or(unlocked) FROM defense_user_progress WHERE user_id=$1 AND game_id=$2 AND content_version_id=$3 AND stage_id=$4`, p.UserID, gameID, versionID, stage.ID).Scan(&stageUnlocked); err != nil || !stageUnlocked {
		s.rejectDefenseResult(w, r, tx, slug, in.SessionID, 403, "stage_locked", "stage is not unlocked for this user")
		return
	}
	totalWaves, maxSpawns, _, _ := defenseBattleBudget(content, in.StageID, in.WavesCompleted, !in.Victory)
	if int64(in.WavesCompleted) > totalWaves || in.Kills > in.Spawned || in.Escaped > in.Spawned-in.Kills || in.Spawned > maxSpawns || in.RemainingHealth > stage.StartingHealth {
		s.rejectDefenseResult(w, r, tx, slug, in.SessionID, 422, "invalid_combat_counters", "combat counters exceed the pinned stage and wave budget")
		return
	}
	if in.Victory && (int64(in.WavesCompleted) != totalWaves || in.Spawned != maxSpawns || in.Kills+in.Escaped != in.Spawned || in.RemainingHealth < 1) {
		s.rejectDefenseResult(w, r, tx, slug, in.SessionID, 422, "invalid_victory", "victory counters do not match all pinned waves")
		return
	}
	if !in.Victory {
		resourceDepleted := false
		if slug == "ai-nexus-defense" {
			for _, metric := range in.ResourceState {
				resourceDepleted = resourceDepleted || metric.Remaining == 0
			}
		}
		if in.RemainingHealth != 0 && !resourceDepleted {
			s.rejectDefenseResult(w, r, tx, slug, in.SessionID, 422, "invalid_defeat", "a defeat must end with zero health or a depleted AI resource")
			return
		}
	}
	if slug == "ai-nexus-defense" && in.Victory {
		for _, metric := range in.ResourceState {
			if metric.Remaining <= 0 {
				s.rejectDefenseResult(w, r, tx, slug, in.SessionID, 422, "invalid_victory", "AI victory requires positive compute, token, trust, and latency headroom")
				return
			}
		}
	}
	serverDuration := s.Now().Sub(started).Milliseconds()
	if in.DurationMS > serverDuration+content.Balance.DurationToleranceMS || in.DurationMS < int64(in.WavesCompleted)*content.Balance.MinWaveDurationMS {
		s.rejectDefenseResult(w, r, tx, slug, in.SessionID, 422, "invalid_duration", "duration is inconsistent with server time or completed waves")
		return
	}
	seenAnswers := map[string]bool{}
	for _, answer := range in.Answers {
		if !validRealmGuardIdentifier(answer.EventID) || !validDefenseAnswerIdentifier(answer.AnswerID) || seenAnswers[answer.EventID] {
			writeError(w, 400, "invalid_answers", "answer events must be valid and unique")
			return
		}
		seenAnswers[answer.EventID] = true
		var storedAnswer string
		if err := tx.QueryRow(r.Context(), `SELECT answer_id FROM defense_event_answers WHERE session_id=$1 AND event_id=$2`, in.SessionID, answer.EventID).Scan(&storedAnswer); err != nil || storedAnswer != answer.AnswerID {
			s.rejectDefenseResult(w, r, tx, slug, in.SessionID, 422, "answer_not_recorded", "result answers must match answers previously validated at their reached event")
			return
		}
	}
	learningScore, learningBreakdown, answers, err := defenseLearningBreakdown(r.Context(), tx, in.SessionID)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	educationEffects, educationEarned, educationSpent, err := defenseStoredAnswerEffects(r.Context(), tx, in.SessionID, content)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	telemetryRecords, err := loadDefenseTelemetryRecords(r.Context(), tx, in.SessionID, true)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	wavesStarted := 0
	terminalDepletedWave := 0
	for _, record := range telemetryRecords {
		if record.Event == "defense.wave.start" {
			wavesStarted++
			if slug == "ai-nexus-defense" {
				var started defenseWaveStartTelemetry
				if decodeDefenseTelemetry(record.Data, &started) == nil && defenseResourceStateDepleted(started.ResourceState) {
					terminalDepletedWave = started.Wave
				}
			}
		}
	}
	requiredEducation := defenseRequiredEducationEvents(content, stage.ID, wavesStarted, terminalDepletedWave)
	if len(seenAnswers) != len(educationEffects) || len(educationEffects) != len(requiredEducation) {
		s.rejectDefenseResult(w, r, tx, slug, in.SessionID, 422, "answer_not_recorded", "result answers must include every server-validated education event exactly once")
		return
	}
	for eventID := range requiredEducation {
		if !seenAnswers[eventID] {
			s.rejectDefenseResult(w, r, tx, slug, in.SessionID, 422, "answer_not_recorded", "every reached education event must be answered before submitting a result")
			return
		}
	}
	version := defenseVersionRecord{ID: versionID, GameID: gameID, ContentVersion: contentVersion, PolicyVersion: policyVersion}
	attestation, err := validateDefenseTelemetryAttestation(telemetryRecords, slug, started, s.Now(), stage, content, version, in, educationEffects, educationEarned, educationSpent)
	if err != nil {
		if resultErr, ok := err.(realmGuardResultError); ok {
			s.rejectDefenseResult(w, r, tx, slug, in.SessionID, resultErr.Status, resultErr.Code, resultErr.Message)
		} else {
			s.dbError(w, r, err)
		}
		return
	}
	score, stars, scoreBreakdown := defenseScore(stage, content.Balance, in)
	attestationJSON, _ := json.Marshal(attestation)
	proofPayload, _ := json.Marshal(map[string]any{"method": defenseVerificationMethod, "session_id": in.SessionID, "user_id": p.UserID, "digest": attestation.Digest, "score": score, "stars": stars, "learning_score": learningScore})
	if s.Secrets == nil {
		writeError(w, 500, "proof_unavailable", "server proof encryption is unavailable")
		return
	}
	serverProof, err := s.Secrets.Seal(string(proofPayload))
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	var resultID uuid.UUID
	err = tx.QueryRow(r.Context(), `INSERT INTO defense_results(session_id,user_id,game_id,content_version_id,stage_id,difficulty,duration_ms,remaining_health,remaining_resource,kills,escaped,spawned,waves_completed,victory,score,stars,learning_score,policy_version,resource_state,score_breakdown,learning_breakdown,answers,request_hash,verification_method,attestation,server_proof) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26) RETURNING id`, in.SessionID, p.UserID, gameID, versionID, in.StageID, in.Difficulty, in.DurationMS, in.RemainingHealth, in.RemainingResource, int(in.Kills), int(in.Escaped), int(in.Spawned), in.WavesCompleted, in.Victory, score, stars, learningScore, policyVersion, in.ResourceState, scoreBreakdown, learningBreakdown, answers, requestHash, defenseVerificationMethod, attestationJSON, serverProof).Scan(&resultID)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	_, err = tx.Exec(r.Context(), `UPDATE game_sessions SET status='finished',ended_at=now(),duration_ms=$2,result=result||jsonb_build_object('defense_result_id',$3::text,'score',$4::bigint,'learning_score',$5::int) WHERE id=$1`, in.SessionID, in.DurationMS, resultID, score, learningScore)
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO defense_user_progress(user_id,game_id,stage_id,difficulty,unlocked,completed,stars,best_score,best_learning_score,attempts,completions,total_playtime_ms,content_version_id) VALUES($1,$2,$3,$4,true,$5,$6,$7,$8,1,CASE WHEN $5 THEN 1 ELSE 0 END,$9,$10) ON CONFLICT(user_id,game_id,content_version_id,stage_id,difficulty) DO UPDATE SET unlocked=true,completed=defense_user_progress.completed OR excluded.completed,stars=GREATEST(defense_user_progress.stars,excluded.stars),best_score=GREATEST(defense_user_progress.best_score,excluded.best_score),best_learning_score=GREATEST(defense_user_progress.best_learning_score,excluded.best_learning_score),attempts=defense_user_progress.attempts+1,completions=defense_user_progress.completions+CASE WHEN excluded.completed THEN 1 ELSE 0 END,total_playtime_ms=LEAST(9223372036854775807,defense_user_progress.total_playtime_ms+excluded.total_playtime_ms),updated_at=now()`, p.UserID, gameID, in.StageID, in.Difficulty, in.Victory, stars, score, learningScore, in.DurationMS, versionID)
	}
	if err == nil && in.Victory {
		_, err = tx.Exec(r.Context(), `UPDATE defense_user_progress SET unlocked=true,updated_at=now() WHERE user_id=$1 AND game_id=$2 AND content_version_id=$3 AND stage_id=(SELECT s->>'id' FROM defense_content_versions v CROSS JOIN LATERAL jsonb_array_elements(v.content->'stages') s WHERE v.id=$3 AND (s->>'number')::int=$4 LIMIT 1)`, p.UserID, gameID, versionID, stage.Number+1)
	}
	if err == nil {
		for _, campaign := range content.Campaigns {
			var completedStages int
			var averageLearning float64
			err = tx.QueryRow(r.Context(), `SELECT count(*) FILTER(WHERE completed),COALESCE(avg(stage_learning) FILTER(WHERE completed),0) FROM (SELECT stage_id,bool_or(completed) completed,max(best_learning_score) stage_learning FROM defense_user_progress WHERE user_id=$1 AND game_id=$2 AND content_version_id=$3 AND stage_id=ANY($4) GROUP BY stage_id) stage_progress`, p.UserID, gameID, versionID, campaign.StageIDs).Scan(&completedStages, &averageLearning)
			if err != nil {
				break
			}
			learning := int(math.Round(averageLearning))
			completed := completedStages == len(campaign.StageIDs) && learning >= campaign.RequiredLearningScore
			_, err = tx.Exec(r.Context(), `INSERT INTO defense_campaign_progress(user_id,game_id,campaign_id,completed,completed_stages,required_stages,learning_score,content_version_id,completed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,CASE WHEN $4 THEN now() END) ON CONFLICT(user_id,game_id,content_version_id,campaign_id) DO UPDATE SET completed=defense_campaign_progress.completed OR excluded.completed,completed_stages=excluded.completed_stages,required_stages=excluded.required_stages,learning_score=excluded.learning_score,completed_at=CASE WHEN defense_campaign_progress.completed_at IS NULL AND excluded.completed THEN now() ELSE defense_campaign_progress.completed_at END,updated_at=now()`, p.UserID, gameID, campaign.ID, completed, completedStages, len(campaign.StageIDs), learning, versionID)
			if err != nil {
				break
			}
		}
	}
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if err = unlockDefenseAchievements(r.Context(), tx, p.UserID, slug); err != nil {
		s.dbError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		s.dbError(w, r, err)
		return
	}
	s.audit(r, "defense.result.accept", "defense_result", resultID.String(), map[string]any{"game": slug, "session_id": in.SessionID, "score": score, "learning_score": learningScore, "verification_method": defenseVerificationMethod, "attestation_digest": attestation.Digest})
	version, _ = s.loadDefenseVersion(r.Context(), versionID)
	progress, _ := s.defenseProgressData(r.Context(), p.UserID, gameID, version, content)
	writeJSON(w, 201, map[string]any{"result": map[string]any{"id": resultID, "session_id": in.SessionID, "score": score, "stars": stars, "verified": true, "verification_method": defenseVerificationMethod, "attestation": attestation, "score_breakdown": scoreBreakdown, "learning_score": learningScore, "learning_breakdown": learningBreakdown, "answers": answers, "resource_state": in.ResourceState, "content_version": contentVersion, "policy_version": policyVersion}, "progress": progress, "duplicate": false, "idempotent": false})
}

func (s *Server) defenseResultByID(ctx context.Context, id uuid.UUID) (map[string]any, error) {
	var sessionID uuid.UUID
	var stageID, difficulty string
	var score, duration, health, resource int64
	var stars, learning, waves int
	var victory, verified bool
	var scoreBreakdown, learningBreakdown, answers, resourceState, attestation json.RawMessage
	var verificationMethod, contentVersion, policyVersion string
	var created time.Time
	err := s.DB.QueryRow(ctx, `SELECT r.session_id,r.stage_id,r.difficulty,r.duration_ms,r.remaining_health,r.remaining_resource,r.waves_completed,r.victory,r.score,r.stars,r.learning_score,r.verified,r.verification_method,r.attestation,r.resource_state,v.content_version,r.policy_version,r.score_breakdown,r.learning_breakdown,r.answers,r.created_at FROM defense_results r JOIN defense_content_versions v ON v.id=r.content_version_id WHERE r.id=$1`, id).Scan(&sessionID, &stageID, &difficulty, &duration, &health, &resource, &waves, &victory, &score, &stars, &learning, &verified, &verificationMethod, &attestation, &resourceState, &contentVersion, &policyVersion, &scoreBreakdown, &learningBreakdown, &answers, &created)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "session_id": sessionID, "stage_id": stageID, "difficulty": difficulty, "duration_ms": duration, "remaining_health": health, "remaining_resource": resource, "waves_completed": waves, "victory": victory, "score": score, "stars": stars, "learning_score": learning, "verified": verified, "verification_method": verificationMethod, "attestation": attestation, "resource_state": resourceState, "content_version": contentVersion, "policy_version": policyVersion, "score_breakdown": scoreBreakdown, "learning_breakdown": learningBreakdown, "answers": answers, "created_at": created}, nil
}

func defensePeriod(r *http.Request, now time.Time) (string, time.Time, bool) {
	period := r.URL.Query().Get("period")
	since := time.Time{}
	switch period {
	case "daily":
		since = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).UTC()
	case "weekly":
		offset := (int(now.Weekday()) + 6) % 7
		since = time.Date(now.Year(), now.Month(), now.Day()-offset, 0, 0, 0, 0, now.Location()).UTC()
	case "monthly":
		since = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).UTC()
	case "", "all_time":
		period = "all_time"
	case "season":
	default:
		return "", time.Time{}, false
	}
	return period, since, true
}

func (s *Server) defenseRankings(w http.ResponseWriter, r *http.Request) {
	slug, ok := defenseSlugParam(w, r)
	if !ok {
		return
	}
	gameID, gameName, err := s.defenseGame(r.Context(), slug)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	version, err := s.loadDefensePublished(r.Context(), slug)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	period, since, ok := defensePeriod(r, s.Now().In(s.serviceLocation(r.Context())))
	if !ok {
		writeError(w, 400, "invalid_period", "period must be daily, weekly, monthly, season, or all_time")
		return
	}
	group := r.URL.Query().Get("group")
	if group == "" {
		group = "individual"
	}
	if !slices.Contains([]string{"individual", "department", "team"}, group) {
		writeError(w, 400, "invalid_group", "group must be individual, department, or team")
		return
	}
	var privacy struct {
		RankingName    string `json:"ranking_name"`
		ShowDepartment bool   `json:"show_department"`
	}
	if err := s.setting(r.Context(), "privacy", &privacy); err != nil {
		writeError(w, 503, "privacy_setting_unavailable", "privacy policy is unavailable")
		return
	}
	if (group == "department" || group == "team") && !privacy.ShowDepartment {
		writeError(w, 403, "organization_ranking_hidden", "organization rankings are disabled by the privacy policy")
		return
	}
	limit, _ := pageParams(r)
	items := []map[string]any{}
	if group == "department" || group == "team" {
		column := "department"
		if group == "team" {
			column = "team"
		}
		query := fmt.Sprintf(`WITH user_best AS (SELECT u.id,u.%s group_name,max(dr.score) score FROM defense_results dr JOIN users u ON u.id=dr.user_id JOIN game_sessions gs ON gs.id=dr.session_id WHERE dr.game_id=$1 AND dr.content_version_id=$2 AND dr.verified AND NOT u.ranking_opt_out AND ($3::timestamptz='0001-01-01' OR dr.created_at >= $3) AND ($4<>'season' OR gs.season_id=(SELECT id FROM seasons WHERE status='active' LIMIT 1)) AND u.%s<>'' GROUP BY u.id,u.%s), totals AS (SELECT group_name,sum(score) score,count(*) members FROM user_best GROUP BY group_name) SELECT row_number() OVER(ORDER BY score DESC),group_name,score,members FROM totals ORDER BY score DESC LIMIT $5`, column, column, column)
		rows, err := s.DB.Query(r.Context(), query, gameID, version.ID, since, period, limit)
		if err != nil {
			s.dbError(w, r, err)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var rank, score int64
			var name string
			var members int
			if err := rows.Scan(&rank, &name, &score, &members); err != nil {
				s.dbError(w, r, err)
				return
			}
			item := map[string]any{"rank": rank, "name": name, "display_name": name, "score": score, "members": members, "game_name": gameName}
			item[group] = name
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			s.dbError(w, r, err)
			return
		}
	} else {
		rows, err := s.DB.Query(r.Context(), `WITH best AS (SELECT u.id,u.username,u.display_name,u.nickname,u.department,u.team,max(dr.score) score FROM defense_results dr JOIN users u ON u.id=dr.user_id JOIN game_sessions gs ON gs.id=dr.session_id WHERE dr.game_id=$1 AND dr.content_version_id=$2 AND dr.verified AND NOT u.ranking_opt_out AND ($3::timestamptz='0001-01-01' OR dr.created_at >= $3) AND ($4<>'season' OR gs.season_id=(SELECT id FROM seasons WHERE status='active' LIMIT 1)) GROUP BY u.id) SELECT row_number() OVER(ORDER BY score DESC),id,username,display_name,nickname,department,team,score FROM best ORDER BY score DESC LIMIT $5`, gameID, version.ID, since, period, limit)
		if err != nil {
			s.dbError(w, r, err)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var rank, score int64
			var id uuid.UUID
			var username, display, nickname, department, team string
			if err := rows.Scan(&rank, &id, &username, &display, &nickname, &department, &team, &score); err != nil {
				s.dbError(w, r, err)
				return
			}
			name := nickname
			if name == "" {
				name = username
			}
			if privacy.RankingName == "real_name" && display != "" {
				name = display
			}
			item := map[string]any{"rank": rank, "user_id": id, "name": name, "display_name": name, "score": score, "game_name": gameName}
			if privacy.ShowDepartment {
				item["department"] = department
				item["team"] = team
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			s.dbError(w, r, err)
			return
		}
	}
	writeJSON(w, 200, map[string]any{"items": items, "period": period, "group": group, "version": defenseVersionJSON(version)})
}

func (s *Server) defenseLearning(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	slug, ok := defenseSlugParam(w, r)
	if !ok {
		return
	}
	gameID, name, err := s.defenseGame(r.Context(), slug)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	version, err := s.loadDefensePublished(r.Context(), slug)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	rows, err := s.DB.Query(r.Context(), `SELECT topic,count(*) FILTER(WHERE correct),count(*),round(avg(score))::int FROM defense_event_answers WHERE user_id=$1 AND game_id=$2 AND content_version_id=$3 GROUP BY topic ORDER BY topic`, p.UserID, gameID, version.ID)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	topics := []map[string]any{}
	correctTotal, answerTotal := 0, 0
	for rows.Next() {
		var topic string
		var correct, total, score int
		if err := rows.Scan(&topic, &correct, &total, &score); err != nil {
			rows.Close()
			s.dbError(w, r, err)
			return
		}
		correctTotal += correct
		answerTotal += total
		topics = append(topics, map[string]any{"topic": topic, "correct": correct, "total": total, "score": score})
	}
	if err := rows.Err(); err != nil {
		s.dbError(w, r, err)
		return
	}
	rows.Close()
	overall := 0
	if answerTotal > 0 {
		overall = int(math.Round(float64(correctTotal) * 100 / float64(answerTotal)))
	}
	campaignRows, err := s.DB.Query(r.Context(), `SELECT campaign_id,completed,completed_stages,required_stages,learning_score,completed_at,updated_at FROM defense_campaign_progress WHERE user_id=$1 AND game_id=$2 AND content_version_id=$3 ORDER BY campaign_id`, p.UserID, gameID, version.ID)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	defer campaignRows.Close()
	campaigns := []map[string]any{}
	for campaignRows.Next() {
		var id string
		var completed bool
		var completedStages, requiredStages, score int
		var completedAt *time.Time
		var updated time.Time
		if err := campaignRows.Scan(&id, &completed, &completedStages, &requiredStages, &score, &completedAt, &updated); err != nil {
			s.dbError(w, r, err)
			return
		}
		campaigns = append(campaigns, map[string]any{"campaign_id": id, "completed": completed, "completed_stages": completedStages, "required_stages": requiredStages, "learning_score": score, "completed_at": completedAt, "updated_at": updated})
	}
	if err := campaignRows.Err(); err != nil {
		s.dbError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"game": map[string]any{"id": gameID, "slug": slug, "name": name}, "version": defenseVersionJSON(version), "policy_version": version.PolicyVersion, "overall_score": overall, "topics": topics, "completed_campaigns": campaigns})
}
