package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"skybloom/game-service/internal/config"
	"skybloom/game-service/internal/gamesession"
	"skybloom/game-service/internal/generation"
	"skybloom/game-service/internal/mapcache"
	"skybloom/game-service/internal/mapgen"
	"skybloom/game-service/internal/models"
	"skybloom/game-service/internal/repository"
)

func TestWebsocketLoadSendsInitialState(t *testing.T) {
	levels := &fakeLevelRepository{
		bootstrap: repository.LevelBootstrap{
			LevelID:             "11111111-1111-1111-1111-111111111111",
			UserID:              "22222222-2222-2222-2222-222222222222",
			DocumentID:          "33333333-3333-3333-3333-333333333333",
			ChapterID:           "44444444-4444-4444-4444-444444444444",
			SubChapterID:        "55555555-5555-5555-5555-555555555555",
			GenerationID:        "generation-1",
			MapSeed:             12345,
			MapAlgorithmVersion: mapgen.Version,
			SummaryMarkdown:     "summary",
			Quizzes: []repository.QuizItem{
				{
					ID:               "66666666-6666-6666-6666-666666666666",
					QuizIndex:        0,
					QuizType:         "true_false",
					QuestionMarkdown: "Hidden question",
					OptionsMarkdown:  []string{"True", "False"},
					AnswerIndex:      1,
				},
			},
		},
	}
	maps := &fakeMapCache{}
	handler := New(config.Config{}, levels, maps).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
	header := http.Header{"X-Authenticated-User-Id": []string{"22222222-2222-2222-2222-222222222222"}}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(Message{
		Type: "game.load",
		Data: map[string]string{"level_id": "11111111-1111-1111-1111-111111111111"},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	var message Message
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}
	if message.Type != "game.initial_state" {
		t.Fatalf("expected initial state message, got %q", message.Type)
	}

	body, err := json.Marshal(message.Data)
	if err != nil {
		t.Fatalf("Marshal data failed: %v", err)
	}
	if strings.Contains(string(body), "quizzes") || strings.Contains(string(body), "answer_index") {
		t.Fatalf("initial state leaked quiz data: %s", string(body))
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("Unmarshal raw initial state failed: %v", err)
	}
	if _, ok := raw["level"]; ok {
		t.Fatalf("initial state should not include level data: %s", string(body))
	}
	var state InitialState
	if err := json.Unmarshal(body, &state); err != nil {
		t.Fatalf("Unmarshal initial state failed: %v", err)
	}
	if state.Map.Seed != 12345 {
		t.Fatalf("unexpected map seed %d", state.Map.Seed)
	}
	if len(state.Map.EnemyPath) == 0 {
		t.Fatal("expected enemy path")
	}
	if maps.cached.Seed != 12345 {
		t.Fatal("expected regenerated map to be cached")
	}
}

func TestWebsocketStartPublishesGeneration(t *testing.T) {
	starter := &fakeStarter{
		result: generation.StartResult{
			GenerationID:        "generation-1",
			UserID:              "22222222-2222-2222-2222-222222222222",
			SubChapterID:        "55555555-5555-5555-5555-555555555555",
			Status:              models.GenerationStatusPending,
			MapStatus:           models.StepStatusPending,
			QuizStatus:          models.StepStatusPending,
			MapSeed:             12345,
			MapAlgorithmVersion: mapgen.Version,
			StatusURL:           "/api/game-service/level-generation/generation-1/status",
		},
	}
	handler := NewWithGeneration(config.Config{}, &fakeLevelRepository{}, &fakeMapCache{}, nil, starter, nil).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
	header := http.Header{"X-Authenticated-User-Id": []string{"22222222-2222-2222-2222-222222222222"}}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(Message{
		Type: "game.start",
		Data: map[string]string{"sub_chapter_id": "55555555-5555-5555-5555-555555555555"},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	var message Message
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}
	if message.Type != "level_generation.started" {
		t.Fatalf("expected level_generation.started message, got %q", message.Type)
	}
	if starter.userID != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("unexpected user_id %q", starter.userID)
	}
	if starter.subChapterID != "55555555-5555-5555-5555-555555555555" {
		t.Fatalf("unexpected sub_chapter_id %q", starter.subChapterID)
	}
}

func TestWebsocketSessionStartInitializesStateAndTicks(t *testing.T) {
	levels := &fakeLevelRepository{
		bootstrap: repository.LevelBootstrap{
			LevelID:             "11111111-1111-1111-1111-111111111111",
			UserID:              "22222222-2222-2222-2222-222222222222",
			SubChapterID:        "55555555-5555-5555-5555-555555555555",
			GenerationID:        "generation-1",
			MapSeed:             12345,
			MapAlgorithmVersion: mapgen.Version,
		},
	}
	sessions := &fakeGameSessionStore{}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, levels, nil, nil, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
	header := http.Header{"X-Authenticated-User-Id": []string{"22222222-2222-2222-2222-222222222222"}}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline failed: %v", err)
	}

	if err := conn.WriteJSON(Message{
		Type: "game.session.start",
		Data: map[string]string{"level_id": "11111111-1111-1111-1111-111111111111"},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	var started Message
	if err := conn.ReadJSON(&started); err != nil {
		t.Fatalf("ReadJSON started failed: %v", err)
	}
	if started.Type != "game.session.started" {
		t.Fatalf("expected game.session.started message, got %q", started.Type)
	}
	startedState := decodeGameState(t, started.Data)
	if startedState.Health != gamesession.InitialHealth {
		t.Fatalf("unexpected health %d", startedState.Health)
	}
	if startedState.Essence != gamesession.InitialEssence {
		t.Fatalf("unexpected essence %d", startedState.Essence)
	}
	if startedState.Wave != gamesession.InitialWave {
		t.Fatalf("unexpected wave %d", startedState.Wave)
	}
	if startedState.Tick != 0 {
		t.Fatalf("expected tick 0, got %d", startedState.Tick)
	}
	if sessions.options.UserID != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("unexpected session user_id %q", sessions.options.UserID)
	}
	if sessions.options.LevelID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("unexpected session level_id %q", sessions.options.LevelID)
	}

	var tick Message
	if err := conn.ReadJSON(&tick); err != nil {
		t.Fatalf("ReadJSON tick failed: %v", err)
	}
	if tick.Type != "game.state" {
		t.Fatalf("expected game.state message, got %q", tick.Type)
	}
	tickState := decodeGameState(t, tick.Data)
	if tickState.Tick != 1 {
		t.Fatalf("expected first tick to be 1, got %d", tickState.Tick)
	}
	if tickState.Health != gamesession.InitialHealth || tickState.Essence != gamesession.InitialEssence || tickState.Wave != gamesession.InitialWave {
		t.Fatalf("unexpected tick state %+v", tickState)
	}
}

func TestWebsocketRequiresAuthentication(t *testing.T) {
	handler := New(config.Config{}, &fakeLevelRepository{}, &fakeMapCache{}).Router()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ws", nil)

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", response.Code)
	}
}

type fakeLevelRepository struct {
	bootstrap repository.LevelBootstrap
}

func (r *fakeLevelRepository) GetBootstrap(_ context.Context, levelID string, userID string) (repository.LevelBootstrap, error) {
	if r.bootstrap.LevelID != levelID || r.bootstrap.UserID != userID {
		return repository.LevelBootstrap{}, models.ErrLevelNotFound
	}
	return r.bootstrap, nil
}

func (r *fakeLevelRepository) Ping(context.Context) error {
	return nil
}

type fakeMapCache struct {
	cached mapgen.GeneratedMap
}

func (c *fakeMapCache) Get(context.Context, int, string) (mapgen.GeneratedMap, error) {
	if c.cached.Seed == 0 {
		return mapgen.GeneratedMap{}, mapcache.ErrMapNotFound
	}
	return c.cached, nil
}

func (c *fakeMapCache) Set(_ context.Context, _ string, levelMap mapgen.GeneratedMap) error {
	c.cached = levelMap
	return nil
}

func startHTTPServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("local test listener unavailable: %v", err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	return server
}

type fakeStarter struct {
	result       generation.StartResult
	userID       string
	subChapterID string
}

func (s *fakeStarter) Start(_ context.Context, userID string, subChapterID string) (generation.StartResult, error) {
	s.userID = userID
	s.subChapterID = subChapterID
	return s.result, nil
}

type fakeGameSessionStore struct {
	options gamesession.StartOptions
}

func (s *fakeGameSessionStore) Start(_ context.Context, options gamesession.StartOptions) (gamesession.State, error) {
	s.options = options
	now := time.Now().UTC()
	return gamesession.State{
		SessionID:    "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		UserID:       options.UserID,
		LevelID:      options.LevelID,
		GenerationID: options.GenerationID,
		SubChapterID: options.SubChapterID,
		Health:       gamesession.InitialHealth,
		Essence:      gamesession.InitialEssence,
		Wave:         gamesession.InitialWave,
		Tick:         0,
		StartedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func decodeGameState(t *testing.T, data any) GameState {
	t.Helper()
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Marshal game state failed: %v", err)
	}
	var state GameState
	if err := json.Unmarshal(body, &state); err != nil {
		t.Fatalf("Unmarshal game state failed: %v", err)
	}
	return state
}
