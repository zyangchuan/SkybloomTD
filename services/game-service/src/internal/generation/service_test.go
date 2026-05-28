package generation

import (
	"context"
	"errors"
	"testing"

	"skybloom/game-service/internal/config"
	"skybloom/game-service/internal/mapgen"
	"skybloom/game-service/internal/models"
)

func TestStartCreatesGenerationOnceForIdempotencyKey(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepository()
	statuses := newFakeStatuses()
	publisher := &fakePublisher{}
	service := NewService(config.Config{}, repo, statuses, publisher)

	first, err := service.Start(ctx, "user-1", "sub-1")
	if err != nil {
		t.Fatalf("first Start failed: %v", err)
	}
	second, err := service.Start(ctx, "user-1", "sub-1")
	if err != nil {
		t.Fatalf("second Start failed: %v", err)
	}

	if first.GenerationID != second.GenerationID {
		t.Fatalf("expected idempotent generation id, got %q and %q", first.GenerationID, second.GenerationID)
	}
	if !second.Reused {
		t.Fatal("expected second result to be marked reused")
	}
	if repo.createCount != 1 {
		t.Fatalf("expected one DB generation, got %d", repo.createCount)
	}
	if publisher.publishCount != 2 {
		t.Fatalf("expected one map and one quiz message, got %d messages", publisher.publishCount)
	}
}

func TestStartReturnsCompletedGenerationWithoutRepublishing(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepository()
	statuses := newFakeStatuses()
	publisher := &fakePublisher{}
	service := NewService(config.Config{}, repo, statuses, publisher)

	key := IdempotencyKey("user-1", "sub-1", mapgen.Version)
	levelID := "level-1"
	repo.byKey[key] = models.LevelGenerationRecord{
		ID:                  "generation-1",
		IdempotencyKey:      key,
		UserID:              "user-1",
		SubChapterID:        "sub-1",
		MapSeed:             123,
		MapAlgorithmVersion: mapgen.Version,
		LevelID:             &levelID,
	}

	result, err := service.Start(ctx, "user-1", "sub-1")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if result.Status != models.GenerationStatusComplete {
		t.Fatalf("expected complete status, got %q", result.Status)
	}
	if publisher.publishCount != 0 {
		t.Fatalf("expected no republished jobs, got %d", publisher.publishCount)
	}
}

func TestStartRepublishesFailedExistingGeneration(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepository()
	statuses := newFakeStatuses()
	publisher := &fakePublisher{}
	service := NewService(config.Config{}, repo, statuses, publisher)

	key := IdempotencyKey("user-1", "sub-1", mapgen.Version)
	repo.byKey[key] = models.LevelGenerationRecord{
		ID:                  "generation-1",
		IdempotencyKey:      key,
		UserID:              "user-1",
		SubChapterID:        "sub-1",
		MapSeed:             123,
		MapAlgorithmVersion: mapgen.Version,
	}
	message := "old validation error"
	statuses.byGeneration["generation-1"] = models.GenerationStatus{
		GenerationID:        "generation-1",
		UserID:              "user-1",
		SubChapterID:        "sub-1",
		Status:              models.GenerationStatusFailed,
		MapStatus:           models.StepStatusComplete,
		QuizStatus:          models.StepStatusFailed,
		MapSeed:             123,
		MapAlgorithmVersion: mapgen.Version,
		Error:               &message,
	}

	result, err := service.Start(ctx, "user-1", "sub-1")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if result.Status != models.GenerationStatusPending {
		t.Fatalf("expected reset pending status, got %q", result.Status)
	}
	if result.MapStatus != models.StepStatusPending || result.QuizStatus != models.StepStatusPending {
		t.Fatalf("expected reset step statuses, got map=%q quiz=%q", result.MapStatus, result.QuizStatus)
	}
	if !result.Reused {
		t.Fatal("expected existing generation record to be marked reused")
	}
	if publisher.publishCount != 2 {
		t.Fatalf("expected republished map and quiz jobs, got %d messages", publisher.publishCount)
	}
	if statuses.byGeneration["generation-1"].Error != nil {
		t.Fatalf("expected old error to be cleared, got %q", *statuses.byGeneration["generation-1"].Error)
	}
}

func TestStartReusesPlayableLevelWithExistingQuizzes(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepository()
	statuses := newFakeStatuses()
	publisher := &fakePublisher{}
	service := NewService(config.Config{}, repo, statuses, publisher)

	generationID := "generation-1"
	mapSeed := int64(123)
	mapVersion := mapgen.Version
	repo.reusable = &models.ReusableLevel{
		LevelID:                "11111111-1111-1111-1111-111111111111",
		UserID:                 "user-1",
		SubChapterID:           "sub-1",
		GenerationID:           &generationID,
		MapSeed:                &mapSeed,
		MapAlgorithmVersion:    &mapVersion,
		GenerationRecordExists: true,
		QuizCount:              10,
	}

	result, err := service.Start(ctx, "user-1", "sub-1")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if result.GenerationID != generationID {
		t.Fatalf("expected reusable generation id %q, got %q", generationID, result.GenerationID)
	}
	if result.LevelID == nil || *result.LevelID != repo.reusable.LevelID {
		t.Fatalf("expected reusable level id %q, got %v", repo.reusable.LevelID, result.LevelID)
	}
	if result.Status != models.GenerationStatusComplete {
		t.Fatalf("expected complete status, got %q", result.Status)
	}
	if !result.Reused {
		t.Fatal("expected result to be marked reused")
	}
	if publisher.publishCount != 0 {
		t.Fatalf("expected no queued jobs, got %d", publisher.publishCount)
	}
}

type fakeRepository struct {
	byKey       map[string]models.LevelGenerationRecord
	createCount int
	reusable    *models.ReusableLevel
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{byKey: map[string]models.LevelGenerationRecord{}}
}

func (r *fakeRepository) CreateGeneration(_ context.Context, generation models.LevelGenerationRecord) error {
	r.createCount++
	r.byKey[generation.IdempotencyKey] = generation
	return nil
}

func (r *fakeRepository) GetGenerationByIdempotencyKey(_ context.Context, idempotencyKey string) (models.LevelGenerationRecord, error) {
	generation, ok := r.byKey[idempotencyKey]
	if !ok {
		return models.LevelGenerationRecord{}, models.ErrGenerationNotFound
	}
	return generation, nil
}

func (r *fakeRepository) FindReusableLevelWithQuizzes(_ context.Context, userID string, subChapterID string) (models.ReusableLevel, error) {
	if r.reusable == nil || r.reusable.UserID != userID || r.reusable.SubChapterID != subChapterID {
		return models.ReusableLevel{}, models.ErrLevelNotFound
	}
	return *r.reusable, nil
}

type fakeStatuses struct {
	byGeneration map[string]models.GenerationStatus
}

func newFakeStatuses() *fakeStatuses {
	return &fakeStatuses{byGeneration: map[string]models.GenerationStatus{}}
}

func (s *fakeStatuses) Set(_ context.Context, status models.GenerationStatus) error {
	s.byGeneration[status.GenerationID] = status
	return nil
}

func (s *fakeStatuses) Get(_ context.Context, generationID string) (models.GenerationStatus, error) {
	status, ok := s.byGeneration[generationID]
	if !ok {
		return models.GenerationStatus{}, models.ErrStatusNotFound
	}
	return status, nil
}

type fakePublisher struct {
	publishCount int
	err          error
}

func (p *fakePublisher) Publish(context.Context, string, any) error {
	p.publishCount++
	if p.err != nil {
		return errors.New("publish failed")
	}
	return nil
}
