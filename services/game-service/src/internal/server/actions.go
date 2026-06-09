package server

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/google/uuid"

	"skybloom/game-service/internal/gameobject"
	"skybloom/game-service/internal/gamesession"
)

func (s *Server) processClientAction(ctx context.Context, runtime *runtimeSession, action clientAction) error {
	switch action.Type {
	case placeTowerAction:
		return s.placeTower(ctx, runtime, action.PlaceTower)
	case evolveTowerAction:
		return s.evolveTower(ctx, runtime, action.EvolveTower)
	default:
		return errors.New("unsupported action")
	}
}

func (s *Server) placeTower(ctx context.Context, runtime *runtimeSession, request placeTowerRequest) error {
	if runtime == nil {
		return errors.New("game session is not running")
	}
	birdType := strings.TrimSpace(request.BirdType)
	stats, err := gameobject.BirdStatsForType(birdType)
	if err != nil {
		return errors.New("unknown bird type")
	}
	if !isInsideMap(runtime.levelMap, request.X, request.Y) {
		return errors.New("tower position is outside the map")
	}
	if isEnemyPath(runtime.levelMap, request.X, request.Y) {
		return errors.New("tower cannot be placed on the enemy path")
	}
	if isOccupied(runtime.birds, request.X, request.Y) {
		return errors.New("tower position is occupied")
	}

	nextEconomy := runtime.economy
	if !nextEconomy.Consume(stats.Cost) {
		return errors.New("insufficient essence")
	}

	bird, err := gameobject.NewBird(uuid.NewString(), birdType, gameobject.Position{X: float64(request.X), Y: float64(request.Y)})
	if err != nil {
		return errors.New("failed to create bird")
	}
	previousEconomy := runtime.economy
	previousBirds := runtime.birds
	runtime.economy = nextEconomy
	runtime.session.Essence = nextEconomy.Essence
	runtime.birds = append(append([]placedBird{}, runtime.birds...), placedBird{birdType: birdType, bird: bird})
	if err := s.saveRuntimeState(ctx, *runtime); err != nil {
		runtime.economy = previousEconomy
		runtime.session.Essence = previousEconomy.Essence
		runtime.birds = previousBirds
		log.Printf("game session placement save failed session_id=%s: %v", runtime.session.SessionID, err)
		return errors.New("failed to save tower placement")
	}

	return nil
}

func (s *Server) evolveTower(ctx context.Context, runtime *runtimeSession, request evolveTowerRequest) error {
	if runtime == nil {
		return errors.New("game session is not running")
	}
	towerID := strings.TrimSpace(request.TowerID)
	baseType := strings.TrimSpace(request.BirdType)

	idx := -1
	for i, placed := range runtime.birds {
		if placed.bird.ID == towerID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return errors.New("tower not found")
	}
	placed := runtime.birds[idx]
	if placed.birdType != baseType {
		return errors.New("bird_type does not match the tower's current type")
	}
	if strings.HasPrefix(placed.birdType, "evolve_") {
		return errors.New("tower is already at max level")
	}

	evolvedType := "evolve_" + baseType
	evolvedStats, err := gameobject.BirdStatsForType(evolvedType)
	if err != nil {
		return errors.New("unknown evolved bird type")
	}
	behaviour, err := gameobject.AttackBehaviourForType(evolvedType)
	if err != nil {
		return errors.New("failed to get evolved attack behaviour")
	}

	baseStats, err := gameobject.BirdStatsForType(baseType)
	if err != nil {
		return errors.New("unknown base bird type")
	}
	nextEconomy := runtime.economy
	if !nextEconomy.Consume(baseStats.Cost) {
		return errors.New("insufficient essence")
	}

	updatedBirds := append([]placedBird{}, runtime.birds...)
	updatedBirds[idx] = placedBird{
		birdType: evolvedType,
		bird: gameobject.Bird{
			ID:              placed.bird.ID,
			Position:        placed.bird.Position,
			Stats:           evolvedStats,
			AttackBehaviour: behaviour,
			LastFiredAtTick: placed.bird.LastFiredAtTick,
		},
	}

	previousEconomy := runtime.economy
	previousBirds := runtime.birds
	runtime.economy = nextEconomy
	runtime.session.Essence = nextEconomy.Essence
	runtime.birds = updatedBirds
	if err := s.saveRuntimeState(ctx, *runtime); err != nil {
		runtime.economy = previousEconomy
		runtime.session.Essence = previousEconomy.Essence
		runtime.birds = previousBirds
		log.Printf("game session evolve save failed session_id=%s: %v", runtime.session.SessionID, err)
		return errors.New("failed to save tower evolution")
	}
	return nil
}

func (s *Server) awardEssence(ctx context.Context, runtime *runtimeSession, amount int) (int, error) {
	if runtime == nil {
		return 0, errors.New("game session is not running")
	}
	if amount <= 0 {
		return runtime.economy.Essence, nil
	}
	previousEconomy := runtime.economy
	runtime.economy.Add(amount)
	runtime.session.Essence = runtime.economy.Essence
	if err := s.saveRuntimeState(ctx, *runtime); err != nil {
		runtime.economy = previousEconomy
		runtime.session.Essence = previousEconomy.Essence
		return previousEconomy.Essence, err
	}
	return runtime.economy.Essence, nil
}

func (s *Server) saveRuntimeState(ctx context.Context, runtime runtimeSession) error {
	return s.sessions.SaveRuntimeState(ctx, runtime.session.SessionID, gamesession.RuntimeState{
		Health:            runtime.session.Health,
		Essence:           runtime.economy.Essence,
		Wave:              runtime.session.Wave,
		Tick:              runtime.session.Tick,
		LoopStarted:       runtime.loopStarted,
		LoopPaused:        false,
		WaveStartedAtTick: runtime.waveStartedAtTick,
		WaveSpawned:       runtime.waveSpawned,
		NextWaveTick:      runtime.nextWaveTick,
		Birds:             storedBirds(runtime.birds),
		Smogs:             storedSmogs(runtime.smogs),
		Projectiles:       storedProjectiles(runtime.projectiles),
	})
}
