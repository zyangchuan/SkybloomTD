package gameobject

import (
	"errors"
	"math"
)

const (
	BirdTypeSparrow    = "sparrow"
	BirdTypeWoodpecker = "woodpecker"
	BirdTypeEagle      = "eagle"
	BirdTypePeacock    = "peacock"

	AttackTypeSingle = "single"
	AttackTypeSplash = "splash"

	StandardProjectileSpeed = 8.0
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
	stats, err := BirdStatsForType(birdType)
	if err != nil {
		return Bird{}, err
	}
	behaviour, err := AttackBehaviourForType(birdType)
	if err != nil {
		return Bird{}, err
	}
	return Bird{
		ID:              id,
		Position:        position,
		Stats:           stats,
		AttackBehaviour: behaviour,
		LastFiredAtTick: -1,
	}, nil
}

func BirdStatsForType(birdType string) (BirdStats, error) {
	switch birdType {
	case BirdTypeSparrow:
		return BirdStats{
			Damage:          10,
			ProjectileSpeed: StandardProjectileSpeed,
			FireRate:        1.0,
			Range:           3.5,
			Cost:            50,
		}, nil
	case BirdTypeWoodpecker:
		return BirdStats{
			Damage:          6,
			ProjectileSpeed: StandardProjectileSpeed,
			FireRate:        2.0,
			Range:           3.5,
			Cost:            65,
		}, nil
	case BirdTypeEagle:
		return BirdStats{
			Damage:          30,
			ProjectileSpeed: StandardProjectileSpeed,
			FireRate:        0.4,
			Range:           6.0,
			Cost:            130,
		}, nil
	case BirdTypePeacock:
		return BirdStats{
			Damage:          7,
			ProjectileSpeed: StandardProjectileSpeed,
			FireRate:        1.0,
			Range:           3.5,
			Cost:            90,
		}, nil
	default:
		return BirdStats{}, errors.New("unknown bird type")
	}
}

func AttackBehaviourForType(birdType string) (AttackBehaviour, error) {
	switch birdType {
	case BirdTypeSparrow, BirdTypeWoodpecker, BirdTypeEagle:
		return SingleAttack{}, nil
	case BirdTypePeacock:
		return SplashAttack{}, nil
	default:
		return nil, errors.New("unknown bird type")
	}
}

func AttackTypeForBirdType(birdType string) (string, error) {
	switch birdType {
	case BirdTypeSparrow, BirdTypeWoodpecker, BirdTypeEagle:
		return AttackTypeSingle, nil
	case BirdTypePeacock:
		return AttackTypeSplash, nil
	default:
		return "", errors.New("unknown bird type")
	}
}

func BirdTypes() []string {
	return []string{
		BirdTypeSparrow,
		BirdTypeWoodpecker,
		BirdTypeEagle,
		BirdTypePeacock,
	}
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
