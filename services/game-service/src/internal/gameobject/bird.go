package gameobject

import (
	"math"
)

type Bird struct {
	ID              string
	Position        Position
	Stats           BirdStats
	AttackBehaviour AttackBehaviour
	LastFiredAtTick int64
}

type BirdStats struct {
	Damage          int     `json:"damage"`
	ProjectileSpeed float64 `json:"projectile_speed"`
	FireRate        float64 `json:"fire_rate"`
	Range           float64 `json:"range"`
	Cost            int     `json:"cost"`
}

type AttackBehaviour interface {
	Attack(bird Bird, target Enemy) []Projectile
}

func NewBird(id string, birdType string, position Position) (Bird, error) {
	definition, err := BirdDefinitionForType(birdType)
	if err != nil {
		return Bird{}, err
	}
	behaviour, err := AttackBehaviourForAttackType(definition.AttackType)
	if err != nil {
		return Bird{}, err
	}
	return Bird{
		ID:              id,
		Position:        position,
		Stats:           definition.Stats,
		AttackBehaviour: behaviour,
		LastFiredAtTick: -1,
	}, nil
}

func (b Bird) GetPosition() Position {
	return b.Position
}

func (b Bird) CanAttack(currentTick int64, ticksPerSecond float64) bool {
	if b.LastFiredAtTick < 0 {
		return true
	}
	if b.Stats.FireRate <= 0 || ticksPerSecond <= 0 {
		return false
	}
	cooldownTicks := int64(math.Ceil(ticksPerSecond / b.Stats.FireRate))
	return currentTick-b.LastFiredAtTick >= cooldownTicks
}

func (b Bird) TargetInRange(target Enemy) bool {
	return target.IsAlive() && b.Position.DistanceTo(target.Position) <= b.Stats.Range
}

func (b *Bird) Attack(target Enemy, currentTick int64) []Projectile {
	if b == nil || b.AttackBehaviour == nil || !b.TargetInRange(target) {
		return nil
	}
	projectiles := b.AttackBehaviour.Attack(*b, target)
	if len(projectiles) > 0 {
		b.LastFiredAtTick = currentTick
	}
	return projectiles
}
