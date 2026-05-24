package worker

import (
	"context"
	"encoding/json"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"

	"skybloom/level-generator-worker/internal/config"
	"skybloom/level-generator-worker/internal/generator"
	"skybloom/level-generator-worker/internal/models"
	"skybloom/level-generator-worker/internal/repository"
	"skybloom/level-generator-worker/internal/source"
)

type Worker struct {
	config    config.Config
	levels    LevelRepository
	sources   SourceFetcher
	generator LevelGenerator
}

type LevelRepository interface {
	Insert(ctx context.Context, sourceContext source.SourceContext, generation generator.LevelGeneration, model string) (repository.SavedLevel, error)
}

type SourceFetcher interface {
	FetchSubChapterContent(ctx context.Context, userID string, subChapterID string) (source.SourceContext, error)
}

type LevelGenerator interface {
	GenerateLevel(ctx context.Context, sourceContext source.SourceContext) (generator.LevelGeneration, error)
}

func New(
	cfg config.Config,
	levels LevelRepository,
	sources SourceFetcher,
	generatorClient LevelGenerator,
) *Worker {
	return &Worker{
		config:    cfg,
		levels:    levels,
		sources:   sources,
		generator: generatorClient,
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

	log.Printf("level-generator worker consuming queue=%s", w.config.Queue)
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
	sourceContext, err := w.sources.FetchSubChapterContent(ctx, job.UserID, job.SubChapterID)
	if err != nil {
		return err
	}
	generation, err := w.generator.GenerateLevel(ctx, sourceContext)
	if err != nil {
		return err
	}
	saved, err := w.levels.Insert(ctx, sourceContext, generation, w.config.Model)
	if err != nil {
		return err
	}
	log.Printf(
		"level job complete task_id=%s level_id=%s sub_chapter_id=%s quiz_count=%d",
		job.TaskID,
		saved.LevelID,
		saved.SubChapterID,
		saved.QuizCount,
	)
	return nil
}
