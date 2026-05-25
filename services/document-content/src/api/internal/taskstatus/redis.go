package taskstatus

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"skybloom/document-content-api/internal/models"
)

type Store struct {
	client *redis.Client
	ttl    time.Duration
}

type NoopStore struct{}

func New(redisURL string, ttl time.Duration) (*Store, error) {
	if strings.TrimSpace(redisURL) == "" {
		return nil, nil
	}

	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}

	return &Store{
		client: redis.NewClient(options),
		ttl:    ttl,
	}, nil
}

func (s *Store) Set(ctx context.Context, status models.TaskStatus) error {
	status.UpdatedAt = time.Now().UTC()
	body, err := json.Marshal(status)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, key(status.TaskID), body, s.ttl).Err()
}

func (s *Store) Get(ctx context.Context, taskID string) (models.TaskStatus, error) {
	raw, err := s.client.Get(ctx, key(taskID)).Result()
	if err == redis.Nil {
		return models.TaskStatus{}, models.ErrTaskStatusNotFound
	}
	if err != nil {
		return models.TaskStatus{}, err
	}

	var status models.TaskStatus
	if err := json.Unmarshal([]byte(raw), &status); err != nil {
		return models.TaskStatus{}, err
	}
	return status, nil
}

func (s *Store) Close() error {
	if s.client == nil {
		return nil
	}
	return s.client.Close()
}

func (NoopStore) Set(context.Context, models.TaskStatus) error {
	return nil
}

func (NoopStore) Get(context.Context, string) (models.TaskStatus, error) {
	return models.TaskStatus{}, models.ErrTaskStatusNotFound
}

func key(taskID string) string {
	return "task:" + taskID
}
