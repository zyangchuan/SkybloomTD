package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"skybloom/game-service/internal/config"
	"skybloom/game-service/internal/gameobject"
	"skybloom/game-service/internal/gamesession"
	"skybloom/game-service/internal/generation"
	"skybloom/game-service/internal/mapgen"
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
			Status:              "pending",
			MapStatus:           "pending",
			QuizStatus:          "pending",
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
	if len(startedState.BirdTypes) != 4 {
		t.Fatalf("expected 4 bird type infos, got %d", len(startedState.BirdTypes))
	}
	if len(startedState.Birds) != 0 {
		t.Fatalf("expected no placed birds at session start, got %d", len(startedState.Birds))
	}
	if len(startedState.Smogs) != 0 {
		t.Fatalf("expected no smogs before first tick, got %d", len(startedState.Smogs))
	}
	if len(startedState.Projectiles) != 0 {
		t.Fatalf("expected no projectiles before first tick, got %d", len(startedState.Projectiles))
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
	if tickState.Health != gamesession.InitialHealth || tickState.Essence != gamesession.InitialEssence || tickState.Wave != 1 {
		t.Fatalf("unexpected tick state %+v", tickState)
	}
	if len(tickState.Smogs) != 1 {
		t.Fatalf("expected first tick to spawn one smog, got %d", len(tickState.Smogs))
	}
	if tickState.Smogs[0].Health != baseSmogHealth {
		t.Fatalf("expected first smog health %d, got %d", baseSmogHealth, tickState.Smogs[0].Health)
	}
	if tickState.Smogs[0].Speed != baseSmogSpeed {
		t.Fatalf("expected first smog speed %.2f, got %f", baseSmogSpeed, tickState.Smogs[0].Speed)
	}
}

func TestWebsocketSessionStartAllowsEmptyQuizList(t *testing.T) {
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
	quizzes := &fakeQuizCache{}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, levels, nil, quizzes, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	if err := conn.WriteJSON(Message{
		Type: "game.session.start",
		Data: map[string]string{"level_id": "11111111-1111-1111-1111-111111111111"},
	}); err != nil {
		t.Fatalf("WriteJSON session start failed: %v", err)
	}
	readMessageOfType(t, conn, "game.session.started")

	if err := conn.WriteJSON(Message{Type: "game.quiz.request"}); err != nil {
		t.Fatalf("WriteJSON quiz request failed: %v", err)
	}
	unavailable := readMessageOfType(t, conn, "game.quiz.unavailable")
	body, err := json.Marshal(unavailable.Data)
	if err != nil {
		t.Fatalf("Marshal quiz unavailable failed: %v", err)
	}
	var state QuizUnavailableState
	if err := json.Unmarshal(body, &state); err != nil {
		t.Fatalf("Unmarshal quiz unavailable failed: %v", err)
	}
	if state.Reason != "no_quizzes_remaining" {
		t.Fatalf("unexpected unavailable reason %q", state.Reason)
	}
}

func TestWebsocketSessionStartRestoresPersistedBirds(t *testing.T) {
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
	sessions := &fakeGameSessionStore{
		state: gamesession.State{
			SessionID:    "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			UserID:       "22222222-2222-2222-2222-222222222222",
			LevelID:      "11111111-1111-1111-1111-111111111111",
			GenerationID: "generation-1",
			SubChapterID: "55555555-5555-5555-5555-555555555555",
			Health:       gamesession.InitialHealth,
			Essence:      950,
			Wave:         1,
			Tick:         14,
			StartedAt:    time.Now().UTC(),
			UpdatedAt:    time.Now().UTC(),
		},
		waveStartedAtTick: 1,
		waveSpawned:       2,
		nextWaveTick:      1,
		birds: []gamesession.StoredBird{
			{
				ID:       "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
				Type:     gameobject.BirdTypeSparrow,
				Position: gameobject.Position{X: 1, Y: 1},
				Stats:    gameobject.BirdStats{Damage: 10, ProjectileSpeed: gameobject.StandardProjectileSpeed, FireRate: 1, Range: 3.5, Cost: 50},
			},
		},
		smogs: []gamesession.StoredSmog{
			{
				ID:        "cccccccc-cccc-cccc-cccc-cccccccccccc",
				Health:    20,
				Position:  gameobject.Position{X: 2.5, Y: 1},
				Speed:     1.5,
				PathIndex: 2,
			},
		},
		projectiles: []gamesession.StoredProjectile{
			{
				ID:              "dddddddd-dddd-dddd-dddd-dddddddddddd",
				Type:            gameobject.ProjectileTypeLocked,
				Damage:          10,
				ProjectileSpeed: gameobject.LockedProjectileSpeed,
				Position:        gameobject.Position{X: 1, Y: 1},
				Direction:       gameobject.Vector{X: 1, Y: 0},
				TargetID:        "cccccccc-cccc-cccc-cccc-cccccccccccc",
				RemainingRange:  1,
			},
		},
	}
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

	started := readMessageOfType(t, conn, "game.session.started")
	state := decodeGameState(t, started.Data)
	if state.SessionID != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Fatalf("expected restored session id, got %q", state.SessionID)
	}
	if state.Essence != 950 {
		t.Fatalf("expected restored essence 950, got %d", state.Essence)
	}
	if len(state.Birds) != 1 {
		t.Fatalf("expected one restored bird, got %d", len(state.Birds))
	}
	if state.Birds[0].ID != "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb" {
		t.Fatalf("unexpected restored bird id %q", state.Birds[0].ID)
	}
	if state.Birds[0].Type != gameobject.BirdTypeSparrow {
		t.Fatalf("unexpected restored bird type %q", state.Birds[0].Type)
	}
	if len(state.Smogs) != 1 {
		t.Fatalf("expected one restored smog, got %d", len(state.Smogs))
	}
	if state.Smogs[0].ID != "cccccccc-cccc-cccc-cccc-cccccccccccc" || state.Smogs[0].Health != 20 {
		t.Fatalf("unexpected restored smog %+v", state.Smogs[0])
	}
	if len(state.Projectiles) != 1 {
		t.Fatalf("expected one restored projectile, got %d", len(state.Projectiles))
	}
	if state.Projectiles[0].TargetID != "cccccccc-cccc-cccc-cccc-cccccccccccc" {
		t.Fatalf("unexpected restored projectile %+v", state.Projectiles[0])
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
