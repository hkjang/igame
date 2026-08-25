// Package realmguard replays a RealmGuard or Defense Series battle from the
// player input ledger the browser recorded.
//
// It is a line-by-line counterpart of web/src/games/realmguard/kernel: the same
// fixed 50ms step, the same order of operations, no wall clock and no RNG. The
// two implementations are held together by shared vectors in testdata, so a
// rules change that lands on only one side fails the build rather than silently
// making every submitted score unverifiable.
package realmguard

// TickMS is the fixed simulation step. Nothing in a battle advances between
// ticks, so a ledger plus a tick count fully determines the outcome.
const (
	TickMS = 50
	Width  = 1280
	Height = 720
)

// Field names mirror the browser projection exactly; the config digest is taken
// over these bytes on both sides.

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Spot struct {
	ID string  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
}

type Enemy struct {
	ID         string   `json:"id"`
	HP         float64  `json:"hp"`
	Speed      float64  `json:"speed"`
	Armor      float64  `json:"armor"`
	Reward     float64  `json:"reward"`
	LifeDamage float64  `json:"lifeDamage"`
	Radius     float64  `json:"radius"`
	Traits     []string `json:"traits"`
	ThreatType string   `json:"threatType"`
}

func (e Enemy) hasTrait(trait string) bool {
	for _, item := range e.Traits {
		if item == trait {
			return true
		}
	}
	return false
}

type Branch struct {
	ID               string  `json:"id"`
	DamageMultiplier float64 `json:"damageMultiplier"`
	RangeMultiplier  float64 `json:"rangeMultiplier"`
	RateMultiplier   float64 `json:"rateMultiplier"`
	Splash           float64 `json:"splash"`
	Slow             float64 `json:"slow"`
	Pierce           float64 `json:"pierce"`
}

type Profile struct {
	ID               string  `json:"id"`
	DamageMultiplier float64 `json:"damageMultiplier"`
}

type Tower struct {
	ID                  string    `json:"id"`
	Cost                float64   `json:"cost"`
	Damage              float64   `json:"damage"`
	Range               float64   `json:"range"`
	FireRate            float64   `json:"fireRate"`
	DamageType          string    `json:"damageType"`
	Blocking            bool      `json:"blocking"`
	EffectiveAgainst    []string  `json:"effectiveAgainst"`
	EffectiveMultiplier float64   `json:"effectiveMultiplier"`
	Branches            []Branch  `json:"branches"`
	Profiles            []Profile `json:"profiles"`
}

type WaveEntry struct {
	Enemy     string   `json:"enemy"`
	Count     float64  `json:"count"`
	Interval  float64  `json:"interval"`
	Delay     float64  `json:"delay"`
	PathIndex float64  `json:"pathIndex"`
	Parallel  bool     `json:"parallel"`
	Modifiers []string `json:"modifiers"`
}

type Wave struct {
	Entries []WaveEntry `json:"entries"`
	Reward  float64     `json:"reward"`
}

type Stage struct {
	ID           string    `json:"id"`
	Mode         string    `json:"mode"`
	Lives        float64   `json:"lives"`
	StartingGold float64   `json:"startingGold"`
	Gimmick      string    `json:"gimmick"`
	Lanes        [][]Point `json:"lanes"`
	Spots        []Spot    `json:"spots"`
	Waves        []Wave    `json:"waves"`
}

type Hero struct {
	ID             string  `json:"id"`
	HP             float64 `json:"hp"`
	Damage         float64 `json:"damage"`
	Range          float64 `json:"range"`
	Speed          float64 `json:"speed"`
	RespawnSeconds float64 `json:"respawnSeconds"`
}

type Skill struct {
	ID       string  `json:"id"`
	Cooldown float64 `json:"cooldown"`
}

type Balance struct {
	EnemyHP          float64   `json:"enemyHp"`
	EnemySpeed       float64   `json:"enemySpeed"`
	Gold             float64   `json:"gold"`
	TowerUpgradeCost []float64 `json:"towerUpgradeCost"`
	HeroLevelXP      []float64 `json:"heroLevelXp"`
	EndlessRamp      float64   `json:"endlessRamp"`
	SellRefundRate   float64   `json:"sellRefundRate"`
}

type Config struct {
	Difficulty string  `json:"difficulty"`
	Stage      Stage   `json:"stage"`
	Enemies    []Enemy `json:"enemies"`
	Towers     []Tower `json:"towers"`
	Hero       Hero    `json:"hero"`
	Skills     []Skill `json:"skills"`
	Balance    Balance `json:"balance"`
}

// Command is one recorded player action. `Tick` is how many simulation steps
// had already run when the player acted.
type Command struct {
	Tick    int     `json:"tick"`
	Op      string  `json:"op"`
	Spot    string  `json:"spot,omitempty"`
	Tower   string  `json:"tower,omitempty"`
	Profile string  `json:"profile,omitempty"`
	Branch  string  `json:"branch,omitempty"`
	Mode    string  `json:"mode,omitempty"`
	Skill   string  `json:"skill,omitempty"`
	X       float64 `json:"x,omitempty"`
	Y       float64 `json:"y,omitempty"`
	Gold    float64 `json:"gold,omitempty"`
	Lives   float64 `json:"lives,omitempty"`
}

// Ledger is the submitted replay envelope.
type Ledger struct {
	RulesVersion string    `json:"rules_version"`
	ConfigDigest string    `json:"config_digest"`
	Ticks        int       `json:"ticks"`
	Commands     []Command `json:"commands"`
}

// Outcome is everything a score is allowed to depend on.
type Outcome struct {
	Victory         bool           `json:"victory"`
	Ticks           int            `json:"ticks"`
	DurationMS      int            `json:"duration_ms"`
	Lives           int            `json:"lives"`
	Gold            int            `json:"gold"`
	EarnedGold      int            `json:"earned_gold"`
	SpentGold       int            `json:"spent_gold"`
	SoldGold        int            `json:"sold_gold"`
	Kills           int            `json:"kills"`
	Escaped         int            `json:"escaped"`
	Spawned         int            `json:"spawned"`
	WavesCompleted  int            `json:"waves_completed"`
	HeroLevel       int            `json:"hero_level"`
	DefeatedByEnemy map[string]int `json:"defeated_by_enemy"`
	EscapedByEnemy  map[string]int `json:"escaped_by_enemy"`
	SpawnedByEnemy  map[string]int `json:"spawned_by_enemy"`
}

// RulesVersion must match what the browser stamped on the ledger.
const RulesVersion = "realmguard-kernel-1"

// Limits keep a hostile ledger from turning verification into a denial of
// service; the browser enforces the same numbers before it submits.
const (
	LedgerLimit = 6000
	TickLimit   = 288000
)
