package realmguard

import (
	"math"
	"sort"
)

// Every float expression that multiplies then adds is wrapped in an explicit
// float64 conversion. Go is allowed to fuse those into a single FMA on some
// architectures, which would round differently from the browser and make honest
// battles fail verification.

func distance(ax, ay, bx, by float64) float64 {
	dx := bx - ax
	dy := by - ay
	return math.Sqrt(float64(dx*dx) + float64(dy*dy))
}

func clamp(value, minimum, maximum float64) float64 {
	return math.Max(minimum, math.Min(maximum, value))
}

// jsRound mirrors Math.round, which rounds halves toward positive infinity.
func jsRound(value float64) float64 {
	return math.Floor(value + 0.5)
}

// jitter replaces the renderer's RNG so split and summon placement replays
// exactly; the browser computes the same 32-bit hash.
func jitter(sequence, span int) float64 {
	hash := uint32(sequence) * 2654435761
	return float64(hash%uint32(2*span+1)) - float64(span)
}

type enemyState struct {
	id          int
	def         int
	hp          float64
	maxHP       float64
	speed       float64
	x           float64
	y           float64
	lane        int
	pathIndex   int
	pathProgres float64
	slowUntil   float64
	slowFactor  float64
	healAt      float64
	hasteUntil  float64
	lastAttack  float64
	modifiers   map[string]bool
	phases      map[string]bool
	alive       bool
}

type towerState struct {
	spotID        string
	def           int
	x             float64
	y             float64
	level         int
	branch        string
	profile       string
	targeting     string
	lastShot      float64
	disabledUntil float64
	soldiers      []Point
	blocked       map[int]bool
}

type heroState struct {
	x           float64
	y           float64
	targetX     float64
	targetY     float64
	level       int
	xp          float64
	lastShot    float64
	hp          float64
	maxHP       float64
	deadUntil   float64
	attackCount int
}

type reinforcement struct {
	x           float64
	y           float64
	nextStrikeA float64
	expiresAt   float64
}

type spawnOrder struct {
	enemy     string
	at        float64
	pathIndex int
	modifiers []string
}

type towerStats struct {
	damage   float64
	rng      float64
	fireRate float64
	splash   float64
	slow     float64
	pierce   int
}

// Kernel is the authoritative battle simulation.
type Kernel struct {
	config     Config
	lanes      [][]Point
	enemies    []*enemyState
	towers     []*towerState
	skillReady map[string]float64
	reinforce  []*reinforcement
	hero       heroState
	heroDamage float64

	armed         string
	enemySequence int
	hitSequence   int
	spawnQueue    []spawnOrder

	gold           float64
	lives          float64
	kills          int
	escaped        int
	spawned        int
	earnedGold     float64
	spentGold      float64
	soldGold       float64
	waveIndex      int
	waveActive     bool
	waveStartedAt  float64
	nextWaveAt     float64
	nextGimmickAt  float64
	simulationTime float64
	tickCount      int
	completed      bool
	victory        bool

	defeatedByEnemy map[string]int
	escapedByEnemy  map[string]int
	spawnedByEnemy  map[string]int
}

// New starts a battle at tick zero.
func New(config Config, accountHeroLevel int) *Kernel {
	lanes := config.Stage.Lanes
	first := lanes[0]
	start := first[minInt(2, len(first)-1)]
	accountBonus := 1 + float64(maxInt(0, accountHeroLevel-1))*0.025
	maxHP := config.Hero.HP * accountBonus
	return &Kernel{
		config:          config,
		lanes:           lanes,
		skillReady:      map[string]float64{},
		heroDamage:      config.Hero.Damage * accountBonus,
		gold:            jsRound(config.Stage.StartingGold * config.Balance.Gold),
		lives:           config.Stage.Lives,
		nextWaveAt:      10000,
		nextGimmickAt:   12000,
		defeatedByEnemy: map[string]int{},
		escapedByEnemy:  map[string]int{},
		spawnedByEnemy:  map[string]int{},
		hero: heroState{
			x: start.X, y: start.Y - 58,
			targetX: start.X, targetY: start.Y - 58,
			level: 1, hp: maxHP, maxHP: maxHP,
		},
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Ticks reports how many steps have run.
func (k *Kernel) Ticks() int { return k.tickCount }

// Finished reports whether the battle has already resolved.
func (k *Kernel) Finished() bool { return k.completed }

// Outcome returns everything a score may depend on.
func (k *Kernel) Outcome() Outcome {
	return Outcome{
		Victory:         k.victory,
		Ticks:           k.tickCount,
		DurationMS:      k.tickCount * TickMS,
		Lives:           int(k.lives),
		Gold:            int(math.Max(0, k.gold)),
		EarnedGold:      int(k.earnedGold),
		SpentGold:       int(k.spentGold),
		SoldGold:        int(k.soldGold),
		Kills:           k.kills,
		Escaped:         k.escaped,
		Spawned:         k.spawned,
		WavesCompleted:  k.waveIndex,
		HeroLevel:       k.hero.level,
		DefeatedByEnemy: cloneCounts(k.defeatedByEnemy),
		EscapedByEnemy:  cloneCounts(k.escapedByEnemy),
		SpawnedByEnemy:  cloneCounts(k.spawnedByEnemy),
	}
}

func cloneCounts(source map[string]int) map[string]int {
	out := make(map[string]int, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

// ------------------------------------------------------------------ commands

// Apply runs one recorded player action.
func (k *Kernel) Apply(command Command) {
	if k.completed {
		return
	}
	switch command.Op {
	case "wave":
		k.startWave(true)
	case "build":
		k.buildTower(command.Spot, command.Tower, command.Profile)
	case "upgrade":
		k.upgradeTower(command.Spot, command.Branch)
	case "sell":
		k.sellTower(command.Spot)
	case "target":
		k.changeTargeting(command.Spot, command.Mode)
	case "skill":
		k.armSkill(command.Skill)
	case "meteor":
		k.castMeteor(command.X, command.Y)
	case "reinforce":
		k.castReinforcement(command.X, command.Y)
	case "hero":
		k.moveHero(command.X, command.Y)
	case "economy":
		k.adjustEconomy(command.Gold, command.Lives)
	case "defeat":
		k.endBattle(false)
	}
}

func (k *Kernel) towerIndex(spotID string) int {
	for index, tower := range k.towers {
		if tower.spotID == spotID {
			return index
		}
	}
	return -1
}

func (k *Kernel) buildTower(spotID, towerID, profile string) {
	if k.towerIndex(spotID) >= 0 {
		return
	}
	definitionIndex := -1
	for index, tower := range k.config.Towers {
		if tower.ID == towerID {
			definitionIndex = index
			break
		}
	}
	var spot *Spot
	for index := range k.config.Stage.Spots {
		if k.config.Stage.Spots[index].ID == spotID {
			spot = &k.config.Stage.Spots[index]
			break
		}
	}
	if definitionIndex < 0 || spot == nil {
		return
	}
	definition := k.config.Towers[definitionIndex]
	if k.gold < definition.Cost {
		return
	}
	if profile != "" {
		known := false
		for _, item := range definition.Profiles {
			if item.ID == profile {
				known = true
				break
			}
		}
		if !known {
			return
		}
	}
	k.gold -= definition.Cost
	k.spentGold += definition.Cost
	var soldiers []Point
	if definition.Blocking {
		nearest, _ := closestPointOnPaths(k.lanes, Point{X: spot.X, Y: spot.Y})
		soldiers = []Point{
			{X: nearest.X - 18, Y: nearest.Y - 16},
			{X: nearest.X + 18, Y: nearest.Y + 16},
		}
	}
	k.towers = append(k.towers, &towerState{
		spotID: spotID, def: definitionIndex, x: spot.X, y: spot.Y,
		level: 1, profile: profile, targeting: "first",
		soldiers: soldiers, blocked: map[int]bool{},
	})
}

func (k *Kernel) upgradeTower(spotID, branch string) {
	index := k.towerIndex(spotID)
	if index < 0 {
		return
	}
	tower := k.towers[index]
	if tower.level >= 3 {
		return
	}
	cost := 100.0
	if tower.level < len(k.config.Balance.TowerUpgradeCost) {
		cost = k.config.Balance.TowerUpgradeCost[tower.level]
	}
	if k.gold < cost {
		return
	}
	k.gold -= cost
	k.spentGold += cost
	tower.level++
	if tower.level == 3 && branch != "" {
		for _, item := range k.config.Towers[tower.def].Branches {
			if item.ID == branch {
				tower.branch = branch
				break
			}
		}
	}
}

func (k *Kernel) sellTower(spotID string) {
	index := k.towerIndex(spotID)
	if index < 0 {
		return
	}
	tower := k.towers[index]
	definition := k.config.Towers[tower.def]
	invested := definition.Cost
	for level := 1; level < tower.level && level < len(k.config.Balance.TowerUpgradeCost); level++ {
		invested += k.config.Balance.TowerUpgradeCost[level]
	}
	refund := jsRound(invested * k.config.Balance.SellRefundRate)
	k.gold += refund
	k.soldGold += refund
	k.towers = append(k.towers[:index], k.towers[index+1:]...)
}

func (k *Kernel) changeTargeting(spotID, mode string) {
	switch mode {
	case "first", "last", "strong", "weak", "closest":
	default:
		return
	}
	index := k.towerIndex(spotID)
	if index < 0 {
		return
	}
	k.towers[index].targeting = mode
}

func (k *Kernel) armSkill(skillID string) {
	var skill *Skill
	for index := range k.config.Skills {
		if k.config.Skills[index].ID == skillID {
			skill = &k.config.Skills[index]
			break
		}
	}
	if skill == nil || k.skillReady[skillID] > k.simulationTime {
		return
	}
	k.skillReady[skillID] = k.simulationTime + float64(skill.Cooldown*1000)
	if skillID == "freeze" {
		for _, enemy := range k.enemies {
			if !k.hasTrait(enemy, "immune_stun") {
				enemy.slowUntil = k.simulationTime + 5000
				enemy.slowFactor = 0.18
			}
		}
		return
	}
	if skillID == "meteor" || skillID == "reinforcement" {
		k.armed = skillID
	}
}

func (k *Kernel) castMeteor(x, y float64) {
	if k.armed != "meteor" {
		return
	}
	k.armed = ""
	px := clamp(x, 0, Width)
	py := clamp(y, 0, Height)
	for _, enemy := range k.snapshot() {
		if distance(px, py, enemy.x, enemy.y) < 125 {
			k.damageEnemy(enemy, 245, "skill", 0)
		}
	}
}

func (k *Kernel) castReinforcement(x, y float64) {
	if k.armed != "reinforcement" {
		return
	}
	k.armed = ""
	k.reinforce = append(k.reinforce, &reinforcement{
		x:           clamp(x, 0, Width),
		y:           clamp(y, 0, Height),
		nextStrikeA: k.simulationTime + 650,
		expiresAt:   k.simulationTime + 8200,
	})
}

func (k *Kernel) moveHero(x, y float64) {
	k.hero.targetX = clamp(x, 35, Width-35)
	k.hero.targetY = clamp(y, 70, Height-35)
}

func (k *Kernel) adjustEconomy(goldDelta, livesDelta float64) {
	delta := jsRound(goldDelta)
	k.gold += delta
	if delta >= 0 {
		k.earnedGold += delta
	} else {
		k.spentGold += math.Abs(delta)
	}
	k.lives = math.Max(0, k.lives+jsRound(livesDelta))
	if k.lives <= 0 {
		k.endBattle(false)
	}
}

// ---------------------------------------------------------------- simulation

func (k *Kernel) snapshot() []*enemyState {
	out := make([]*enemyState, len(k.enemies))
	copy(out, k.enemies)
	return out
}

// Tick advances the battle by one fixed step.
func (k *Kernel) Tick() {
	if k.completed {
		return
	}
	k.tickCount++
	k.simulationTime += TickMS
	time := k.simulationTime
	if !k.waveActive && time >= k.nextWaveAt {
		k.startWave(false)
	}
	if k.waveActive {
		k.processSpawnQueue(time)
	}
	for _, enemy := range k.snapshot() {
		k.updateEnemy(enemy, time)
		if k.completed {
			return
		}
	}
	towers := make([]*towerState, len(k.towers))
	copy(towers, k.towers)
	for _, tower := range towers {
		k.updateTower(tower, time)
	}
	k.updateHero(time)
	k.updateReinforcements(time)
	k.updateStageGimmick(time)
	if !k.completed && k.waveActive && len(k.spawnQueue) == 0 && len(k.enemies) == 0 {
		k.completeWave()
	}
}

func (k *Kernel) startWave(requestedEarly bool) {
	if k.waveActive || k.completed {
		return
	}
	stage := k.config.Stage
	if stage.Mode == "campaign" && k.waveIndex >= len(stage.Waves) {
		return
	}
	base := stage.Waves[k.waveIndex%len(stage.Waves)]
	cycle := k.waveIndex / len(stage.Waves)
	k.spawnQueue = expandWave(base.Entries, cycle)
	k.waveActive = true
	k.waveStartedAt = k.simulationTime
	secondsSaved := math.Max(0, math.Ceil((k.nextWaveAt-k.simulationTime)/1000))
	if requestedEarly && secondsSaved > 0 {
		bonus := secondsSaved * 3
		k.gold += bonus
		k.earnedGold += bonus
	}
}

func (k *Kernel) processSpawnQueue(time float64) {
	elapsed := time - k.waveStartedAt
	for len(k.spawnQueue) > 0 && k.spawnQueue[0].at <= elapsed {
		order := k.spawnQueue[0]
		k.spawnQueue = k.spawnQueue[1:]
		k.spawnEnemy(order)
	}
}

func (k *Kernel) spawnEnemy(order spawnOrder) *enemyState {
	index := 0
	for candidate, enemy := range k.config.Enemies {
		if enemy.ID == order.enemy {
			index = candidate
			break
		}
	}
	definition := k.config.Enemies[index]
	endlessScale := 1 + float64(k.waveIndex/maxInt(1, len(k.config.Stage.Waves)))*k.config.Balance.EndlessRamp
	modifiers := map[string]bool{}
	for _, modifier := range order.modifiers {
		modifiers[modifier] = true
	}
	armored := 1.0
	if modifiers["armored"] {
		armored = 1.3
	}
	maxHP := jsRound(definition.HP * k.config.Balance.EnemyHP * endlessScale * armored)
	flying := definition.hasTrait("flying") || modifiers["flying"]
	lane := maxInt(0, minInt(order.pathIndex, len(k.lanes)-1))
	start := k.lanes[lane][0]
	swift := 1.0
	if modifiers["swift"] {
		swift = 1.24
	}
	offset := 0.0
	if flying {
		offset = 20
	}
	k.enemySequence++
	enemy := &enemyState{
		id: k.enemySequence, def: index, hp: maxHP, maxHP: maxHP,
		speed:     definition.Speed * k.config.Balance.EnemySpeed * swift,
		x:         start.X,
		y:         start.Y - offset,
		lane:      lane,
		pathIndex: 1, slowFactor: 1,
		healAt:    k.simulationTime + 2500,
		modifiers: modifiers, phases: map[string]bool{}, alive: true,
	}
	k.enemies = append(k.enemies, enemy)
	k.spawned++
	k.spawnedByEnemy[definition.ID]++
	return enemy
}

func (k *Kernel) hasTrait(enemy *enemyState, trait string) bool {
	return k.config.Enemies[enemy.def].hasTrait(trait) || enemy.modifiers[trait]
}

func (k *Kernel) traitSet(enemy *enemyState) map[string]bool {
	traits := map[string]bool{}
	for _, trait := range k.config.Enemies[enemy.def].Traits {
		traits[trait] = true
	}
	for trait := range enemy.modifiers {
		traits[trait] = true
	}
	return traits
}

func (k *Kernel) updateEnemy(enemy *enemyState, time float64) {
	if !enemy.alive {
		return
	}
	definition := k.config.Enemies[enemy.def]
	if k.hasTrait(enemy, "regenerating") && time >= enemy.healAt {
		enemy.hp = math.Min(enemy.maxHP, enemy.hp+float64(enemy.maxHP*0.025))
		enemy.healAt = time + 1600
	}
	if k.hasTrait(enemy, "healer") && time >= enemy.healAt {
		for _, ally := range k.enemies {
			if ally.id != enemy.id && distance(enemy.x, enemy.y, ally.x, ally.y) < 105 {
				ally.hp = math.Min(ally.maxHP, ally.hp+float64(ally.maxHP*0.06))
			}
		}
		enemy.healAt = time + 2300
	}
	phasingSkip := k.hasTrait(enemy, "phasing") && int(math.Floor(time/500))%3 == 0
	if !k.hasTrait(enemy, "flying") && !phasingSkip {
		var barracks *towerState
		for _, tower := range k.towers {
			if !k.config.Towers[tower.def].Blocking || tower.disabledUntil > time {
				continue
			}
			inReach := false
			for _, soldier := range tower.soldiers {
				if distance(enemy.x, enemy.y, soldier.X, soldier.Y) <= definition.Radius+22 {
					inReach = true
					break
				}
			}
			if !inReach {
				continue
			}
			if tower.blocked[enemy.id] || len(tower.blocked) < len(tower.soldiers) {
				barracks = tower
				break
			}
		}
		if barracks != nil {
			barracks.blocked[enemy.id] = true
			if time-enemy.lastAttack >= 850 {
				enemy.lastAttack = time
				if k.hasTrait(enemy, "siege") {
					k.siegeDisrupt(enemy)
				}
			}
			return
		}
	}
	if k.hero.deadUntil == 0 && !k.hasTrait(enemy, "flying") &&
		distance(enemy.x, enemy.y, k.hero.x, k.hero.y) <= definition.Radius+28 {
		if time-enemy.lastAttack >= 850 {
			enemy.lastAttack = time
			k.damageHero(math.Max(8, float64(definition.LifeDamage*11)+float64(enemy.maxHP*0.008)))
			if k.hasTrait(enemy, "siege") {
				k.siegeDisrupt(enemy)
			}
		}
		return
	}
	path := k.lanes[enemy.lane]
	if enemy.pathIndex >= len(path) {
		k.enemyEscaped(enemy)
		return
	}
	point := path[enemy.pathIndex]
	speed := enemy.speed * movementMultiplier(
		k.traitSet(enemy),
		enemy.hp/enemy.maxHP,
		time < enemy.slowUntil,
		enemy.slowFactor,
		time < enemy.hasteUntil,
	)
	displayYOffset := 0.0
	if k.hasTrait(enemy, "flying") {
		displayYOffset = 20
	}
	targetY := point.Y - displayYOffset
	travel := distance(enemy.x, enemy.y, point.X, targetY)
	step := speed * (float64(TickMS) / 1000)
	if travel <= step {
		enemy.x = point.X
		enemy.y = targetY
		enemy.pathIndex++
	} else {
		enemy.x += float64((point.X - enemy.x) / travel * step)
		enemy.y += float64((targetY - enemy.y) / travel * step)
	}
	enemy.pathProgres = normalizedPathProgress(path, enemy.pathIndex, Point{X: enemy.x, Y: enemy.y}, displayYOffset)
}

func (k *Kernel) enemyEscaped(enemy *enemyState) {
	definition := k.config.Enemies[enemy.def]
	k.lives = math.Max(0, k.lives-definition.LifeDamage)
	k.escaped++
	k.escapedByEnemy[definition.ID]++
	k.removeEnemy(enemy)
	if k.lives <= 0 {
		k.endBattle(false)
	}
}

func (k *Kernel) statsFor(tower *towerState) towerStats {
	definition := k.config.Towers[tower.def]
	levelDamage := []float64{1, 1.45, 2.05}
	levelRange := []float64{1, 1.08, 1.16}
	damageScale := 1.0
	rangeScale := 1.0
	if tower.level >= 1 && tower.level <= 3 {
		damageScale = levelDamage[tower.level-1]
		rangeScale = levelRange[tower.level-1]
	}
	var branch *Branch
	for index := range definition.Branches {
		if definition.Branches[index].ID == tower.branch {
			branch = &definition.Branches[index]
			break
		}
	}
	stats := towerStats{
		damage:   definition.Damage * damageScale,
		rng:      definition.Range * rangeScale,
		fireRate: definition.FireRate,
	}
	if branch != nil {
		stats.damage *= branch.DamageMultiplier
		stats.rng *= branch.RangeMultiplier
		stats.fireRate *= branch.RateMultiplier
		stats.pierce = int(branch.Pierce)
	}
	switch {
	case branch != nil && branch.Splash >= 0:
		stats.splash = branch.Splash
	case definition.DamageType == "siege":
		stats.splash = 48
	}
	switch {
	case branch != nil && branch.Slow >= 0:
		stats.slow = branch.Slow
	case definition.ID == "windward":
		stats.slow = 0.52
	case definition.DamageType == "frost":
		stats.slow = 0.2
	}
	return stats
}

func (k *Kernel) effectiveness(tower Tower, enemy Enemy) float64 {
	if enemy.ThreatType == "" {
		return 1
	}
	matched := false
	for _, item := range tower.EffectiveAgainst {
		if item == enemy.ThreatType {
			matched = true
			break
		}
	}
	if !matched {
		return 1
	}
	multiplier := tower.EffectiveMultiplier
	if multiplier < 0 {
		multiplier = 1.5
	}
	return math.Max(1, multiplier)
}

func (k *Kernel) updateTower(tower *towerState, time float64) {
	definition := k.config.Towers[tower.def]
	stats := k.statsFor(tower)
	if tower.disabledUntil > time || time-tower.lastShot < stats.fireRate*1000 {
		return
	}
	skyBranch := ""
	if len(definition.Branches) > 1 {
		skyBranch = definition.Branches[1].ID
	}
	targets := make([]*enemyState, 0, len(k.enemies))
	for _, enemy := range k.enemies {
		if !enemy.alive {
			continue
		}
		reach := stats.rng
		if k.hasTrait(enemy, "stealth") && enemy.pathProgres < 0.72 {
			reach = stats.rng * 0.62
		}
		if distance(tower.x, tower.y, enemy.x, enemy.y) > reach {
			continue
		}
		if definition.Blocking && !(skyBranch != "" && tower.branch == skyBranch) && k.hasTrait(enemy, "flying") {
			continue
		}
		targets = append(targets, enemy)
	}
	if len(targets) == 0 {
		return
	}
	sort.SliceStable(targets, func(i, j int) bool {
		return targetComparator(tower.targeting, tower.x, tower.y, targets[i], targets[j]) < 0
	})
	tower.lastShot = time
	k.fireTower(tower, targets[0], stats)
}

func (k *Kernel) fireTower(tower *towerState, target *enemyState, stats towerStats) {
	definition := k.config.Towers[tower.def]
	targetX := target.x
	targetY := target.y
	profileMultiplier := 1.0
	for _, profile := range definition.Profiles {
		if profile.ID == tower.profile {
			profileMultiplier = profile.DamageMultiplier
			break
		}
	}
	k.damageEnemy(target,
		stats.damage*profileMultiplier*k.effectiveness(definition, k.config.Enemies[target.def]),
		definition.DamageType, stats.slow)
	if stats.splash != 0 {
		for _, enemy := range k.snapshot() {
			if enemy.id != target.id && distance(targetX, targetY, enemy.x, enemy.y) <= stats.splash {
				k.damageEnemy(enemy,
					stats.damage*profileMultiplier*0.48*k.effectiveness(definition, k.config.Enemies[enemy.def]),
					definition.DamageType, stats.slow)
			}
		}
	}
	if stats.pierce > 0 {
		candidates := make([]*enemyState, 0, len(k.enemies))
		for _, enemy := range k.enemies {
			if enemy.id != target.id && distance(tower.x, tower.y, enemy.x, enemy.y) <= stats.rng {
				candidates = append(candidates, enemy)
			}
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			return candidates[i].pathProgres > candidates[j].pathProgres
		})
		if len(candidates) > stats.pierce {
			candidates = candidates[:stats.pierce]
		}
		for _, enemy := range candidates {
			k.damageEnemy(enemy,
				stats.damage*profileMultiplier*0.62*k.effectiveness(definition, k.config.Enemies[enemy.def]),
				definition.DamageType, stats.slow)
		}
	}
}

func (k *Kernel) damageEnemy(enemy *enemyState, amount float64, damageType string, slow float64) {
	if !enemy.alive {
		return
	}
	if k.hasTrait(enemy, "phasing") && damageType != "skill" {
		k.hitSequence++
		if k.hitSequence%4 == 0 {
			return
		}
	}
	definition := k.config.Enemies[enemy.def]
	enemy.hp -= effectiveDamage(amount, damageType, definition.Armor, k.traitSet(enemy))
	if slow != 0 && !k.hasTrait(enemy, "boss") && !k.hasTrait(enemy, "immune_stun") {
		enemy.slowUntil = k.simulationTime + 1900
		enemy.slowFactor = math.Max(0.35, 1-slow)
	}
	if k.hasTrait(enemy, "boss") {
		k.handleBossPhases(enemy)
	}
	if enemy.hp <= 0 {
		k.killEnemy(enemy)
	}
}

func (k *Kernel) killEnemy(enemy *enemyState) {
	if !enemy.alive {
		return
	}
	enemy.alive = false
	definition := k.config.Enemies[enemy.def]
	k.gold += definition.Reward
	k.earnedGold += definition.Reward
	k.kills++
	k.defeatedByEnemy[definition.ID]++
	k.heroGainXP(1)
	if k.hasTrait(enemy, "splitting") {
		for index := 0; index < 2; index++ {
			k.spawnSplit(enemy)
		}
	}
	k.removeEnemy(enemy)
}

func (k *Kernel) spawnSplit(parent *enemyState) {
	index := 0
	for candidate, enemy := range k.config.Enemies {
		if enemy.ID == "mireling" {
			index = candidate
			break
		}
	}
	definition := k.config.Enemies[index]
	k.enemySequence++
	id := k.enemySequence
	k.enemies = append(k.enemies, &enemyState{
		id: id, def: index, hp: 22, maxHP: 22,
		speed:       parent.speed * 1.2,
		x:           parent.x + jitter(id*2, 8),
		y:           parent.y + jitter(id*2+1, 8),
		lane:        parent.lane,
		pathIndex:   parent.pathIndex,
		pathProgres: parent.pathProgres,
		slowFactor:  1,
		modifiers:   map[string]bool{}, phases: map[string]bool{}, alive: true,
	})
	k.spawned++
	k.spawnedByEnemy[definition.ID]++
}

func (k *Kernel) removeEnemy(enemy *enemyState) {
	enemy.alive = false
	for index, item := range k.enemies {
		if item == enemy {
			k.enemies = append(k.enemies[:index], k.enemies[index+1:]...)
			break
		}
	}
	for _, tower := range k.towers {
		delete(tower.blocked, enemy.id)
	}
}

func (k *Kernel) updateHero(time float64) {
	hero := &k.hero
	if hero.deadUntil > 0 {
		if time < hero.deadUntil {
			return
		}
		hero.deadUntil = 0
		hero.hp = hero.maxHP
		first := k.lanes[0]
		start := first[minInt(2, len(first)-1)]
		hero.x = start.X
		hero.y = start.Y - 58
		hero.targetX = hero.x
		hero.targetY = hero.y
	}
	travel := distance(hero.x, hero.y, hero.targetX, hero.targetY)
	if travel > 4 {
		step := math.Min(travel, k.config.Hero.Speed*float64(TickMS)/1000)
		hero.x += float64((hero.targetX - hero.x) / travel * step)
		hero.y += float64((hero.targetY - hero.y) / travel * step)
	}
	if time-hero.lastShot < math.Max(420, 920-float64(hero.level*35)) {
		return
	}
	candidates := make([]*enemyState, 0, len(k.enemies))
	for _, enemy := range k.enemies {
		if distance(hero.x, hero.y, enemy.x, enemy.y) <= k.config.Hero.Range {
			candidates = append(candidates, enemy)
		}
	}
	if len(candidates) == 0 {
		return
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].pathProgres > candidates[j].pathProgres
	})
	target := candidates[0]
	hero.lastShot = time
	hero.attackCount++
	damage := k.heroDamage * (1 + float64(hero.level-1)*0.12)
	heroID := k.config.Hero.ID
	if hero.attackCount%5 == 0 {
		damage *= 1.75
	}
	slow := 0.0
	if heroID == "nyra" {
		slow = 0.25
	}
	k.damageEnemy(target, damage, "hero", slow)
	if hero.attackCount%11 == 0 {
		wideSlow := 0.2
		if heroID == "brann" {
			wideSlow = 0.7
		}
		for _, enemy := range k.snapshot() {
			if enemy.id != target.id && distance(target.x, target.y, enemy.x, enemy.y) < 82 {
				k.damageEnemy(enemy, damage*0.55, "hero", wideSlow)
			}
		}
	}
	if hero.attackCount%25 == 0 {
		ultimateSlow := 0.15
		if heroID == "nyra" {
			ultimateSlow = 0.6
		}
		for _, enemy := range k.snapshot() {
			k.damageEnemy(enemy, damage*0.72, "hero", ultimateSlow)
		}
	}
}

func (k *Kernel) damageHero(amount float64) {
	hero := &k.hero
	if hero.deadUntil > 0 {
		return
	}
	hero.hp = math.Max(0, hero.hp-amount)
	if hero.hp > 0 {
		return
	}
	hero.deadUntil = k.simulationTime + float64(k.config.Hero.RespawnSeconds*1000)
}

func (k *Kernel) heroGainXP(amount float64) {
	hero := &k.hero
	if hero.level >= 10 {
		return
	}
	hero.xp += amount
	required := math.Inf(1)
	if hero.level < len(k.config.Balance.HeroLevelXP) {
		required = k.config.Balance.HeroLevelXP[hero.level]
	}
	if hero.xp >= required {
		hero.level++
	}
}

func (k *Kernel) siegeDisrupt(enemy *enemyState) {
	candidates := make([]*towerState, 0, len(k.towers))
	for _, tower := range k.towers {
		if tower.disabledUntil <= k.simulationTime {
			candidates = append(candidates, tower)
		}
	}
	if len(candidates) == 0 {
		return
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return distance(enemy.x, enemy.y, candidates[i].x, candidates[i].y) <
			distance(enemy.x, enemy.y, candidates[j].x, candidates[j].y)
	})
	candidates[0].disabledUntil = k.simulationTime + 2200
}

func (k *Kernel) handleBossPhases(enemy *enemyState) {
	definition := k.config.Enemies[enemy.def]
	ratio := enemy.hp / enemy.maxHP
	if ratio <= 0.66 && !enemy.phases["phase-2"] {
		enemy.phases["phase-2"] = true
		if definition.ID == "hollow_king" {
			if len(k.towers) > 0 {
				k.towers[k.waveIndex%maxInt(1, len(k.towers))].disabledUntil = k.simulationTime + 5000
			}
		} else {
			k.spawnBossMinions(enemy, 3, "glintfox")
		}
	}
	if ratio <= 0.33 && !enemy.phases["phase-3"] {
		enemy.phases["phase-3"] = true
		if definition.ID == "hollow_king" {
			k.spawnBossMinions(enemy, 5, "veilrunner")
		} else {
			enemy.speed *= 1.65
			enemy.modifiers["swift"] = true
		}
	}
}

func (k *Kernel) spawnBossMinions(boss *enemyState, count int, enemyID string) {
	for index := 0; index < count; index++ {
		minion := k.spawnEnemy(spawnOrder{enemy: enemyID, pathIndex: boss.lane, modifiers: []string{"summoned"}})
		minion.pathIndex = boss.pathIndex
		minion.pathProgres = boss.pathProgres
		minion.x = boss.x + jitter(minion.id*2, 18)
		minion.y = boss.y + jitter(minion.id*2+1, 18)
	}
}

func (k *Kernel) updateStageGimmick(time float64) {
	gimmick := k.config.Stage.Gimmick
	if gimmick == "" || time < k.nextGimmickAt {
		return
	}
	k.nextGimmickAt = time + 12000
	switch gimmick {
	case "ember_vents":
		hotspots := []Point{{X: 360, Y: 350}, {X: 820, Y: 280}}
		point := hotspots[(k.waveIndex+k.kills)%len(hotspots)]
		for _, enemy := range k.snapshot() {
			if distance(point.X, point.Y, enemy.x, enemy.y) < 115 {
				k.damageEnemy(enemy, 58, "skill", 0)
			}
		}
	case "winter_blessing":
		for _, enemy := range k.enemies {
			enemy.slowUntil = time + 3500
			enemy.slowFactor = 0.62
		}
	default:
		for _, enemy := range k.enemies {
			if !k.hasTrait(enemy, "boss") {
				enemy.hasteUntil = math.Max(enemy.hasteUntil, time+3500)
			}
		}
	}
}

func (k *Kernel) updateReinforcements(time float64) {
	for _, unit := range k.reinforce {
		if time >= unit.expiresAt {
			continue
		}
		for time >= unit.nextStrikeA {
			unit.nextStrikeA += 650
			candidates := make([]*enemyState, 0, len(k.enemies))
			for _, enemy := range k.enemies {
				if !k.hasTrait(enemy, "flying") && distance(unit.x, unit.y, enemy.x, enemy.y) < 70 {
					candidates = append(candidates, enemy)
				}
			}
			if len(candidates) == 0 {
				continue
			}
			sort.SliceStable(candidates, func(i, j int) bool {
				return candidates[i].pathProgres > candidates[j].pathProgres
			})
			k.damageEnemy(candidates[0], 28, "hero", 0.35)
		}
	}
	kept := k.reinforce[:0]
	for _, unit := range k.reinforce {
		if time < unit.expiresAt {
			kept = append(kept, unit)
		}
	}
	k.reinforce = kept
}

func (k *Kernel) completeWave() {
	stage := k.config.Stage
	wave := stage.Waves[k.waveIndex%len(stage.Waves)]
	k.gold += wave.Reward
	k.earnedGold += wave.Reward
	k.waveIndex++
	k.waveActive = false
	if k.completed {
		return
	}
	if stage.Mode == "campaign" && k.waveIndex >= len(stage.Waves) {
		k.endBattle(true)
		return
	}
	k.nextWaveAt = k.simulationTime + 10000
}

func (k *Kernel) endBattle(victory bool) {
	if k.completed {
		return
	}
	k.completed = true
	k.victory = victory
}

// Replay runs a whole ledger and reports what actually happened.
func Replay(config Config, commands []Command, ticks, accountHeroLevel int) Outcome {
	kernel := New(config, accountHeroLevel)
	cursor := 0
	for tick := 0; tick < ticks; tick++ {
		for cursor < len(commands) && commands[cursor].Tick <= tick {
			kernel.Apply(commands[cursor])
			cursor++
		}
		if kernel.Finished() {
			break
		}
		kernel.Tick()
	}
	for cursor < len(commands) {
		kernel.Apply(commands[cursor])
		cursor++
	}
	return kernel.Outcome()
}
