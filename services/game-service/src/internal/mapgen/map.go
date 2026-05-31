package mapgen

import (
	"fmt"
	"sort"
)

const (
	Width   = 18
	Height  = 12
	Version = 1
)

const (
	pathGenerationAttempts = 256
	minTurnColumnGap       = 2
	pathMinY               = 2
	pathMaxY               = Height - 3
)

type GeneratedMap struct {
	Version   int         `json:"version"`
	Seed      int64       `json:"seed"`
	Width     int         `json:"width"`
	Height    int         `json:"height"`
	EnemyPath []PathTile  `json:"enemy_path"`
	Objects   []MapObject `json:"objects"`
}

type PathTile struct {
	X    int    `json:"x"`
	Y    int    `json:"y"`
	Kind string `json:"kind"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	Axis string `json:"axis,omitempty"`
	Turn string `json:"turn,omitempty"`
}

type MapObject struct {
	Type string `json:"type"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
}

type point struct {
	x int
	y int
}

func Generate(seed int64, version int) (GeneratedMap, error) {
	if version != Version {
		return GeneratedMap{}, fmt.Errorf("unsupported map algorithm version %d", version)
	}

	rng := newRNG(seed)
	points := generatePath(rng)
	return GeneratedMap{
		Version:   version,
		Seed:      seed,
		Width:     Width,
		Height:    Height,
		EnemyPath: annotatePath(points),
		Objects:   generateObjects(rng, points),
	}, nil
}

func generatePath(rng *rng) []point {
	for attempt := 0; attempt < pathGenerationAttempts; attempt++ {
		points := generatePathCandidate(rng)
		if hasPathGap(points) {
			return points
		}
	}
	return fallbackPath()
}

func generatePathCandidate(rng *rng) []point {
	y := randomPathY(rng)
	current := point{x: 0, y: y}
	points := []point{current}

	turnCount := 4 + rng.Intn(2)
	for _, column := range chooseTurnColumns(rng, turnCount) {
		appendHorizontal(&points, &current, column)
		appendVertical(&points, &current, nextPathY(rng, current.y))
	}
	appendHorizontal(&points, &current, Width-1)
	return points
}

func chooseTurnColumns(rng *rng, count int) []int {
	candidates := make([]int, 0, Width-6)
	for x := 3; x <= Width-4; x++ {
		candidates = append(candidates, x)
	}
	shuffle(rng, candidates)

	columns := make([]int, 0, count)
	for _, candidate := range candidates {
		if len(columns) == count {
			break
		}
		if isFarFromColumns(candidate, columns, minTurnColumnGap) {
			columns = append(columns, candidate)
		}
	}
	sort.Ints(columns)
	return columns
}

func isFarFromColumns(candidate int, columns []int, minGap int) bool {
	for _, column := range columns {
		if abs(candidate-column) < minGap {
			return false
		}
	}
	return true
}

func appendHorizontal(points *[]point, current *point, targetX int) {
	step := 1
	if targetX < current.x {
		step = -1
	}
	for current.x != targetX {
		current.x += step
		*points = append(*points, *current)
	}
}

func appendVertical(points *[]point, current *point, targetY int) {
	step := 1
	if targetY < current.y {
		step = -1
	}
	for current.y != targetY {
		current.y += step
		*points = append(*points, *current)
	}
}

func nextPathY(rng *rng, currentY int) int {
	for attempt := 0; attempt < 24; attempt++ {
		y := randomPathY(rng)
		if abs(y-currentY) >= 2 {
			return y
		}
	}
	if currentY <= (pathMinY+pathMaxY)/2 {
		return min(pathMaxY, currentY+3)
	}
	return max(pathMinY, currentY-3)
}

func randomPathY(rng *rng) int {
	return pathMinY + rng.Intn(pathMaxY-pathMinY+1)
}

func hasPathGap(points []point) bool {
	seen := map[point]int{}
	for i, p := range points {
		if _, ok := seen[p]; ok {
			return false
		}
		seen[p] = i
	}

	for i := 0; i < len(points); i++ {
		for j := i + 2; j < len(points); j++ {
			if manhattan(points[i], points[j]) <= 1 {
				return false
			}
		}
	}
	return true
}

func fallbackPath() []point {
	points := []point{{x: 0, y: pathMinY}}
	current := points[0]
	for _, turn := range []point{
		{x: 3, y: pathMaxY},
		{x: 7, y: pathMinY},
		{x: 11, y: pathMaxY},
		{x: 14, y: pathMinY},
	} {
		appendHorizontal(&points, &current, turn.x)
		appendVertical(&points, &current, turn.y)
	}
	appendHorizontal(&points, &current, Width-1)
	return points
}

func annotatePath(points []point) []PathTile {
	tiles := make([]PathTile, 0, len(points))
	for i, p := range points {
		tile := PathTile{X: p.x, Y: p.y}
		switch {
		case i == 0:
			tile.Kind = "start"
			tile.To = side(points[i], points[i+1])
		case i == len(points)-1:
			tile.Kind = "end"
			tile.From = side(points[i], points[i-1])
		default:
			from := side(points[i], points[i-1])
			to := side(points[i], points[i+1])
			tile.From = from
			tile.To = to
			if isStraight(from, to) {
				tile.Kind = "straight"
				tile.Axis = axis(from, to)
			} else {
				tile.Kind = "turn"
				tile.Turn = turnDirection(movement(points[i-1], points[i]), movement(points[i], points[i+1]))
			}
		}
		tiles = append(tiles, tile)
	}
	return tiles
}

func side(origin point, neighbor point) string {
	switch {
	case neighbor.x < origin.x:
		return "west"
	case neighbor.x > origin.x:
		return "east"
	case neighbor.y < origin.y:
		return "north"
	default:
		return "south"
	}
}

func movement(from point, to point) string {
	switch {
	case to.x > from.x:
		return "east"
	case to.x < from.x:
		return "west"
	case to.y > from.y:
		return "south"
	default:
		return "north"
	}
}

func isStraight(from string, to string) bool {
	return (from == "west" && to == "east") ||
		(from == "east" && to == "west") ||
		(from == "north" && to == "south") ||
		(from == "south" && to == "north")
}

func axis(from string, to string) string {
	if (from == "west" || from == "east") && (to == "west" || to == "east") {
		return "horizontal"
	}
	return "vertical"
}

func turnDirection(in string, out string) string {
	diff := (directionIndex(out) - directionIndex(in) + 4) % 4
	if diff == 1 {
		return "right"
	}
	return "left"
}

func directionIndex(direction string) int {
	switch direction {
	case "north":
		return 0
	case "east":
		return 1
	case "south":
		return 2
	default:
		return 3
	}
}

func generateObjects(rng *rng, points []point) []MapObject {
	blockedCells := map[point]bool{}
	for _, p := range points {
		for y := max(0, p.y-1); y <= min(Height-1, p.y+1); y++ {
			for x := max(0, p.x-1); x <= min(Width-1, p.x+1); x++ {
				blockedCells[point{x: x, y: y}] = true
			}
		}
	}

	candidates := make([]point, 0, Width*Height-len(blockedCells))
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			p := point{x: x, y: y}
			if !blockedCells[p] {
				candidates = append(candidates, p)
			}
		}
	}
	shuffle(rng, candidates)

	specs := []struct {
		objectType string
		minCount   int
		variance   int
	}{
		{objectType: "tree", minCount: 22, variance: 8},
		{objectType: "tree_stump", minCount: 5, variance: 4},
		{objectType: "bush", minCount: 18, variance: 10},
		{objectType: "rock", minCount: 10, variance: 6},
	}

	objects := []MapObject{}
	next := 0
	for _, spec := range specs {
		count := spec.minCount + rng.Intn(spec.variance+1)
		for i := 0; i < count && next < len(candidates); i++ {
			p := candidates[next]
			next++
			objects = append(objects, MapObject{Type: spec.objectType, X: p.x, Y: p.y})
		}
	}
	return objects
}

type rng struct {
	state uint64
}

func newRNG(seed int64) *rng {
	state := uint64(seed)
	if state == 0 {
		state = 0x9e3779b97f4a7c15
	}
	return &rng{state: state}
}

func (r *rng) Intn(n int) int {
	if n <= 0 {
		return 0
	}
	return int(r.next() % uint64(n))
}

func (r *rng) next() uint64 {
	x := r.state
	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	r.state = x
	return x * 2685821657736338717
}

func shuffle[T any](rng *rng, values []T) {
	for i := len(values) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		values[i], values[j] = values[j], values[i]
	}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func manhattan(a point, b point) int {
	return abs(a.x-b.x) + abs(a.y-b.y)
}

func chebyshev(a point, b point) int {
	return max(abs(a.x-b.x), abs(a.y-b.y))
}
