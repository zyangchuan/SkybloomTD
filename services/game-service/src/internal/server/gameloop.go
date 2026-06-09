package server

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"skybloom/game-service/internal/gameobject"
)

func (s *Server) runGameLoop(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, runtime runtimeSession, actions <-chan clientAction, done chan<- struct{}) {
	ticker := time.NewTicker(gameTickInterval)
	defer ticker.Stop()
	defer close(done)

	for {
		select {
		case <-ctx.Done():
			return
		case action := <-actions:
			switch action.Type {
			case placeTowerAction, evolveTowerAction:
				if err := s.processClientAction(ctx, &runtime, action); err != nil {
					if writeErr := writeActionRejected(conn, writeMu, action.Type, err.Error()); writeErr != nil {
						log.Printf("websocket action rejection write failed: %v", writeErr)
						return
					}
					continue
				}
				accepted := runtime.birds[len(runtime.birds)-1]
				if action.Type == evolveTowerAction {
					for _, b := range runtime.birds {
						if b.bird.ID == action.EvolveTower.TowerID {
							accepted = b
							break
						}
					}
				}
				if err := writeActionAccepted(conn, writeMu, action.Type, accepted); err != nil {
					log.Printf("websocket action accepted write failed: %v", err)
					return
				}
			case awardQuizEssenceAction:
				runtime.loopStarted = true
				essence, err := s.awardEssence(ctx, &runtime, action.EssenceReward)
				if action.Result != nil {
					action.Result <- actionResult{Essence: essence, Err: err}
				}
			case pauseGameAction:
				runtime.loopPaused = true
			case resumeGameAction:
				runtime.loopPaused = false
			default:
				if action.Result != nil {
					action.Result <- actionResult{Err: errors.New("unsupported action")}
				}
			}
		case now := <-ticker.C:
			if runtime.loopPaused {
				continue
			}
			events := advanceRuntimeTick(&runtime, now)
			if err := s.saveRuntimeState(ctx, runtime); err != nil {
				log.Printf("game session runtime save failed session_id=%s: %v", runtime.session.SessionID, err)
			}
			if err := writeWebsocketJSON(conn, writeMu, Message{Type: "game.state", Data: gameStateFromRuntime(runtime, runtime.session.UpdatedAt, nil, events)}); err != nil {
				log.Printf("game state write failed session_id=%s: %v", runtime.session.SessionID, err)
				return
			}
			if runtime.session.Health <= 0 {
				if err := writeGameOver(conn, writeMu, runtime, "health_depleted"); err != nil {
					log.Printf("game over write failed session_id=%s: %v", runtime.session.SessionID, err)
				}
				return
			}
			if gameWon(runtime) {
				if err := writeGameVictory(conn, writeMu, runtime, "all_waves_cleared"); err != nil {
					log.Printf("game victory write failed session_id=%s: %v", runtime.session.SessionID, err)
				}
				return
			}
		}
	}
}

func advanceRuntimeTick(runtime *runtimeSession, now time.Time) []GameEvent {
	if runtime == nil {
		return nil
	}
	if !runtime.loopStarted {
		return nil
	}
	runtime.session.Tick++
	runtime.session.Essence = runtime.economy.Essence
	runtime.session.UpdatedAt = now.UTC()

	events := make([]GameEvent, 0)
	events = append(events, moveSmogs(runtime, gameTickInterval.Seconds())...)
	if runtime.session.Health <= 0 {
		return events
	}
	events = append(events, spawnSmogs(runtime)...)
	events = append(events, fireBirds(runtime)...)
	events = append(events, resolveProjectiles(runtime, gameTickInterval.Seconds())...)
	runtime.smogs = aliveSmogs(runtime.smogs)
	events = append(events, scheduleNextWaveIfCleared(runtime)...)
	return events
}

func spawnSmogs(runtime *runtimeSession) []GameEvent {
	if len(runtime.path) == 0 || runtime.nextWaveTick <= 0 || runtime.session.Tick < runtime.nextWaveTick {
		return nil
	}
	wave, ok := activeWaveDefinition(runtime)
	if !ok || runtime.waveSpawned >= wave.Count {
		return nil
	}

	events := make([]GameEvent, 0)
	if runtime.waveSpawned == 0 {
		runtime.session.Wave = wave.Wave
		runtime.waveStartedAtTick = runtime.session.Tick
		events = append(events, GameEvent{Type: "wave.started", Wave: wave.Wave})
	}

	// Calculate subwave/group spawn ticks dynamically
	groupSize := wave.Count / 3
	if groupSize < 1 {
		groupSize = 1
	}
	groupIndex := int64(runtime.waveSpawned / groupSize)
	indexInGroup := int64(runtime.waveSpawned % groupSize)

	groupDurationTicks := int64(groupSize) * smogSpawnIntervalTicks
	groupStartIntervalTicks := groupDurationTicks + groupGapTicks

	nextSpawnTick := runtime.waveStartedAtTick + (groupIndex * groupStartIntervalTicks) + (indexInGroup * smogSpawnIntervalTicks)

	if runtime.session.Tick < nextSpawnTick {
		return events
	}

	smog := gameobject.Smog{
		ID:        uuid.NewString(),
		Health:    wave.Health,
		Position:  runtime.path[0],
		Speed:     wave.Speed,
		PathIndex: 0,
	}
	runtime.waveSpawned++
	runtime.smogs = append(runtime.smogs, smog)
	events = append(events, GameEvent{Type: "smog.spawned", SmogID: smog.ID, Wave: wave.Wave, Health: smog.Health})
	return events
}

func activeWaveDefinition(runtime *runtimeSession) (waveDefinition, bool) {
	waves := waveDefinitions()
	if runtime.session.Wave <= 0 || runtime.waveSpawned == 0 {
		nextIndex := runtime.session.Wave
		if nextIndex < 0 || nextIndex >= len(waves) {
			return waveDefinition{}, false
		}
		return waves[nextIndex], true
	}
	for _, wave := range waves {
		if wave.Wave == runtime.session.Wave {
			return wave, true
		}
	}
	return waveDefinition{}, false
}

func currentWaveDefinition(waveNumber int) (waveDefinition, bool) {
	for _, wave := range waveDefinitions() {
		if wave.Wave == waveNumber {
			return wave, true
		}
	}
	return waveDefinition{}, false
}

func scheduleNextWaveIfCleared(runtime *runtimeSession) []GameEvent {
	if runtime.session.Wave <= 0 || len(runtime.smogs) > 0 {
		return nil
	}
	currentWave, ok := currentWaveDefinition(runtime.session.Wave)
	if !ok || runtime.waveSpawned < currentWave.Count {
		return nil
	}
	if runtime.session.Wave >= len(waveDefinitions()) && runtime.nextWaveTick == 0 {
		return nil
	}

	events := []GameEvent{{Type: "wave.cleared", Wave: runtime.session.Wave}}
	if runtime.session.Wave >= len(waveDefinitions()) {
		runtime.nextWaveTick = 0
		return events
	}

	runtime.waveStartedAtTick = 0
	runtime.waveSpawned = 0
	runtime.nextWaveTick = runtime.session.Tick + waveClearDelayTicks
	return events
}

func gameWon(runtime runtimeSession) bool {
	if runtime.session.Health <= 0 || runtime.session.Wave < len(waveDefinitions()) || len(runtime.smogs) > 0 {
		return false
	}
	finalWave, ok := currentWaveDefinition(runtime.session.Wave)
	return ok && runtime.waveSpawned >= finalWave.Count && runtime.nextWaveTick == 0
}

func moveSmogs(runtime *runtimeSession, deltaSeconds float64) []GameEvent {
	if runtime.session.Health <= 0 {
		return nil
	}
	events := make([]GameEvent, 0)
	nextSmogs := make([]gameobject.Smog, 0, len(runtime.smogs))
	for i := range runtime.smogs {
		smog := runtime.smogs[i]
		smog.Move(deltaSeconds, runtime.path)
		if smogReachedEnd(smog, runtime.path) {
			runtime.session.Health -= baseHealthDamage
			if runtime.session.Health < 0 {
				runtime.session.Health = 0
			}
			events = append(events, GameEvent{
				Type:   "smog.escaped",
				SmogID: smog.ID,
				Damage: float64(baseHealthDamage),
				Health: runtime.session.Health,
			})
			continue
		}
		nextSmogs = append(nextSmogs, smog)
	}
	runtime.smogs = nextSmogs
	return events
}

func fireBirds(runtime *runtimeSession) []GameEvent {
	if runtime.session.Health <= 0 {
		return nil
	}
	events := make([]GameEvent, 0)
	for i := range runtime.birds {
		bird := &runtime.birds[i].bird
		if !bird.CanAttack(runtime.session.Tick, gameTicksPerSecond) {
			continue
		}
		targetIndex := targetSmogIndex(*bird, runtime.smogs)
		if targetIndex < 0 {
			continue
		}
		target := runtime.smogs[targetIndex]
		projectiles := bird.Attack(target, runtime.session.Tick)
		if len(projectiles) == 0 {
			continue
		}
		runtime.projectiles = append(runtime.projectiles, projectiles...)
		events = append(events, GameEvent{
			Type:          "bird.attack",
			BirdID:        bird.ID,
			SmogID:        target.ID,
			ProjectileIDs: projectileIDs(projectiles),
		})
	}
	return events
}

func resolveProjectiles(runtime *runtimeSession, deltaSeconds float64) []GameEvent {
	events := make([]GameEvent, 0)
	active := make([]gameobject.Projectile, 0, len(runtime.projectiles))
	for i := range runtime.projectiles {
		projectile := runtime.projectiles[i]
		projectile.Move(deltaSeconds)
		switch projectile.Type {
		case gameobject.ProjectileTypeLocked:
			if !projectile.HasArrived() {
				active = append(active, projectile)
				continue
			}
			target := findSmogByID(runtime.smogs, projectile.TargetID)
			if target == nil {
				continue
			}
			beforeHealth := target.Health
			if projectile.ApplyLockedDamage(target) {
				events = append(events, GameEvent{
					Type:         "smog.damage",
					SmogID:       target.ID,
					ProjectileID: projectile.ID,
					Damage:       float64(beforeHealth - target.Health),
					Health:       target.Health,
				})
			}
		case gameobject.ProjectileTypeDirectional:
			hit, damage := collideDirectionalProjectile(&projectile, runtime.smogs)
			if hit != nil {
				events = append(events, GameEvent{
					Type:         "smog.damage",
					SmogID:       hit.ID,
					ProjectileID: projectile.ID,
					Damage:       damage,
					Health:       hit.Health,
				})
			}
			if !projectile.IsExpired() {
				active = append(active, projectile)
			}
		default:
			if !projectile.IsExpired() {
				active = append(active, projectile)
			}
		}
	}
	runtime.projectiles = active
	return events
}

func collideDirectionalProjectile(projectile *gameobject.Projectile, smogs []gameobject.Smog) (*gameobject.Smog, float64) {
	if projectile == nil || projectile.Type != gameobject.ProjectileTypeDirectional || projectile.IsExpired() {
		return nil, 0
	}
	for i := range smogs {
		if !smogs[i].IsAlive() {
			continue
		}
		if projectile.Position.DistanceTo(smogs[i].Position) > projectile.HitRadius {
			continue
		}
		beforeHealth := smogs[i].Health
		smogs[i].TakeDamage(projectile.Damage)
		projectile.RemainingRange = 0
		return &smogs[i], float64(beforeHealth - smogs[i].Health)
	}
	return nil, 0
}

func targetSmogIndex(bird gameobject.Bird, smogs []gameobject.Smog) int {
	bestIndex := -1
	for i := range smogs {
		if !bird.TargetInRange(smogs[i]) {
			continue
		}
		if bestIndex < 0 || smogs[i].PathIndex > smogs[bestIndex].PathIndex {
			bestIndex = i
		}
	}
	return bestIndex
}

func aliveSmogs(smogs []gameobject.Smog) []gameobject.Smog {
	active := make([]gameobject.Smog, 0, len(smogs))
	for _, smog := range smogs {
		if smog.IsAlive() {
			active = append(active, smog)
		}
	}
	return active
}

func findSmogByID(smogs []gameobject.Smog, id string) *gameobject.Smog {
	for i := range smogs {
		if smogs[i].ID == id {
			return &smogs[i]
		}
	}
	return nil
}

func projectileIDs(projectiles []gameobject.Projectile) []string {
	ids := make([]string, 0, len(projectiles))
	for _, projectile := range projectiles {
		ids = append(ids, projectile.ID)
	}
	return ids
}

func smogReachedEnd(smog gameobject.Smog, path []gameobject.Position) bool {
	if len(path) == 0 {
		return false
	}
	return smog.PathIndex >= len(path)-1 && smog.Position.DistanceTo(path[len(path)-1]) < 0.000001
}

func waveDefinitions() []waveDefinition {
	return []waveDefinition{
		scaledWaveDefinition(1, 15),
		scaledWaveDefinition(2, 24),
		scaledWaveDefinition(3, 36),
	}
}

func scaledWaveDefinition(wave int, count int) waveDefinition {
	return waveDefinition{
		Wave:   wave,
		Count:  count,
		Health: scaledSmogHealth(wave),
		Speed:  scaledSmogSpeed(wave),
	}
}

func scaledSmogHealth(wave int) int {
	waveOffset := wave - 1
	if waveOffset < 0 {
		waveOffset = 0
	}
	return baseSmogHealth + (waveOffset * 40) + (waveOffset * waveOffset * 5)
}

func scaledSmogSpeed(wave int) float64 {
	waveOffset := float64(wave - 1)
	if waveOffset < 0 {
		waveOffset = 0
	}
	return baseSmogSpeed + (waveOffset * 0.3) + (waveOffset * waveOffset * 0.03)
}
