package generation

import (
	"context"
	"errors"
	"testing"

	"skybloom/game-service/internal/config"
	"skybloom/game-service/internal/mapgen"
	"skybloom/game-service/internal/models"
)

func TestStartPublishesMapAndQuizJobs(t *testing.T) {
	repo := &fakeGenerationRepository{
		generationByKeyErr: models.ErrGenerationNotFound,
		reusableErr:        models.ErrLevelNotFound,
	}
	statuses := &fakeStatusStore{}
	publisher := &fakePublisher{}
	service := NewService(config.Config{PublicAPIBasePath: "/api/game-service"}, repo, statuses, publisher)

	result, err := service.Start(context.Background(), "user-1", "subchapter-1")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if result.Status != models.GenerationStatusPending ||
		result.MapStatus != models.StepStatusPending ||
		result.QuizStatus != models.StepStatusPending ||
		result.MapAlgorithmVersion != mapgen.Version {
		t.Fatalf("unexpected start result: %#v", result)
	}
	if repo.created.ID == "" || repo.created.UserID != "user-1" || repo.created.SubChapterID != "subchapter-1" {
		t.Fatalf("generation record was not created correctly: %#v", repo.created)
	}
	if len(statuses.set) != 1 || statuses.set[0].GenerationID != repo.created.ID {
		t.Fatalf("expected pending status for created generation, got %#v", statuses.set)
	}
	if len(publisher.messages) != 2 {
		t.Fatalf("expected map and quiz jobs, got %d", len(publisher.messages))
	}
	if publisher.messages[0].job.JobType != models.JobTypeMapGenerate ||
		publisher.messages[1].job.JobType != models.JobTypeQuizGenerate {
		t.Fatalf("unexpected published jobs: %#v", publisher.messages)
	}
}

func TestStartMarksStatusFailedWhenPublishFails(t *testing.T) {
	repo := &fakeGenerationRepository{
		generationByKeyErr: models.ErrGenerationNotFound,
		reusableErr:        models.ErrLevelNotFound,
	}
	statuses := &fakeStatusStore{}
	publisher := &fakePublisher{err: errors.New("rabbitmq unavailable")}
	service := NewService(config.Config{}, repo, statuses, publisher)

	_, err := service.Start(context.Background(), "user-1", "subchapter-1")
	if err == nil {
		t.Fatalf("expected publish failure")
	}

	if len(statuses.set) != 2 {
		t.Fatalf("expected pending and failed statuses, got %#v", statuses.set)
	}
	failed := statuses.set[1]
	if failed.Status != models.GenerationStatusFailed || failed.Error == nil || *failed.Error == "" {
		t.Fatalf("expected failed generation status with error, got %#v", failed)
	}
}

type fakeGenerationRepository struct {
	created            models.LevelGenerationRecord
	generationByKey    models.LevelGenerationRecord
	generationByKeyErr error
	reusable           models.ReusableLevel
	reusableErr        error
	clearedGeneration  string
}

func (r *fakeGenerationRepository) CreateGeneration(_ context.Context, generation models.LevelGenerationRecord) error {
	r.created = generation
	return nil
}

func (r *fakeGenerationRepository) FindReusableLevelWithQuizzes(_ context.Context, _ string, _ string) (models.ReusableLevel, error) {
	return r.reusable, r.reusableErr
}

func (r *fakeGenerationRepository) GetGenerationByIdempotencyKey(_ context.Context, _ string) (models.LevelGenerationRecord, error) {
	return r.generationByKey, r.generationByKeyErr
}

func (r *fakeGenerationRepository) ClearGenerationLevelID(_ context.Context, generationID string) error {
	r.clearedGeneration = generationID
	return nil
}

type fakeStatusStore struct {
	set []models.GenerationStatus
	get models.GenerationStatus
	err error
}

func (s *fakeStatusStore) Set(_ context.Context, status models.GenerationStatus) error {
	s.set = append(s.set, status)
	return s.err
}

func (s *fakeStatusStore) Get(_ context.Context, _ string) (models.GenerationStatus, error) {
	if s.err != nil {
		return models.GenerationStatus{}, s.err
	}
	return s.get, nil
}

type publishedGenerationMessage struct {
	messageID string
	job       models.LevelJob
}

type fakePublisher struct {
	messages []publishedGenerationMessage
	err      error
}

func (p *fakePublisher) Publish(_ context.Context, messageID string, value any) error {
	if p.err != nil {
		return p.err
	}
	job, ok := value.(models.LevelJob)
	if !ok {
		return errors.New("unexpected published value")
	}
	p.messages = append(p.messages, publishedGenerationMessage{messageID: messageID, job: job})
	return nil
}
