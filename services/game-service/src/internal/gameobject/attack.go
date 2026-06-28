package gameobject

import "math"

const (
	FeatherSpreadRadians    = math.Pi / 12
	FeatherHalfWidthRadians = math.Pi / 36
)

type SingleAttack struct{}

func (a SingleAttack) Attack(bird Bird, target Enemy, _ []Enemy) []AttackHit {
	if !target.IsAlive() {
		return nil
	}
	return []AttackHit{{
		EnemyID: target.ID,
		Damage:  float64(bird.Stats.Damage),
	}}
}

type SplashAttack struct{}

func (a SplashAttack) Attack(bird Bird, target Enemy, enemies []Enemy) []AttackHit {
	if !target.IsAlive() {
		return nil
	}
	center := bird.Position.DirectionTo(target.Position)
	if center == (Vector{}) {
		return nil
	}
	featherDirections := []Vector{
		center,
		center.Rotate(-FeatherSpreadRadians),
		center.Rotate(FeatherSpreadRadians),
	}
	hits := make([]AttackHit, 0)
	for _, enemy := range enemies {
		if !enemy.IsAlive() {
			continue
		}
		if !enemyInFeatherFan(bird.Position, enemy.Position, bird.Stats.Range, featherDirections) {
			continue
		}
		hits = append(hits, AttackHit{
			EnemyID: enemy.ID,
			Damage:  float64(bird.Stats.Damage),
		})
	}
	return hits
}

func enemyInFeatherFan(origin Position, enemy Position, maxRange float64, featherDirections []Vector) bool {
	distance := origin.DistanceTo(enemy)
	if distance <= 0 || distance > maxRange {
		return false
	}
	direction := origin.DirectionTo(enemy)
	minDot := math.Cos(FeatherHalfWidthRadians)
	for _, featherDirection := range featherDirections {
		if direction.Dot(featherDirection) >= minDot {
			return true
		}
	}
	return false
}
