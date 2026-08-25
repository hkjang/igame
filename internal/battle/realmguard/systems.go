package realmguard

import (
	"math"
	"sort"
)

// Ports of web/src/games/realmguard/systems. Kept in one file so the two
// implementations can be diffed side by side.

func pathLength(path []Point) float64 {
	total := 0.0
	for index := 1; index < len(path); index++ {
		total += distance(path[index-1].X, path[index-1].Y, path[index].X, path[index].Y)
	}
	return total
}

// normalizedPathProgress reports comparable 0..1 progress even when lanes use
// different waypoint counts.
func normalizedPathProgress(path []Point, nextPointIndex int, current Point, displayYOffset float64) float64 {
	total := pathLength(path)
	if total <= 0 || len(path) < 2 {
		return 0
	}
	if nextPointIndex >= len(path) {
		return 1
	}
	next := maxInt(1, nextPointIndex)
	travelled := 0.0
	for index := 1; index < next; index++ {
		travelled += distance(path[index-1].X, path[index-1].Y, path[index].X, path[index].Y)
	}
	anchor := path[next-1]
	travelled += math.Min(
		distance(anchor.X, anchor.Y, current.X, current.Y+displayYOffset),
		distance(anchor.X, anchor.Y, path[next].X, path[next].Y),
	)
	return math.Max(0, math.Min(1, travelled/total))
}

func closestPointOnSegment(point, start, end Point) Point {
	dx := end.X - start.X
	dy := end.Y - start.Y
	lengthSquared := float64(dx*dx) + float64(dy*dy)
	if lengthSquared == 0 {
		return start
	}
	projection := math.Max(0, math.Min(1,
		(float64((point.X-start.X)*dx)+float64((point.Y-start.Y)*dy))/lengthSquared))
	return Point{X: start.X + float64(dx*projection), Y: start.Y + float64(dy*projection)}
}

// closestPointOnPaths finds a true segment projection across every lane, not
// just a waypoint.
func closestPointOnPaths(paths [][]Point, point Point) (Point, int) {
	closest := point
	if len(paths) > 0 && len(paths[0]) > 0 {
		closest = paths[0][0]
	}
	best := math.Inf(1)
	laneIndex := 0
	for pathIndex, path := range paths {
		for index := 1; index < len(path); index++ {
			candidate := closestPointOnSegment(point, path[index-1], path[index])
			candidateDistance := distance(point.X, point.Y, candidate.X, candidate.Y)
			if candidateDistance < best {
				closest = candidate
				best = candidateDistance
				laneIndex = pathIndex
			}
		}
	}
	return closest, laneIndex
}

func targetComparator(mode string, originX, originY float64, a, b *enemyState) float64 {
	switch mode {
	case "last":
		return a.pathProgres - b.pathProgres
	case "strong":
		return b.hp - a.hp
	case "weak":
		return a.hp - b.hp
	case "closest":
		return distance(originX, originY, a.x, a.y) - distance(originX, originY, b.x, b.y)
	}
	return b.pathProgres - a.pathProgres
}

func effectiveDamage(amount float64, damageType string, armor float64, traits map[string]bool) float64 {
	armorBonus := 0.0
	if traits["armored"] {
		armorBonus = 0.18
	}
	baseArmor := math.Min(0.75, armor+armorBonus)
	magic := damageType == "arcane" || damageType == "magic"
	physicalArmor := baseArmor
	switch {
	case damageType == "true" || magic || damageType == "skill":
		physicalArmor = 0
	case damageType == "siege":
		physicalArmor = baseArmor * 0.45
	}
	magicResistance := 0.0
	if magic && traits["magic_resist"] {
		magicResistance = 0.48
	}
	return math.Max(1, amount*(1-physicalArmor)*(1-magicResistance))
}

func movementMultiplier(traits map[string]bool, hpRatio float64, slowed bool, slowFactor float64, hasted bool) float64 {
	slow := 1.0
	if slowed && !traits["immune_stun"] {
		slow = slowFactor
	}
	berserk := 1.0
	if traits["berserk"] && hpRatio <= 0.35 {
		berserk = 1.5
	}
	haste := 1.0
	if hasted {
		haste = 1.12
	}
	return slow * haste * berserk
}

// expandWave turns sequential and parallel wave groups into a deterministic,
// time-sorted spawn plan.
func expandWave(entries []WaveEntry, cycle int) []spawnOrder {
	plan := make([]spawnOrder, 0, len(entries)*8)
	cursor := 0.0
	for _, entry := range entries {
		delay := math.Max(0, entry.Delay) * 1000
		start := cursor + delay
		if entry.Parallel {
			start = delay
		}
		count := math.Max(1, entry.Count+float64(cycle*2))
		interval := math.Max(150, entry.Interval*1000)
		pathIndex := maxInt(0, int(math.Floor(entry.PathIndex)))
		modifiers := append([]string(nil), entry.Modifiers...)
		for index := 0; float64(index) < count; index++ {
			plan = append(plan, spawnOrder{
				enemy:     entry.Enemy,
				at:        start + float64(float64(index)*interval),
				pathIndex: pathIndex,
				modifiers: modifiers,
			})
		}
		if !entry.Parallel {
			cursor = start + float64(count*interval)
		}
	}
	sort.SliceStable(plan, func(i, j int) bool {
		if plan[i].at != plan[j].at {
			return plan[i].at < plan[j].at
		}
		if plan[i].pathIndex != plan[j].pathIndex {
			return plan[i].pathIndex < plan[j].pathIndex
		}
		return plan[i].enemy < plan[j].enemy
	})
	return plan
}
