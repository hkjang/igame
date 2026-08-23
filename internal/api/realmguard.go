package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const realmGuardSlug = "realmguard"

var realmGuardSections = []string{"stages", "waves", "enemies", "bosses", "towers", "heroes", "skills", "balance"}

type realmGuardVersionRecord struct {
	ID             uuid.UUID
	VersionNo      int
	Label          string
	Status         string
	ContentVersion string
	StageVersion   string
	BalanceVersion string
	AssetVersion   string
	Checksum       string
	Notes          string
	RawContent     json.RawMessage
	CreatedBy      *uuid.UUID
	ApprovedBy     *uuid.UUID
	CreatedAt      time.Time
	TestedAt       *time.Time
	RequestedAt    *time.Time
	ApprovedAt     *time.Time
	ReviewComment  string
	ReviewedAt     *time.Time
	PublishedAt    *time.Time
	UpdatedAt      time.Time
}

type realmGuardPoint struct {
	ID string  `json:"id,omitempty"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
}

type realmGuardStageDefinition struct {
	ID           string              `json:"id"`
	Number       int                 `json:"number"`
	Name         string              `json:"name"`
	Subtitle     string              `json:"subtitle"`
	Mode         string              `json:"mode"`
	Theme        string              `json:"theme"`
	Path         []realmGuardPoint   `json:"path"`
	Paths        [][]realmGuardPoint `json:"paths,omitempty"`
	TowerSpots   []realmGuardPoint   `json:"tower_spots"`
	StartingGold int                 `json:"starting_gold"`
	Lives        int                 `json:"lives"`
	Version      string              `json:"version"`
	Gimmick      string              `json:"gimmick,omitempty"`
}

type realmGuardWaveEntry struct {
	Enemy     string   `json:"enemy"`
	Count     int      `json:"count"`
	Interval  float64  `json:"interval"`
	Delay     float64  `json:"delay,omitempty"`
	Modifiers []string `json:"modifiers,omitempty"`
	PathIndex *int     `json:"path_index,omitempty"`
	Parallel  bool     `json:"parallel,omitempty"`
}

type realmGuardWaveDefinition struct {
	ID      string                `json:"id"`
	StageID string                `json:"stage_id"`
	Number  int                   `json:"number"`
	Label   string                `json:"label"`
	Entries []realmGuardWaveEntry `json:"entries"`
	Reward  int                   `json:"reward"`
}

type realmGuardTowerDefinition struct {
	Name            string  `json:"name"`
	Role            string  `json:"role"`
	Color           int     `json:"color"`
	ID              string  `json:"id"`
	Cost            int     `json:"cost"`
	Damage          float64 `json:"damage"`
	Range           float64 `json:"range"`
	FireRate        float64 `json:"fire_rate"`
	ProjectileSpeed float64 `json:"projectile_speed"`
	DamageType      string  `json:"damage_type"`
	Branches        []struct {
		ID               string   `json:"id"`
		Name             string   `json:"name"`
		Description      string   `json:"description"`
		DamageMultiplier *float64 `json:"damage_multiplier,omitempty"`
		RangeMultiplier  *float64 `json:"range_multiplier,omitempty"`
		RateMultiplier   *float64 `json:"rate_multiplier,omitempty"`
		Splash           *float64 `json:"splash,omitempty"`
		Slow             *float64 `json:"slow,omitempty"`
		Pierce           *int     `json:"pierce,omitempty"`
	} `json:"branches"`
}

type realmGuardEnemyDefinition struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Color      int      `json:"color"`
	HP         float64  `json:"hp"`
	Speed      float64  `json:"speed"`
	Armor      float64  `json:"armor"`
	Reward     int      `json:"reward"`
	LifeDamage int      `json:"life_damage"`
	Radius     float64  `json:"radius"`
	Traits     []string `json:"traits"`
}

type realmGuardHeroDefinition struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Title          string  `json:"title"`
	Color          int     `json:"color"`
	HP             float64 `json:"hp"`
	Damage         float64 `json:"damage"`
	Range          float64 `json:"range"`
	Speed          float64 `json:"speed"`
	RespawnSeconds float64 `json:"respawn_seconds"`
	Skill1         string  `json:"skill1"`
	Skill2         string  `json:"skill2"`
	Ultimate       string  `json:"ultimate"`
	UnlockStage    int     `json:"unlock_stage"`
}

type realmGuardSkillDefinition struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Color       string  `json:"color"`
	Cooldown    float64 `json:"cooldown"`
	UnlockStage int     `json:"unlock_stage"`
}

type realmGuardDifficultyBalance struct {
	DifficultyBonus int64   `json:"difficulty_bonus"`
	EnemyHP         float64 `json:"enemy_hp"`
	EnemySpeed      float64 `json:"enemy_speed"`
	Gold            float64 `json:"gold"`
}

type realmGuardBalanceDefinition struct {
	Difficulties          map[string]realmGuardDifficultyBalance `json:"difficulties"`
	TowerUpgradeCost      []int64                                `json:"tower_upgrade_cost"`
	HeroLevelXP           []int64                                `json:"hero_level_xp"`
	ClearTimeTargetMS     int64                                  `json:"clear_time_target_ms"`
	ClearTimeBonusDivisor int64                                  `json:"clear_time_bonus_divisor"`
	EndlessWaveBonus      int64                                  `json:"endless_wave_bonus"`
	DurationToleranceMS   int64                                  `json:"duration_tolerance_ms"`
	MinWaveDurationMS     int64                                  `json:"min_wave_duration_ms"`
	EndlessRamp           float64                                `json:"endless_ramp"`
	SellRefundRate        float64                                `json:"sell_refund_rate"`
}

type realmGuardDecodedContent struct {
	Sections map[string]json.RawMessage
	Stages   []realmGuardStageDefinition
	Waves    []realmGuardWaveDefinition
	Towers   []realmGuardTowerDefinition
	Enemies  []realmGuardEnemyDefinition
	Bosses   []realmGuardEnemyDefinition
	Heroes   []realmGuardHeroDefinition
	Skills   []realmGuardSkillDefinition
	Balance  realmGuardBalanceDefinition
}

type rowScanner interface {
	Scan(dest ...any) error
}

const realmGuardVersionColumns = `id,version_no,label,status,content_version,stage_version,balance_version,asset_version,checksum,notes,content,created_by,approved_by,created_at,tested_at,approval_requested_at,approved_at,review_comment,reviewed_at,published_at,updated_at`

func scanRealmGuardVersion(row rowScanner) (realmGuardVersionRecord, error) {
	var version realmGuardVersionRecord
	err := row.Scan(&version.ID, &version.VersionNo, &version.Label, &version.Status, &version.ContentVersion, &version.StageVersion, &version.BalanceVersion, &version.AssetVersion, &version.Checksum, &version.Notes, &version.RawContent, &version.CreatedBy, &version.ApprovedBy, &version.CreatedAt, &version.TestedAt, &version.RequestedAt, &version.ApprovedAt, &version.ReviewComment, &version.ReviewedAt, &version.PublishedAt, &version.UpdatedAt)
	return version, err
}

func realmGuardChecksum(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *Server) loadRealmGuardPublished(ctx context.Context) (realmGuardVersionRecord, error) {
	version, err := scanRealmGuardVersion(s.DB.QueryRow(ctx, `SELECT `+realmGuardVersionColumns+` FROM realmguard_content_versions WHERE status='published'`))
	if err != nil {
		return version, err
	}
	return s.normalizeRealmGuardChecksum(ctx, version), nil
}

func (s *Server) loadRealmGuardVersion(ctx context.Context, id uuid.UUID) (realmGuardVersionRecord, error) {
	version, err := scanRealmGuardVersion(s.DB.QueryRow(ctx, `SELECT `+realmGuardVersionColumns+` FROM realmguard_content_versions WHERE id=$1`, id))
	if err != nil {
		return version, err
	}
	return s.normalizeRealmGuardChecksum(ctx, version), nil
}

func (s *Server) normalizeRealmGuardChecksum(ctx context.Context, version realmGuardVersionRecord) realmGuardVersionRecord {
	checksum := realmGuardChecksum(version.RawContent)
	if version.Checksum != checksum {
		version.Checksum = checksum
		_, _ = s.DB.Exec(ctx, `UPDATE realmguard_content_versions SET checksum=$2 WHERE id=$1 AND checksum<>$2`, version.ID, checksum)
	}
	return version
}

func realmGuardVersionJSON(version realmGuardVersionRecord) map[string]any {
	return map[string]any{
		"id": version.ID, "version_no": version.VersionNo, "label": version.Label, "status": version.Status,
		"content_version": version.ContentVersion, "stage_version": version.StageVersion, "balance_version": version.BalanceVersion,
		"asset_version": version.AssetVersion, "checksum": version.Checksum, "notes": version.Notes,
		"created_by": version.CreatedBy, "approved_by": version.ApprovedBy, "created_at": version.CreatedAt,
		"tested_at": version.TestedAt, "approval_requested_at": version.RequestedAt, "approved_at": version.ApprovedAt,
		"review_comment": version.ReviewComment, "reviewed_at": version.ReviewedAt,
		"published_at": version.PublishedAt, "updated_at": version.UpdatedAt,
	}
}

func decodeRealmGuardContent(raw []byte) (realmGuardDecodedContent, error) {
	var decoded realmGuardDecodedContent
	if err := json.Unmarshal(raw, &decoded.Sections); err != nil {
		return decoded, fmt.Errorf("content must be a JSON object")
	}
	decode := func(section string, target any) error {
		value, ok := decoded.Sections[section]
		if !ok {
			return fmt.Errorf("missing %s section", section)
		}
		if err := json.Unmarshal(value, target); err != nil {
			return fmt.Errorf("invalid %s section", section)
		}
		return nil
	}
	for section, target := range map[string]any{
		"stages": &decoded.Stages, "waves": &decoded.Waves, "towers": &decoded.Towers, "enemies": &decoded.Enemies,
		"bosses": &decoded.Bosses, "heroes": &decoded.Heroes, "skills": &decoded.Skills, "balance": &decoded.Balance,
	} {
		if err := decode(section, target); err != nil {
			return decoded, err
		}
	}
	return decoded, nil
}

func validateRealmGuardContent(raw []byte) error {
	content, err := decodeRealmGuardContent(raw)
	if err != nil {
		return err
	}
	if len(content.Stages) < 11 {
		return fmt.Errorf("content requires at least 10 campaign stages and one endless stage")
	}
	campaigns, endless := 0, 0
	stageNumbers := map[int]bool{}
	campaignNumbers := map[int]bool{}
	stageIDs := map[string]realmGuardStageDefinition{}
	stagePathCounts := map[string]int{}
	for _, stage := range content.Stages {
		if !validRealmGuardIdentifier(stage.ID) || stageIDs[stage.ID].ID != "" {
			return fmt.Errorf("stage IDs must be unique and non-empty")
		}
		if stage.Number < 1 || stageNumbers[stage.Number] {
			return fmt.Errorf("stage numbers must be unique positive integers")
		}
		stageNumbers[stage.Number] = true
		if stage.Mode == "campaign" {
			campaigns++
			campaignNumbers[stage.Number] = true
		} else if stage.Mode == "endless" {
			endless++
		} else {
			return fmt.Errorf("stage %s has invalid mode", stage.ID)
		}
		paths := stage.Paths
		if len(paths) == 0 && len(stage.Path) > 0 {
			paths = [][]realmGuardPoint{stage.Path}
		}
		if strings.TrimSpace(stage.Name) == "" || strings.TrimSpace(stage.Subtitle) == "" || len(stage.Name) > 120 || len(stage.Subtitle) > 240 || len(paths) == 0 || len(stage.TowerSpots) < 8 || stage.StartingGold < 0 || stage.StartingGold > 1000000 || stage.Lives < 1 || stage.Lives > 200 || stage.Version == "" || len(stage.Version) > 100 || !slices.Contains([]string{"verdant", "ember", "frost", "void"}, stage.Theme) || !slices.Contains([]string{"", "ember_vents", "winter_blessing", "time_surge"}, stage.Gimmick) {
			return fmt.Errorf("stage %s lacks a valid path, tower spots, economy, lives, or version", stage.ID)
		}
		for _, path := range paths {
			if len(path) < 2 {
				return fmt.Errorf("stage %s paths must contain at least two points per lane", stage.ID)
			}
		}
		spotIDs := map[string]bool{}
		for _, path := range paths {
			for _, point := range path {
				if math.IsNaN(point.X) || math.IsInf(point.X, 0) || math.IsNaN(point.Y) || math.IsInf(point.Y, 0) || point.X < -64 || point.X > 1344 || point.Y < 0 || point.Y > 720 {
					return fmt.Errorf("stage %s has an invalid or out-of-bounds point", stage.ID)
				}
			}
		}
		for _, spot := range stage.TowerSpots {
			if !validRealmGuardIdentifier(spot.ID) || spotIDs[spot.ID] || !finite(spot.X, spot.Y) || spot.X < 0 || spot.X > 1280 || spot.Y < 0 || spot.Y > 720 {
				return fmt.Errorf("stage %s tower spot IDs must be unique", stage.ID)
			}
			spotIDs[spot.ID] = true
		}
		stageIDs[stage.ID] = stage
		stagePathCounts[stage.ID] = len(paths)
	}
	if campaigns < 10 || endless != 1 {
		return fmt.Errorf("content requires at least 10 contiguous campaign stages and exactly one endless stage")
	}
	for number := 1; number <= campaigns; number++ {
		if !campaignNumbers[number] {
			return fmt.Errorf("campaign stage numbers must be contiguous from 1")
		}
	}
	waveCounts := map[string]int{}
	waveNumbers := map[string]map[int]bool{}
	enemyIDs := map[string]bool{}
	if len(content.Enemies) > 16 || len(content.Bosses) > 4 {
		return fmt.Errorf("content supports at most 16 enemies and 4 bosses so ranked telemetry remains bounded")
	}
	for _, enemy := range append(append([]realmGuardEnemyDefinition{}, content.Enemies...), content.Bosses...) {
		if !validRealmGuardIdentifier(enemy.ID) || strings.TrimSpace(enemy.Name) == "" || len(enemy.Name) > 120 || enemy.Color < 0 || enemy.Color > 0xffffff || enemyIDs[enemy.ID] || enemy.Reward < 0 || enemy.Reward > 1000000 || enemy.LifeDamage < 1 || enemy.LifeDamage > 200 || enemy.HP <= 0 || enemy.HP > 1e9 || enemy.Speed <= 0 || enemy.Speed > 5000 || enemy.Radius <= 0 || enemy.Radius > 200 || enemy.Armor < 0 || enemy.Armor > 1 || !finite(enemy.HP, enemy.Speed, enemy.Radius, enemy.Armor) {
			return fmt.Errorf("enemy and boss IDs must be unique with valid rewards and life damage")
		}
		for _, trait := range enemy.Traits {
			if !slices.Contains([]string{"armored", "swift", "flying", "regenerating", "healer", "splitting", "phasing", "siege", "boss", "magic_resist", "stealth", "berserk", "immune_stun"}, trait) {
				return fmt.Errorf("enemy %s contains an invalid trait", enemy.ID)
			}
		}
		enemyIDs[enemy.ID] = true
	}
	if len(content.Enemies) < 10 || len(content.Bosses) < 2 {
		return fmt.Errorf("content requires at least 10 enemies and 2 bosses")
	}
	if !realmGuardTelemetryPayloadFits(enemyIDs) {
		return fmt.Errorf("enemy identifiers exceed the 4 KiB ranked telemetry payload budget")
	}
	for _, required := range []string{"mireling", "thornback", "glintfox", "cloudray", "bloomseer", "shardling", "ironroot", "veilrunner", "rammer", "rimeheart", "hollow_king", "timewyrm"} {
		if !enemyIDs[required] {
			return fmt.Errorf("engine-required enemy or boss %s is missing", required)
		}
	}
	waveIDs := map[string]bool{}
	for _, wave := range content.Waves {
		if !validRealmGuardIdentifier(wave.ID) || strings.TrimSpace(wave.Label) == "" || len(wave.Label) > 120 || waveIDs[wave.ID] || stageIDs[wave.StageID].ID == "" || wave.Number < 1 || len(wave.Entries) == 0 || len(wave.Entries) > 8 || wave.Reward < 0 || wave.Reward > 1000000 {
			return fmt.Errorf("waves require unique IDs, valid stages, entries, numbers, and rewards")
		}
		waveIDs[wave.ID] = true
		if waveNumbers[wave.StageID] == nil {
			waveNumbers[wave.StageID] = map[int]bool{}
		}
		if waveNumbers[wave.StageID][wave.Number] {
			return fmt.Errorf("wave numbers must be unique within each stage")
		}
		waveNumbers[wave.StageID][wave.Number] = true
		waveCounts[wave.StageID]++
		waveSpawnCount := 0
		for _, entry := range wave.Entries {
			if !enemyIDs[entry.Enemy] || entry.Count < 1 || entry.Count > 10000 || entry.Interval < .01 || entry.Interval > 3600 || entry.Delay < 0 || entry.Delay > 86400 || !finite(entry.Interval, entry.Delay) || entry.PathIndex != nil && (*entry.PathIndex < 0 || *entry.PathIndex >= stagePathCounts[wave.StageID]) {
				return fmt.Errorf("wave %s contains an invalid enemy entry", wave.ID)
			}
			waveSpawnCount += entry.Count
			for _, modifier := range entry.Modifiers {
				if !slices.Contains([]string{"armored", "swift", "flying", "regenerating", "healer", "splitting", "phasing", "siege", "magic_resist", "stealth", "berserk", "immune_stun"}, modifier) {
					return fmt.Errorf("wave %s contains an invalid modifier", wave.ID)
				}
			}
		}
		if waveSpawnCount > 500 {
			return fmt.Errorf("wave %s exceeds the 500-enemy eager expansion budget", wave.ID)
		}
	}
	for stageID := range stageIDs {
		if waveCounts[stageID] < 8 || waveCounts[stageID] > 15 {
			return fmt.Errorf("stage %s must contain 8 through 15 waves", stageID)
		}
		for number := 1; number <= waveCounts[stageID]; number++ {
			if !waveNumbers[stageID][number] {
				return fmt.Errorf("stage %s wave numbers must be contiguous from 1", stageID)
			}
		}
	}
	if len(content.Towers) < 4 {
		return fmt.Errorf("content requires at least 4 base towers")
	}
	towerIDs, branchIDs, branchCount := map[string]bool{}, map[string]bool{}, 0
	for _, tower := range content.Towers {
		if !validRealmGuardIdentifier(tower.ID) || strings.TrimSpace(tower.Name) == "" || strings.TrimSpace(tower.Role) == "" || len(tower.Name) > 120 || len(tower.Role) > 240 || tower.Color < 0 || tower.Color > 0xffffff || towerIDs[tower.ID] || len(tower.Branches) != 2 || tower.Cost < 0 || tower.Cost > 1000000 || tower.Damage <= 0 || tower.Damage > 1e9 || tower.Range <= 0 || tower.Range > 2000 || tower.FireRate < .01 || tower.FireRate > 3600 || tower.ProjectileSpeed <= 0 || tower.ProjectileSpeed > 10000 || !finite(tower.Damage, tower.Range, tower.FireRate, tower.ProjectileSpeed) || !slices.Contains([]string{"physical", "arcane", "siege", "frost"}, tower.DamageType) {
			return fmt.Errorf("each base tower requires a unique ID, valid combat stats, and exactly two advanced branches")
		}
		towerIDs[tower.ID] = true
		for _, branch := range tower.Branches {
			if !validRealmGuardIdentifier(branch.ID) || strings.TrimSpace(branch.Name) == "" || strings.TrimSpace(branch.Description) == "" || len(branch.Name) > 120 || len(branch.Description) > 500 || branchIDs[branch.ID] {
				return fmt.Errorf("advanced tower branch IDs must be unique")
			}
			branchIDs[branch.ID] = true
			for _, value := range []*float64{branch.DamageMultiplier, branch.RangeMultiplier, branch.RateMultiplier} {
				if value != nil && (!finite(*value) || *value <= 0 || *value > 100) {
					return fmt.Errorf("tower branch %s has an invalid modifier", branch.ID)
				}
			}
			if branch.Splash != nil && (!finite(*branch.Splash) || *branch.Splash < 0 || *branch.Splash > 5000) || branch.Slow != nil && (!finite(*branch.Slow) || *branch.Slow < 0 || *branch.Slow > 1) {
				return fmt.Errorf("tower branch %s has an invalid splash or slow modifier", branch.ID)
			}
			if branch.Pierce != nil && *branch.Pierce < 0 {
				return fmt.Errorf("tower branch %s has invalid pierce", branch.ID)
			}
			branchCount++
		}
	}
	for _, required := range []string{"sunspire", "runebloom", "stonepulse", "windward"} {
		if !towerIDs[required] {
			return fmt.Errorf("engine-required base tower %s is missing", required)
		}
	}
	if branchCount < 8 || len(content.Heroes) < 3 || len(content.Skills) < 3 {
		return fmt.Errorf("content requires at least 8 advanced towers, 3 heroes, and 3 skills")
	}
	heroIDs := map[string]bool{}
	initialHeroes := 0
	for _, hero := range content.Heroes {
		if !validRealmGuardIdentifier(hero.ID) || strings.TrimSpace(hero.Name) == "" || strings.TrimSpace(hero.Title) == "" || hero.Color < 0 || hero.Color > 0xffffff || heroIDs[hero.ID] || hero.HP <= 0 || hero.HP > 1e9 || hero.Damage <= 0 || hero.Damage > 1e9 || hero.Range <= 0 || hero.Range > 2000 || hero.Speed <= 0 || hero.Speed > 5000 || hero.RespawnSeconds <= 0 || hero.RespawnSeconds > 3600 || !finite(hero.HP, hero.Damage, hero.Range, hero.Speed, hero.RespawnSeconds) || strings.TrimSpace(hero.Skill1) == "" || strings.TrimSpace(hero.Skill2) == "" || strings.TrimSpace(hero.Ultimate) == "" || !campaignNumbers[hero.UnlockStage] {
			return fmt.Errorf("heroes require unique IDs, valid stats, abilities, and unlock stages")
		}
		heroIDs[hero.ID] = true
		if hero.UnlockStage <= 1 {
			initialHeroes++
		}
	}
	for _, required := range []string{"aerin", "brann", "nyra"} {
		if !heroIDs[required] {
			return fmt.Errorf("engine-required hero %s is missing", required)
		}
	}
	skillIDs := map[string]bool{}
	initialSkills := 0
	for _, skill := range content.Skills {
		if !validRealmGuardIdentifier(skill.ID) || strings.TrimSpace(skill.Name) == "" || strings.TrimSpace(skill.Description) == "" || !validRealmGuardColor(skill.Color) || skillIDs[skill.ID] || skill.Cooldown <= 0 || skill.Cooldown > 86400 || !finite(skill.Cooldown) || !campaignNumbers[skill.UnlockStage] {
			return fmt.Errorf("skills require unique IDs, positive cooldowns, and unlock stages")
		}
		skillIDs[skill.ID] = true
		if skill.UnlockStage <= 1 {
			initialSkills++
		}
	}
	if len(content.Skills) != 3 {
		return fmt.Errorf("the v0.2 engine supports exactly three active skills")
	}
	for _, required := range []string{"meteor", "reinforcement", "freeze"} {
		if !skillIDs[required] {
			return fmt.Errorf("engine-required active skill %s is missing", required)
		}
	}
	if initialHeroes < 1 || initialSkills < 1 {
		return fmt.Errorf("content requires at least one hero and one skill unlocked at stage 1")
	}
	for _, difficulty := range []string{"casual", "normal", "veteran"} {
		value, ok := content.Balance.Difficulties[difficulty]
		if !ok {
			return fmt.Errorf("balance is missing %s difficulty", difficulty)
		}
		if value.EnemyHP <= 0 || value.EnemyHP > 100 || value.EnemySpeed <= 0 || value.EnemySpeed > 100 || value.Gold <= 0 || value.Gold > 100 || value.DifficultyBonus < 0 || value.DifficultyBonus > 1e12 || !finite(value.EnemyHP, value.EnemySpeed, value.Gold) {
			return fmt.Errorf("balance %s multipliers must be positive finite values", difficulty)
		}
	}
	if len(content.Balance.TowerUpgradeCost) < 3 || content.Balance.TowerUpgradeCost[0] != 0 {
		return fmt.Errorf("tower_upgrade_cost requires at least three non-negative levels beginning with zero")
	}
	for _, cost := range content.Balance.TowerUpgradeCost {
		if cost < 0 || cost > 1000000 {
			return fmt.Errorf("tower_upgrade_cost requires non-negative values")
		}
	}
	if content.Balance.ClearTimeTargetMS <= 0 || content.Balance.ClearTimeTargetMS > 30*24*60*60*1000 || content.Balance.ClearTimeBonusDivisor <= 0 || content.Balance.ClearTimeBonusDivisor > 1000000000 || content.Balance.EndlessWaveBonus < 0 || content.Balance.EndlessWaveBonus > 1000000000 || content.Balance.DurationToleranceMS < 0 || content.Balance.DurationToleranceMS > 600000 || content.Balance.MinWaveDurationMS < 5000 || content.Balance.MinWaveDurationMS > 600000 || content.Balance.EndlessRamp < 0 || content.Balance.EndlessRamp > 10 || content.Balance.SellRefundRate <= 0 || content.Balance.SellRefundRate > 1 || !finite(content.Balance.EndlessRamp, content.Balance.SellRefundRate) {
		return fmt.Errorf("balance server validation values are invalid")
	}
	if len(content.Balance.HeroLevelXP) < 10 || content.Balance.HeroLevelXP[0] != 0 {
		return fmt.Errorf("hero_level_xp requires at least ten levels beginning with zero")
	}
	previous := int64(-1)
	for _, threshold := range content.Balance.HeroLevelXP {
		if threshold < 0 || threshold <= previous {
			return fmt.Errorf("hero_level_xp must be strictly increasing non-negative thresholds")
		}
		previous = threshold
	}
	for _, stage := range content.Stages {
		if stage.Mode != "endless" {
			continue
		}
		budget := realmGuardWaveCapacity(content, stage.ID, realmGuardMaxEndlessWaves, false)
		if int64(budget.BaseSpawns) > math.MaxInt32 || int64(budget.MaxSpawns) > math.MaxInt32 {
			return fmt.Errorf("endless stage %s exceeds the ranked 32-bit combat counter budget", stage.ID)
		}
	}
	return nil
}

func finite(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}
	return true
}

func validRealmGuardIdentifier(value string) bool {
	if len(value) < 1 || len(value) > 32 {
		return false
	}
	for index, character := range []byte(value) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' && index > 0 || index > 0 && (character == '-' || character == '_') {
			continue
		}
		return false
	}
	return true
}

func realmGuardTelemetryPayloadFits(enemyIDs map[string]bool) bool {
	histogram := make(map[string]int, len(enemyIDs))
	for id := range enemyIDs {
		histogram[id] = 2147483647
	}
	sample, err := json.Marshal(map[string]any{
		"stage_id": strings.Repeat("s", 32), "mode": "campaign", "difficulty": "veteran",
		"duration_ms": int64(315360000000), "lives": 200, "gold": int64(9000000000000000),
		"earned_gold": int64(9000000000000000), "spent_gold": int64(9000000000000000), "sold_gold": int64(9000000000000000),
		"kills": 2147483647, "escaped": 2147483647, "spawned": 2147483647, "waves": 10000, "waves_completed": 10000,
		"hero_id": strings.Repeat("h", 32), "hero_level": 10,
		"content_version": strings.Repeat("c", 100), "balance_version": strings.Repeat("b", 100), "stage_version": strings.Repeat("s", 100), "asset_version": strings.Repeat("a", 100),
		"victory": false, "defeated_by_enemy": histogram, "escaped_by_enemy": histogram, "spawned_by_enemy": histogram,
	})
	return err == nil && len(sample) <= 4<<10
}

func validRealmGuardColor(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	_, err := hex.DecodeString(value[1:])
	return err == nil
}

func (s *Server) realmGuardConfig(w http.ResponseWriter, r *http.Request) {
	version, err := s.loadRealmGuardPublished(r.Context())
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	response, err := realmGuardConfigPayload(version)
	if err != nil {
		writeError(w, 500, "invalid_published_content", err.Error())
		return
	}
	writeJSON(w, 200, response)
}

func realmGuardConfigPayload(version realmGuardVersionRecord) (map[string]any, error) {
	content, err := decodeRealmGuardContent(version.RawContent)
	if err != nil {
		return nil, err
	}
	var stages []map[string]any
	_ = json.Unmarshal(content.Sections["stages"], &stages)
	slices.SortFunc(stages, func(a, b map[string]any) int {
		numberA, _ := a["number"].(float64)
		numberB, _ := b["number"].(float64)
		return int(numberA - numberB)
	})
	var waves []map[string]any
	_ = json.Unmarshal(content.Sections["waves"], &waves)
	slices.SortFunc(waves, func(a, b map[string]any) int {
		stageA, _ := a["stage_id"].(string)
		stageB, _ := b["stage_id"].(string)
		if stageA != stageB {
			return strings.Compare(stageA, stageB)
		}
		numberA, _ := a["number"].(float64)
		numberB, _ := b["number"].(float64)
		return int(numberA - numberB)
	})
	wavesByStage := map[string][]map[string]any{}
	for _, wave := range waves {
		stageID, _ := wave["stage_id"].(string)
		embedded := make(map[string]any, len(wave))
		for key, value := range wave {
			embedded[key] = value
		}
		delete(embedded, "stage_id")
		delete(embedded, "number")
		wavesByStage[stageID] = append(wavesByStage[stageID], embedded)
	}
	for _, stage := range stages {
		stageID, _ := stage["id"].(string)
		stage["waves"] = wavesByStage[stageID]
	}
	var towers []map[string]any
	_ = json.Unmarshal(content.Sections["towers"], &towers)
	advanced := []map[string]any{}
	for _, tower := range towers {
		branches, _ := tower["branches"].([]any)
		for _, rawBranch := range branches {
			if branch, ok := rawBranch.(map[string]any); ok {
				copy := map[string]any{"base_tower_id": tower["id"]}
				for key, value := range branch {
					copy[key] = value
				}
				advanced = append(advanced, copy)
			}
		}
	}
	response := map[string]any{"version": realmGuardVersionJSON(version), "stages": stages, "waves": waves, "towers": towers, "base_towers": towers, "advanced_towers": advanced}
	for _, section := range []string{"enemies", "bosses", "heroes", "skills", "balance"} {
		var value any
		_ = json.Unmarshal(content.Sections[section], &value)
		response[section] = value
	}
	return response, nil
}

func (s *Server) realmGuardVersion(w http.ResponseWriter, r *http.Request) {
	version, err := s.loadRealmGuardPublished(r.Context())
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"version": realmGuardVersionJSON(version)})
}

// execer is satisfied by both *pgxpool.Pool and pgx.Tx, so seeding helpers can
// run standalone or join a caller's transaction.
type execer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func ensureRealmGuardUserWith(ctx context.Context, db execer, userID uuid.UUID, content realmGuardDecodedContent) error {
	heroIDs, heroUnlocked := make([]string, 0, len(content.Heroes)), make([]bool, 0, len(content.Heroes))
	for _, hero := range content.Heroes {
		heroIDs = append(heroIDs, hero.ID)
		heroUnlocked = append(heroUnlocked, hero.UnlockStage <= 1)
	}
	if _, err := db.Exec(ctx, `INSERT INTO realmguard_user_heroes(user_id,hero_id,unlocked)
		SELECT $1,hero.id,hero.unlocked FROM unnest($2::text[],$3::boolean[]) AS hero(id,unlocked)
		ON CONFLICT DO NOTHING`, userID, heroIDs, heroUnlocked); err != nil {
		return err
	}
	skillIDsAll, skillUnlocked := make([]string, 0, len(content.Skills)), make([]bool, 0, len(content.Skills))
	for _, skill := range content.Skills {
		skillIDsAll = append(skillIDsAll, skill.ID)
		skillUnlocked = append(skillUnlocked, skill.UnlockStage <= 1)
	}
	if _, err := db.Exec(ctx, `INSERT INTO realmguard_user_skills(user_id,skill_id,unlocked)
		SELECT $1,skill.id,skill.unlocked FROM unnest($2::text[],$3::boolean[]) AS skill(id,unlocked)
		ON CONFLICT DO NOTHING`, userID, skillIDsAll, skillUnlocked); err != nil {
		return err
	}
	heroID := ""
	for _, hero := range content.Heroes {
		if hero.UnlockStage <= 1 {
			heroID = hero.ID
			break
		}
	}
	skillIDs := []string{}
	for _, skill := range content.Skills {
		if skill.UnlockStage <= 1 {
			skillIDs = append(skillIDs, skill.ID)
		}
	}
	_, err := db.Exec(ctx, `INSERT INTO realmguard_user_loadouts(user_id,hero_id,skill_ids) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, userID, heroID, skillIDs)
	if err != nil {
		return err
	}
	var savedHero string
	var savedSkills, unlockedHeroes, unlockedSkills []string
	if err := db.QueryRow(ctx, `SELECT l.hero_id,l.skill_ids,
		COALESCE((SELECT array_agg(hero_id) FROM realmguard_user_heroes WHERE user_id=$1 AND unlocked),'{}'::text[]),
		COALESCE((SELECT array_agg(skill_id) FROM realmguard_user_skills WHERE user_id=$1 AND unlocked),'{}'::text[])
		FROM realmguard_user_loadouts l WHERE l.user_id=$1`, userID).Scan(&savedHero, &savedSkills, &unlockedHeroes, &unlockedSkills); err != nil {
		return err
	}
	validHeroes, validSkills := map[string]bool{}, map[string]bool{}
	for _, hero := range content.Heroes {
		validHeroes[hero.ID] = true
	}
	for _, skill := range content.Skills {
		validSkills[skill.ID] = true
	}
	if !validHeroes[savedHero] || !slices.Contains(unlockedHeroes, savedHero) {
		savedHero = heroID
		for _, candidate := range content.Heroes {
			if validHeroes[candidate.ID] && slices.Contains(unlockedHeroes, candidate.ID) {
				savedHero = candidate.ID
				break
			}
		}
	}
	sanitizedSkills := []string{}
	for _, skillID := range savedSkills {
		if len(sanitizedSkills) == 3 {
			break
		}
		if validSkills[skillID] && slices.Contains(unlockedSkills, skillID) && !slices.Contains(sanitizedSkills, skillID) {
			sanitizedSkills = append(sanitizedSkills, skillID)
		}
	}
	if len(sanitizedSkills) == 0 {
		for _, skillID := range skillIDs {
			if slices.Contains(unlockedSkills, skillID) {
				sanitizedSkills = append(sanitizedSkills, skillID)
			}
		}
	}
	if _, err := db.Exec(ctx, `UPDATE realmguard_user_loadouts SET hero_id=$2,skill_ids=$3,updated_at=CASE WHEN hero_id<>$2 OR skill_ids<>$3 THEN now() ELSE updated_at END WHERE user_id=$1`, userID, savedHero, sanitizedSkills); err != nil {
		return err
	}
	firstCampaign := realmGuardFirstCampaign(content)
	if firstCampaign == nil {
		return fmt.Errorf("RealmGuard content has no campaign stage")
	}
	if _, err := db.Exec(ctx, `INSERT INTO realmguard_user_progress(user_id,stage_id,difficulty,unlocked)
		SELECT $1,$2,d.difficulty,true FROM unnest(ARRAY['casual','normal','veteran']) AS d(difficulty)
		ON CONFLICT DO NOTHING`, userID, firstCampaign.ID); err != nil {
		return err
	}
	return nil
}

func (s *Server) ensureRealmGuardUser(ctx context.Context, userID uuid.UUID, content realmGuardDecodedContent) error {
	return ensureRealmGuardUserWith(ctx, s.DB, userID, content)
}

func realmGuardCampaigns(content realmGuardDecodedContent) []realmGuardStageDefinition {
	stages := make([]realmGuardStageDefinition, 0, len(content.Stages))
	for _, stage := range content.Stages {
		if stage.Mode == "campaign" {
			stages = append(stages, stage)
		}
	}
	slices.SortFunc(stages, func(a, b realmGuardStageDefinition) int { return a.Number - b.Number })
	return stages
}

func realmGuardFirstCampaign(content realmGuardDecodedContent) *realmGuardStageDefinition {
	stages := realmGuardCampaigns(content)
	if len(stages) == 0 {
		return nil
	}
	return &stages[0]
}

func (s *Server) realmGuardProgressData(ctx context.Context, p Principal, version realmGuardVersionRecord, content realmGuardDecodedContent) (map[string]any, error) {
	if err := s.ensureRealmGuardUser(ctx, p.UserID, content); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `SELECT stage_id,difficulty,unlocked,completed,stars,best_score,best_duration_ms,best_hero_id,wins,attempts,total_playtime_ms,updated_at FROM realmguard_user_progress WHERE user_id=$1 ORDER BY stage_id,difficulty`, p.UserID)
	if err != nil {
		return nil, err
	}
	items := []map[string]any{}
	totalPlaytime := int64(0)
	type stageAggregate struct {
		Stars        int
		BestScore    int64
		Difficulties []string
	}
	aggregates := map[string]*stageAggregate{}
	for rows.Next() {
		var stageID, difficulty, heroID string
		var unlocked, completed bool
		var stars, wins, attempts int
		var score, playtime int64
		var duration *int64
		var updated time.Time
		if err := rows.Scan(&stageID, &difficulty, &unlocked, &completed, &stars, &score, &duration, &heroID, &wins, &attempts, &playtime, &updated); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, map[string]any{"stage_id": stageID, "difficulty": difficulty, "unlocked": unlocked, "completed": completed, "stars": stars, "best_score": score, "best_duration_ms": duration, "best_hero_id": heroID, "wins": wins, "attempts": attempts, "total_playtime_ms": playtime, "updated_at": updated})
		totalPlaytime += playtime
		aggregate := aggregates[stageID]
		if aggregate == nil {
			aggregate = &stageAggregate{}
			aggregates[stageID] = aggregate
		}
		if stars > aggregate.Stars {
			aggregate.Stars = stars
		}
		if score > aggregate.BestScore {
			aggregate.BestScore = score
		}
		if completed {
			aggregate.Difficulties = append(aggregate.Difficulties, difficulty)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	stageItems := []map[string]any{}
	totalStars, highestStage, unlockedStage, lastCampaign := 0, 0, 0, 0
	for _, stage := range content.Stages {
		if stage.Mode != "campaign" {
			continue
		}
		lastCampaign = max(lastCampaign, stage.Number)
		aggregate := aggregates[stage.ID]
		if aggregate == nil {
			aggregate = &stageAggregate{}
		}
		totalStars += aggregate.Stars
		if len(aggregate.Difficulties) > 0 && stage.Number > highestStage {
			highestStage = stage.Number
		}
		if aggregateProgressUnlocked(items, stage.ID) && stage.Number > unlockedStage {
			unlockedStage = stage.Number
		}
		stageItems = append(stageItems, map[string]any{"stage_id": stage.ID, "stars": aggregate.Stars, "best_score": aggregate.BestScore, "difficulties": aggregate.Difficulties})
	}
	campaignComplete := lastCampaign > 0 && highestStage == lastCampaign
	endlessUnlocked := campaignComplete
	if campaignComplete {
		unlockedStage = lastCampaign + 1
	}
	heroRows, err := s.DB.Query(ctx, `SELECT hero_id,unlocked,level,xp,updated_at FROM realmguard_user_heroes WHERE user_id=$1 ORDER BY hero_id`, p.UserID)
	if err != nil {
		return nil, err
	}
	heroes := []map[string]any{}
	heroLevels := map[string]int{}
	for heroRows.Next() {
		var id string
		var unlocked bool
		var level int
		var xp int64
		var updated time.Time
		if err := heroRows.Scan(&id, &unlocked, &level, &xp, &updated); err != nil {
			heroRows.Close()
			return nil, err
		}
		heroes = append(heroes, map[string]any{"hero_id": id, "unlocked": unlocked, "level": level, "xp": xp, "updated_at": updated})
		heroLevels[id] = level
	}
	if err := heroRows.Err(); err != nil {
		return nil, err
	}
	heroRows.Close()
	skillRows, err := s.DB.Query(ctx, `SELECT skill_id,unlocked,level,updated_at FROM realmguard_user_skills WHERE user_id=$1 ORDER BY skill_id`, p.UserID)
	if err != nil {
		return nil, err
	}
	skills := []map[string]any{}
	for skillRows.Next() {
		var id string
		var unlocked bool
		var level int
		var updated time.Time
		if err := skillRows.Scan(&id, &unlocked, &level, &updated); err != nil {
			skillRows.Close()
			return nil, err
		}
		skills = append(skills, map[string]any{"skill_id": id, "unlocked": unlocked, "level": level, "updated_at": updated})
	}
	if err := skillRows.Err(); err != nil {
		return nil, err
	}
	skillRows.Close()
	var heroID string
	var skillIDs []string
	var settings json.RawMessage
	var loadoutUpdated time.Time
	if err := s.DB.QueryRow(ctx, `SELECT hero_id,skill_ids,settings,updated_at FROM realmguard_user_loadouts WHERE user_id=$1`, p.UserID).Scan(&heroID, &skillIDs, &settings, &loadoutUpdated); err != nil {
		return nil, err
	}
	return map[string]any{
		"version": realmGuardVersionJSON(version), "items": items, "stages": stageItems, "heroes": heroes, "skills": skills,
		"hero_levels": heroLevels, "loadout": map[string]any{"hero_id": heroID, "skill_ids": skillIDs, "settings": settings, "updated_at": loadoutUpdated},
		"total_stars": totalStars, "highest_stage": highestStage, "unlocked_stage": unlockedStage, "total_playtime_ms": totalPlaytime,
		"campaign_complete": campaignComplete, "campaign_completed": campaignComplete, "endless_unlocked": endlessUnlocked,
	}, nil
}

func aggregateProgressUnlocked(items []map[string]any, stageID string) bool {
	for _, item := range items {
		if item["stage_id"] == stageID && item["unlocked"] == true {
			return true
		}
	}
	return false
}

func (s *Server) realmGuardProgress(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	version, err := s.loadRealmGuardPublished(r.Context())
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	content, err := decodeRealmGuardContent(version.RawContent)
	if err != nil {
		writeError(w, 500, "invalid_published_content", err.Error())
		return
	}
	data, err := s.realmGuardProgressData(r.Context(), p, version, content)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	writeJSON(w, 200, data)
}

type realmGuardProgressInput struct {
	HeroID     string           `json:"hero_id"`
	SkillIDs   *[]string        `json:"skill_ids,omitempty"`
	Settings   *json.RawMessage `json:"settings,omitempty"`
	StageID    string           `json:"stage_id,omitempty"`
	Difficulty string           `json:"difficulty,omitempty"`
	HeroLevel  *int             `json:"hero_level,omitempty"`
	Stars      *int             `json:"stars,omitempty"`
	Score      *int64           `json:"score,omitempty"`
	Unlocks    json.RawMessage  `json:"unlocks,omitempty"`
}

func (s *Server) putRealmGuardProgress(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	var in realmGuardProgressInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.HeroLevel != nil || in.Stars != nil || in.Score != nil || len(in.Unlocks) > 0 || in.StageID != "" || in.Difficulty != "" {
		writeError(w, 400, "authoritative_progress", "stage progress, hero level, stars, score, and unlocks can only be changed by a verified result")
		return
	}
	version, err := s.loadRealmGuardPublished(r.Context())
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	content, err := decodeRealmGuardContent(version.RawContent)
	if err != nil {
		writeError(w, 500, "invalid_published_content", err.Error())
		return
	}
	if err := s.ensureRealmGuardUser(r.Context(), p.UserID, content); err != nil {
		s.dbError(w, r, err)
		return
	}
	var currentHero string
	var currentSkills []string
	var currentSettings json.RawMessage
	if err := s.DB.QueryRow(r.Context(), `SELECT hero_id,skill_ids,settings FROM realmguard_user_loadouts WHERE user_id=$1`, p.UserID).Scan(&currentHero, &currentSkills, &currentSettings); err != nil {
		s.dbError(w, r, err)
		return
	}
	in.HeroID = strings.TrimSpace(in.HeroID)
	if in.HeroID == "" {
		in.HeroID = currentHero
	}
	skillIDs := currentSkills
	if in.SkillIDs != nil {
		skillIDs = *in.SkillIDs
	}
	if in.HeroID == "" || len(skillIDs) > 3 {
		writeError(w, 400, "invalid_loadout", "hero_id and no more than three skills are required")
		return
	}
	var heroUnlocked bool
	heroExists := false
	for _, hero := range content.Heroes {
		if hero.ID == in.HeroID {
			heroExists = true
			break
		}
	}
	if !heroExists {
		writeError(w, 400, "invalid_loadout", "the selected hero is not present in the active content")
		return
	}
	if err := s.DB.QueryRow(r.Context(), `SELECT unlocked FROM realmguard_user_heroes WHERE user_id=$1 AND hero_id=$2`, p.UserID, in.HeroID).Scan(&heroUnlocked); err != nil || !heroUnlocked {
		writeError(w, 403, "hero_locked", "the selected hero is not unlocked")
		return
	}
	seen := map[string]bool{}
	contentSkillIDs := map[string]bool{}
	for _, skill := range content.Skills {
		contentSkillIDs[skill.ID] = true
	}
	for _, skillID := range skillIDs {
		if skillID == "" || seen[skillID] || !contentSkillIDs[skillID] {
			writeError(w, 400, "invalid_loadout", "skill IDs must be non-empty and unique")
			return
		}
		seen[skillID] = true
		var unlocked bool
		if err := s.DB.QueryRow(r.Context(), `SELECT unlocked FROM realmguard_user_skills WHERE user_id=$1 AND skill_id=$2`, p.UserID, skillID).Scan(&unlocked); err != nil || !unlocked {
			writeError(w, 403, "skill_locked", "one or more selected skills are not unlocked")
			return
		}
	}
	settings := currentSettings
	if in.Settings != nil {
		settings = *in.Settings
		var settingObject map[string]any
		if len(settings) > 64<<10 || json.Unmarshal(settings, &settingObject) != nil || settingObject == nil {
			writeError(w, 400, "invalid_settings", "settings must be a JSON object no larger than 64 KiB")
			return
		}
	}
	_, err = s.DB.Exec(r.Context(), `UPDATE realmguard_user_loadouts SET hero_id=$2,skill_ids=$3,settings=$4,updated_at=now() WHERE user_id=$1`, p.UserID, in.HeroID, skillIDs, settings)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	s.audit(r, "realmguard.loadout.update", "realmguard_loadout", p.UserID.String(), map[string]any{"hero_id": in.HeroID, "skill_ids": skillIDs})
	data, err := s.realmGuardProgressData(r.Context(), p, version, content)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	writeJSON(w, 200, data)
}

type realmGuardResultInput struct {
	GameID          string          `json:"game_id,omitempty"`
	SessionID       uuid.UUID       `json:"session_id"`
	SessionToken    string          `json:"session_token"`
	StageID         string          `json:"stage_id"`
	Mode            string          `json:"mode"`
	Difficulty      string          `json:"difficulty"`
	DurationMS      int64           `json:"duration_ms,omitempty"`
	DurationSeconds float64         `json:"duration_seconds,omitempty"`
	RemainingLives  *int            `json:"remaining_lives,omitempty"`
	Lives           *int            `json:"lives,omitempty"`
	RemainingGold   *int            `json:"remaining_gold,omitempty"`
	Gold            *int            `json:"gold,omitempty"`
	EarnedGold      int             `json:"earned_gold"`
	SpentGold       int             `json:"spent_gold"`
	SoldGold        int             `json:"sold_gold"`
	Kills           int             `json:"kills"`
	Escaped         int             `json:"escaped"`
	Spawned         int             `json:"spawned"`
	DefeatedByEnemy map[string]int  `json:"defeated_by_enemy"`
	EscapedByEnemy  map[string]int  `json:"escaped_by_enemy"`
	SpawnedByEnemy  map[string]int  `json:"spawned_by_enemy"`
	WavesCompleted  int             `json:"waves_completed,omitempty"`
	Waves           int             `json:"waves,omitempty"`
	HeroID          string          `json:"hero_id"`
	HeroLevel       int             `json:"hero_level,omitempty"`
	ContentVersion  string          `json:"content_version"`
	BalanceVersion  string          `json:"balance_version"`
	StageVersion    string          `json:"stage_version"`
	AssetVersion    string          `json:"asset_version"`
	Victory         *bool           `json:"victory,omitempty"`
	Proof           string          `json:"proof,omitempty"`
	Events          json.RawMessage `json:"events,omitempty"`
}

type realmGuardScoreBreakdown struct {
	LivesBonus       int64 `json:"lives_bonus"`
	GoldBonus        int64 `json:"gold_bonus"`
	ClearTimeBonus   int64 `json:"clear_time_bonus"`
	EndlessWaveBonus int64 `json:"endless_wave_bonus"`
	DifficultyBonus  int64 `json:"difficulty_bonus"`
	Total            int64 `json:"total"`
}

func calculateRealmGuardScore(stage realmGuardStageDefinition, balance realmGuardBalanceDefinition, mode, difficulty string, durationMS int64, lives, gold, waves int, cleared bool) (int64, int, realmGuardScoreBreakdown) {
	breakdown := realmGuardScoreBreakdown{LivesBonus: realmGuardScoreProduct(int64(lives), 1000), GoldBonus: realmGuardScoreProduct(int64(gold), 10)}
	breakdown.DifficultyBonus = balance.Difficulties[difficulty].DifficultyBonus
	if mode == "campaign" && cleared && balance.ClearTimeBonusDivisor > 0 {
		breakdown.ClearTimeBonus = max(int64(0), balance.ClearTimeTargetMS-durationMS) / balance.ClearTimeBonusDivisor
	}
	if mode == "endless" {
		breakdown.EndlessWaveBonus = realmGuardScoreProduct(int64(waves), balance.EndlessWaveBonus)
	}
	breakdown.Total = realmGuardScoreTotal(breakdown.LivesBonus, breakdown.GoldBonus, breakdown.ClearTimeBonus, breakdown.EndlessWaveBonus, breakdown.DifficultyBonus)
	stars := 0
	if mode == "campaign" && cleared {
		switch {
		case lives >= 18:
			stars = 3
		case lives >= 10:
			stars = 2
		case lives >= 1:
			stars = 1
		}
	}
	return breakdown.Total, stars, breakdown
}

func realmGuardScoreProduct(left, right int64) int64 {
	const maximum = int64(1<<63 - 1)
	if left <= 0 || right <= 0 {
		return 0
	}
	if left > maximum/right {
		return maximum
	}
	return left * right
}

func realmGuardScoreTotal(values ...int64) int64 {
	const maximum = int64(1<<63 - 1)
	total := int64(0)
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if total > maximum-value {
			return maximum
		}
		total += value
	}
	return total
}

type realmGuardWaveBudget struct {
	BaseSpawns       int
	MaxSpawns        int
	Rewards          int
	EnemyCounts      map[string]int
	RewardCounts     map[int]int
	LifeDamageCounts map[int]int
	MinLifeDamage    int
	MaxLifeDamage    int
}

func realmGuardWaveCapacity(content realmGuardDecodedContent, stageID string, completed int, includeCurrent bool) realmGuardWaveBudget {
	stageWaves := make([]realmGuardWaveDefinition, 0)
	for _, wave := range content.Waves {
		if wave.StageID == stageID {
			stageWaves = append(stageWaves, wave)
		}
	}
	slices.SortFunc(stageWaves, func(a, b realmGuardWaveDefinition) int { return a.Number - b.Number })
	if len(stageWaves) == 0 || completed < 0 {
		return realmGuardWaveBudget{}
	}
	definitions := map[string]realmGuardEnemyDefinition{}
	for _, enemy := range append(append([]realmGuardEnemyDefinition{}, content.Enemies...), content.Bosses...) {
		definitions[enemy.ID] = enemy
	}
	count := completed
	if includeCurrent {
		count++
	}
	budget := realmGuardWaveBudget{EnemyCounts: map[string]int{}, RewardCounts: map[int]int{}, LifeDamageCounts: map[int]int{}}
	addPotential := func(enemyID string, amount int) {
		definition, ok := definitions[enemyID]
		if !ok || amount <= 0 {
			return
		}
		budget.MaxSpawns += amount
		budget.EnemyCounts[enemyID] += amount
		budget.RewardCounts[definition.Reward] += amount
		budget.LifeDamageCounts[definition.LifeDamage] += amount
		if budget.MinLifeDamage == 0 || definition.LifeDamage < budget.MinLifeDamage {
			budget.MinLifeDamage = definition.LifeDamage
		}
		budget.MaxLifeDamage = max(budget.MaxLifeDamage, definition.LifeDamage)
	}
	for index := 0; index < count; index++ {
		wave := stageWaves[index%len(stageWaves)]
		cycle := index / len(stageWaves)
		for _, entry := range wave.Entries {
			spawnCount := entry.Count + cycle*2
			budget.BaseSpawns += spawnCount
			addPotential(entry.Enemy, spawnCount)
			if slices.Contains(definitions[entry.Enemy].Traits, "splitting") || slices.Contains(entry.Modifiers, "splitting") {
				addPotential("mireling", spawnCount*2)
			}
			switch entry.Enemy {
			case "hollow_king":
				addPotential("veilrunner", spawnCount*5)
			case "timewyrm":
				addPotential("glintfox", spawnCount*3)
			}
		}
		if index < completed {
			budget.Rewards += wave.Reward
		}
	}
	return budget
}

func realmGuardRewardBounds(kills int, counts map[int]int) (minimum, maximum int) {
	rewards := make([]int, 0, len(counts))
	for reward := range counts {
		rewards = append(rewards, reward)
	}
	slices.Sort(rewards)
	remaining := kills
	for _, reward := range rewards {
		used := min(remaining, counts[reward])
		minimum += used * reward
		remaining -= used
		if remaining == 0 {
			break
		}
	}
	remaining = kills
	for index := len(rewards) - 1; index >= 0; index-- {
		reward := rewards[index]
		used := min(remaining, counts[reward])
		maximum += used * reward
		remaining -= used
		if remaining == 0 {
			break
		}
	}
	return minimum, maximum
}

func realmGuardLifeDamageReachable(escaped, minimumDamage, maximumDamage int, counts map[int]int) bool {
	if escaped == 0 {
		return minimumDamage <= 0 && maximumDamage >= 0
	}
	if escaped < 0 || minimumDamage < 0 || maximumDamage < minimumDamage || maximumDamage > 1000 {
		return false
	}
	reachable := make([][]bool, escaped+1)
	for index := range reachable {
		reachable[index] = make([]bool, maximumDamage+1)
	}
	reachable[0][0] = true
	for damage, available := range counts {
		available = min(available, escaped)
		for group := 1; available > 0; group *= 2 {
			used := min(group, available)
			available -= used
			for count := escaped; count >= used; count-- {
				for total := maximumDamage; total >= used*damage; total-- {
					if reachable[count-used][total-used*damage] {
						reachable[count][total] = true
					}
				}
			}
		}
	}
	for damage := minimumDamage; damage <= maximumDamage; damage++ {
		if reachable[escaped][damage] {
			return true
		}
	}
	return false
}

func realmGuardHeroLevel(xp int64, thresholds []int64) int {
	level := 1
	for index, threshold := range thresholds {
		if xp >= threshold {
			level = index + 1
		}
	}
	return min(100, max(1, level))
}

func realmGuardBattleHeroLevel(kills int, thresholds []int64) int {
	level := 1
	xp := int64(kills)
	for level < 10 && level < len(thresholds) && xp >= thresholds[level] {
		level++
	}
	return level
}

func realmGuardInitialGold(stage realmGuardStageDefinition, balance realmGuardBalanceDefinition, difficulty string) int {
	return int(math.Round(float64(stage.StartingGold) * balance.Difficulties[difficulty].Gold))
}

func realmGuardMinimumDurationMS(wavesCompleted int, cleared bool, balance realmGuardBalanceDefinition) int64 {
	wavesReached := wavesCompleted
	if !cleared {
		wavesReached++
	}
	return int64(wavesReached) * balance.MinWaveDurationMS
}

func realmGuardMaxSoldGold(spent int, towers []realmGuardTowerDefinition, refundRate float64) int {
	if spent <= 0 || refundRate <= 0 {
		return 0
	}
	minimumCost := spent
	for _, tower := range towers {
		if tower.Cost > 0 && tower.Cost < minimumCost {
			minimumCost = tower.Cost
		}
	}
	if minimumCost < 1 {
		minimumCost = 1
	}
	// Each individual sale is rounded by the engine. At most spent/minCost
	// independently rounded sales are possible, so half a gold per sale is
	// the conservative rounding allowance.
	transactions := spent/minimumCost + 1
	return int(math.Ceil(float64(spent)*refundRate)) + (transactions+1)/2
}

func validateRealmGuardCombatOutcome(cleared bool, stageLives, remainingLives, kills, escaped, spawned int, budget realmGuardWaveBudget) error {
	if remainingLives < 0 || remainingLives > stageLives || escaped > 0 && remainingLives > stageLives-escaped {
		return rejectRealmGuardResult(422, "invalid_lives", "remaining lives are inconsistent with the stage and escaped enemies")
	}
	// Bound every counter before any addition, multiplication, or reachability
	// allocation. This also avoids integer overflow in kills+escaped for hostile
	// JSON values while keeping partially spawned defeat waves valid.
	if kills > budget.MaxSpawns || escaped > budget.MaxSpawns || spawned > budget.MaxSpawns || kills > spawned || escaped > spawned-kills {
		return rejectRealmGuardResult(422, "invalid_kills", "spawned, killed, or escaped enemy counts exceed the pinned waves")
	}
	if !cleared {
		if remainingLives != 0 || escaped < 1 {
			return rejectRealmGuardResult(422, "invalid_defeat", "a submitted defeat requires zero remaining lives and at least one escaped enemy")
		}
		if budget.MaxLifeDamage < 1 || escaped*budget.MaxLifeDamage < stageLives || escaped*budget.MinLifeDamage > stageLives+budget.MaxLifeDamage-1 || !realmGuardLifeDamageReachable(escaped, stageLives, stageLives+budget.MaxLifeDamage-1, budget.LifeDamageCounts) {
			return rejectRealmGuardResult(422, "invalid_lives", "escaped enemy count cannot account for the pinned stage life loss")
		}
	} else if !realmGuardLifeDamageReachable(escaped, stageLives-remainingLives, stageLives-remainingLives, budget.LifeDamageCounts) {
		return rejectRealmGuardResult(422, "invalid_lives", "remaining lives do not match a reachable set of escaped pinned enemies")
	}
	if cleared && (spawned < budget.BaseSpawns || kills != spawned-escaped) {
		return rejectRealmGuardResult(422, "invalid_kills", "a cleared campaign must spawn every configured enemy and leave no enemy unresolved")
	}
	return nil
}

func (s *Server) submitRealmGuardResult(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	var in realmGuardResultInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.DurationMS == 0 && in.DurationSeconds > 0 {
		in.DurationMS = int64(in.DurationSeconds * 1000)
	}
	if in.DurationMS <= 0 || in.SessionID == uuid.Nil || in.SessionToken == "" {
		writeError(w, 400, "invalid_result", "session_id, session_token, and a positive duration are required")
		return
	}
	if in.RemainingLives == nil {
		in.RemainingLives = in.Lives
	}
	if in.RemainingGold == nil {
		in.RemainingGold = in.Gold
	}
	if in.RemainingLives == nil || in.RemainingGold == nil {
		writeError(w, 400, "invalid_result", "remaining_lives and remaining_gold are required")
		return
	}
	if in.WavesCompleted == 0 && in.Waves > 0 {
		in.WavesCompleted = in.Waves
	}
	if !slices.Contains([]string{"campaign", "endless"}, in.Mode) || !slices.Contains([]string{"casual", "normal", "veteran"}, in.Difficulty) {
		writeError(w, 400, "invalid_result", "mode or difficulty is invalid")
		return
	}
	if in.StageID == "" || in.HeroID == "" || in.Kills < 0 || in.Escaped < 0 || in.Spawned < 0 || in.WavesCompleted < 0 || in.EarnedGold < 0 || in.SpentGold < 0 || in.SoldGold < 0 || *in.RemainingGold < 0 || len(in.Proof) > 512 {
		writeError(w, 400, "invalid_result", "result counters or identifiers are invalid")
		return
	}
	if len(in.Events) == 0 {
		in.Events = []byte("[]")
	}
	if len(in.Events) > 256<<10 || !json.Valid(in.Events) {
		writeError(w, 400, "invalid_events", "events must be valid JSON no larger than 256 KiB")
		return
	}
	if err := s.submitRealmGuardResultTx(r.Context(), p, in, w, r); err != nil {
		if apiErr, ok := err.(realmGuardResultError); ok {
			s.audit(r, "realmguard.result.reject", "game_session", in.SessionID.String(), map[string]any{"code": apiErr.Code, "reason": apiErr.Message})
			writeError(w, apiErr.Status, apiErr.Code, apiErr.Message)
			return
		}
		s.Log.Error("RealmGuard result transaction failed", "session_id", in.SessionID, "user_id", p.UserID, "error", err)
		s.dbError(w, r, err)
	}
}

type realmGuardResultError struct {
	Status  int
	Code    string
	Message string
}

func (e realmGuardResultError) Error() string { return e.Message }

func rejectRealmGuardResult(status int, code, message string) error {
	return realmGuardResultError{Status: status, Code: code, Message: message}
}

func (s *Server) submitRealmGuardResultTx(ctx context.Context, p Principal, in realmGuardResultInput, w http.ResponseWriter, r *http.Request) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	hash := sha256.Sum256([]byte(in.SessionToken))
	var gameID uuid.UUID
	var seasonID *uuid.UUID
	var started time.Time
	var sessionStatus string
	var version realmGuardVersionRecord
	err = tx.QueryRow(ctx, `SELECT gs.game_id,gs.season_id,gs.started_at,gs.status,rv.`+strings.ReplaceAll(realmGuardVersionColumns, ",", ",rv.")+`
		FROM game_sessions gs JOIN games g ON g.id=gs.game_id JOIN realmguard_content_versions rv ON rv.id=gs.realmguard_content_version_id
		WHERE gs.id=$1 AND gs.user_id=$2 AND gs.session_token_hash=$3 AND g.slug='realmguard' FOR UPDATE OF gs`, in.SessionID, p.UserID, hash[:]).Scan(
		&gameID, &seasonID, &started, &sessionStatus, &version.ID, &version.VersionNo, &version.Label, &version.Status, &version.ContentVersion, &version.StageVersion, &version.BalanceVersion, &version.AssetVersion, &version.Checksum, &version.Notes, &version.RawContent, &version.CreatedBy, &version.ApprovedBy, &version.CreatedAt, &version.TestedAt, &version.RequestedAt, &version.ApprovedAt, &version.ReviewComment, &version.ReviewedAt, &version.PublishedAt, &version.UpdatedAt)
	if err != nil {
		return rejectRealmGuardResult(409, "invalid_session", "RealmGuard session, token, or pinned content version is invalid")
	}
	if in.GameID != "" && in.GameID != realmGuardSlug && in.GameID != gameID.String() {
		return rejectRealmGuardResult(409, "game_mismatch", "game_id does not identify the RealmGuard game pinned to this session")
	}
	content, err := decodeRealmGuardContent(version.RawContent)
	if err != nil {
		return err
	}
	if sessionStatus == "finished" {
		return s.writeStoredRealmGuardResult(ctx, tx, p.UserID, in.SessionID, version, w)
	}
	if sessionStatus != "active" {
		return rejectRealmGuardResult(409, "session_finished", "the RealmGuard session cannot accept a result")
	}
	if err := ensureRealmGuardUserWith(ctx, tx, p.UserID, content); err != nil {
		return err
	}
	var stage *realmGuardStageDefinition
	for index := range content.Stages {
		if content.Stages[index].ID == in.StageID {
			stage = &content.Stages[index]
			break
		}
	}
	if stage == nil || stage.Mode != in.Mode {
		return rejectRealmGuardResult(422, "invalid_stage", "stage_id and mode do not match the pinned content")
	}
	if in.ContentVersion != version.ContentVersion || in.BalanceVersion != version.BalanceVersion || in.StageVersion != stage.Version || in.AssetVersion != version.AssetVersion {
		return rejectRealmGuardResult(409, "version_mismatch", "result versions do not match the session-pinned immutable snapshot")
	}
	if _, ok := content.Balance.Difficulties[in.Difficulty]; !ok {
		return rejectRealmGuardResult(422, "invalid_difficulty", "difficulty is not present in the pinned balance")
	}
	if err := ensureRealmGuardStageUnlocked(ctx, tx, p.UserID, *stage, content); err != nil {
		return err
	}
	serverElapsed := s.Now().Sub(started).Milliseconds()
	if in.DurationMS > serverElapsed+content.Balance.DurationToleranceMS {
		return rejectRealmGuardResult(422, "invalid_duration", "active battle duration exceeds session wall time")
	}
	stageWaveCount := 0
	for _, wave := range content.Waves {
		if wave.StageID == stage.ID {
			stageWaveCount++
		}
	}
	if stage.Mode == "campaign" && in.WavesCompleted > stageWaveCount || stage.Mode == "endless" && in.WavesCompleted > realmGuardMaxEndlessWaves {
		return rejectRealmGuardResult(422, "invalid_waves", "completed waves exceed the stage limit")
	}
	cleared := stage.Mode == "campaign" && in.WavesCompleted == stageWaveCount && *in.RemainingLives > 0
	if in.DurationMS < realmGuardMinimumDurationMS(in.WavesCompleted, cleared, content.Balance) {
		return rejectRealmGuardResult(422, "invalid_duration", "active battle duration is too short for the reached waves")
	}
	if in.Victory != nil && *in.Victory != cleared {
		return rejectRealmGuardResult(422, "victory_mismatch", "victory does not match server-derived stage completion")
	}
	waveBudget := realmGuardWaveCapacity(content, stage.ID, in.WavesCompleted, !cleared)
	if err := validateRealmGuardCombatOutcome(cleared, stage.Lives, *in.RemainingLives, in.Kills, in.Escaped, in.Spawned, waveBudget); err != nil {
		return err
	}
	telemetryRecords, err := s.loadRealmGuardTelemetryRecords(ctx, tx, in.SessionID)
	if err != nil {
		return err
	}
	attestation, err := validateRealmGuardTelemetryAttestation(telemetryRecords, started, s.Now(), *stage, content, version, in, cleared)
	if err != nil {
		return err
	}
	minimumKillGold, maximumKillGold := realmGuardRewardBounds(in.Kills, waveBudget.RewardCounts)
	minimumEarned := minimumKillGold + waveBudget.Rewards + attestation.EarlyBonus
	maximumEarned := maximumKillGold + waveBudget.Rewards + attestation.EarlyBonus
	initialGold := realmGuardInitialGold(*stage, content.Balance, in.Difficulty)
	if in.EarnedGold < minimumEarned || in.EarnedGold > maximumEarned || in.SoldGold > realmGuardMaxSoldGold(in.SpentGold, content.Towers, content.Balance.SellRefundRate) || *in.RemainingGold != initialGold+in.EarnedGold+in.SoldGold-in.SpentGold {
		return rejectRealmGuardResult(422, "invalid_gold", "gold balance or earned/sold gold exceeds the pinned economy budget")
	}
	var accountHeroLevel int
	var heroXP int64
	var heroUnlocked bool
	heroExists := false
	for _, hero := range content.Heroes {
		if hero.ID == in.HeroID {
			heroExists = true
			break
		}
	}
	if !heroExists {
		return rejectRealmGuardResult(422, "invalid_hero", "hero is not present in the pinned content")
	}
	if err := tx.QueryRow(ctx, `SELECT unlocked,level,xp FROM realmguard_user_heroes WHERE user_id=$1 AND hero_id=$2 FOR UPDATE`, p.UserID, in.HeroID).Scan(&heroUnlocked, &accountHeroLevel, &heroXP); err != nil || !heroUnlocked {
		return rejectRealmGuardResult(422, "invalid_hero", "hero is unknown or locked")
	}
	battleHeroLevel := realmGuardBattleHeroLevel(in.Kills, content.Balance.HeroLevelXP)
	if in.HeroLevel < 1 || in.HeroLevel > 10 || in.HeroLevel != battleHeroLevel {
		return rejectRealmGuardResult(422, "hero_level_mismatch", "hero_level must match the battle level derived from verified kills")
	}
	score, stars, breakdown := calculateRealmGuardScore(*stage, content.Balance, in.Mode, in.Difficulty, in.DurationMS, *in.RemainingLives, *in.RemainingGold, in.WavesCompleted, cleared)
	breakdownJSON, _ := json.Marshal(breakdown)
	attestationJSON, _ := json.Marshal(attestation)
	proofPayload, _ := json.Marshal(map[string]any{"method": realmGuardVerificationMethod, "session_id": in.SessionID, "user_id": p.UserID, "digest": attestation.Digest, "score": score, "stars": stars})
	if s.Secrets == nil {
		return fmt.Errorf("server proof encryption is unavailable")
	}
	serverProof, err := s.Secrets.Seal(string(proofPayload))
	if err != nil {
		return fmt.Errorf("mint RealmGuard server proof: %w", err)
	}
	var resultID, scoreID uuid.UUID
	err = tx.QueryRow(ctx, `INSERT INTO realmguard_results(session_id,user_id,content_version_id,stage_id,mode,difficulty,duration_ms,remaining_lives,remaining_gold,earned_gold,spent_gold,sold_gold,kills,escaped,spawned,waves_completed,hero_id,hero_level,score,stars,score_breakdown,proof,events,verification_method,attestation)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,'[]'::jsonb,$23,$24) RETURNING id`, in.SessionID, p.UserID, version.ID, stage.ID, in.Mode, in.Difficulty, in.DurationMS, *in.RemainingLives, *in.RemainingGold, in.EarnedGold, in.SpentGold, in.SoldGold, in.Kills, in.Escaped, in.Spawned, in.WavesCompleted, in.HeroID, battleHeroLevel, score, stars, breakdownJSON, "server:"+serverProof, realmGuardVerificationMethod, attestationJSON).Scan(&resultID)
	if err != nil {
		return err
	}
	metadata, _ := json.Marshal(map[string]any{"realmguard_result_id": resultID, "stage_id": stage.ID, "mode": in.Mode, "difficulty": in.Difficulty, "hero_id": in.HeroID, "battle_hero_level": battleHeroLevel, "account_hero_level": accountHeroLevel, "stars": stars, "content_version": version.ContentVersion, "stage_content_version": version.StageVersion, "stage_version": stage.Version, "balance_version": version.BalanceVersion, "asset_version": version.AssetVersion, "score_breakdown": breakdown, "verification_method": realmGuardVerificationMethod, "attestation_digest": attestation.Digest})
	err = tx.QueryRow(ctx, `INSERT INTO scores(user_id,game_id,session_id,season_id,score,metadata,verified,rejection_reason) VALUES($1,$2,$3,$4,$5,$6,true,'') RETURNING id`, p.UserID, gameID, in.SessionID, seasonID, score, metadata).Scan(&scoreID)
	if err != nil {
		return err
	}
	resultSummary, _ := json.Marshal(map[string]any{"realmguard_result_id": resultID, "score": score, "stars": stars, "verified": true, "stage_id": stage.ID, "mode": in.Mode, "verification_method": realmGuardVerificationMethod, "attestation_digest": attestation.Digest})
	if _, err = tx.Exec(ctx, `UPDATE game_sessions SET status='finished',ended_at=now(),duration_ms=$2,result=result||$3 WHERE id=$1`, in.SessionID, in.DurationMS, resultSummary); err != nil {
		return err
	}
	if err := updateRealmGuardProgressTx(ctx, tx, p.UserID, version.ID, *stage, in, score, stars, cleared); err != nil {
		return err
	}
	heroXP += int64(max(1, in.WavesCompleted))
	newAccountHeroLevel := realmGuardHeroLevel(heroXP, content.Balance.HeroLevelXP)
	if _, err = tx.Exec(ctx, `UPDATE realmguard_user_heroes SET xp=$3,level=$4,updated_at=now() WHERE user_id=$1 AND hero_id=$2`, p.UserID, in.HeroID, heroXP, newAccountHeroLevel); err != nil {
		return err
	}
	if cleared {
		if err := unlockRealmGuardContentTx(ctx, tx, p.UserID, *stage, content); err != nil {
			return err
		}
	}
	if err := unlockRealmGuardAchievementsTx(ctx, tx, p.UserID, *stage, in, stars, cleared, battleHeroLevel, content); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO realmguard_user_loadouts(user_id,hero_id) VALUES($1,$2) ON CONFLICT(user_id) DO UPDATE SET hero_id=excluded.hero_id,updated_at=now()`, p.UserID, in.HeroID); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	s.audit(r, "realmguard.result.accept", "realmguard_result", resultID.String(), map[string]any{"score_id": scoreID, "score": score, "stars": stars, "stage_id": stage.ID, "version_id": version.ID, "verification_method": realmGuardVerificationMethod, "attestation_digest": attestation.Digest, "client_evidence_ignored": in.Proof != "" || string(in.Events) != "[]"})
	progress, err := s.realmGuardProgressData(ctx, p, version, content)
	response := map[string]any{"result": map[string]any{"id": resultID, "score_id": scoreID, "score": score, "stars": stars, "verified": true, "victory": cleared, "battle_hero_level": battleHeroLevel, "account_hero_level": newAccountHeroLevel, "account_hero_xp": heroXP, "breakdown": breakdown, "verification_method": realmGuardVerificationMethod, "attestation": attestation, "content_version": version.ContentVersion, "stage_content_version": version.StageVersion, "stage_version": stage.Version, "balance_version": version.BalanceVersion, "asset_version": version.AssetVersion}}
	if err == nil {
		response["progress"] = progress
	} else {
		response["progress"] = nil
		response["progress_unavailable"] = true
	}
	writeJSON(w, 201, response)
	return nil
}

func (s *Server) writeStoredRealmGuardResult(ctx context.Context, tx pgx.Tx, userID, sessionID uuid.UUID, version realmGuardVersionRecord, w http.ResponseWriter) error {
	var resultID, scoreID uuid.UUID
	var stageID, mode, difficulty, heroID string
	var score int64
	var stars, battleHeroLevel int
	var breakdown, attestation json.RawMessage
	var verificationMethod string
	err := tx.QueryRow(ctx, `SELECT rr.id,s.id,rr.stage_id,rr.mode,rr.difficulty,rr.hero_id,rr.hero_level,rr.score,rr.stars,rr.score_breakdown,rr.verification_method,rr.attestation
		FROM realmguard_results rr JOIN scores s ON s.session_id=rr.session_id
		WHERE rr.session_id=$1 AND rr.user_id=$2 AND rr.verified AND s.verified`, sessionID, userID).Scan(&resultID, &scoreID, &stageID, &mode, &difficulty, &heroID, &battleHeroLevel, &score, &stars, &breakdown, &verificationMethod, &attestation)
	if err == pgx.ErrNoRows {
		return rejectRealmGuardResult(409, "session_finished", "the RealmGuard session was finished without an authoritative result")
	}
	if err != nil {
		return err
	}
	stageVersion := ""
	if content, decodeErr := decodeRealmGuardContent(version.RawContent); decodeErr == nil {
		for _, stage := range content.Stages {
			if stage.ID == stageID {
				stageVersion = stage.Version
				break
			}
		}
	}
	var accountHeroLevel int
	var accountHeroXP int64
	_ = tx.QueryRow(ctx, `SELECT level,xp FROM realmguard_user_heroes WHERE user_id=$1 AND hero_id=$2`, userID, heroID).Scan(&accountHeroLevel, &accountHeroXP)
	writeJSON(w, 200, map[string]any{"result": map[string]any{"id": resultID, "score_id": scoreID, "stage_id": stageID, "mode": mode, "difficulty": difficulty, "hero_id": heroID, "battle_hero_level": battleHeroLevel, "account_hero_level": accountHeroLevel, "account_hero_xp": accountHeroXP, "score": score, "stars": stars, "victory": mode == "campaign" && stars > 0, "verified": true, "breakdown": breakdown, "verification_method": verificationMethod, "attestation": attestation, "content_version": version.ContentVersion, "stage_content_version": version.StageVersion, "stage_version": stageVersion, "balance_version": version.BalanceVersion, "asset_version": version.AssetVersion}, "idempotent": true})
	return nil
}

func ensureRealmGuardStageUnlocked(ctx context.Context, tx pgx.Tx, userID uuid.UUID, stage realmGuardStageDefinition, content realmGuardDecodedContent) error {
	first := realmGuardFirstCampaign(content)
	if first != nil && stage.ID == first.ID {
		return nil
	}
	if stage.Mode == "endless" {
		campaigns := realmGuardCampaigns(content)
		if len(campaigns) == 0 {
			return rejectRealmGuardResult(403, "stage_locked", "endless mode requires a completed campaign")
		}
		var completed bool
		err := tx.QueryRow(ctx, `SELECT COALESCE(bool_or(completed),false) FROM realmguard_user_progress WHERE user_id=$1 AND stage_id=$2`, userID, campaigns[len(campaigns)-1].ID).Scan(&completed)
		if err != nil {
			return err
		}
		if !completed {
			return rejectRealmGuardResult(403, "stage_locked", "endless mode unlocks after the final campaign stage")
		}
		return nil
	}
	var unlocked bool
	err := tx.QueryRow(ctx, `SELECT COALESCE(bool_or(unlocked),false) FROM realmguard_user_progress WHERE user_id=$1 AND stage_id=$2`, userID, stage.ID).Scan(&unlocked)
	if err != nil {
		return err
	}
	if !unlocked {
		return rejectRealmGuardResult(403, "stage_locked", "stage is not unlocked")
	}
	return nil
}

func updateRealmGuardProgressTx(ctx context.Context, tx pgx.Tx, userID, versionID uuid.UUID, stage realmGuardStageDefinition, in realmGuardResultInput, score int64, stars int, cleared bool) error {
	_, err := tx.Exec(ctx, `INSERT INTO realmguard_user_progress(user_id,stage_id,difficulty,unlocked,completed,stars,best_score,best_duration_ms,best_hero_id,wins,attempts,total_playtime_ms,content_version_id)
		VALUES($1,$2,$3,true,$4,$5,$6,CASE WHEN $4 THEN $7::bigint END,$8::text,CASE WHEN $4 THEN 1 ELSE 0 END,1,$7::bigint,$9)
		ON CONFLICT(user_id,stage_id,difficulty) DO UPDATE SET unlocked=true,completed=realmguard_user_progress.completed OR excluded.completed,
		stars=GREATEST(realmguard_user_progress.stars,excluded.stars),best_score=GREATEST(realmguard_user_progress.best_score,excluded.best_score),
		best_duration_ms=CASE WHEN excluded.completed THEN LEAST(COALESCE(realmguard_user_progress.best_duration_ms,excluded.best_duration_ms),excluded.best_duration_ms) ELSE realmguard_user_progress.best_duration_ms END,
		best_hero_id=CASE WHEN excluded.best_score>=realmguard_user_progress.best_score THEN excluded.best_hero_id ELSE realmguard_user_progress.best_hero_id END,
		wins=realmguard_user_progress.wins+CASE WHEN excluded.completed THEN 1 ELSE 0 END,attempts=realmguard_user_progress.attempts+1,
		total_playtime_ms=realmguard_user_progress.total_playtime_ms+excluded.total_playtime_ms,content_version_id=excluded.content_version_id,updated_at=now()`, userID, stage.ID, in.Difficulty, cleared, stars, score, in.DurationMS, in.HeroID, versionID)
	return err
}

func unlockRealmGuardContentTx(ctx context.Context, tx pgx.Tx, userID uuid.UUID, completedStage realmGuardStageDefinition, content realmGuardDecodedContent) error {
	campaigns := realmGuardCampaigns(content)
	nextMilestone := completedStage.Number + 1
	nextID := ""
	for _, stage := range campaigns {
		if stage.Number > completedStage.Number {
			nextMilestone = stage.Number
			nextID = stage.ID
			break
		}
	}
	if nextID != "" {
		for _, difficulty := range []string{"casual", "normal", "veteran"} {
			if _, err := tx.Exec(ctx, `INSERT INTO realmguard_user_progress(user_id,stage_id,difficulty,unlocked) VALUES($1,$2,$3,true) ON CONFLICT(user_id,stage_id,difficulty) DO UPDATE SET unlocked=true,updated_at=now()`, userID, nextID, difficulty); err != nil {
				return err
			}
		}
	} else if completedStage.Mode == "campaign" {
		for _, stage := range content.Stages {
			if stage.Mode != "endless" {
				continue
			}
			for _, difficulty := range []string{"casual", "normal", "veteran"} {
				if _, err := tx.Exec(ctx, `INSERT INTO realmguard_user_progress(user_id,stage_id,difficulty,unlocked) VALUES($1,$2,$3,true) ON CONFLICT(user_id,stage_id,difficulty) DO UPDATE SET unlocked=true,updated_at=now()`, userID, stage.ID, difficulty); err != nil {
					return err
				}
			}
		}
	}
	for _, hero := range content.Heroes {
		if hero.UnlockStage <= nextMilestone {
			if _, err := tx.Exec(ctx, `UPDATE realmguard_user_heroes SET unlocked=true,updated_at=now() WHERE user_id=$1 AND hero_id=$2`, userID, hero.ID); err != nil {
				return err
			}
		}
	}
	for _, skill := range content.Skills {
		if skill.UnlockStage <= nextMilestone {
			if _, err := tx.Exec(ctx, `UPDATE realmguard_user_skills SET unlocked=true,updated_at=now() WHERE user_id=$1 AND skill_id=$2`, userID, skill.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func unlockRealmGuardAchievementsTx(ctx context.Context, tx pgx.Tx, userID uuid.UUID, stage realmGuardStageDefinition, in realmGuardResultInput, stars int, cleared bool, heroLevel int, content realmGuardDecodedContent) error {
	codes := []string{"realmguard-first-defense"}
	if cleared {
		codes = append(codes, "realmguard-first-victory", fmt.Sprintf("realmguard-stage-%d", stage.Number))
	}
	if stars == 3 {
		codes = append(codes, "realmguard-three-star")
	}
	campaigns := realmGuardCampaigns(content)
	if cleared && len(campaigns) > 0 && stage.ID == campaigns[len(campaigns)-1].ID {
		codes = append(codes, "realmguard-campaign-master")
	}
	if in.Mode == "endless" && in.WavesCompleted >= 10 {
		codes = append(codes, "realmguard-endless-10")
	}
	if in.Mode == "endless" && in.WavesCompleted >= 25 {
		codes = append(codes, "realmguard-endless-25")
	}
	if cleared && in.Difficulty == "veteran" {
		codes = append(codes, "realmguard-veteran")
	}
	if cleared && *in.RemainingLives == stage.Lives {
		codes = append(codes, "realmguard-flawless")
	}
	if *in.RemainingGold >= 1000 {
		codes = append(codes, "realmguard-wealthy")
	}
	if heroLevel >= 10 {
		codes = append(codes, "realmguard-hero-master")
	}
	_, err := tx.Exec(ctx, `INSERT INTO user_achievements(user_id,achievement_id) SELECT $1,id FROM achievements WHERE code=ANY($2) ON CONFLICT DO NOTHING`, userID, codes)
	return err
}

func (s *Server) realmGuardRankings(w http.ResponseWriter, r *http.Request) {
	version, err := s.loadRealmGuardPublished(r.Context())
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "campaign"
	}
	if !slices.Contains([]string{"campaign", "endless"}, mode) {
		writeError(w, 400, "invalid_mode", "mode must be campaign or endless")
		return
	}
	difficulty := r.URL.Query().Get("difficulty")
	if difficulty != "" && !slices.Contains([]string{"casual", "normal", "veteran"}, difficulty) {
		writeError(w, 400, "invalid_difficulty", "difficulty must be casual, normal, or veteran")
		return
	}
	group := r.URL.Query().Get("group")
	if group == "" || group == "stage" {
		group = "individual"
	}
	if !slices.Contains([]string{"individual", "department", "hero"}, group) {
		writeError(w, 400, "invalid_group", "group must be individual, department, or hero")
		return
	}
	metric := r.URL.Query().Get("metric")
	if metric == "" {
		metric = "score"
	}
	if metric != "score" && metric != "stars" {
		writeError(w, 400, "invalid_metric", "metric must be score or stars")
		return
	}
	if metric == "stars" && group != "department" {
		writeError(w, 400, "metric_not_supported", "stars metric is available for department rankings; use score for individual and hero rankings")
		return
	}
	period, since, err := s.realmGuardRankingPeriod(r)
	if err != nil {
		writeError(w, 400, "invalid_period", err.Error())
		return
	}
	stageID, heroID := r.URL.Query().Get("stage_id"), r.URL.Query().Get("hero_id")
	limit, _ := pageParams(r)
	if group == "department" && metric == "stars" {
		s.realmGuardDepartmentStars(w, r, version, mode, difficulty, stageID, heroID, period, since, limit)
		return
	}
	if group == "department" {
		var privacy struct {
			ShowDepartment bool `json:"show_department"`
		}
		_ = s.setting(r.Context(), "privacy", &privacy)
		if !privacy.ShowDepartment {
			writeJSON(w, 200, map[string]any{"items": []any{}, "group": group, "metric": metric, "period": period, "department_hidden": true, "version": realmGuardVersionJSON(version)})
			return
		}
		rows, queryErr := s.DB.Query(r.Context(), `WITH user_best AS (
			SELECT u.id,u.department,MAX(rr.score) score FROM realmguard_results rr JOIN users u ON u.id=rr.user_id JOIN scores s ON s.session_id=rr.session_id
			WHERE rr.content_version_id=$1 AND rr.verified AND s.verified AND s.moderation_status='valid' AND NOT u.ranking_opt_out AND rr.mode=$2
			AND (rr.mode='endless' OR rr.stars>0)
			AND ($3='' OR rr.difficulty=$3) AND ($4='' OR rr.stage_id=$4) AND ($5='' OR rr.hero_id=$5)
			AND ($6::timestamptz='0001-01-01' OR rr.created_at>=$6) AND ($7<>'season' OR s.season_id=(SELECT id FROM seasons WHERE status='active' LIMIT 1)) AND u.department<>'' GROUP BY u.id,u.department),
			department_totals AS (SELECT department,SUM(score) score,COUNT(*) members FROM user_best GROUP BY department)
			SELECT row_number() OVER(ORDER BY score DESC),department,score,members FROM department_totals ORDER BY score DESC LIMIT $8`, version.ID, mode, difficulty, stageID, heroID, since, period, limit)
		if queryErr != nil {
			s.dbError(w, r, queryErr)
			return
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var rank, score int64
			var name string
			var members int
			if err := rows.Scan(&rank, &name, &score, &members); err != nil {
				s.dbError(w, r, err)
				return
			}
			items = append(items, map[string]any{"rank": rank, "name": name, "department": name, "score": score, "members": members})
		}
		if err := rows.Err(); err != nil {
			s.dbError(w, r, err)
			return
		}
		writeJSON(w, 200, map[string]any{"items": items, "group": group, "metric": metric, "period": period, "version": realmGuardVersionJSON(version)})
		return
	}
	if group == "hero" {
		rows, queryErr := s.DB.Query(r.Context(), `WITH user_best AS (
			SELECT u.id,rr.hero_id,MAX(rr.score) score FROM realmguard_results rr JOIN users u ON u.id=rr.user_id JOIN scores s ON s.session_id=rr.session_id
			WHERE rr.content_version_id=$1 AND rr.verified AND s.verified AND s.moderation_status='valid' AND NOT u.ranking_opt_out AND rr.mode=$2
			AND (rr.mode='endless' OR rr.stars>0)
			AND ($3='' OR rr.difficulty=$3) AND ($4='' OR rr.stage_id=$4) AND ($5='' OR rr.hero_id=$5)
			AND ($6::timestamptz='0001-01-01' OR rr.created_at>=$6) AND ($7<>'season' OR s.season_id=(SELECT id FROM seasons WHERE status='active' LIMIT 1)) GROUP BY u.id,rr.hero_id),
			hero_totals AS (SELECT hero_id,SUM(score) score,COUNT(*) members FROM user_best GROUP BY hero_id)
			SELECT row_number() OVER(ORDER BY score DESC),hero_id,score,members FROM hero_totals ORDER BY score DESC LIMIT $8`, version.ID, mode, difficulty, stageID, heroID, since, period, limit)
		if queryErr != nil {
			s.dbError(w, r, queryErr)
			return
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var rank, score int64
			var name string
			var members int
			if err := rows.Scan(&rank, &name, &score, &members); err != nil {
				s.dbError(w, r, err)
				return
			}
			items = append(items, map[string]any{"rank": rank, "name": name, "hero_id": name, "score": score, "members": members})
		}
		if err := rows.Err(); err != nil {
			s.dbError(w, r, err)
			return
		}
		writeJSON(w, 200, map[string]any{"items": items, "group": group, "metric": metric, "period": period, "version": realmGuardVersionJSON(version)})
		return
	}
	rows, err := s.DB.Query(r.Context(), `WITH best AS (
		SELECT DISTINCT ON (u.id) u.id,u.username,u.display_name,u.nickname,u.department,rr.stage_id,rr.hero_id,rr.difficulty,rr.score,rr.stars,rr.duration_ms,rr.created_at
		FROM realmguard_results rr JOIN users u ON u.id=rr.user_id JOIN scores s ON s.session_id=rr.session_id
		WHERE rr.content_version_id=$1 AND rr.verified AND s.verified AND s.moderation_status='valid' AND NOT u.ranking_opt_out AND rr.mode=$2
		AND (rr.mode='endless' OR rr.stars>0)
		AND ($3='' OR rr.difficulty=$3) AND ($4='' OR rr.stage_id=$4) AND ($5='' OR rr.hero_id=$5)
		AND ($6::timestamptz='0001-01-01' OR rr.created_at>=$6) AND ($7<>'season' OR s.season_id=(SELECT id FROM seasons WHERE status='active' LIMIT 1))
		ORDER BY u.id,rr.score DESC,rr.created_at ASC)
		SELECT row_number() OVER(ORDER BY score DESC,created_at),id,username,display_name,nickname,department,stage_id,hero_id,difficulty,score,stars,duration_ms,created_at FROM best ORDER BY score DESC,created_at LIMIT $8`, version.ID, mode, difficulty, stageID, heroID, since, period, limit)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	defer rows.Close()
	var privacy struct {
		RankingName    string `json:"ranking_name"`
		ShowDepartment bool   `json:"show_department"`
	}
	_ = s.setting(r.Context(), "privacy", &privacy)
	items := []map[string]any{}
	for rows.Next() {
		var rank, score, duration int64
		var userID uuid.UUID
		var username, display, nickname, department, itemStage, itemHero, itemDifficulty string
		var stars int
		var created time.Time
		if err := rows.Scan(&rank, &userID, &username, &display, &nickname, &department, &itemStage, &itemHero, &itemDifficulty, &score, &stars, &duration, &created); err != nil {
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
		item := map[string]any{"rank": rank, "user_id": userID, "name": name, "display_name": name, "stage_id": itemStage, "hero_id": itemHero, "difficulty": itemDifficulty, "score": score, "stars": stars, "duration_ms": duration, "created_at": created}
		if privacy.ShowDepartment {
			item["department"] = department
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		s.dbError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "group": group, "metric": metric, "period": period, "version": realmGuardVersionJSON(version)})
}

func (s *Server) realmGuardDepartmentStars(w http.ResponseWriter, r *http.Request, version realmGuardVersionRecord, mode, difficulty, stageID, heroID, period string, since time.Time, limit int) {
	var privacy struct {
		ShowDepartment bool `json:"show_department"`
	}
	_ = s.setting(r.Context(), "privacy", &privacy)
	if !privacy.ShowDepartment {
		writeJSON(w, 200, map[string]any{"items": []any{}, "group": "department", "metric": "stars", "period": period, "department_hidden": true, "version": realmGuardVersionJSON(version)})
		return
	}
	rows, err := s.DB.Query(r.Context(), `WITH user_stage AS (
		SELECT rr.user_id,u.department,rr.stage_id,MAX(rr.stars) stars
		FROM realmguard_results rr JOIN users u ON u.id=rr.user_id JOIN scores s ON s.session_id=rr.session_id
		WHERE rr.content_version_id=$1 AND rr.verified AND s.verified AND s.moderation_status='valid' AND NOT u.ranking_opt_out AND u.department<>'' AND rr.mode=$2
		AND (rr.mode='endless' OR rr.stars>0)
		AND ($3='' OR rr.difficulty=$3) AND ($4='' OR rr.stage_id=$4) AND ($5='' OR rr.hero_id=$5)
		AND ($6::timestamptz='0001-01-01' OR rr.created_at>=$6) AND ($7<>'season' OR s.season_id=(SELECT id FROM seasons WHERE status='active' LIMIT 1))
		GROUP BY rr.user_id,u.department,rr.stage_id),
		user_total AS (SELECT user_id,department,SUM(stars) stars FROM user_stage GROUP BY user_id,department),
		department_total AS (SELECT department,SUM(stars) stars,COUNT(*) members FROM user_total GROUP BY department)
		SELECT row_number() OVER(ORDER BY stars DESC),department,stars,members FROM department_total WHERE stars>0 ORDER BY stars DESC LIMIT $8`, version.ID, mode, difficulty, stageID, heroID, since, period, limit)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var rank, stars int64
		var department string
		var members int
		if err := rows.Scan(&rank, &department, &stars, &members); err != nil {
			s.dbError(w, r, err)
			return
		}
		items = append(items, map[string]any{"rank": rank, "name": department, "department": department, "stars": stars, "members": members})
	}
	if err := rows.Err(); err != nil {
		s.dbError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "group": "department", "metric": "stars", "period": period, "version": realmGuardVersionJSON(version)})
}

func (s *Server) realmGuardRankingPeriod(r *http.Request) (string, time.Time, error) {
	period := r.URL.Query().Get("period")
	if period == "" || period == "all" || period == "all_time" {
		return "all", time.Time{}, nil
	}
	now := s.Now().In(s.serviceLocation(r.Context()))
	switch period {
	case "daily":
		return period, time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).UTC(), nil
	case "weekly":
		offset := (int(now.Weekday()) + 6) % 7
		return period, time.Date(now.Year(), now.Month(), now.Day()-offset, 0, 0, 0, 0, now.Location()).UTC(), nil
	case "monthly":
		return period, time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).UTC(), nil
	case "season":
		return period, time.Time{}, nil
	default:
		return "", time.Time{}, fmt.Errorf("period must be daily, weekly, monthly, season, or all")
	}
}

func realmGuardIDParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 400, "invalid_id", "invalid RealmGuard version identifier")
		return uuid.Nil, false
	}
	return id, true
}
