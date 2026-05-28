package gameobject

type Smog struct {
	ID        string
	Health    int
	Position  Position
	Speed     float64
	PathIndex int
}

func (s *Smog) Move(deltaSeconds float64, path []Position) {
	if s == nil || deltaSeconds <= 0 || s.Speed <= 0 || len(path) == 0 {
		return
	}
	if s.PathIndex < 0 {
		s.PathIndex = 0
	}
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

func (s *Smog) TakeDamage(damage float64) {
	if s == nil || damage <= 0 || s.Health <= 0 {
		return
	}
	s.Health -= int(damage)
	if s.Health < 0 {
		s.Health = 0
	}
}

func (s Smog) IsAlive() bool {
	return s.Health > 0
}
