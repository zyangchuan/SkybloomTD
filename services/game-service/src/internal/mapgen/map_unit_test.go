package mapgen

import (
	"reflect"
	"testing"
)

func TestMapGenerateDeterministic(t *testing.T) {
	first, err := Generate(42, Version)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	second, err := Generate(42, Version)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("expected same seed to generate same map")
	}
}

func TestGeneratedPathRules(t *testing.T) {
	for seed := int64(0); seed < 100; seed++ {
		levelMap, err := Generate(seed, Version)
		if err != nil {
			t.Fatalf("Generate(%d) failed: %v", seed, err)
		}
		assertGeneratedPathRules(t, seed, levelMap.EnemyPath)
	}
}

func assertGeneratedPathRules(t *testing.T, seed int64, path []PathTile) {
	t.Helper()

	points := make([]point, 0, len(path))
	seen := map[point]int{}
	for index, tile := range path {
		p := point{x: tile.X, y: tile.Y}
		if p.x < 0 || p.x >= Width || p.y < 0 || p.y >= Height {
			t.Fatalf("seed %d path tile %d is out of bounds: %#v", seed, index, tile)
		}
		if p.y < pathMinY || p.y > pathMaxY {
			t.Fatalf("seed %d path tile %d uses UI row: %#v", seed, index, tile)
		}
		if previous, ok := seen[p]; ok {
			t.Fatalf("seed %d path revisits tile %v at indexes %d and %d", seed, p, previous, index)
		}
		seen[p] = index
		points = append(points, p)
	}

	for i := 0; i < len(points); i++ {
		for j := i + 2; j < len(points); j++ {
			if manhattan(points[i], points[j]) <= 1 {
				t.Fatalf("seed %d path tiles %v and %v at indexes %d and %d are too close", seed, points[i], points[j], i, j)
			}
		}
	}
}
