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
	InitialEssence = 100
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

type StoredEnemy struct {
	ID        string              `json:"id"`
	Type      string              `json:"type"`
	Health    int                 `json:"health"`
	Position  gameobject.Position `json:"position"`
	Speed     float64             `json:"speed"`
	PathIndex int                 `json:"path_index"`
}

type StoredProjectile struct {
	ID              string                    `json:"id"`
	Type            gameobject.ProjectileType `json:"type"`
	Damage          float64                   `json:"damage"`
	ProjectileSpeed float64                   `json:"projectile_speed"`
	Position        gameobject.Position       `json:"position"`
	Direction       gameobject.Vector         `json:"direction"`
	TargetID        string                    `json:"target_id,omitempty"`
	RemainingRange  float64                   `json:"remaining_range"`
	HitRadius       float64                   `json:"hit_radius"`
}

type RuntimeState struct {
	GenerationID      string
	Health            int
	Essence           int
	Wave              int
	Tick              int64
	LoopStarted       bool
	LoopPaused        bool
	WaveStartedAtTick int64
	WaveSpawned       int
	NextWaveTick      int64
	LastQuizStartedAt time.Time
	Birds             []StoredBird
	Enemies           []StoredEnemy
	Projectiles       []StoredProjectile
}

type QuizMistake struct {
	ID               string     `json:"id"`
	LevelID          string     `json:"level_id"`
	GenerationID     string     `json:"generation_id"`
	QuizID           string     `json:"quiz_id"`
	QuizIndex        int        `json:"quiz_index"`
	QuizType         string     `json:"quiz_type"`
	QuestionMarkdown string     `json:"question_markdown"`
	OptionsMarkdown  []string   `json:"options_markdown"`
	AnswerIndex      int        `json:"answer_index"`
	SelectedIndex    int        `json:"selected_index"`
	CreatedAt        *time.Time `json:"created_at"`
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
		"session_id":           state.SessionID,
		"user_id":              state.UserID,
		"level_id":             state.LevelID,
		"generation_id":        state.GenerationID,
		"sub_chapter_id":       state.SubChapterID,
		"health":               state.Health,
		"essence":              state.Essence,
		"wave":                 state.Wave,
		"wave_number":          state.Wave,
		"tick":                 state.Tick,
		"loop_started":         false,
		"loop_paused":          false,
		"wave_started_at_tick": 0,
		"wave_spawned":         0,
		"next_wave_tick":       1,
		"birds":                "[]",
		"enemies":              "[]",
		"projectiles":          "[]",
		"started_at":           state.StartedAt.Format(time.RFC3339Nano),
		"updated_at":           state.UpdatedAt.Format(time.RFC3339Nano),
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

func (s *Store) LoadRuntimeState(ctx context.Context, sessionID string) (RuntimeState, error) {
	_ = ctx
	raw, err := s.client.Do("HGETALL", key(sessionID))
	if err != nil {
		return RuntimeState{}, err
	}
	values := hashValues(raw)
	if len(values) == 0 {
		return RuntimeState{}, ErrSessionNotFound
	}
	var birds []StoredBird
	if err := unmarshalStoredSlice(values["birds"], &birds); err != nil {
		return RuntimeState{}, err
	}
	var enemies []StoredEnemy
	if err := unmarshalStoredSlice(values["enemies"], &enemies); err != nil {
		return RuntimeState{}, err
	}
	var projectiles []StoredProjectile
	if err := unmarshalStoredSlice(values["projectiles"], &projectiles); err != nil {
		return RuntimeState{}, err
	}

	state, err := parseState(values)
	if err != nil {
		return RuntimeState{}, err
	}
	return RuntimeState{
		GenerationID:      state.GenerationID,
		Health:            state.Health,
		Essence:           state.Essence,
		Wave:              state.Wave,
		Tick:              state.Tick,
		LoopStarted:       boolValue(values["loop_started"], false),
		LoopPaused:        boolValue(values["loop_paused"], false),
		WaveStartedAtTick: int64Value(values["wave_started_at_tick"], 0),
		WaveSpawned:       intValue(values["wave_spawned"], 0),
		NextWaveTick:      int64Value(values["next_wave_tick"], 0),
		LastQuizStartedAt: timeValue(values["last_quiz_started_at"]),
		Birds:             birds,
		Enemies:           enemies,
		Projectiles:       projectiles,
	}, nil
}

func (s *Store) LoadBirds(ctx context.Context, sessionID string) ([]StoredBird, error) {
	runtime, err := s.LoadRuntimeState(ctx, sessionID)
	if errors.Is(err, ErrSessionNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return runtime.Birds, nil
}

func (s *Store) SaveRuntimeState(ctx context.Context, sessionID string, runtime RuntimeState) error {
	_ = ctx
	if s == nil || s.client == nil {
		return errors.New("game session store is not configured")
	}
	birdsBody, err := json.Marshal(runtime.Birds)
	if err != nil {
		return err
	}
	enemiesBody, err := json.Marshal(runtime.Enemies)
	if err != nil {
		return err
	}
	projectilesBody, err := json.Marshal(runtime.Projectiles)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	redisKey := key(sessionID)
	args := []string{
		"HSET",
		redisKey,
		"health",
		strconv.Itoa(runtime.Health),
		"essence",
		strconv.Itoa(runtime.Essence),
		"wave",
		strconv.Itoa(runtime.Wave),
		"wave_number",
		strconv.Itoa(runtime.Wave),
		"tick",
		strconv.FormatInt(runtime.Tick, 10),
		"loop_started",
		strconv.FormatBool(runtime.LoopStarted),
		"loop_paused",
		strconv.FormatBool(runtime.LoopPaused),
		"wave_started_at_tick",
		strconv.FormatInt(runtime.WaveStartedAtTick, 10),
		"wave_spawned",
		strconv.Itoa(runtime.WaveSpawned),
		"next_wave_tick",
		strconv.FormatInt(runtime.NextWaveTick, 10),
		"last_quiz_started_at",
		formatOptionalTime(runtime.LastQuizStartedAt),
		"birds",
		string(birdsBody),
		"enemies",
		string(enemiesBody),
		"projectiles",
		string(projectilesBody),
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

func (s *Store) Delete(ctx context.Context, sessionID string) error {
	_ = ctx
	if s == nil || s.client == nil {
		return errors.New("game session store is not configured")
	}
	raw, err := s.client.Do("HGETALL", key(sessionID))
	if errors.Is(err, redisclient.ErrNil) {
		raw = nil
		err = nil
	}
	if err != nil {
		return err
	}
	values := hashValues(raw)
	args := []string{"DEL", key(sessionID), mistakesKey(sessionID)}
	if values["user_id"] != "" && values["level_id"] != "" {
		args = append(args, indexKey(values["user_id"], values["level_id"]))
	}
	_, err = s.client.Do(args...)
	return err
}

func (s *Store) SaveQuizMistake(ctx context.Context, sessionID string, userID string, mistake QuizMistake) error {
	_ = ctx
	if s == nil || s.client == nil {
		return errors.New("game session store is not configured")
	}
	state, err := s.getBySessionID(sessionID)
	if err != nil {
		return err
	}
	if state.UserID != userID {
		return ErrSessionNotFound
	}
	if mistake.ID == "" {
		mistake.ID = uuid.NewString()
	}
	if mistake.LevelID == "" {
		mistake.LevelID = state.LevelID
	}
	if mistake.GenerationID == "" {
		mistake.GenerationID = state.GenerationID
	}
	now := time.Now().UTC()
	mistake.CreatedAt = &now
	body, err := json.Marshal(mistake)
	if err != nil {
		return err
	}
	redisKey := mistakesKey(sessionID)
	if _, err := s.client.Do("RPUSH", redisKey, string(body)); err != nil {
		return err
	}
	if _, err := s.client.Do("EXPIRE", redisKey, strconv.Itoa(int(s.ttl.Seconds()))); err != nil {
		return err
	}
	return s.refreshTTL(sessionID, state.UserID, state.LevelID)
}

func (s *Store) ListQuizMistakes(ctx context.Context, sessionID string, userID string) ([]QuizMistake, error) {
	_ = ctx
	if s == nil || s.client == nil {
		return nil, errors.New("game session store is not configured")
	}
	state, err := s.getBySessionID(sessionID)
	if err != nil {
		return nil, err
	}
	if state.UserID != userID {
		return nil, ErrSessionNotFound
	}
	raw, err := s.client.Do("LRANGE", mistakesKey(sessionID), "0", "-1")
	if errors.Is(err, redisclient.ErrNil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected redis mistakes payload type %T", raw)
	}
	mistakes := make([]QuizMistake, 0, len(values))
	for _, value := range values {
		body, ok := value.(string)
		if !ok || strings.TrimSpace(body) == "" {
			continue
		}
		var mistake QuizMistake
		if err := json.Unmarshal([]byte(body), &mistake); err != nil {
			return nil, err
		}
		mistakes = append(mistakes, mistake)
	}
	return mistakes, nil
}

func (s *Store) ClearQuizMistakes(ctx context.Context, sessionID string, userID string) error {
	_ = ctx
	if s == nil || s.client == nil {
		return errors.New("game session store is not configured")
	}
	state, err := s.getBySessionID(sessionID)
	if err != nil {
		return err
	}
	if state.UserID != userID {
		return ErrSessionNotFound
	}
	_, err = s.client.Do("DEL", mistakesKey(sessionID))
	return err
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

func mistakesKey(sessionID string) string {
	return key(sessionID) + ":mistakes"
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
	if _, err := s.client.Do("EXPIRE", mistakesKey(sessionID), seconds); err != nil && !strings.Contains(err.Error(), "no such key") {
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

func unmarshalStoredSlice(body string, target any) error {
	if strings.TrimSpace(body) == "" {
		body = "[]"
	}
	return json.Unmarshal([]byte(body), target)
}

func intValue(value string, fallback int) int {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func int64Value(value string, fallback int64) int64 {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func boolValue(value string, fallback bool) bool {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func timeValue(value string) time.Time {
	if strings.TrimSpace(value) == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
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
