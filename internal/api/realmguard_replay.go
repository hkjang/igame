package api

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"

	battle "github.com/hkjang/igame/internal/battle/realmguard"
)

// realmGuardReplayMethod marks results whose numbers the server derived itself
// by replaying the player's inputs, rather than checking numbers the browser
// reported.
const realmGuardReplayMethod = "server_replay_v1"

// realmGuardMaxSpeedup is the fastest the battle clock may run against the wall
// clock. The client offers 1x and 2x, so a session may compress at most in half.
const realmGuardMaxSpeedup = 2

type realmGuardReplayAttestation struct {
	Method       string `json:"method"`
	RulesVersion string `json:"rules_version"`
	ConfigDigest string `json:"config_digest"`
	Ticks        int    `json:"ticks"`
	Commands     int    `json:"commands"`
	AccountHero  int    `json:"account_hero_level"`
}

func realmGuardBlocking(towers []realmGuardTowerDefinition, id string) bool {
	if id == "windward" {
		return true
	}
	return len(towers) > 0 && towers[len(towers)-1].ID == id
}

func optionalFloat(value *float64, fallback float64) float64 {
	if value == nil {
		return fallback
	}
	return *value
}

func optionalInt(value *int, fallback float64) float64 {
	if value == nil {
		return fallback
	}
	return float64(*value)
}

// realmGuardKernelConfig projects the pinned content into the exact numbers the
// battle kernel reads. It mirrors the browser projection field for field; when
// the two disagree the submitted digest will not match and the result is
// refused rather than scored against rules the player never faced.
func realmGuardKernelConfig(content realmGuardDecodedContent, stage realmGuardStageDefinition, difficulty, heroID string) (battle.Config, error) {
	balance, ok := content.Balance.Difficulties[difficulty]
	if !ok {
		return battle.Config{}, fmt.Errorf("difficulty %q is not present in the pinned balance", difficulty)
	}
	lanes := [][]battle.Point{}
	for _, lane := range stage.Paths {
		if len(lane) >= 2 {
			lanes = append(lanes, realmGuardPoints(lane))
		}
	}
	if len(lanes) == 0 && len(stage.Path) >= 2 {
		lanes = append(lanes, realmGuardPoints(stage.Path))
	}
	if len(lanes) == 0 {
		return battle.Config{}, fmt.Errorf("stage %q has no lane with at least two waypoints", stage.ID)
	}
	spots := make([]battle.Spot, 0, len(stage.TowerSpots))
	for _, spot := range stage.TowerSpots {
		spots = append(spots, battle.Spot{ID: spot.ID, X: spot.X, Y: spot.Y})
	}
	waves := make([]realmGuardWaveDefinition, 0, len(content.Waves))
	for _, wave := range content.Waves {
		if wave.StageID == stage.ID {
			waves = append(waves, wave)
		}
	}
	slices.SortStableFunc(waves, func(a, b realmGuardWaveDefinition) int { return a.Number - b.Number })
	projected := make([]battle.Wave, 0, len(waves))
	for _, wave := range waves {
		entries := make([]battle.WaveEntry, 0, len(wave.Entries))
		for _, entry := range wave.Entries {
			modifiers := entry.Modifiers
			if modifiers == nil {
				modifiers = []string{}
			}
			entries = append(entries, battle.WaveEntry{
				Enemy:     entry.Enemy,
				Count:     math.Max(1, float64(entry.Count)),
				Interval:  math.Max(0.15, entry.Interval),
				Delay:     math.Max(0, entry.Delay),
				PathIndex: math.Max(0, math.Floor(optionalInt(entry.PathIndex, 0))),
				Parallel:  entry.Parallel,
				Modifiers: modifiers,
			})
		}
		projected = append(projected, battle.Wave{Entries: entries, Reward: math.Max(0, float64(wave.Reward))})
	}
	enemies := make([]battle.Enemy, 0, len(content.Enemies)+len(content.Bosses))
	for _, enemy := range append(append([]realmGuardEnemyDefinition{}, content.Enemies...), content.Bosses...) {
		traits := enemy.Traits
		if traits == nil {
			traits = []string{}
		}
		enemies = append(enemies, battle.Enemy{
			ID: enemy.ID, HP: enemy.HP, Speed: enemy.Speed, Armor: enemy.Armor,
			Reward: float64(enemy.Reward), LifeDamage: float64(enemy.LifeDamage),
			Radius: enemy.Radius, Traits: traits, ThreatType: "",
		})
	}
	towers := make([]battle.Tower, 0, len(content.Towers))
	for _, tower := range content.Towers {
		branches := make([]battle.Branch, 0, len(tower.Branches))
		for _, branch := range tower.Branches {
			branches = append(branches, battle.Branch{
				ID:               branch.ID,
				DamageMultiplier: optionalFloat(branch.DamageMultiplier, 1),
				RangeMultiplier:  optionalFloat(branch.RangeMultiplier, 1),
				RateMultiplier:   optionalFloat(branch.RateMultiplier, 1),
				Splash:           optionalFloat(branch.Splash, 0),
				Slow:             optionalFloat(branch.Slow, 0),
				Pierce:           optionalInt(branch.Pierce, 0),
			})
		}
		towers = append(towers, battle.Tower{
			ID: tower.ID, Cost: float64(tower.Cost), Damage: tower.Damage, Range: tower.Range,
			FireRate: tower.FireRate, DamageType: tower.DamageType,
			Blocking:         realmGuardBlocking(content.Towers, tower.ID),
			EffectiveAgainst: []string{}, EffectiveMultiplier: -1,
			Branches: branches, Profiles: []battle.Profile{},
		})
	}
	hero := battle.Hero{}
	for _, item := range content.Heroes {
		if item.ID == heroID {
			hero = battle.Hero{
				ID: item.ID, HP: item.HP, Damage: item.Damage, Range: item.Range,
				Speed: item.Speed, RespawnSeconds: item.RespawnSeconds,
			}
			break
		}
	}
	if hero.ID == "" {
		return battle.Config{}, fmt.Errorf("hero %q is not present in the pinned content", heroID)
	}
	skills := make([]battle.Skill, 0, len(content.Skills))
	for _, skill := range content.Skills {
		skills = append(skills, battle.Skill{ID: skill.ID, Cooldown: skill.Cooldown})
	}
	return battle.Config{
		Difficulty: difficulty,
		Stage: battle.Stage{
			ID: stage.ID, Mode: stage.Mode, Lives: float64(stage.Lives),
			StartingGold: float64(stage.StartingGold), Gimmick: stage.Gimmick,
			Lanes: lanes, Spots: spots, Waves: projected,
		},
		Enemies: enemies,
		Towers:  towers,
		Hero:    hero,
		Skills:  skills,
		Balance: battle.Balance{
			EnemyHP: balance.EnemyHP, EnemySpeed: balance.EnemySpeed, Gold: balance.Gold,
			TowerUpgradeCost: realmGuardFloats(content.Balance.TowerUpgradeCost),
			HeroLevelXP:      realmGuardFloats(content.Balance.HeroLevelXP),
			EndlessRamp:      content.Balance.EndlessRamp,
			SellRefundRate:   content.Balance.SellRefundRate,
		},
	}, nil
}

func realmGuardPoints(source []realmGuardPoint) []battle.Point {
	points := make([]battle.Point, 0, len(source))
	for _, point := range source {
		points = append(points, battle.Point{X: point.X, Y: point.Y})
	}
	return points
}

func realmGuardFloats(source []int64) []float64 {
	values := make([]float64, 0, len(source))
	for _, value := range source {
		values = append(values, float64(value))
	}
	return values
}

// replayRealmGuardBattle turns the submitted ledger into the authoritative
// outcome. Everything a score depends on comes back from here; nothing the
// browser claimed about the battle is carried through.
func replayRealmGuardBattle(raw json.RawMessage, content realmGuardDecodedContent, stage realmGuardStageDefinition, difficulty, heroID string, accountHeroLevel int) (battle.Outcome, realmGuardReplayAttestation, error) {
	var ledger battle.Ledger
	if len(raw) == 0 {
		return battle.Outcome{}, realmGuardReplayAttestation{}, rejectRealmGuardResult(422, "missing_ledger", "전투 입력 원장이 없어 서버가 결과를 재현할 수 없습니다. 전투를 다시 진행해 주세요.")
	}
	if err := json.Unmarshal(raw, &ledger); err != nil {
		return battle.Outcome{}, realmGuardReplayAttestation{}, rejectRealmGuardResult(422, "invalid_ledger", "전투 입력 원장을 해석할 수 없습니다.")
	}
	if ledger.RulesVersion != battle.RulesVersion {
		return battle.Outcome{}, realmGuardReplayAttestation{}, rejectRealmGuardResult(409, "ledger_rules_mismatch", "전투 규칙 버전이 서버와 다릅니다. 새로고침 후 다시 시도해 주세요.")
	}
	if ledger.Ticks < 0 || ledger.Ticks > battle.TickLimit {
		return battle.Outcome{}, realmGuardReplayAttestation{}, rejectRealmGuardResult(422, "invalid_ledger", "전투 길이가 허용 범위를 벗어났습니다.")
	}
	if len(ledger.Commands) > battle.LedgerLimit {
		return battle.Outcome{}, realmGuardReplayAttestation{}, rejectRealmGuardResult(422, "invalid_ledger", "전투 입력 수가 허용 범위를 벗어났습니다.")
	}
	previous := 0
	for _, command := range ledger.Commands {
		if command.Tick < previous || command.Tick > ledger.Ticks {
			return battle.Outcome{}, realmGuardReplayAttestation{}, rejectRealmGuardResult(422, "invalid_ledger", "전투 입력 순서가 올바르지 않습니다.")
		}
		previous = command.Tick
	}
	config, err := realmGuardKernelConfig(content, stage, difficulty, heroID)
	if err != nil {
		return battle.Outcome{}, realmGuardReplayAttestation{}, rejectRealmGuardResult(422, "invalid_content", err.Error())
	}
	digest, err := battle.Digest(config)
	if err != nil {
		return battle.Outcome{}, realmGuardReplayAttestation{}, err
	}
	if digest != ledger.ConfigDigest {
		return battle.Outcome{}, realmGuardReplayAttestation{}, rejectRealmGuardResult(409, "content_projection_mismatch", "게시된 콘텐츠 해석이 화면과 서버에서 달라 결과를 검증할 수 없습니다. 관리자에게 재게시를 요청해 주세요.")
	}
	outcome := battle.Replay(config, ledger.Commands, ledger.Ticks, accountHeroLevel)
	return outcome, realmGuardReplayAttestation{
		Method:       realmGuardReplayMethod,
		RulesVersion: ledger.RulesVersion,
		ConfigDigest: digest,
		Ticks:        ledger.Ticks,
		Commands:     len(ledger.Commands),
		AccountHero:  accountHeroLevel,
	}, nil
}

// applyRealmGuardReplay overwrites everything the browser claimed about the
// battle with what the replay actually produced. The remaining validation then
// runs against the server's own numbers, so a rewritten client can only change
// its inputs, never its result.
func applyRealmGuardReplay(in realmGuardResultInput, outcome battle.Outcome) realmGuardResultInput {
	lives := outcome.Lives
	gold := outcome.Gold
	victory := outcome.Victory
	in.RemainingLives = &lives
	in.Lives = &lives
	in.RemainingGold = &gold
	in.Gold = &gold
	in.Victory = &victory
	in.DurationMS = int64(outcome.DurationMS)
	in.DurationSeconds = float64(outcome.DurationMS) / 1000
	in.EarnedGold = outcome.EarnedGold
	in.SpentGold = outcome.SpentGold
	in.SoldGold = outcome.SoldGold
	in.Kills = outcome.Kills
	in.Escaped = outcome.Escaped
	in.Spawned = outcome.Spawned
	in.WavesCompleted = outcome.WavesCompleted
	in.Waves = outcome.WavesCompleted
	in.HeroLevel = outcome.HeroLevel
	in.DefeatedByEnemy = outcome.DefeatedByEnemy
	in.EscapedByEnemy = outcome.EscapedByEnemy
	in.SpawnedByEnemy = outcome.SpawnedByEnemy
	return in
}
