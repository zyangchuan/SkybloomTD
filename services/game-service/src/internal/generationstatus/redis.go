package generationstatus

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"skybloom/game-service/internal/models"
	"skybloom/game-service/internal/redisclient"
)

type Store struct {
	client *redisclient.Client
	ttl    time.Duration
}

func New(redisURL string, ttl time.Duration) (*Store, error) {
	if strings.TrimSpace(redisURL) == "" {
		return nil, nil
	}
	client, err := redisclient.New(redisURL)
	if err != nil {
		return nil, err
	}
	return &Store{client: client, ttl: ttl}, nil
}

func (s *Store) Set(ctx context.Context, status models.GenerationStatus) error {
	_ = ctx
	status.UpdatedAt = time.Now().UTC()
	values := map[string]any{
		"generation_id":         status.GenerationID,
		"user_id":               status.UserID,
		"sub_chapter_id":        status.SubChapterID,
		"status":                deriveStatus(status.Status, status.MapStatus, status.QuizStatus),
		"map_status":            status.MapStatus,
		"quiz_status":           status.QuizStatus,
		"map_seed":              status.MapSeed,
		"map_algorithm_version": status.MapAlgorithmVersion,
		"updated_at":            status.UpdatedAt.Format(time.RFC3339Nano),
	}
	if status.LevelID != nil {
		values["level_id"] = *status.LevelID
	}
	if status.Error != nil {
		values["error"] = *status.Error
	}
	redisKey := key(status.GenerationID)
	if err := s.hset(redisKey, values); err != nil {
		return err
	}
	if status.LevelID == nil {
		if _, err := s.client.Do("HDEL", redisKey, "level_id"); err != nil {
			return err
		}
	}
	if status.Error == nil {
		if _, err := s.client.Do("HDEL", redisKey, "error"); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Get(ctx context.Context, generationID string) (models.GenerationStatus, error) {
	_ = ctx
	raw, err := s.client.Do("HGETALL", key(generationID))
	if err != nil {
		return models.GenerationStatus{}, err
	}
	values := hashValues(raw)
	if len(values) == 0 {
		return models.GenerationStatus{}, models.ErrStatusNotFound
	}
	return parse(values)
}

func (s *Store) MarkStep(ctx context.Context, job models.LevelJob, step string, stepStatus string, update models.StatusUpdate) error {
	_ = ctx
	if strings.TrimSpace(job.GenerationID) == "" {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	mapSeed := job.MapSeed
	if update.MapSeed != nil {
		mapSeed = *update.MapSeed
	}
	mapVersion := job.MapAlgorithmVersion
	if update.MapAlgorithmVersion != nil {
		mapVersion = *update.MapAlgorithmVersion
	}
	values := map[string]any{
		"generation_id":         job.GenerationID,
		"user_id":               job.UserID,
		"sub_chapter_id":        job.SubChapterID,
		"map_seed":              mapSeed,
		"map_algorithm_version": mapVersion,
		step + "_status":        stepStatus,
		"updated_at":            now,
	}
	if update.LevelID != nil {
		values["level_id"] = *update.LevelID
	}
	if update.Error != nil {
		values["error"] = *update.Error
		values["status"] = models.GenerationStatusFailed
	}

	redisKey := key(job.GenerationID)
	if err := s.hset(redisKey, values); err != nil {
		return err
	}
	if update.Error != nil {
		return nil
	}

	rawStatus, err := s.client.Do("HGET", redisKey, "status")
	if err != nil && !errors.Is(err, redisclient.ErrNil) {
		return err
	}
	status, _ := rawStatus.(string)
	if status == models.GenerationStatusFailed {
		return nil
	}

	rawFields, err := s.client.Do("HMGET", redisKey, "map_status", "quiz_status")
	if err != nil {
		return err
	}
	fields, _ := rawFields.([]any)
	if stringField(fields, 0) == models.StepStatusComplete &&
		stringField(fields, 1) == models.StepStatusComplete {
		return s.hset(redisKey, map[string]any{
			"status":     models.GenerationStatusComplete,
			"updated_at": now,
		})
	}
	return nil
}

func (s *Store) Close() error {
	if s.client == nil {
		return nil
	}
	return s.client.Close()
}

func (s *Store) hset(redisKey string, values map[string]any) error {
	args := []string{"HSET", redisKey}
	for field, value := range values {
		args = append(args, field, asString(value))
	}
	if _, err := s.client.Do(args...); err != nil {
		return err
	}
	_, err := s.client.Do("EXPIRE", redisKey, strconv.Itoa(int(s.ttl.Seconds())))
	return err
}

func parse(values map[string]string) (models.GenerationStatus, error) {
	mapSeed, err := strconv.ParseInt(values["map_seed"], 10, 64)
	if err != nil {
		return models.GenerationStatus{}, err
	}
	mapVersion, err := strconv.Atoi(values["map_algorithm_version"])
	if err != nil {
		return models.GenerationStatus{}, err
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, values["updated_at"])
	if err != nil {
		return models.GenerationStatus{}, err
	}

	status := models.GenerationStatus{
		GenerationID:        values["generation_id"],
		UserID:              values["user_id"],
		SubChapterID:        values["sub_chapter_id"],
		Status:              deriveStatus(values["status"], values["map_status"], values["quiz_status"]),
		MapStatus:           values["map_status"],
		QuizStatus:          values["quiz_status"],
		MapSeed:             mapSeed,
		MapAlgorithmVersion: mapVersion,
		UpdatedAt:           updatedAt,
	}
	if levelID := strings.TrimSpace(values["level_id"]); levelID != "" {
		status.LevelID = &levelID
	}
	if message := strings.TrimSpace(values["error"]); message != "" {
		status.Error = &message
	}
	return status, nil
}

func deriveStatus(current string, mapStatus string, quizStatus string) string {
	if current == models.GenerationStatusFailed ||
		mapStatus == models.StepStatusFailed ||
		quizStatus == models.StepStatusFailed {
		return models.GenerationStatusFailed
	}
	if mapStatus == models.StepStatusComplete && quizStatus == models.StepStatusComplete {
		return models.GenerationStatusComplete
	}
	return models.GenerationStatusPending
}

func stringField(values []any, index int) string {
	if index >= len(values) || values[index] == nil {
		return ""
	}
	value, ok := values[index].(string)
	if !ok {
		return ""
	}
	return value
}

func key(generationID string) string {
	return "game-level-generation:" + generationID
}

func asString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		return fmt.Sprint(typed)
	}
}

func hashValues(raw any) map[string]string {
	values := map[string]string{}
	items, ok := raw.([]any)
	if !ok {
		return values
	}
	for i := 0; i+1 < len(items); i += 2 {
		field, fieldOK := items[i].(string)
		value, valueOK := items[i+1].(string)
		if fieldOK && valueOK {
			values[field] = value
		}
	}
	return values
}
