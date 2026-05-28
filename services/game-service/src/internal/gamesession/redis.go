package gamesession

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"skybloom/game-service/internal/redisclient"
)

const (
	InitialHealth  = 100
	InitialEssence = 0
	InitialWave    = 0
)

type StartOptions struct {
	UserID       string
	LevelID      string
	GenerationID string
	SubChapterID string
}

type State struct {
	SessionID    string    `json:"session_id"`
	UserID       string    `json:"user_id"`
	LevelID      string    `json:"level_id"`
	GenerationID string    `json:"generation_id"`
	SubChapterID string    `json:"sub_chapter_id"`
	Health       int       `json:"health"`
	Essence      int       `json:"essence"`
	Wave         int       `json:"wave"`
	Tick         int64     `json:"tick"`
	StartedAt    time.Time `json:"started_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

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
	if ttl <= 0 {
		ttl = 2 * time.Hour
	}
	return &Store{client: client, ttl: ttl}, nil
}

func (s *Store) Start(ctx context.Context, options StartOptions) (State, error) {
	_ = ctx
	if s == nil || s.client == nil {
		return State{}, errors.New("game session store is not configured")
	}
	now := time.Now().UTC()
	state := State{
		SessionID:    uuid.NewString(),
		UserID:       options.UserID,
		LevelID:      options.LevelID,
		GenerationID: options.GenerationID,
		SubChapterID: options.SubChapterID,
		Health:       InitialHealth,
		Essence:      InitialEssence,
		Wave:         InitialWave,
		Tick:         0,
		StartedAt:    now,
		UpdatedAt:    now,
	}
	values := map[string]any{
		"session_id":     state.SessionID,
		"user_id":        state.UserID,
		"level_id":       state.LevelID,
		"generation_id":  state.GenerationID,
		"sub_chapter_id": state.SubChapterID,
		"health":         state.Health,
		"essence":        state.Essence,
		"wave":           state.Wave,
		"wave_number":    state.Wave,
		"tick":           state.Tick,
		"started_at":     state.StartedAt.Format(time.RFC3339Nano),
		"updated_at":     state.UpdatedAt.Format(time.RFC3339Nano),
	}
	redisKey := key(state.SessionID)
	args := []string{"HSET", redisKey}
	for field, value := range values {
		args = append(args, field, asString(value))
	}
	if _, err := s.client.Do(args...); err != nil {
		return State{}, err
	}
	_, err := s.client.Do("EXPIRE", redisKey, strconv.Itoa(int(s.ttl.Seconds())))
	if err != nil {
		return State{}, err
	}
	return state, nil
}

func (s *Store) Close() error {
	if s.client == nil {
		return nil
	}
	return s.client.Close()
}

func key(sessionID string) string {
	return "game-session:v1:" + sessionID
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
