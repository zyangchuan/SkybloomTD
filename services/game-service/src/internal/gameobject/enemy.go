package gameobject

import "errors"

const (
	EnemyTypeSmog  = "smog"
	EnemyTypeJunk  = "junk"
	EnemyTypeNoise = "noise"
)

type Enemy struct {
	ID        string
	Type      string
	Health    int
	Position  Position
	Speed     float64
	PathIndex int
}

type EnemyStats struct {
	Health int     `json:"health"`
	Speed  float64 `json:"speed"`
}

func EnemyStatsForType(enemyType string) (EnemyStats, error) {
	switch enemyType {
	case EnemyTypeSmog:
		return EnemyStats{
			Health: 40,
			Speed:  0.8,
		}, nil
	case EnemyTypeJunk:
		return EnemyStats{
			Health: 300,
			Speed:  0.3,
		}, nil
	case EnemyTypeNoise:
		return EnemyStats{
			Health: 15,
			Speed:  1.4,
		}, nil
	default:
		return EnemyStats{}, errors.New("unknown enemy type")
	}
}

func EnemyTypes() []string {
	return []string{
		EnemyTypeSmog,
		EnemyTypeJunk,
		EnemyTypeNoise,
	}
}

func (s *Enemy) Move(deltaSeconds float64, path []Position) {
	if s == nil || deltaSeconds <= 0 || s.Speed <= 0 || len(path) == 0 {
		return
	}
	s.PathIndex = max(0, s.PathIndex)
	if s.PathIndex >= len(path)-1 {
		return
	}

	remainingDistance := s.Speed * deltaSeconds
	for remainingDistance > 0 && s.PathIndex < len(path)-1 {
		target := path[s.PathIndex+1]
		distanceToTarget := s.Position.DistanceTo(target)
		if distanceToTarget == 0 {
			s.PathIndex++
			continue
		}
		if remainingDistance >= distanceToTarget {
			s.Position = target
			s.PathIndex++
			remainingDistance -= distanceToTarget
			continue
		}

		direction := s.Position.DirectionTo(target)
		s.Position.X += direction.X * remainingDistance
		s.Position.Y += direction.Y * remainingDistance
		return
	}
}

func (s *Enemy) TakeDamage(damage float64) {
	if s == nil || damage <= 0 || s.Health <= 0 {
		return
	}
	s.Health -= int(damage)
	if s.Health < 0 {
		s.Health = 0
	}
}

func (s Enemy) IsAlive() bool {
	return s.Health > 0
}
