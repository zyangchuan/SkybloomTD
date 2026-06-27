package gameobject

import (
	"math"

	"github.com/google/uuid"
)

type ProjectileType string

const (
	ProjectileTypeLocked      ProjectileType = "locked"
	ProjectileTypeDirectional ProjectileType = "directional"
)

type Projectile struct {
	ID              string
	Type            ProjectileType
	Damage          float64
	ProjectileSpeed float64
	Position        Position
	Direction       Vector
	TargetID        string
	RemainingRange  float64
	HitRadius       float64
}

func NewLockedProjectile(bird Bird, target Enemy, projectileSpeed float64) Projectile {
	if projectileSpeed <= 0 {
		projectileSpeed = LockedProjectileSpeed
	}
	return Projectile{
		ID:              uuid.NewString(),
		Type:            ProjectileTypeLocked,
		Damage:          float64(bird.Stats.Damage),
		ProjectileSpeed: projectileSpeed,
		Position:        bird.Position,
		Direction:       bird.Position.DirectionTo(target.Position),
		TargetID:        target.ID,
		RemainingRange:  bird.Position.DistanceTo(target.Position),
	}
}

func NewDirectionalProjectile(bird Bird, direction Vector) Projectile {
	return Projectile{
		ID:              uuid.NewString(),
		Type:            ProjectileTypeDirectional,
		Damage:          float64(bird.Stats.Damage),
		ProjectileSpeed: bird.Stats.ProjectileSpeed,
		Position:        bird.Position,
		Direction:       direction.Normalize(),
		RemainingRange:  bird.Stats.Range,
		HitRadius:       DirectionalHitRadius,
	}
}

func (p *Projectile) Move(deltaSeconds float64) {
	if p == nil || deltaSeconds <= 0 || p.ProjectileSpeed <= 0 || p.RemainingRange <= 0 {
		return
	}
	distance := math.Min(p.ProjectileSpeed*deltaSeconds, p.RemainingRange)
	direction := p.Direction.Normalize()
	p.Position.X += direction.X * distance
	p.Position.Y += direction.Y * distance
	p.RemainingRange -= distance
	if p.RemainingRange < 0 {
		p.RemainingRange = 0
	}
}

func (p Projectile) IsExpired() bool {
	return p.RemainingRange <= 0
}

func (p Projectile) HasArrived() bool {
	return p.Type == ProjectileTypeLocked && p.RemainingRange <= 0
}

func (p *Projectile) ApplyLockedDamage(target *Enemy) bool {
	if p == nil || p.Type != ProjectileTypeLocked || !p.HasArrived() || target == nil || target.ID != p.TargetID || !target.IsAlive() {
		return false
	}
	target.TakeDamage(p.Damage)
	return true
}

func (p *Projectile) Collide(enemies []*Enemy) *Enemy {
	if p == nil || p.Type != ProjectileTypeDirectional || p.IsExpired() {
		return nil
	}
	for _, enemy := range enemies {
		if enemy == nil || !enemy.IsAlive() {
			continue
		}
		if p.Position.DistanceTo(enemy.Position) <= p.HitRadius {
			enemy.TakeDamage(p.Damage)
			p.RemainingRange = 0
			return enemy
		}
	}
	return nil
}
