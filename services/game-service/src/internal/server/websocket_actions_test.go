package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"skybloom/game-service/internal/config"
	"skybloom/game-service/internal/gameobject"
	"skybloom/game-service/internal/gamesession"
	"skybloom/game-service/internal/mapgen"
	"skybloom/game-service/internal/quizcache"
	"skybloom/game-service/internal/repository"
)

// ---------------------------------------------------------------------------
// Helpers shared across tests in this file
// ---------------------------------------------------------------------------

// standardLevels returns a bootstrap with a level the fakeGameSessionStore expects.
func standardLevels() *fakeLevelRepository {
	return &fakeLevelRepository{
		bootstrap: repository.LevelBootstrap{
			LevelID:             "11111111-1111-1111-1111-111111111111",
			UserID:              "22222222-2222-2222-2222-222222222222",
			SubChapterID:        "55555555-5555-5555-5555-555555555555",
			GenerationID:        "generation-1",
			MapSeed:             12345,
			MapAlgorithmVersion: mapgen.Version,
		},
	}
}

// smallMap returns a 4×4 map with the enemy path on column 0 so towers can be
// placed safely at any (x≥1, y) position without hitting path or bounds issues.
func smallMap() *fakeMapCache {
	return &fakeMapCache{
		cached: mapgen.GeneratedMap{
			Version: mapgen.Version,
			Seed:    12345,
			Width:   4,
			Height:  4,
			EnemyPath: []mapgen.PathTile{
				{X: 0, Y: 0, Kind: "start"},
				{X: 0, Y: 1, Kind: "straight"},
				{X: 0, Y: 2, Kind: "straight"},
				{X: 0, Y: 3, Kind: "end"},
			},
		},
	}
}

// quietSession returns a store configured with a session that has no active waves
// and sufficient essence, so the game loop runs without spawning enemies during tests.
func quietSession() *fakeGameSessionStore {
	return &fakeGameSessionStore{
		state: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			UserID:    "22222222-2222-2222-2222-222222222222",
			LevelID:   "11111111-1111-1111-1111-111111111111",
			Health:    gamesession.InitialHealth,
			Essence:   gamesession.InitialEssence,
		},
		nextWaveTick: 100000, // far enough that no smogs spawn during the test
	}
}

// startSession sends game.session.start and waits for game.session.started.
func startSession(t *testing.T, conn interface {
	WriteJSON(any) error
}) {
	t.Helper()
	if err := conn.WriteJSON(Message{
		Type: "game.session.start",
		Data: map[string]string{"level_id": "11111111-1111-1111-1111-111111111111"},
	}); err != nil {
		t.Fatalf("WriteJSON session start failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Ping / unknown message
// ---------------------------------------------------------------------------

func TestWebsocketPingReturnsPong(t *testing.T) {
	handler := New(config.Config{}, standardLevels(), &fakeMapCache{}).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	if err := conn.WriteJSON(Message{Type: "ping"}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	msg := readMessageOfType(t, conn, "pong")
	if msg.Type != "pong" {
		t.Fatalf("expected pong, got %q", msg.Type)
	}
}

func TestWebsocketUnknownMessageTypeReturnsError(t *testing.T) {
	handler := New(config.Config{}, standardLevels(), &fakeMapCache{}).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	if err := conn.WriteJSON(Message{Type: "game.does_not_exist"}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	errMsg := readMessageOfType(t, conn, "error")
	body, _ := json.Marshal(errMsg.Data)
	if !jsonContains(body, "unsupported message type") {
		t.Fatalf("expected 'unsupported message type' in error, got: %s", body)
	}
}

// ---------------------------------------------------------------------------
// Place tower edge cases
// ---------------------------------------------------------------------------

func TestWebsocketPlaceTowerNoSessionReturnsRejection(t *testing.T) {
	handler := New(config.Config{}, standardLevels(), &fakeMapCache{}).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	// Send place_tower without starting a session first.
	if err := conn.WriteJSON(Message{
		Type: "game.action.place_tower",
		Data: map[string]any{"bird_type": gameobject.BirdTypeSparrow, "x": 1, "y": 1},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	rejected := readMessageOfType(t, conn, "game.action.rejected")
	assertRejection(t, rejected, placeTowerAction, "game session is not running")
}

func TestWebsocketPlaceTowerUnknownBirdTypeReturnsRejection(t *testing.T) {
	sessions := quietSession()
	handler := NewWithGenerationCachesAndSessions(config.Config{}, standardLevels(), smallMap(), nil, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	startSession(t, conn)
	readMessageOfType(t, conn, "game.session.started")

	if err := conn.WriteJSON(Message{
		Type: "game.action.place_tower",
		Data: map[string]any{"bird_type": "fire_dragon", "x": 2, "y": 2},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	rejected := readMessageOfType(t, conn, "game.action.rejected")
	assertRejection(t, rejected, placeTowerAction, "unknown bird type")
}

func TestWebsocketPlaceTowerOutsideBoundsReturnsRejection(t *testing.T) {
	sessions := quietSession()
	handler := NewWithGenerationCachesAndSessions(config.Config{}, standardLevels(), smallMap(), nil, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	startSession(t, conn)
	readMessageOfType(t, conn, "game.session.started")

	// (10, 10) is outside the 4×4 map.
	if err := conn.WriteJSON(Message{
		Type: "game.action.place_tower",
		Data: map[string]any{"bird_type": gameobject.BirdTypeSparrow, "x": 10, "y": 10},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	rejected := readMessageOfType(t, conn, "game.action.rejected")
	assertRejection(t, rejected, placeTowerAction, "tower position is outside the map")
}

func TestWebsocketPlaceTowerOccupiedCellReturnsRejection(t *testing.T) {
	sessions := &fakeGameSessionStore{
		state: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			UserID:    "22222222-2222-2222-2222-222222222222",
			LevelID:   "11111111-1111-1111-1111-111111111111",
			Health:    gamesession.InitialHealth,
			Essence:   gamesession.InitialEssence,
		},
		birds: []gamesession.StoredBird{
			{
				ID:       "existing-bird",
				Type:     gameobject.BirdTypeSparrow,
				Position: gameobject.Position{X: 2, Y: 2},
				Stats: gameobject.BirdStats{
					Damage:          10,
					ProjectileSpeed: gameobject.StandardProjectileSpeed,
					FireRate:        1,
					Range:           3.5,
					Cost:            50,
				},
			},
		},
		nextWaveTick: 100000,
	}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, standardLevels(), smallMap(), nil, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	startSession(t, conn)
	readMessageOfType(t, conn, "game.session.started")

	// Try to place a second bird at the already-occupied (2,2).
	if err := conn.WriteJSON(Message{
		Type: "game.action.place_tower",
		Data: map[string]any{"bird_type": gameobject.BirdTypeSparrow, "x": 2, "y": 2},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	rejected := readMessageOfType(t, conn, "game.action.rejected")
	assertRejection(t, rejected, placeTowerAction, "tower position is occupied")
}

func TestWebsocketPlaceTowerInsufficientEssenceReturnsRejection(t *testing.T) {
	// Sparrow costs 50 essence; set up the session with only 10.
	sessions := &fakeGameSessionStore{
		state: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			UserID:    "22222222-2222-2222-2222-222222222222",
			LevelID:   "11111111-1111-1111-1111-111111111111",
			Health:    gamesession.InitialHealth,
			Essence:   10, // less than Sparrow cost (50)
		},
		nextWaveTick: 100000,
	}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, standardLevels(), smallMap(), nil, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	startSession(t, conn)
	readMessageOfType(t, conn, "game.session.started")

	if err := conn.WriteJSON(Message{
		Type: "game.action.place_tower",
		Data: map[string]any{"bird_type": gameobject.BirdTypeSparrow, "x": 2, "y": 2},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	rejected := readMessageOfType(t, conn, "game.action.rejected")
	assertRejection(t, rejected, placeTowerAction, "insufficient essence")
}

// ---------------------------------------------------------------------------
// Quiz edge cases
// ---------------------------------------------------------------------------

func TestWebsocketQuizRequestWithoutSessionReturnsError(t *testing.T) {
	handler := New(config.Config{}, standardLevels(), &fakeMapCache{}).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	if err := conn.WriteJSON(Message{Type: "game.quiz.request"}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	errMsg := readMessageOfType(t, conn, "error")
	body, _ := json.Marshal(errMsg.Data)
	if !jsonContains(body, "game session is not running") {
		t.Fatalf("expected 'game session is not running' error, got: %s", body)
	}
}

func TestWebsocketQuizAnswerWithoutSessionReturnsError(t *testing.T) {
	handler := New(config.Config{}, standardLevels(), &fakeMapCache{}).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	if err := conn.WriteJSON(Message{
		Type: "game.quiz.answer",
		Data: map[string]any{"quiz_id": "66666666-6666-6666-6666-666666666666", "selected_index": 0},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	errMsg := readMessageOfType(t, conn, "error")
	body, _ := json.Marshal(errMsg.Data)
	if !jsonContains(body, "game session is not running") {
		t.Fatalf("expected 'game session is not running' error, got: %s", body)
	}
}

func TestWebsocketQuizAnswerWrongQuizIDReturnsError(t *testing.T) {
	// Use a fresh session (no pre-configured SessionID) so Start() populates
	// session.GenerationID from the level bootstrap options. loop.generationID
	// is derived from session.GenerationID, which must match the quiz cache.
	sessions := &fakeGameSessionStore{}
	quizzes := &fakeQuizCache{
		quizzes: quizcache.LevelQuizzes{
			GenerationID: "generation-1",
			LevelID:      "11111111-1111-1111-1111-111111111111",
			UserID:       "22222222-2222-2222-2222-222222222222",
			SubChapterID: "55555555-5555-5555-5555-555555555555",
			Quizzes: []quizcache.CachedQuiz{
				{
					ID:               "66666666-6666-6666-6666-666666666666",
					QuizIndex:        0,
					QuizType:         "true_false",
					QuestionMarkdown: "The sky is blue?",
					OptionsMarkdown:  []string{"True", "False"},
					AnswerIndex:      0,
				},
			},
		},
	}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, standardLevels(), nil, quizzes, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	startSession(t, conn)
	readMessageOfType(t, conn, "game.session.started")

	// Send the correct UUID format but a different ID than the current quiz.
	if err := conn.WriteJSON(Message{
		Type: "game.quiz.answer",
		Data: map[string]any{
			"quiz_id":        "77777777-7777-7777-7777-777777777777", // wrong ID
			"selected_index": 0,
		},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	errMsg := readMessageOfType(t, conn, "error")
	body, _ := json.Marshal(errMsg.Data)
	if !jsonContains(body, "quiz_id is not the current quiz") {
		t.Fatalf("expected 'quiz_id is not the current quiz', got: %s", body)
	}
}

func TestWebsocketQuizAnswerOutOfRangeIndexReturnsError(t *testing.T) {
	sessions := &fakeGameSessionStore{}
	quizzes := &fakeQuizCache{
		quizzes: quizcache.LevelQuizzes{
			GenerationID: "generation-1",
			LevelID:      "11111111-1111-1111-1111-111111111111",
			UserID:       "22222222-2222-2222-2222-222222222222",
			SubChapterID: "55555555-5555-5555-5555-555555555555",
			Quizzes: []quizcache.CachedQuiz{
				{
					ID:               "66666666-6666-6666-6666-666666666666",
					QuizIndex:        0,
					QuizType:         "true_false",
					QuestionMarkdown: "The sky is blue?",
					OptionsMarkdown:  []string{"True", "False"}, // 2 options: valid range [0,1]
					AnswerIndex:      0,
				},
			},
		},
	}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, standardLevels(), nil, quizzes, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	startSession(t, conn)
	readMessageOfType(t, conn, "game.session.started")

	// selected_index=5 is beyond the 2 available options.
	if err := conn.WriteJSON(Message{
		Type: "game.quiz.answer",
		Data: map[string]any{
			"quiz_id":        "66666666-6666-6666-6666-666666666666",
			"selected_index": 5,
		},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	errMsg := readMessageOfType(t, conn, "error")
	body, _ := json.Marshal(errMsg.Data)
	if !jsonContains(body, "selected_index is out of range") {
		t.Fatalf("expected 'selected_index is out of range', got: %s", body)
	}
}

// ---------------------------------------------------------------------------
// Unit tests: advanceRuntimeTick / game loop functions
// ---------------------------------------------------------------------------

func TestAdvanceRuntimeTickMultipleSmogsEscapeHealthClampsAtZero(t *testing.T) {
	// baseHealthDamage=10; start at Health=5 so the first escape clamps to 0
	// and subsequent escapes keep it at 0 (not negative).
	path := []gameobject.Position{{X: 0, Y: 0}, {X: 1, Y: 0}}
	runtime := runtimeSession{
		session: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			Health:    5, // less than baseHealthDamage(10)
		},
		economy:     gamesession.NewEconomy(100),
		loopStarted: true,
		path:        path,
		// Three smogs at the very end of the path — all escape on the next tick.
		smogs: []gameobject.Smog{
			{ID: "smog-1", Health: 30, Speed: 0, Position: gameobject.Position{X: 1, Y: 0}, PathIndex: 1},
			{ID: "smog-2", Health: 30, Speed: 0, Position: gameobject.Position{X: 1, Y: 0}, PathIndex: 1},
			{ID: "smog-3", Health: 30, Speed: 0, Position: gameobject.Position{X: 1, Y: 0}, PathIndex: 1},
		},
	}

	events := advanceRuntimeTick(&runtime, time.Now())

	if runtime.session.Health != 0 {
		t.Fatalf("expected health clamped to 0, got %d", runtime.session.Health)
	}
	if len(runtime.smogs) != 0 {
		t.Fatalf("expected all escaped smogs removed, got %d", len(runtime.smogs))
	}
	escapedCount := 0
	for _, e := range events {
		if e.Type == "smog.escaped" {
			escapedCount++
		}
	}
	if escapedCount != 3 {
		t.Fatalf("expected 3 smog.escaped events, got %d", escapedCount)
	}
}

func TestFireBirdsReturnsNilWhenHealthIsZero(t *testing.T) {
	bird, err := gameobject.NewBird("bird-1", gameobject.BirdTypeSparrow, gameobject.Position{X: 0, Y: 0})
	if err != nil {
		t.Fatalf("NewBird failed: %v", err)
	}
	runtime := runtimeSession{
		session: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			Health:    0, // game over
			Tick:      5,
		},
		economy:     gamesession.NewEconomy(100),
		loopStarted: true,
		birds:       []placedBird{{birdType: gameobject.BirdTypeSparrow, bird: bird}},
		smogs: []gameobject.Smog{
			{ID: "smog-1", Health: 30, Position: gameobject.Position{X: 0.2, Y: 0}},
		},
	}

	events := fireBirds(&runtime)

	if len(events) != 0 {
		t.Fatalf("expected no bird attack events when health is 0, got %d", len(events))
	}
}

func TestTargetSmogIndexPrioritizesHighestPathIndex(t *testing.T) {
	// Sparrow range=3.5; bird is at origin. All three smogs are within range.
	// The one with the highest PathIndex should be selected.
	bird, err := gameobject.NewBird("bird-1", gameobject.BirdTypeSparrow, gameobject.Position{X: 0, Y: 0})
	if err != nil {
		t.Fatalf("NewBird failed: %v", err)
	}

	smogs := []gameobject.Smog{
		{ID: "near-start", Health: 10, Position: gameobject.Position{X: 1, Y: 0}, PathIndex: 1},
		{ID: "far-ahead", Health: 10, Position: gameobject.Position{X: 2, Y: 0}, PathIndex: 5},
		{ID: "closest", Health: 10, Position: gameobject.Position{X: 0.5, Y: 0}, PathIndex: 0},
	}

	idx := targetSmogIndex(bird, smogs)

	if idx < 0 {
		t.Fatal("expected a valid target index, got -1 (no target found)")
	}
	if smogs[idx].ID != "far-ahead" {
		t.Fatalf("expected target 'far-ahead' (PathIndex=5), got %q (PathIndex=%d)",
			smogs[idx].ID, smogs[idx].PathIndex)
	}
}

func TestTargetSmogIndexReturnsNegativeOneWhenNoSmogsInRange(t *testing.T) {
	bird, err := gameobject.NewBird("bird-1", gameobject.BirdTypeSparrow, gameobject.Position{X: 0, Y: 0})
	if err != nil {
		t.Fatalf("NewBird failed: %v", err)
	}

	// All smogs are far beyond the Sparrow's 3.5 range.
	smogs := []gameobject.Smog{
		{ID: "too-far", Health: 10, Position: gameobject.Position{X: 10, Y: 0}, PathIndex: 9},
	}

	idx := targetSmogIndex(bird, smogs)
	if idx != -1 {
		t.Fatalf("expected -1 when no smogs in range, got %d", idx)
	}
}

func TestGameWonReturnsFalseWhenHealthIsZero(t *testing.T) {
	// Even if all waves are cleared, a dead game is not a victory.
	runtime := runtimeSession{
		session:     gamesession.State{Health: 0, Wave: len(waveDefinitions())},
		waveSpawned: 100, // more than enough
		nextWaveTick: 0,
	}

	if gameWon(runtime) {
		t.Fatal("expected gameWon false when health is 0")
	}
}

func TestGameWonReturnsFalseWhenSmogsAreStillAlive(t *testing.T) {
	runtime := runtimeSession{
		session:     gamesession.State{Health: gamesession.InitialHealth, Wave: len(waveDefinitions())},
		waveSpawned: 100,
		nextWaveTick: 0,
		smogs:       []gameobject.Smog{{ID: "last-one", Health: 1}},
	}

	if gameWon(runtime) {
		t.Fatal("expected gameWon false when smogs are still alive")
	}
}

func TestGameWonReturnsTrueWhenAllConditionsMet(t *testing.T) {
	waves := waveDefinitions()
	finalWave := waves[len(waves)-1]

	runtime := runtimeSession{
		session:      gamesession.State{Health: gamesession.InitialHealth, Wave: len(waves)},
		waveSpawned:  finalWave.Count,
		nextWaveTick: 0,
		smogs:        nil,
	}

	if !gameWon(runtime) {
		t.Fatal("expected gameWon true when all conditions are met")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// assertRejection verifies a game.action.rejected message has the expected
// action name and error string.
func assertRejection(t *testing.T, msg Message, wantAction, wantError string) {
	t.Helper()
	body, err := json.Marshal(msg.Data)
	if err != nil {
		t.Fatalf("Marshal rejected data failed: %v", err)
	}
	var data struct {
		Action string `json:"action"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		t.Fatalf("Unmarshal rejected data failed: %v", err)
	}
	if data.Action != wantAction {
		t.Fatalf("expected action %q, got %q", wantAction, data.Action)
	}
	if data.Error != wantError {
		t.Fatalf("expected error %q, got %q", wantError, data.Error)
	}
}

// jsonContains reports whether the raw JSON bytes contain the given substring.
func jsonContains(body []byte, substr string) bool {
	return strings.Contains(string(body), substr)
}
