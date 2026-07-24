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
		{birdType: BirdTypePhoenix, want: BirdStats{Damage: 25, ProjectileSpeed: StandardProjectileSpeed, FireRate: 0.8, Range: 2.0, Cost: 50}},
		{birdType: BirdTypeSunGod, want: BirdStats{Damage: 18, ProjectileSpeed: StandardProjectileSpeed, FireRate: 3.2, Range: 4.5, Spread: math.Pi / 36, Pierce: 5, Cost: 150}},
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

func TestPhoenixUsesRingAttackType(t *testing.T) {
	got, err := AttackTypeForBirdType(BirdTypePhoenix)
	if err != nil {
		t.Fatalf("AttackTypeForBirdType failed: %v", err)
	}
	if got != AttackTypeRing {
		t.Fatalf("expected phoenix attack type %q, got %q", AttackTypeRing, got)
	}

	behaviour, err := AttackBehaviourForType(BirdTypePhoenix)
	if err != nil {
		t.Fatalf("AttackBehaviourForType failed: %v", err)
	}
	if behaviour.RequiresTarget() {
		t.Fatal("expected phoenix ring attack to not require a target")
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

func TestSingleAttackWithoutPierceOnlyHitsTarget(t *testing.T) {
	bird := Bird{
		Position:        Position{X: 0, Y: 0},
		Stats:           BirdStats{Damage: 20, Range: 3},
		AttackBehaviour: SingleAttack{},
	}
	target := Enemy{ID: "enemy-target", Health: 30, Position: Position{X: 2.5, Y: 0}, PathIndex: 2}
	closerEnemy := Enemy{ID: "enemy-closer", Health: 30, Position: Position{X: 1.5, Y: 0}, PathIndex: 1}

	hits := bird.Attack(target, []Enemy{target, closerEnemy}, 1)

	if len(hits) != 1 || hits[0].EnemyID != target.ID {
		t.Fatalf("expected single attack without pierce to stop at target, got %#v", hits)
	}
}

func TestSingleAttackWithPierceDamagesUpToPierceEnemiesOnAttackLine(t *testing.T) {
	bird := Bird{
		Position:        Position{X: 0, Y: 0},
		Stats:           BirdStats{Damage: 20, Range: 3, Pierce: 2},
		AttackBehaviour: SingleAttack{},
	}
	target := Enemy{ID: "enemy-target", Health: 30, Position: Position{X: 2.5, Y: 0}, PathIndex: 2}
	closerEnemy := Enemy{ID: "enemy-closer", Health: 30, Position: Position{X: 1.5, Y: 0}, PathIndex: 1}
	farEnemy := Enemy{ID: "enemy-far", Health: 30, Position: Position{X: 3.5, Y: 0}, PathIndex: 3}
	targetBeyondRange := Enemy{ID: "enemy-out-of-range", Health: 30, Position: Position{X: 3.5, Y: 0}}

	if hits := bird.Attack(targetBeyondRange, []Enemy{targetBeyondRange}, 1); len(hits) != 0 {
		t.Fatalf("expected target beyond range to avoid targeting, got %#v", hits)
	}

	hits := bird.Attack(target, []Enemy{target, closerEnemy, farEnemy}, 1)

	if len(hits) != 2 {
		t.Fatalf("expected pierce single attack to hit 2 enemies, got %#v", hits)
	}
	want := map[string]bool{
		target.ID:   true,
		farEnemy.ID: true,
	}
	for _, hit := range hits {
		if !want[hit.EnemyID] {
			t.Fatalf("unexpected pierce single hit %#v", hit)
		}
	}
}

func TestRingAttackHitsAllEnemiesInRangeWithoutTarget(t *testing.T) {
	bird := Bird{
		Position:        Position{X: 0, Y: 0},
		Stats:           BirdStats{Damage: 12, Range: 2},
		AttackBehaviour: RingAttack{},
	}
	enemies := []Enemy{
		{ID: "enemy-near", Health: 30, Position: Position{X: 1, Y: 0}},
		{ID: "enemy-edge", Health: 30, Position: Position{X: 0, Y: 2}},
		{ID: "enemy-far", Health: 30, Position: Position{X: 2.1, Y: 0}},
		{ID: "enemy-defeated", Health: 0, Position: Position{X: 1, Y: 1}},
	}

	hits := bird.Attack(Enemy{}, enemies, 1)

	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %#v", hits)
	}
	want := map[string]bool{"enemy-near": true, "enemy-edge": true}
	for _, hit := range hits {
		if !want[hit.EnemyID] {
			t.Fatalf("unexpected hit %#v", hit)
		}
		if hit.Damage != 12 {
			t.Fatalf("expected damage 12, got %f", hit.Damage)
		}
	}
	if bird.LastFiredAtTick != 1 {
		t.Fatalf("expected last fired tick to update, got %d", bird.LastFiredAtTick)
	}
}

func TestSplashAttackUsesConfiguredSpreadForThreeAttackLines(t *testing.T) {
	target := Enemy{ID: "enemy-center", Health: 30, Position: Position{X: 2, Y: 0}}
	wideLaneEnemy := Enemy{
		ID:       "enemy-wide-lane",
		Health:   30,
		Position: Position{X: 2 * math.Cos(math.Pi/6), Y: 2 * math.Sin(math.Pi/6)},
	}
	enemies := []Enemy{target, wideLaneEnemy}
	defaultBird := Bird{
		Position: Position{X: 0, Y: 0},
		Stats:    BirdStats{Damage: 10, Range: 3},
	}
	wideSpreadBird := defaultBird
	wideSpreadBird.Stats.Spread = math.Pi / 6

	defaultHits := SplashAttack{}.Attack(defaultBird, target, enemies)
	if len(defaultHits) != 1 || defaultHits[0].EnemyID != target.ID {
		t.Fatalf("expected default spread to only hit center enemy, got %#v", defaultHits)
	}

	wideHits := SplashAttack{}.Attack(wideSpreadBird, target, enemies)
	if len(wideHits) != 2 {
		t.Fatalf("expected configured wider spread to hit both enemies, got %#v", wideHits)
	}
}

func TestSplashAttackOnlyHitsOneEnemyPerAttackLine(t *testing.T) {
	target := Enemy{ID: "enemy-center", Health: 30, Position: Position{X: 2, Y: 0}}
	earlierEnemy := Enemy{ID: "enemy-earlier", Health: 30, Position: Position{X: 1, Y: 0}, PathIndex: 1}
	laterEnemy := Enemy{ID: "enemy-later", Health: 30, Position: Position{X: 2.5, Y: 0}, PathIndex: 2}
	bird := Bird{
		Position: Position{X: 0, Y: 0},
		Stats:    BirdStats{Damage: 10, Range: 3, Spread: math.Pi / 6},
	}

	hits := SplashAttack{}.Attack(bird, target, []Enemy{target, earlierEnemy, laterEnemy})

	if len(hits) != 1 {
		t.Fatalf("expected one hit on the center attack line, got %#v", hits)
	}
	if hits[0].EnemyID != laterEnemy.ID {
		t.Fatalf("expected attack line to use single-target selection, got %#v", hits)
	}
}

func TestSplashAttackWithPierceDamagesUpToPierceEnemiesOnAttackLines(t *testing.T) {
	target := Enemy{ID: "enemy-target", Health: 30, Position: Position{X: 2.5, Y: 0}, PathIndex: 2}
	closerEnemy := Enemy{ID: "enemy-closer", Health: 30, Position: Position{X: 1.5, Y: 0}, PathIndex: 1}
	farEnemy := Enemy{ID: "enemy-far", Health: 30, Position: Position{X: 3.5, Y: 0}, PathIndex: 3}
	bird := Bird{
		Position:        Position{X: 0, Y: 0},
		Stats:           BirdStats{Damage: 10, Range: 3, Pierce: 2},
		AttackBehaviour: SplashAttack{},
	}

	hits := bird.Attack(target, []Enemy{target, closerEnemy, farEnemy}, 1)

	if len(hits) != 2 {
		t.Fatalf("expected splash line with pierce to hit 2 enemies, got %#v", hits)
	}
	want := map[string]bool{
		target.ID:   true,
		farEnemy.ID: true,
	}
	for _, hit := range hits {
		if !want[hit.EnemyID] {
			t.Fatalf("unexpected pierce splash hit %#v", hit)
		}
	}
}

func almostEqual(a float64, b float64) bool {
	return math.Abs(a-b) < 0.000001
}
