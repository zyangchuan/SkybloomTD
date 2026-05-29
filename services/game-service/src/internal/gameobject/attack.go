package gameobject

import "math"

const (
	LockedProjectileSpeed = 16.0
	DirectionalHitRadius  = 0.35
	SplashSpreadRadians   = math.Pi / 12
)

type SingleAttack struct {
	ProjectileSpeed float64
}

func (a SingleAttack) Attack(bird Bird, target Smog) []Projectile {
	projectileSpeed := a.ProjectileSpeed
	if projectileSpeed <= 0 {
		projectileSpeed = LockedProjectileSpeed
	}
	return []Projectile{
		NewLockedProjectile(bird, target, projectileSpeed),
	}
}

type SplashAttack struct{}

func (a SplashAttack) Attack(bird Bird, target Smog) []Projectile {
	direction := bird.Position.DirectionTo(target.Position)
	return []Projectile{
		NewDirectionalProjectile(bird, direction),
		NewDirectionalProjectile(bird, direction.Rotate(-SplashSpreadRadians)),
		NewDirectionalProjectile(bird, direction.Rotate(SplashSpreadRadians)),
	}
}
