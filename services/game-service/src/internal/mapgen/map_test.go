package mapgen

import "testing"

func TestGenerateUsesCurrentGridSizeWithoutVersionBump(t *testing.T) {
	if Version != 1 {
		t.Fatalf("expected map algorithm version to stay 1, got %d", Version)
	}

	levelMap, err := Generate(42, Version)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if levelMap.Width != 18 || levelMap.Height != 12 {
		t.Fatalf("expected 18x12 map, got %dx%d", levelMap.Width, levelMap.Height)
	}
}

func TestGeneratedPathKeepsOneTileGap(t *testing.T) {
	for seed := int64(0); seed < 1000; seed++ {
		levelMap, err := Generate(seed, Version)
		if err != nil {
			t.Fatalf("Generate(%d) failed: %v", seed, err)
		}
		assertPathKeepsOneTileGap(t, seed, levelMap.EnemyPath)
	}
}

func TestGeneratedObjectsStayAwayFromPath(t *testing.T) {
	for seed := int64(0); seed < 1000; seed++ {
		levelMap, err := Generate(seed, Version)
		if err != nil {
			t.Fatalf("Generate(%d) failed: %v", seed, err)
		}
		assertObjectsStayAwayFromPath(t, seed, levelMap.EnemyPath, levelMap.Objects)
	}
}

func assertPathKeepsOneTileGap(t *testing.T, seed int64, path []PathTile) {
	t.Helper()

	points := make([]point, 0, len(path))
	seen := map[point]int{}
	for index, tile := range path {
		p := point{x: tile.X, y: tile.Y}
		if p.x < 0 || p.x >= Width || p.y < 0 || p.y >= Height {
			t.Fatalf("seed %d path tile %d is out of bounds: %#v", seed, index, tile)
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

func assertObjectsStayAwayFromPath(t *testing.T, seed int64, path []PathTile, objects []MapObject) {
	t.Helper()

	pathPoints := make([]point, 0, len(path))
	for _, tile := range path {
		pathPoints = append(pathPoints, point{x: tile.X, y: tile.Y})
	}

	seenObjects := map[point]string{}
	for _, object := range objects {
		objectPoint := point{x: object.X, y: object.Y}
		if objectPoint.x < 0 || objectPoint.x >= Width || objectPoint.y < 0 || objectPoint.y >= Height {
			t.Fatalf("seed %d object is out of bounds: %#v", seed, object)
		}
		if previous, ok := seenObjects[objectPoint]; ok {
			t.Fatalf("seed %d objects overlap at %v: %s and %s", seed, objectPoint, previous, object.Type)
		}
		seenObjects[objectPoint] = object.Type
		for _, pathPoint := range pathPoints {
			if chebyshev(objectPoint, pathPoint) <= 1 {
				t.Fatalf("seed %d object %#v is next to path tile %v", seed, object, pathPoint)
			}
		}
	}
}
