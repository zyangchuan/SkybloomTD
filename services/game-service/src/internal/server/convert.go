package server

import (
	"time"

	"skybloom/game-service/internal/gameobject"
	"skybloom/game-service/internal/gamesession"
	"skybloom/game-service/internal/mapgen"
	"skybloom/game-service/internal/quizcache"
	"skybloom/game-service/internal/quiztext"
	"skybloom/game-service/internal/repository"
)

func gameStateFromRuntime(runtime runtimeSession, serverTime time.Time, birdTypes []BirdTypeInfo, events []GameEvent) GameState {
	session := runtime.session
	return GameState{
		SessionID:   session.SessionID,
		LevelID:     session.LevelID,
		Health:      session.Health,
		Essence:     runtime.economy.Essence,
		Wave:        session.Wave,
		Tick:        session.Tick,
		ServerTime:  serverTime.UTC(),
		BirdTypes:   birdTypes,
		Birds:       placedBirdStates(runtime.birds),
		Smogs:       smogStates(runtime.smogs),
		Projectiles: projectileStates(runtime.projectiles),
		Events:      events,
	}
}

func placedBirdStates(birds []placedBird) []PlacedBirdState {
	states := make([]PlacedBirdState, 0, len(birds))
	for _, placed := range birds {
		states = append(states, PlacedBirdState{
			ID:              placed.bird.ID,
			Type:            placed.birdType,
			Position:        placed.bird.Position,
			Stats:           placed.bird.Stats,
			LastFiredAtTick: placed.bird.LastFiredAtTick,
		})
	}
	return states
}

func smogStates(smogs []gameobject.Smog) []SmogState {
	states := make([]SmogState, 0, len(smogs))
	for _, smog := range smogs {
		states = append(states, SmogState{
			ID:        smog.ID,
			Health:    smog.Health,
			Position:  smog.Position,
			Speed:     smog.Speed,
			PathIndex: smog.PathIndex,
		})
	}
	return states
}

func projectileStates(projectiles []gameobject.Projectile) []ProjectileState {
	states := make([]ProjectileState, 0, len(projectiles))
	for _, projectile := range projectiles {
		states = append(states, ProjectileState{
			ID:              projectile.ID,
			Type:            projectile.Type,
			Damage:          projectile.Damage,
			ProjectileSpeed: projectile.ProjectileSpeed,
			Position:        projectile.Position,
			Direction:       projectile.Direction,
			TargetID:        projectile.TargetID,
			RemainingRange:  projectile.RemainingRange,
			HitRadius:       projectile.HitRadius,
		})
	}
	return states
}

func storedBirds(birds []placedBird) []gamesession.StoredBird {
	stored := make([]gamesession.StoredBird, 0, len(birds))
	for _, placed := range birds {
		stored = append(stored, gamesession.StoredBird{
			ID:              placed.bird.ID,
			Type:            placed.birdType,
			Position:        placed.bird.Position,
			Stats:           placed.bird.Stats,
			LastFiredAtTick: placed.bird.LastFiredAtTick,
		})
	}
	return stored
}

func storedSmogs(smogs []gameobject.Smog) []gamesession.StoredSmog {
	stored := make([]gamesession.StoredSmog, 0, len(smogs))
	for _, smog := range smogs {
		stored = append(stored, gamesession.StoredSmog{
			ID:        smog.ID,
			Health:    smog.Health,
			Position:  smog.Position,
			Speed:     smog.Speed,
			PathIndex: smog.PathIndex,
		})
	}
	return stored
}

func storedProjectiles(projectiles []gameobject.Projectile) []gamesession.StoredProjectile {
	stored := make([]gamesession.StoredProjectile, 0, len(projectiles))
	for _, projectile := range projectiles {
		stored = append(stored, gamesession.StoredProjectile{
			ID:              projectile.ID,
			Type:            projectile.Type,
			Damage:          projectile.Damage,
			ProjectileSpeed: projectile.ProjectileSpeed,
			Position:        projectile.Position,
			Direction:       projectile.Direction,
			TargetID:        projectile.TargetID,
			RemainingRange:  projectile.RemainingRange,
			HitRadius:       projectile.HitRadius,
		})
	}
	return stored
}

func smogsFromStored(stored []gamesession.StoredSmog) []gameobject.Smog {
	smogs := make([]gameobject.Smog, 0, len(stored))
	for _, item := range stored {
		smogs = append(smogs, gameobject.Smog{
			ID:        item.ID,
			Health:    item.Health,
			Position:  item.Position,
			Speed:     item.Speed,
			PathIndex: item.PathIndex,
		})
	}
	return smogs
}

func projectilesFromStored(stored []gamesession.StoredProjectile) []gameobject.Projectile {
	projectiles := make([]gameobject.Projectile, 0, len(stored))
	for _, item := range stored {
		projectiles = append(projectiles, gameobject.Projectile{
			ID:              item.ID,
			Type:            item.Type,
			Damage:          item.Damage,
			ProjectileSpeed: item.ProjectileSpeed,
			Position:        item.Position,
			Direction:       item.Direction,
			TargetID:        item.TargetID,
			RemainingRange:  item.RemainingRange,
			HitRadius:       item.HitRadius,
		})
	}
	return projectiles
}

func placedBirdsFromStored(stored []gamesession.StoredBird) ([]placedBird, error) {
	birds := make([]placedBird, 0, len(stored))
	for _, item := range stored {
		behaviour, err := gameobject.AttackBehaviourForType(item.Type)
		if err != nil {
			return nil, err
		}
		birds = append(birds, placedBird{
			birdType: item.Type,
			bird: gameobject.Bird{
				ID:              item.ID,
				Position:        item.Position,
				Stats:           item.Stats,
				AttackBehaviour: behaviour,
				LastFiredAtTick: item.LastFiredAtTick,
			},
		})
	}
	return birds, nil
}

func quizPromptState(quiz quizcache.CachedQuiz, remaining int) QuizPromptState {
	return QuizPromptState{
		QuizID:           quiz.ID,
		QuizType:         quiz.QuizType,
		QuestionMarkdown: quiztext.SanitizeMarkdown(quiz.QuestionMarkdown),
		OptionsMarkdown:  quiztext.SanitizeMarkdownSlice(quiz.OptionsMarkdown),
		Remaining:        remaining,
	}
}

func quizMistakeSummaryState(levelID string, mistakes []repository.QuizMistakeSummaryItem) QuizMistakeSummaryState {
	items := make([]QuizMistakeSummaryItem, 0, len(mistakes))
	for _, mistake := range mistakes {
		options := quiztext.SanitizeMarkdownSlice(mistake.OptionsMarkdown)
		items = append(items, QuizMistakeSummaryItem{
			ID:                     mistake.ID,
			LevelID:                mistake.LevelID,
			GenerationID:           mistake.GenerationID,
			QuizID:                 mistake.QuizID,
			QuizIndex:              mistake.QuizIndex,
			QuizType:               mistake.QuizType,
			QuestionMarkdown:       quiztext.SanitizeMarkdown(mistake.QuestionMarkdown),
			OptionsMarkdown:        options,
			AnswerIndex:            mistake.AnswerIndex,
			SelectedIndex:          mistake.SelectedIndex,
			CorrectOptionMarkdown:  optionMarkdown(options, mistake.AnswerIndex),
			SelectedOptionMarkdown: optionMarkdown(options, mistake.SelectedIndex),
			CreatedAt:              mistake.CreatedAt,
		})
	}
	return QuizMistakeSummaryState{
		LevelID:  levelID,
		Count:    len(items),
		Mistakes: items,
	}
}

func birdTypeCatalog() []BirdTypeInfo {
	birdTypes := gameobject.BirdTypes()
	catalog := make([]BirdTypeInfo, 0, len(birdTypes))
	for _, birdType := range birdTypes {
		stats, err := gameobject.BirdStatsForType(birdType)
		if err != nil {
			continue
		}
		attack, err := gameobject.AttackTypeForBirdType(birdType)
		if err != nil {
			continue
		}
		catalog = append(catalog, BirdTypeInfo{
			Type:   birdType,
			Stats:  stats,
			Attack: attack,
		})
	}
	return catalog
}

func gamePath(levelMap mapgen.GeneratedMap) []gameobject.Position {
	path := make([]gameobject.Position, 0, len(levelMap.EnemyPath))
	for _, tile := range levelMap.EnemyPath {
		path = append(path, gameobject.Position{X: float64(tile.X), Y: float64(tile.Y)})
	}
	return path
}

func normalizeRuntimeState(runtime gamesession.RuntimeState) gamesession.RuntimeState {
	if runtime.NextWaveTick == 0 && runtime.Wave <= 0 {
		runtime.NextWaveTick = 1
	}
	if runtime.NextWaveTick == 0 && runtime.Wave > 0 && runtime.WaveSpawned == 0 {
		if wave, ok := currentWaveDefinition(runtime.Wave); ok {
			runtime.WaveSpawned = wave.Count
		}
	}
	if runtime.WaveStartedAtTick == 0 && runtime.Wave > 0 && runtime.WaveSpawned > 0 {
		runtime.WaveStartedAtTick = runtime.Tick
	}
	return runtime
}

func optionMarkdown(options []string, index int) string {
	if index < 0 || index >= len(options) {
		return ""
	}
	return options[index]
}

func cachedQuizByID(quizzes []quizcache.CachedQuiz, quizID string) (quizcache.CachedQuiz, bool) {
	for _, quiz := range quizzes {
		if quiz.ID == quizID {
			return quiz, true
		}
	}
	return quizcache.CachedQuiz{}, false
}
