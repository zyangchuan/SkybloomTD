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

func TestSingleAttackCreatesTargetHit(t *testing.T) {
	bird := Bird{
		ID:       "bird-1",
		Position: Position{X: 0, Y: 0},
		Stats: BirdStats{
			Damage: 10,
			Range:  5,
		},
		AttackBehaviour: SingleAttack{},
	}
	target := Enemy{ID: "enemy-1", Health: 20, Position: Position{X: 3, Y: 0}}

	hits := bird.AttackBehaviour.Attack(bird, target, nil)
	if len(hits) != 1 {
		t.Fatalf("expected one hit, got %d", len(hits))
	}
	if hits[0].EnemyID != target.ID {
		t.Fatalf("expected target id %q, got %q", target.ID, hits[0].EnemyID)
	}
	if !almostEqual(hits[0].Damage, 10) {
		t.Fatalf("expected hit damage 10, got %f", hits[0].Damage)
	}
}

func TestSingleAttackSkipsDeadTarget(t *testing.T) {
	bird := Bird{Stats: BirdStats{Damage: 10}, AttackBehaviour: SingleAttack{}}
	target := Enemy{ID: "enemy-1", Health: 0, Position: Position{X: 1, Y: 0}}

	hits := bird.AttackBehaviour.Attack(bird, target, nil)
	if len(hits) != 0 {
		t.Fatalf("expected dead target to receive no hits, got %+v", hits)
	}
}

func TestSplashAttackHitsEnemiesWithinThreeFeatherFan(t *testing.T) {
	bird := Bird{
		ID:       "bird-1",
		Position: Position{X: 0, Y: 0},
		Stats: BirdStats{
			Damage: 7,
			Range:  2.1,
		},
		AttackBehaviour: SplashAttack{},
	}
	target := Enemy{ID: "enemy-1", Health: 20, Position: Position{X: 1, Y: 0}}
	enemies := []Enemy{
		target,
		{ID: "center-lane", Health: 20, Position: Position{X: 2.0, Y: 0.1}},
		{ID: "upper-feather", Health: 20, Position: Position{X: math.Cos(FeatherSpreadRadians) * 2, Y: math.Sin(FeatherSpreadRadians) * 2}},
		{ID: "between-feathers", Health: 20, Position: Position{X: math.Cos(FeatherSpreadRadians/2) * 2, Y: math.Sin(FeatherSpreadRadians/2) * 2}},
		{ID: "out-of-range", Health: 20, Position: Position{X: 4.0, Y: 0}},
		{ID: "dead", Health: 0, Position: Position{X: 1.1, Y: 0}},
	}

	hits := bird.AttackBehaviour.Attack(bird, target, enemies)
	if len(hits) != 3 {
		t.Fatalf("expected three feather fan hits, got %+v", hits)
	}
	wantIDs := map[string]bool{
		"enemy-1":          true,
		"center-lane":      true,
		"upper-feather":    true,
		"between-feathers": false,
		"out-of-range":     false,
		"dead":             false,
	}
	for _, hit := range hits {
		if !wantIDs[hit.EnemyID] {
			t.Fatalf("unexpected feather fan hit %+v in %+v", hit, hits)
		}
		delete(wantIDs, hit.EnemyID)
	}
	for _, hit := range hits {
		if !almostEqual(hit.Damage, 7) {
			t.Fatalf("expected splash damage 7, got %+v", hits)
		}
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
			stats:    BirdStats{Damage: 10, ProjectileSpeed: StandardProjectileSpeed, FireRate: 1.0, Range: 2.1, Cost: 50},
		},
		{
			birdType: BirdTypeWoodpecker,
			stats:    BirdStats{Damage: 6, ProjectileSpeed: StandardProjectileSpeed, FireRate: 2.0, Range: 2.1, Cost: 65},
		},
		{
			birdType: BirdTypeEagle,
			stats:    BirdStats{Damage: 30, ProjectileSpeed: StandardProjectileSpeed, FireRate: 0.4, Range: 3.2, Cost: 130},
		},
		{
			birdType: BirdTypePeacock,
			stats:    BirdStats{Damage: 7, ProjectileSpeed: StandardProjectileSpeed, FireRate: 1.0, Range: 2.1, Cost: 90},
			splash:   true,
		},
		{
			birdType: BirdTypeFalcon,
			stats:    BirdStats{Damage: 24, ProjectileSpeed: StandardProjectileSpeed, FireRate: 0.9, Range: 3.6, Cost: 0},
		},
		{
			birdType: BirdTypeKingfisher,
			stats:    BirdStats{Damage: 9, ProjectileSpeed: StandardProjectileSpeed, FireRate: 3.0, Range: 2.4, Cost: 0},
			splash:   true,
		},
		{
			birdType: BirdTypePhoenix,
			stats:    BirdStats{Damage: 28, ProjectileSpeed: StandardProjectileSpeed, FireRate: 0.8, Range: 3.0, Cost: 0},
			splash:   true,
		},
		{
			birdType: BirdTypeSunGod,
			stats:    BirdStats{Damage: 32, ProjectileSpeed: StandardProjectileSpeed, FireRate: 1.6, Range: 3.6, Cost: 0},
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

func TestBirdDefinitionsBackBirdTypeCatalog(t *testing.T) {
	types := BirdTypes()
	if len(types) != len(birdDefinitions) {
		t.Fatalf("expected catalog length %d, got %d", len(birdDefinitions), len(types))
	}
	for _, birdType := range types {
		definition, err := BirdDefinitionForType(birdType)
		if err != nil {
			t.Fatalf("BirdDefinitionForType(%q) failed: %v", birdType, err)
		}
		if definition.Type != birdType {
			t.Fatalf("expected definition type %q, got %q", birdType, definition.Type)
		}
		if definition.AttackType != AttackTypeSingle && definition.AttackType != AttackTypeSplash {
			t.Fatalf("unexpected attack type %q for %q", definition.AttackType, birdType)
		}
	}
}

func TestEnemyStatsForTypeReturnsStatsForEachEnemyType(t *testing.T) {
	cases := []struct {
		enemyType string
		stats     EnemyStats
	}{
		{
			enemyType: EnemyTypeSmog,
			stats:     EnemyStats{Health: 40, Speed: 0.8},
		},
		{
			enemyType: EnemyTypeJunk,
			stats:     EnemyStats{Health: 300, Speed: 0.3},
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

func TestEnemyDefinitionsBackEnemyTypeCatalog(t *testing.T) {
	types := EnemyTypes()
	if len(types) != len(enemyDefinitions) {
		t.Fatalf("expected catalog length %d, got %d", len(enemyDefinitions), len(types))
	}
	for _, enemyType := range types {
		definition, err := EnemyDefinitionForType(enemyType)
		if err != nil {
			t.Fatalf("EnemyDefinitionForType(%q) failed: %v", enemyType, err)
		}
		if definition.Type != enemyType {
			t.Fatalf("expected definition type %q, got %q", enemyType, definition.Type)
		}
		if definition.Stats.Health <= 0 {
			t.Fatalf("expected positive health for %q, got %d", enemyType, definition.Stats.Health)
		}
		if definition.Stats.Speed <= 0 {
			t.Fatalf("expected positive speed for %q, got %f", enemyType, definition.Stats.Speed)
		}
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
