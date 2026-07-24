package gameobject

import "math"

const (
	DefaultSplashSpreadRadians = math.Pi / 12
	FeatherHalfWidthRadians    = math.Pi / 36
)

type SingleAttack struct{}

func (a SingleAttack) RequiresTarget() bool {
	return true
}

func (a SingleAttack) Attack(bird Bird, target Enemy, enemies []Enemy) []AttackHit {
	if !target.IsAlive() {
		return nil
	}
	if !hasAttackPierce(bird) {
		return attackHit(bird, target)
	}
	lineDirection := bird.Position.DirectionTo(target.Position)
	if lineDirection == (Vector{}) {
		return nil
	}
	lineEnemies := enemiesOnAttackLine(bird.Position, lineDirection, math.Inf(1), enemies, nil)
	return attackHits(bird, limitEnemiesByPierce(lineEnemies, bird.Stats.Pierce))
}

type SplashAttack struct{}

func (a SplashAttack) RequiresTarget() bool {
	return true
}

func (a SplashAttack) Attack(bird Bird, target Enemy, enemies []Enemy) []AttackHit {
	if !target.IsAlive() {
		return nil
	}
	center := bird.Position.DirectionTo(target.Position)
	if center == (Vector{}) {
		return nil
	}
	spread := bird.Stats.Spread
	if spread <= 0 {
		spread = DefaultSplashSpreadRadians
	}
	featherDirections := []Vector{
		center.Rotate(-spread),
		center,
		center.Rotate(spread),
	}
	hits := make([]AttackHit, 0, len(featherDirections))
	hitEnemyIDs := make(map[string]bool, len(featherDirections))
	for _, featherDirection := range featherDirections {
		lineEnemies := enemiesOnAttackLine(bird.Position, featherDirection, splashAttackLineRange(bird), enemies, hitEnemyIDs)
		if len(lineEnemies) == 0 {
			continue
		}
		if !hasAttackPierce(bird) {
			lineEnemies = lineEnemies[:1]
		} else {
			lineEnemies = limitEnemiesByPierce(lineEnemies, bird.Stats.Pierce)
		}
		for _, enemy := range lineEnemies {
			hitEnemyIDs[enemy.ID] = true
		}
		hits = append(hits, attackHits(bird, lineEnemies)...)
	}
	return hits
}

func splashAttackLineRange(bird Bird) float64 {
	if hasAttackPierce(bird) {
		return math.Inf(1)
	}
	return bird.Stats.Range
}

func hasAttackPierce(bird Bird) bool {
	return bird.Stats.Pierce > 0
}

func limitEnemiesByPierce(enemies []Enemy, pierce int) []Enemy {
	if pierce <= 0 || len(enemies) <= pierce {
		return enemies
	}
	return enemies[:pierce]
}

func enemiesOnAttackLine(origin Position, direction Vector, maxRange float64, enemies []Enemy, excluded map[string]bool) []Enemy {
	lineEnemies := make([]Enemy, 0)
	for _, enemy := range enemies {
		if excluded[enemy.ID] || !enemyOnAttackLine(origin, direction, maxRange, enemy) {
			continue
		}
		lineEnemies = append(lineEnemies, enemy)
	}
	sortEnemiesForAttackLine(lineEnemies)
	return lineEnemies
}

func sortEnemiesForAttackLine(enemies []Enemy) {
	for i := 1; i < len(enemies); i++ {
		enemy := enemies[i]
		j := i - 1
		for j >= 0 && enemies[j].PathIndex < enemy.PathIndex {
			enemies[j+1] = enemies[j]
			j--
		}
		enemies[j+1] = enemy
	}
}

func enemyOnAttackLine(origin Position, lineDirection Vector, maxRange float64, enemy Enemy) bool {
	if !enemy.IsAlive() {
		return false
	}
	distance := origin.DistanceTo(enemy.Position)
	if distance <= 0 || distance > maxRange {
		return false
	}
	enemyDirection := origin.DirectionTo(enemy.Position)
	minDot := math.Cos(FeatherHalfWidthRadians)
	return enemyDirection.Dot(lineDirection) >= minDot
}

func attackHit(bird Bird, enemy Enemy) []AttackHit {
	return []AttackHit{{
		EnemyID: enemy.ID,
		Damage:  float64(bird.Stats.Damage),
	}}
}

func attackHits(bird Bird, enemies []Enemy) []AttackHit {
	hits := make([]AttackHit, 0, len(enemies))
	for _, enemy := range enemies {
		hits = append(hits, attackHit(bird, enemy)...)
	}
	return hits
}

type RingAttack struct{}

func (a RingAttack) RequiresTarget() bool {
	return false
}

func (a RingAttack) Attack(bird Bird, _ Enemy, enemies []Enemy) []AttackHit {
	hits := make([]AttackHit, 0)
	for _, enemy := range enemies {
		if !enemy.IsAlive() {
			continue
		}
		if bird.Position.DistanceTo(enemy.Position) > bird.Stats.Range {
			continue
		}
		hits = append(hits, AttackHit{
			EnemyID: enemy.ID,
			Damage:  float64(bird.Stats.Damage),
		})
	}
	return hits
}
