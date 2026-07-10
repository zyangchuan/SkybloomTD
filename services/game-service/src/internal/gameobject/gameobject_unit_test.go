package gameobject

import (
	"math"
	"testing"
)

func TestEnemyStatsForType(t *testing.T) {
	tests := []struct {
		enemyType string
		want      EnemyStats
	}{
		{enemyType: EnemyTypeSmog, want: EnemyStats{Health: 20, Speed: 0.8}},
		{enemyType: EnemyTypeJunk, want: EnemyStats{Health: 500, Speed: 0.3}},
		{enemyType: EnemyTypeNoise, want: EnemyStats{Health: 10, Speed: 1.4}},
	}

	for _, tt := range tests {
		t.Run(tt.enemyType, func(t *testing.T) {
			got, err := EnemyStatsForType(tt.enemyType)
			if err != nil {
				t.Fatalf("EnemyStatsForType failed: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %#v, got %#v", tt.want, got)
			}
		})
	}
}

func TestBirdStatsForType(t *testing.T) {
	tests := []struct {
		birdType string
		want     BirdStats
	}{
		{birdType: BirdTypeSparrow, want: BirdStats{Damage: 20, ProjectileSpeed: StandardProjectileSpeed, FireRate: 1.0, Range: 2.1, Cost: 50}},
		{birdType: BirdTypeWoodpecker, want: BirdStats{Damage: 10, ProjectileSpeed: StandardProjectileSpeed, FireRate: 2.0, Range: 2.1, Cost: 65}},
		{birdType: BirdTypeEagle, want: BirdStats{Damage: 50, ProjectileSpeed: StandardProjectileSpeed, FireRate: 0.5, Range: 3.2, Cost: 130}},
	}

	for _, tt := range tests {
		t.Run(tt.birdType, func(t *testing.T) {
			got, err := BirdStatsForType(tt.birdType)
			if err != nil {
				t.Fatalf("BirdStatsForType failed: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %#v, got %#v", tt.want, got)
			}
		})
	}
}

func TestEnemyMovementAlongPath(t *testing.T) {
	enemy := Enemy{
		Position: Position{X: 0, Y: 0},
		Speed:    2,
	}
	path := []Position{
		{X: 0, Y: 0},
		{X: 2, Y: 0},
		{X: 2, Y: 2},
	}

	enemy.Move(1.5, path)

	if enemy.PathIndex != 1 {
		t.Fatalf("expected path index 1, got %d", enemy.PathIndex)
	}
	if !almostEqual(enemy.Position.X, 2) || !almostEqual(enemy.Position.Y, 1) {
		t.Fatalf("expected position (2, 1), got (%f, %f)", enemy.Position.X, enemy.Position.Y)
	}
}

func TestProjectileDamageApplication(t *testing.T) {
	bird := Bird{
		Position: Position{X: 0, Y: 0},
		Stats:    BirdStats{Damage: 20},
	}
	enemy := Enemy{
		ID:       "enemy-1",
		Health:   30,
		Position: Position{X: 1, Y: 0},
	}
	projectile := NewLockedProjectile(bird, enemy, 1)
	projectile.RemainingRange = 0

	if !projectile.ApplyLockedDamage(&enemy) {
		t.Fatalf("expected projectile to apply damage")
	}
	if enemy.Health != 10 {
		t.Fatalf("expected enemy health 10, got %d", enemy.Health)
	}
}

func almostEqual(a float64, b float64) bool {
	return math.Abs(a-b) < 0.000001
}
