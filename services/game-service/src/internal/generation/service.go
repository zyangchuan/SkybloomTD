package generation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"skybloom/game-service/internal/config"
	"skybloom/game-service/internal/mapgen"
	"skybloom/game-service/internal/models"
)

type Repository interface {
	CreateGeneration(ctx context.Context, generation models.LevelGenerationRecord) error
	FindReusableLevelWithQuizzes(ctx context.Context, userID string, subChapterID string) (models.ReusableLevel, error)
	GetGenerationByIdempotencyKey(ctx context.Context, idempotencyKey string) (models.LevelGenerationRecord, error)
}

type StatusStore interface {
	Set(ctx context.Context, status models.GenerationStatus) error
	Get(ctx context.Context, generationID string) (models.GenerationStatus, error)
}

type Publisher interface {
	Publish(ctx context.Context, messageID string, value any) error
}

type Service struct {
	config     config.Config
	repository Repository
	statuses   StatusStore
	publisher  Publisher
}

type StartResult struct {
	GenerationID        string  `json:"generation_id"`
	UserID              string  `json:"user_id"`
	SubChapterID        string  `json:"sub_chapter_id"`
	Status              string  `json:"status"`
	MapStatus           string  `json:"map_status"`
	QuizStatus          string  `json:"quiz_status"`
	MapSeed             int64   `json:"map_seed"`
	MapAlgorithmVersion int     `json:"map_algorithm_version"`
	LevelID             *string `json:"level_id,omitempty"`
	Reused              bool    `json:"reused"`
	StatusURL           string  `json:"status_url"`
}

func NewService(cfg config.Config, repository Repository, statuses StatusStore, publisher Publisher) *Service {
	return &Service{
		config:     cfg,
		repository: repository,
		statuses:   statuses,
		publisher:  publisher,
	}
}

func (s *Service) Start(ctx context.Context, userID string, subChapterID string) (StartResult, error) {
	if s.repository == nil || s.statuses == nil || s.publisher == nil {
		return StartResult{}, errors.New("level generation dependencies are not configured")
	}
	mapVersion := mapgen.Version
	key := IdempotencyKey(userID, subChapterID, mapVersion)

	existing, err := s.repository.GetGenerationByIdempotencyKey(ctx, key)
	if err == nil {
		return s.resultForExisting(ctx, existing)
	}
	if !errors.Is(err, models.ErrGenerationNotFound) {
		return StartResult{}, err
	}

	reusable, err := s.repository.FindReusableLevelWithQuizzes(ctx, userID, subChapterID)
	if err == nil && reusable.HasPlayableMap() {
		levelID := reusable.LevelID
		status := models.GenerationStatus{
			GenerationID:        *reusable.GenerationID,
			UserID:              reusable.UserID,
			SubChapterID:        reusable.SubChapterID,
			Status:              models.GenerationStatusComplete,
			MapStatus:           models.StepStatusComplete,
			QuizStatus:          models.StepStatusComplete,
			MapSeed:             *reusable.MapSeed,
			MapAlgorithmVersion: *reusable.MapAlgorithmVersion,
			LevelID:             &levelID,
			UpdatedAt:           time.Now().UTC(),
		}
		if err := s.statuses.Set(ctx, status); err != nil {
			return StartResult{}, err
		}
		return s.resultFromStatus(status, true), nil
	}
	if err != nil && !errors.Is(err, models.ErrLevelNotFound) {
		return StartResult{}, err
	}

	generation := models.LevelGenerationRecord{
		ID:                  randomHexID(),
		IdempotencyKey:      key,
		UserID:              userID,
		SubChapterID:        subChapterID,
		MapSeed:             randomSeed(),
		MapAlgorithmVersion: mapVersion,
	}
	if err := s.repository.CreateGeneration(ctx, generation); err != nil {
		existing, lookupErr := s.repository.GetGenerationByIdempotencyKey(ctx, key)
		if lookupErr == nil {
			return s.resultForExisting(ctx, existing)
		}
		return StartResult{}, err
	}

	status := pendingStatus(generation)
	if err := s.statuses.Set(ctx, status); err != nil {
		return StartResult{}, err
	}
	if err := s.publishJobs(ctx, generation); err != nil {
		message := err.Error()
		status.Status = models.GenerationStatusFailed
		status.Error = &message
		_ = s.statuses.Set(ctx, status)
		return StartResult{}, err
	}
	return s.resultFromStatus(status, false), nil
}

func (s *Service) resultForExisting(ctx context.Context, generation models.LevelGenerationRecord) (StartResult, error) {
	if generation.LevelID != nil && *generation.LevelID != "" {
		status := completeStatus(generation)
		if err := s.statuses.Set(ctx, status); err != nil {
			return StartResult{}, err
		}
		return s.resultFromStatus(status, true), nil
	}

	status, err := s.statuses.Get(ctx, generation.ID)
	if err == nil {
		if statusFailed(status) {
			return s.resetAndPublish(ctx, generation, true)
		}
		return s.resultFromStatus(status, true), nil
	}
	if !errors.Is(err, models.ErrStatusNotFound) {
		return StartResult{}, err
	}

	return s.resetAndPublish(ctx, generation, true)
}

func (s *Service) resetAndPublish(ctx context.Context, generation models.LevelGenerationRecord, reused bool) (StartResult, error) {
	status := pendingStatus(generation)
	if err := s.statuses.Set(ctx, status); err != nil {
		return StartResult{}, err
	}
	if err := s.publishJobs(ctx, generation); err != nil {
		message := err.Error()
		status.Status = models.GenerationStatusFailed
		status.Error = &message
		_ = s.statuses.Set(ctx, status)
		return StartResult{}, err
	}
	return s.resultFromStatus(status, reused), nil
}

func (s *Service) publishJobs(ctx context.Context, generation models.LevelGenerationRecord) error {
	mapJob := models.LevelJob{
		JobType:             models.JobTypeMapGenerate,
		TaskID:              randomHexID(),
		GenerationID:        generation.ID,
		UserID:              generation.UserID,
		SubChapterID:        generation.SubChapterID,
		MapSeed:             generation.MapSeed,
		MapAlgorithmVersion: generation.MapAlgorithmVersion,
	}
	quizJob := mapJob
	quizJob.JobType = models.JobTypeQuizGenerate
	quizJob.TaskID = randomHexID()

	if err := s.publisher.Publish(ctx, mapJob.TaskID, mapJob); err != nil {
		return fmt.Errorf("publish map generation job: %w", err)
	}
	if err := s.publisher.Publish(ctx, quizJob.TaskID, quizJob); err != nil {
		return fmt.Errorf("publish quiz generation job: %w", err)
	}
	return nil
}

func pendingStatus(generation models.LevelGenerationRecord) models.GenerationStatus {
	return models.GenerationStatus{
		GenerationID:        generation.ID,
		UserID:              generation.UserID,
		SubChapterID:        generation.SubChapterID,
		Status:              models.GenerationStatusPending,
		MapStatus:           models.StepStatusPending,
		QuizStatus:          models.StepStatusPending,
		MapSeed:             generation.MapSeed,
		MapAlgorithmVersion: generation.MapAlgorithmVersion,
		UpdatedAt:           time.Now().UTC(),
	}
}

func completeStatus(generation models.LevelGenerationRecord) models.GenerationStatus {
	status := pendingStatus(generation)
	status.Status = models.GenerationStatusComplete
	status.MapStatus = models.StepStatusComplete
	status.QuizStatus = models.StepStatusComplete
	status.LevelID = generation.LevelID
	return status
}

func statusFailed(status models.GenerationStatus) bool {
	return status.Status == models.GenerationStatusFailed ||
		status.MapStatus == models.StepStatusFailed ||
		status.QuizStatus == models.StepStatusFailed
}

func (s *Service) resultFromStatus(status models.GenerationStatus, reused bool) StartResult {
	basePath := strings.TrimRight(s.config.PublicAPIBasePath, "/")
	if basePath == "" {
		basePath = "/api/game-service"
	}
	return StartResult{
		GenerationID:        status.GenerationID,
		UserID:              status.UserID,
		SubChapterID:        status.SubChapterID,
		Status:              status.Status,
		MapStatus:           status.MapStatus,
		QuizStatus:          status.QuizStatus,
		MapSeed:             status.MapSeed,
		MapAlgorithmVersion: status.MapAlgorithmVersion,
		LevelID:             status.LevelID,
		Reused:              reused,
		StatusURL:           basePath + "/level-generation/" + status.GenerationID + "/status",
	}
}

func IdempotencyKey(userID string, subChapterID string, mapVersion int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d", userID, subChapterID, mapVersion)))
	return hex.EncodeToString(sum[:])
}

func randomHexID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("random source unavailable: %v", err))
	}
	return hex.EncodeToString(b[:])
}

func randomSeed() int64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("random source unavailable: %v", err))
	}
	seed := int64(binary.BigEndian.Uint64(b[:]) & ((1 << 53) - 1))
	if seed == 0 {
		return 1
	}
	return seed
}
