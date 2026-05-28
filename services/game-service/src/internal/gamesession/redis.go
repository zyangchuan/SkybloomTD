package gamesession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"skybloom/game-service/internal/gameobject"
	"skybloom/game-service/internal/redisclient"
)

var ErrSessionNotFound = errors.New("game session not found")

const (
	InitialHealth  = 100
	InitialEssence = 1000
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

type StoredBird struct {
	ID              string               `json:"id"`
	Type            string               `json:"type"`
	Position        gameobject.Position  `json:"position"`
	Stats           gameobject.BirdStats `json:"stats"`
	LastFiredAtTick int64                `json:"last_fired_at_tick"`
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
	if existing, err := s.getExisting(options.UserID, options.LevelID); err == nil {
		if err := s.refreshTTL(existing.SessionID, existing.UserID, existing.LevelID); err != nil {
			return State{}, err
		}
		return existing, nil
	} else if !errors.Is(err, ErrSessionNotFound) {
		return State{}, err
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
		"birds":          "[]",
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
	if err := s.refreshTTL(state.SessionID, state.UserID, state.LevelID); err != nil {
		return State{}, err
	}
	return state, nil
}

func (s *Store) LoadBirds(ctx context.Context, sessionID string) ([]StoredBird, error) {
	_ = ctx
	raw, err := s.client.Do("HGET", key(sessionID), "birds")
	if errors.Is(err, redisclient.ErrNil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	body, ok := raw.(string)
	if !ok || strings.TrimSpace(body) == "" {
		return nil, nil
	}
	var birds []StoredBird
	if err := json.Unmarshal([]byte(body), &birds); err != nil {
		return nil, err
	}
	return birds, nil
}

func (s *Store) SaveRuntimeState(ctx context.Context, sessionID string, economy Economy, birds []StoredBird) error {
	_ = ctx
	if s == nil || s.client == nil {
		return errors.New("game session store is not configured")
	}
	body, err := json.Marshal(birds)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	redisKey := key(sessionID)
	args := []string{
		"HSET",
		redisKey,
		"essence",
		strconv.Itoa(economy.Essence),
		"birds",
		string(body),
		"updated_at",
		now.Format(time.RFC3339Nano),
	}
	if _, err := s.client.Do(args...); err != nil {
		return err
	}
	state, err := s.getBySessionID(sessionID)
	if errors.Is(err, ErrSessionNotFound) {
		return s.refreshSessionKeyTTL(sessionID)
	}
	if err != nil {
		return err
	}
	return s.refreshTTL(sessionID, state.UserID, state.LevelID)
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

func indexKey(userID string, levelID string) string {
	return "game-session:v1:user:" + userID + ":level:" + levelID
}

func (s *Store) getExisting(userID string, levelID string) (State, error) {
	raw, err := s.client.Do("GET", indexKey(userID, levelID))
	if errors.Is(err, redisclient.ErrNil) {
		return State{}, ErrSessionNotFound
	}
	if err != nil {
		return State{}, err
	}
	sessionID, ok := raw.(string)
	if !ok || strings.TrimSpace(sessionID) == "" {
		return State{}, ErrSessionNotFound
	}
	state, err := s.getBySessionID(sessionID)
	if errors.Is(err, ErrSessionNotFound) {
		_, _ = s.client.Do("DEL", indexKey(userID, levelID))
		return State{}, ErrSessionNotFound
	}
	if err != nil {
		return State{}, err
	}
	if state.UserID != userID || state.LevelID != levelID {
		return State{}, ErrSessionNotFound
	}
	return state, nil
}

func (s *Store) getBySessionID(sessionID string) (State, error) {
	raw, err := s.client.Do("HGETALL", key(sessionID))
	if err != nil {
		return State{}, err
	}
	values := hashValues(raw)
	if len(values) == 0 {
		return State{}, ErrSessionNotFound
	}
	return parseState(values)
}

func (s *Store) refreshTTL(sessionID string, userID string, levelID string) error {
	if err := s.refreshSessionKeyTTL(sessionID); err != nil {
		return err
	}
	seconds := strconv.Itoa(int(s.ttl.Seconds()))
	if _, err := s.client.Do("SET", indexKey(userID, levelID), sessionID, "EX", seconds); err != nil {
		return err
	}
	return nil
}

func (s *Store) refreshSessionKeyTTL(sessionID string) error {
	_, err := s.client.Do("EXPIRE", key(sessionID), strconv.Itoa(int(s.ttl.Seconds())))
	return err
}

func parseState(values map[string]string) (State, error) {
	health, err := strconv.Atoi(values["health"])
	if err != nil {
		return State{}, err
	}
	essence, err := strconv.Atoi(values["essence"])
	if err != nil {
		return State{}, err
	}
	wave, err := strconv.Atoi(values["wave"])
	if err != nil {
		return State{}, err
	}
	tick, err := strconv.ParseInt(values["tick"], 10, 64)
	if err != nil {
		return State{}, err
	}
	startedAt, err := time.Parse(time.RFC3339Nano, values["started_at"])
	if err != nil {
		return State{}, err
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, values["updated_at"])
	if err != nil {
		return State{}, err
	}
	return State{
		SessionID:    values["session_id"],
		UserID:       values["user_id"],
		LevelID:      values["level_id"],
		GenerationID: values["generation_id"],
		SubChapterID: values["sub_chapter_id"],
		Health:       health,
		Essence:      essence,
		Wave:         wave,
		Tick:         tick,
		StartedAt:    startedAt,
		UpdatedAt:    updatedAt,
	}, nil
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
