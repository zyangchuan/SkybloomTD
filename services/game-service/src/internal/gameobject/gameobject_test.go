package gameobject

import (
	"math"
	"testing"
)

func TestPositionDistanceAndDirectionNormalization(t *testing.T) {
	origin := Position{X: 0, Y: 0}
	target := Position{X: 3, Y: 4}

	if got := origin.DistanceTo(target); !almostEqual(got, 5) {
		t.Fatalf("expected distance 5, got %f", got)
	}

	direction := origin.DirectionTo(target)
	if !almostEqual(direction.X, 0.6) || !almostEqual(direction.Y, 0.8) {
		t.Fatalf("unexpected normalized direction %+v", direction)
	}

	zero := Vector{}.Normalize()
	if zero.X != 0 || zero.Y != 0 {
		t.Fatalf("expected zero vector normalization to stay zero, got %+v", zero)
	}
}

func TestBirdCooldownUsesFireRate(t *testing.T) {
	bird := Bird{
		Stats:           BirdStats{FireRate: 2.0},
		LastFiredAtTick: -1,
	}
	if !bird.CanAttack(0, 20) {
		t.Fatal("bird should attack before it has ever fired")
	}

	bird.LastFiredAtTick = 100
	if bird.CanAttack(109, 20) {
		t.Fatal("bird should still be cooling down")
	}
	if !bird.CanAttack(110, 20) {
		t.Fatal("bird should be ready after 10 ticks at 2 shots/sec")
	}
}

func TestSingleAttackCreatesLockedProjectile(t *testing.T) {
	bird := Bird{
		ID:       "bird-1",
		Position: Position{X: 0, Y: 0},
		Stats: BirdStats{
			Damage:          10,
			ProjectileSpeed: StandardProjectileSpeed,
			Range:           5,
		},
		AttackBehaviour: SingleAttack{},
	}
	target := Enemy{ID: "enemy-1", Health: 20, Position: Position{X: 3, Y: 0}}

	projectiles := bird.AttackBehaviour.Attack(bird, target)
	if len(projectiles) != 1 {
		t.Fatalf("expected one projectile, got %d", len(projectiles))
	}
	projectile := projectiles[0]
	if projectile.Type != ProjectileTypeLocked {
		t.Fatalf("expected locked projectile, got %q", projectile.Type)
	}
	if projectile.TargetID != target.ID {
		t.Fatalf("expected target id %q, got %q", target.ID, projectile.TargetID)
	}
	if !almostEqual(projectile.ProjectileSpeed, LockedProjectileSpeed) {
		t.Fatalf("expected locked projectile speed %f, got %f", LockedProjectileSpeed, projectile.ProjectileSpeed)
	}
	if !almostEqual(projectile.RemainingRange, 3) {
		t.Fatalf("expected remaining range 3, got %f", projectile.RemainingRange)
	}
}

func TestLockedProjectileAppliesDamageOnArrivalWithoutCollision(t *testing.T) {
	bird := Bird{
		Position: Position{X: 0, Y: 0},
		Stats:    BirdStats{Damage: 10},
	}
	target := Enemy{ID: "enemy-1", Health: 20, Position: Position{X: 1, Y: 0}}
	projectile := NewLockedProjectile(bird, target, LockedProjectileSpeed)

	projectile.Move(1)
	if !projectile.HasArrived() {
		t.Fatal("expected locked projectile to arrive")
	}
	if !projectile.ApplyLockedDamage(&target) {
		t.Fatal("expected locked projectile to apply damage")
	}
	if target.Health != 10 {
		t.Fatalf("expected target health 10, got %d", target.Health)
	}
}

func TestLockedProjectileExpiresHarmlesslyIfTargetDead(t *testing.T) {
	bird := Bird{
		Position: Position{X: 0, Y: 0},
		Stats:    BirdStats{Damage: 10},
	}
	target := Enemy{ID: "enemy-1", Health: 0, Position: Position{X: 1, Y: 0}}
	projectile := NewLockedProjectile(bird, target, LockedProjectileSpeed)

	projectile.Move(1)
	if !projectile.HasArrived() {
		t.Fatal("expected locked projectile to arrive")
	}
	if projectile.ApplyLockedDamage(&target) {
		t.Fatal("dead target should not receive locked projectile damage")
	}
	if target.Health != 0 {
		t.Fatalf("expected dead target health to stay 0, got %d", target.Health)
	}
}

func TestSplashAttackCreatesThreeDirectionalProjectiles(t *testing.T) {
	bird := Bird{
		ID:       "bird-1",
		Position: Position{X: 0, Y: 0},
		Stats: BirdStats{
			Damage:          7,
			ProjectileSpeed: StandardProjectileSpeed,
			Range:           3.5,
		},
		AttackBehaviour: SplashAttack{},
	}
	target := Enemy{ID: "enemy-1", Health: 20, Position: Position{X: 1, Y: 0}}

	projectiles := bird.AttackBehaviour.Attack(bird, target)
	if len(projectiles) != 3 {
		t.Fatalf("expected three projectiles, got %d", len(projectiles))
	}
	for _, projectile := range projectiles {
		if projectile.Type != ProjectileTypeDirectional {
			t.Fatalf("expected directional projectile, got %q", projectile.Type)
		}
		if projectile.TargetID != "" {
			t.Fatalf("directional projectile should not target-lock, got target %q", projectile.TargetID)
		}
	}

	assertVector(t, projectiles[0].Direction, Vector{X: 1, Y: 0})
	assertVector(t, projectiles[1].Direction, Vector{X: math.Cos(SplashSpreadRadians), Y: -math.Sin(SplashSpreadRadians)})
	assertVector(t, projectiles[2].Direction, Vector{X: math.Cos(SplashSpreadRadians), Y: math.Sin(SplashSpreadRadians)})
}

func TestDirectionalProjectileHitsFirstEnemyWithinRadius(t *testing.T) {
	projectile := Projectile{
		Type:            ProjectileTypeDirectional,
		Damage:          5,
		ProjectileSpeed: 1,
		Position:        Position{X: 0, Y: 0},
		Direction:       Vector{X: 1, Y: 0},
		RemainingRange:  10,
		HitRadius:       DirectionalHitRadius,
	}
	first := &Enemy{ID: "first", Health: 10, Position: Position{X: 1.2, Y: 0}}
	second := &Enemy{ID: "second", Health: 10, Position: Position{X: 1.1, Y: 0}}

	projectile.Move(1)
	hit := projectile.Collide([]*Enemy{first, second})
	if hit == nil || hit.ID != first.ID {
		t.Fatalf("expected first enemy to be hit, got %+v", hit)
	}
	if first.Health != 5 {
		t.Fatalf("expected first enemy health 5, got %d", first.Health)
	}
	if second.Health != 10 {
		t.Fatalf("expected second enemy health unchanged, got %d", second.Health)
	}
	if !projectile.IsExpired() {
		t.Fatal("projectile should expire after hit")
	}
}

func TestDirectionalProjectileExpiresAfterConsumingRange(t *testing.T) {
	projectile := Projectile{
		Type:            ProjectileTypeDirectional,
		ProjectileSpeed: 1,
		Position:        Position{X: 0, Y: 0},
		Direction:       Vector{X: 1, Y: 0},
		RemainingRange:  0.5,
	}

	projectile.Move(1)
	if !projectile.IsExpired() {
		t.Fatal("expected projectile to expire")
	}
	if !almostEqual(projectile.Position.X, 0.5) || !almostEqual(projectile.Position.Y, 0) {
		t.Fatalf("expected projectile to stop at range boundary, got %+v", projectile.Position)
	}
}

func TestBirdFactoryReturnsStatsAndBehaviourForEachBirdType(t *testing.T) {
	cases := []struct {
		birdType string
		stats    BirdStats
		splash   bool
	}{
		{
			birdType: BirdTypeSparrow,
			stats:    BirdStats{Damage: 10, ProjectileSpeed: StandardProjectileSpeed, FireRate: 1.0, Range: 3.5, Cost: 50},
		},
		{
			birdType: BirdTypeWoodpecker,
			stats:    BirdStats{Damage: 6, ProjectileSpeed: StandardProjectileSpeed, FireRate: 2.0, Range: 3.5, Cost: 65},
		},
		{
			birdType: BirdTypeEagle,
			stats:    BirdStats{Damage: 30, ProjectileSpeed: StandardProjectileSpeed, FireRate: 0.4, Range: 6.0, Cost: 130},
		},
		{
			birdType: BirdTypePeacock,
			stats:    BirdStats{Damage: 7, ProjectileSpeed: StandardProjectileSpeed, FireRate: 1.0, Range: 3.5, Cost: 90},
			splash:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.birdType, func(t *testing.T) {
			bird, err := NewBird("bird-1", tc.birdType, Position{X: 1, Y: 2})
			if err != nil {
				t.Fatalf("NewBird failed: %v", err)
			}
			if bird.Stats != tc.stats {
				t.Fatalf("unexpected stats %+v", bird.Stats)
			}
			if bird.LastFiredAtTick != -1 {
				t.Fatalf("expected new bird to have never fired, got %d", bird.LastFiredAtTick)
			}
			_, isSplash := bird.AttackBehaviour.(SplashAttack)
			_, isSingle := bird.AttackBehaviour.(SingleAttack)
			if tc.splash && !isSplash {
				t.Fatalf("expected splash attack, got %T", bird.AttackBehaviour)
			}
			if !tc.splash && !isSingle {
				t.Fatalf("expected single attack, got %T", bird.AttackBehaviour)
			}
		})
	}
}

func TestEnemyStatsForTypeReturnsStatsForEachEnemyType(t *testing.T) {
	cases := []struct {
		enemyType string
		stats     EnemyStats
	}{
		{
			enemyType: EnemyTypeSmog,
			stats:     EnemyStats{Health: 40, Speed: 1},
		},
		{
			enemyType: EnemyTypeJunk,
			stats:     EnemyStats{Health: 200, Speed: 0.2},
		},
		{
			enemyType: EnemyTypeNoise,
			stats:     EnemyStats{Health: 15, Speed: 1.4},
		},
	}

	for _, tc := range cases {
		t.Run(tc.enemyType, func(t *testing.T) {
			stats, err := EnemyStatsForType(tc.enemyType)
			if err != nil {
				t.Fatalf("EnemyStatsForType failed: %v", err)
			}
			if stats != tc.stats {
				t.Fatalf("unexpected stats %+v", stats)
			}
		})
	}
}

func almostEqual(a float64, b float64) bool {
	return math.Abs(a-b) < 0.000001
}

func assertVector(t *testing.T, got Vector, want Vector) {
	t.Helper()
	if !almostEqual(got.X, want.X) || !almostEqual(got.Y, want.Y) {
		t.Fatalf("expected vector %+v, got %+v", want, got)
	}
}
