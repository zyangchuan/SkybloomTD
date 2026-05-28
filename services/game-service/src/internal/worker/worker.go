package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"

	"skybloom/game-service/internal/config"
	"skybloom/game-service/internal/generator"
	"skybloom/game-service/internal/mapgen"
	"skybloom/game-service/internal/models"
	"skybloom/game-service/internal/quizcache"
	"skybloom/game-service/internal/repository"
	"skybloom/game-service/internal/source"
)

type Worker struct {
	config    config.Config
	levels    LevelRepository
	sources   SourceFetcher
	generator LevelGenerator
	statuses  GenerationStatusStore
	maps      MapCache
	quizzes   QuizCache
}

type LevelRepository interface {
	AttachGenerationToLevel(ctx context.Context, generationID string, levelID string, options repository.LevelInsertOptions) (repository.SavedLevel, error)
	FindReusableLevelWithQuizzes(ctx context.Context, userID string, subChapterID string) (models.ReusableLevel, error)
	GetBootstrap(ctx context.Context, levelID string, userID string) (repository.LevelBootstrap, error)
	Insert(ctx context.Context, sourceContext source.SourceContext, generation generator.LevelGeneration, model string, options repository.LevelInsertOptions) (repository.SavedLevel, error)
}

type SourceFetcher interface {
	FetchSubChapterContent(ctx context.Context, userID string, subChapterID string) (source.SourceContext, error)
}

type LevelGenerator interface {
	GenerateLevel(ctx context.Context, sourceContext source.SourceContext) (generator.LevelGeneration, error)
}

type GenerationStatusStore interface {
	MarkStep(ctx context.Context, job models.LevelJob, step string, stepStatus string, update models.StatusUpdate) error
}

type MapCache interface {
	Set(ctx context.Context, generationID string, levelMap mapgen.GeneratedMap) error
}

type QuizCache interface {
	Set(ctx context.Context, generationID string, quizzes quizcache.LevelQuizzes) error
}

func New(
	cfg config.Config,
	levels LevelRepository,
	sources SourceFetcher,
	generatorClient LevelGenerator,
) *Worker {
	return NewWithStores(cfg, levels, sources, generatorClient, NoopGenerationStatusStore{}, NoopMapCache{})
}

func NewWithStores(
	cfg config.Config,
	levels LevelRepository,
	sources SourceFetcher,
	generatorClient LevelGenerator,
	statuses GenerationStatusStore,
	maps MapCache,
) *Worker {
	return NewWithStoresAndQuizCache(cfg, levels, sources, generatorClient, statuses, maps, NoopQuizCache{})
}

func NewWithStoresAndQuizCache(
	cfg config.Config,
	levels LevelRepository,
	sources SourceFetcher,
	generatorClient LevelGenerator,
	statuses GenerationStatusStore,
	maps MapCache,
	quizzes QuizCache,
) *Worker {
	if statuses == nil {
		statuses = NoopGenerationStatusStore{}
	}
	if maps == nil {
		maps = NoopMapCache{}
	}
	if quizzes == nil {
		quizzes = NoopQuizCache{}
	}
	return &Worker{
		config:    cfg,
		levels:    levels,
		sources:   sources,
		generator: generatorClient,
		statuses:  statuses,
		maps:      maps,
		quizzes:   quizzes,
	}
}

func (w *Worker) Consume(ctx context.Context) error {
	conn, err := amqp.Dial(w.config.RabbitMQURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if _, err := ch.QueueDeclare(w.config.Queue, true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.Qos(1, 0, false); err != nil {
		return err
	}
	deliveries, err := ch.Consume(w.config.Queue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	log.Printf("game-service generation worker consuming queue=%s", w.config.Queue)
	for delivery := range deliveries {
		var job models.LevelJob
		if err := json.Unmarshal(delivery.Body, &job); err != nil {
			log.Printf("invalid level job: %v", err)
			_ = delivery.Nack(false, false)
			continue
		}
		if err := w.ProcessJob(ctx, job); err != nil {
			log.Printf("level job failed task_id=%s sub_chapter_id=%s: %v", job.TaskID, job.SubChapterID, err)
			_ = delivery.Nack(false, false)
			continue
		}
		_ = delivery.Ack(false)
	}
	return nil
}

func (w *Worker) ProcessJob(ctx context.Context, job models.LevelJob) error {
	switch job.JobType {
	case models.JobTypeMapGenerate:
		return w.ProcessMapJob(ctx, job)
	case models.JobTypeQuizGenerate:
		return w.ProcessQuizJob(ctx, job)
	default:
		return fmt.Errorf("unsupported level job type %q", job.JobType)
	}
}

func (w *Worker) ProcessMapJob(ctx context.Context, job models.LevelJob) error {
	if err := w.statuses.MarkStep(ctx, job, "map", models.StepStatusRunning, models.StatusUpdate{}); err != nil {
		log.Printf("map status running update failed generation_id=%s: %v", job.GenerationID, err)
	}

	levelMap, err := mapgen.Generate(job.MapSeed, job.MapAlgorithmVersion)
	if err != nil {
		w.markStepFailed(ctx, job, "map", err)
		return err
	}
	if err := w.maps.Set(ctx, job.GenerationID, levelMap); err != nil {
		w.markStepFailed(ctx, job, "map", err)
		return err
	}
	if err := w.statuses.MarkStep(ctx, job, "map", models.StepStatusComplete, models.StatusUpdate{}); err != nil {
		return err
	}

	log.Printf(
		"map job complete task_id=%s generation_id=%s seed=%d version=%d path_tiles=%d objects=%d",
		job.TaskID,
		job.GenerationID,
		job.MapSeed,
		job.MapAlgorithmVersion,
		len(levelMap.EnemyPath),
		len(levelMap.Objects),
	)
	return nil
}

func (w *Worker) ProcessQuizJob(ctx context.Context, job models.LevelJob) error {
	if err := w.statuses.MarkStep(ctx, job, "quiz", models.StepStatusRunning, models.StatusUpdate{}); err != nil {
		log.Printf("quiz status running update failed generation_id=%s: %v", job.GenerationID, err)
	}

	options := repository.LevelInsertOptions{
		GenerationID:        job.GenerationID,
		MapSeed:             job.MapSeed,
		MapAlgorithmVersion: job.MapAlgorithmVersion,
	}
	reusable, err := w.levels.FindReusableLevelWithQuizzes(ctx, job.UserID, job.SubChapterID)
	if err == nil {
		saved, err := w.levels.AttachGenerationToLevel(ctx, job.GenerationID, reusable.LevelID, options)
		if err != nil {
			w.markStepFailed(ctx, job, "quiz", err)
			return err
		}
		quizCount, err := w.cacheLevelQuizzes(ctx, job, saved.LevelID)
		if err != nil {
			w.markStepFailed(ctx, job, "quiz", err)
			return err
		}
		update := models.StatusUpdate{LevelID: &saved.LevelID}
		if reusable.HasPlayableMap() {
			update.MapSeed = reusable.MapSeed
			update.MapAlgorithmVersion = reusable.MapAlgorithmVersion
		}
		if err := w.statuses.MarkStep(ctx, job, "quiz", models.StepStatusComplete, update); err != nil {
			return err
		}
		log.Printf(
			"quiz job reused existing quizzes task_id=%s generation_id=%s level_id=%s sub_chapter_id=%s quiz_count=%d",
			job.TaskID,
			job.GenerationID,
			saved.LevelID,
			saved.SubChapterID,
			quizCount,
		)
		return nil
	}
	if !errors.Is(err, models.ErrLevelNotFound) {
		w.markStepFailed(ctx, job, "quiz", err)
		return err
	}

	sourceContext, err := w.sources.FetchSubChapterContent(ctx, job.UserID, job.SubChapterID)
	if err != nil {
		w.markStepFailed(ctx, job, "quiz", err)
		return err
	}
	generation, err := w.generator.GenerateLevel(ctx, sourceContext)
	if err != nil {
		w.markStepFailed(ctx, job, "quiz", err)
		return err
	}
	saved, err := w.levels.Insert(ctx, sourceContext, generation, w.config.Model, options)
	if err != nil {
		w.markStepFailed(ctx, job, "quiz", err)
		return err
	}
	quizCount, err := w.cacheLevelQuizzes(ctx, job, saved.LevelID)
	if err != nil {
		w.markStepFailed(ctx, job, "quiz", err)
		return err
	}
	if err := w.statuses.MarkStep(ctx, job, "quiz", models.StepStatusComplete, models.StatusUpdate{LevelID: &saved.LevelID}); err != nil {
		return err
	}
	log.Printf(
		"quiz job complete task_id=%s generation_id=%s level_id=%s sub_chapter_id=%s quiz_count=%d",
		job.TaskID,
		job.GenerationID,
		saved.LevelID,
		saved.SubChapterID,
		quizCount,
	)
	return nil
}

func (w *Worker) cacheLevelQuizzes(ctx context.Context, job models.LevelJob, levelID string) (int, error) {
	level, err := w.levels.GetBootstrap(ctx, levelID, job.UserID)
	if err != nil {
		return 0, fmt.Errorf("load level quizzes for cache: %w", err)
	}
	if len(level.Quizzes) == 0 {
		return 0, fmt.Errorf("level %s has no quizzes", levelID)
	}
	if err := w.quizzes.Set(ctx, job.GenerationID, quizcache.FromLevelBootstrap(level, job.GenerationID)); err != nil {
		return 0, fmt.Errorf("cache level quizzes: %w", err)
	}
	if level.GenerationID != "" && level.GenerationID != job.GenerationID {
		if err := w.quizzes.Set(ctx, level.GenerationID, quizcache.FromLevelBootstrap(level, level.GenerationID)); err != nil {
			return 0, fmt.Errorf("cache level quizzes for bound generation: %w", err)
		}
	}
	return len(level.Quizzes), nil
}

func (w *Worker) markStepFailed(ctx context.Context, job models.LevelJob, step string, err error) {
	message := err.Error()
	if statusErr := w.statuses.MarkStep(ctx, job, step, models.StepStatusFailed, models.StatusUpdate{Error: &message}); statusErr != nil {
		log.Printf("%s status failure update failed generation_id=%s: %v", step, job.GenerationID, statusErr)
	}
}

type NoopGenerationStatusStore struct{}

func (NoopGenerationStatusStore) MarkStep(context.Context, models.LevelJob, string, string, models.StatusUpdate) error {
	return nil
}

type NoopMapCache struct{}

func (NoopMapCache) Set(context.Context, string, mapgen.GeneratedMap) error {
	return nil
}

type NoopQuizCache struct{}

func (NoopQuizCache) Set(context.Context, string, quizcache.LevelQuizzes) error {
	return nil
}
