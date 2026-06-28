package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"skybloom/game-service/internal/config"
	"skybloom/game-service/internal/gameobject"
	"skybloom/game-service/internal/gamesession"
	"skybloom/game-service/internal/generation"
	"skybloom/game-service/internal/mapcache"
	"skybloom/game-service/internal/mapgen"
	"skybloom/game-service/internal/models"
	"skybloom/game-service/internal/quizcache"
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
	if startedState.LoopStarted {
		t.Fatal("expected game loop to wait for game.session.run")
	}
	if sessions.loopStarted {
		t.Fatal("expected loop_started to remain false until game.session.run")
	}
	if len(startedState.BirdTypes) != 8 {
		t.Fatalf("expected 8 bird type infos, got %d", len(startedState.BirdTypes))
	}
	if len(startedState.EnemyTypes) != 3 {
		t.Fatalf("expected 3 enemy type infos, got %d", len(startedState.EnemyTypes))
	}
	if len(startedState.Birds) != 0 {
		t.Fatalf("expected no placed birds at session start, got %d", len(startedState.Birds))
	}
	if len(startedState.Enemies) != 0 {
		t.Fatalf("expected no enemies before first tick, got %d", len(startedState.Enemies))
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

	runGameSession(t, conn)

	var tickState GameState
	for i := 0; i < 10; i++ {
		tick := readMessageOfType(t, conn, "game.state")
		tickState = decodeGameState(t, tick.Data)
		if tickState.Tick > 0 {
			break
		}
	}
	if tickState.Tick != 1 {
		t.Fatalf("expected first tick to be 1, got %d", tickState.Tick)
	}
	if !tickState.LoopStarted || !sessions.loopStarted {
		t.Fatalf("expected loop_started after game.session.run, state=%t persisted=%t", tickState.LoopStarted, sessions.loopStarted)
	}
	if tickState.Health != gamesession.InitialHealth || tickState.Essence != gamesession.InitialEssence || tickState.Wave != 1 {
		t.Fatalf("unexpected tick state %+v", tickState)
	}
	if len(tickState.Enemies) != 1 {
		t.Fatalf("expected first tick to spawn one enemy, got %d", len(tickState.Enemies))
	}
	enemyStats, err := gameobject.EnemyStatsForType(gameobject.EnemyTypeSmog)
	if err != nil {
		t.Fatalf("load enemy stats: %v", err)
	}
	if tickState.Enemies[0].Health != enemyStats.Health {
		t.Fatalf("expected first enemy health %d, got %d", enemyStats.Health, tickState.Enemies[0].Health)
	}
	if tickState.Enemies[0].Speed != enemyStats.Speed {
		t.Fatalf("expected first enemy speed %.2f, got %f", enemyStats.Speed, tickState.Enemies[0].Speed)
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
	runGameSession(t, conn)

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

func TestWebsocketQuizRequestBeforeRunReturnsGameNotStarted(t *testing.T) {
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
					QuestionMarkdown: "Sky is blue?",
					OptionsMarkdown:  []string{"True", "False"},
					AnswerIndex:      0,
				},
			},
		},
	}
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
	if state.Reason != "game_not_started" {
		t.Fatalf("expected game_not_started, got %+v", state)
	}
}

func TestWebsocketPlaceTowerConsumesEssenceAndPersistsBird(t *testing.T) {
	levels := standardLevels()
	maps := smallMap()
	stats, err := gameobject.BirdStatsForType(gameobject.BirdTypeSparrow)
	if err != nil {
		t.Fatalf("load sparrow stats: %v", err)
	}
	startingEssence := stats.Cost + 10
	sessions := quietSession()
	sessions.state.Essence = startingEssence
	handler := NewWithGenerationCachesAndSessions(config.Config{}, levels, maps, nil, nil, nil, nil, sessions).Router()
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
		t.Fatalf("WriteJSON session start failed: %v", err)
	}
	readMessageOfType(t, conn, "game.session.started")

	if err := conn.WriteJSON(Message{
		Type: "game.action.place_tower",
		Data: map[string]any{"bird_type": gameobject.BirdTypeSparrow, "x": 1, "y": 1},
	}); err != nil {
		t.Fatalf("WriteJSON place tower failed: %v", err)
	}

	accepted := readMessageOfType(t, conn, "game.action.accepted")
	body, err := json.Marshal(accepted.Data)
	if err != nil {
		t.Fatalf("Marshal accepted action failed: %v", err)
	}
	var acceptedData struct {
		Action string `json:"action"`
		BirdID string `json:"bird_id"`
		Bird   struct {
			Type     string              `json:"type"`
			Position gameobject.Position `json:"position"`
		} `json:"bird"`
	}
	if err := json.Unmarshal(body, &acceptedData); err != nil {
		t.Fatalf("Unmarshal accepted action failed: %v", err)
	}
	if acceptedData.Action != placeTowerAction {
		t.Fatalf("unexpected accepted action %q", acceptedData.Action)
	}
	if acceptedData.BirdID == "" {
		t.Fatal("expected accepted action to include bird_id")
	}
	if acceptedData.Bird.Type != gameobject.BirdTypeSparrow {
		t.Fatalf("unexpected bird type %q", acceptedData.Bird.Type)
	}
	if acceptedData.Bird.Position.X != 1 || acceptedData.Bird.Position.Y != 1 {
		t.Fatalf("unexpected bird position %+v", acceptedData.Bird.Position)
	}

	if sessions.economy.Essence != startingEssence-stats.Cost {
		t.Fatalf("unexpected persisted essence %d", sessions.economy.Essence)
	}
	if len(sessions.birds) != 1 {
		t.Fatalf("expected one persisted bird, got %d", len(sessions.birds))
	}
	if sessions.birds[0].Type != gameobject.BirdTypeSparrow {
		t.Fatalf("unexpected persisted bird type %q", sessions.birds[0].Type)
	}
}

func TestWebsocketQuizRequestAndCorrectAnswerAwardsEssence(t *testing.T) {
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
					QuestionMarkdown: "Sky is blue?",
					OptionsMarkdown:  []string{"True", "False"},
					AnswerIndex:      0,
				},
			},
		},
	}
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
	runGameSession(t, conn)

	if err := conn.WriteJSON(Message{Type: "game.quiz.request"}); err != nil {
		t.Fatalf("WriteJSON quiz request failed: %v", err)
	}
	prompt := readMessageOfType(t, conn, "game.quiz.presented")
	body, err := json.Marshal(prompt.Data)
	if err != nil {
		t.Fatalf("Marshal quiz prompt failed: %v", err)
	}
	if strings.Contains(string(body), "answer_index") {
		t.Fatalf("quiz prompt leaked answer index: %s", string(body))
	}
	var promptState QuizPromptState
	if err := json.Unmarshal(body, &promptState); err != nil {
		t.Fatalf("Unmarshal quiz prompt failed: %v", err)
	}
	if promptState.QuizID != "66666666-6666-6666-6666-666666666666" {
		t.Fatalf("unexpected quiz id %q", promptState.QuizID)
	}
	if len(promptState.OptionsMarkdown) != 2 {
		t.Fatalf("expected quiz options, got %+v", promptState.OptionsMarkdown)
	}

	if err := conn.WriteJSON(Message{
		Type: "game.quiz.answer",
		Data: map[string]any{"quiz_id": promptState.QuizID, "selected_index": 0},
	}); err != nil {
		t.Fatalf("WriteJSON quiz answer failed: %v", err)
	}
	result := readMessageOfType(t, conn, "game.quiz.result")
	body, err = json.Marshal(result.Data)
	if err != nil {
		t.Fatalf("Marshal quiz result failed: %v", err)
	}
	var resultState QuizResultState
	if err := json.Unmarshal(body, &resultState); err != nil {
		t.Fatalf("Unmarshal quiz result failed: %v", err)
	}
	if !resultState.Correct {
		t.Fatalf("expected correct result, got %+v", resultState)
	}
	if resultState.CorrectIndex != 0 || resultState.CorrectOptionMarkdown != "True" || resultState.SelectedOptionMarkdown != "True" {
		t.Fatalf("unexpected answer details %+v", resultState)
	}
	if resultState.EssenceAwarded != correctQuizEssenceAward {
		t.Fatalf("expected essence award %d, got %d", correctQuizEssenceAward, resultState.EssenceAwarded)
	}
	if resultState.Essence != gamesession.InitialEssence+correctQuizEssenceAward {
		t.Fatalf("expected essence %d, got %d", gamesession.InitialEssence+correctQuizEssenceAward, resultState.Essence)
	}
	if sessions.economy.Essence != gamesession.InitialEssence+correctQuizEssenceAward {
		t.Fatalf("expected persisted essence %d, got %d", gamesession.InitialEssence+correctQuizEssenceAward, sessions.economy.Essence)
	}
	if len(quizzes.quizzes.Quizzes) != 0 {
		t.Fatalf("expected answered quiz to be removed, got %d quizzes", len(quizzes.quizzes.Quizzes))
	}
}

func TestWebsocketQuizRequestServesRandomRemainingQuiz(t *testing.T) {
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
	quizzes := &fakeQuizCache{
		peekRandomIndex: 1,
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
					QuestionMarkdown: "First quiz?",
					OptionsMarkdown:  []string{"True", "False"},
					AnswerIndex:      0,
				},
				{
					ID:               "77777777-7777-7777-7777-777777777777",
					QuizIndex:        1,
					QuizType:         "mcq",
					QuestionMarkdown: "Second quiz?",
					OptionsMarkdown:  []string{"A", "B", "C"},
					AnswerIndex:      2,
				},
			},
		},
	}
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
	runGameSession(t, conn)

	if err := conn.WriteJSON(Message{Type: "game.quiz.request"}); err != nil {
		t.Fatalf("WriteJSON quiz request failed: %v", err)
	}
	prompt := readMessageOfType(t, conn, "game.quiz.presented")
	body, err := json.Marshal(prompt.Data)
	if err != nil {
		t.Fatalf("Marshal quiz prompt failed: %v", err)
	}
	var promptState QuizPromptState
	if err := json.Unmarshal(body, &promptState); err != nil {
		t.Fatalf("Unmarshal quiz prompt failed: %v", err)
	}
	if promptState.QuizID != "77777777-7777-7777-7777-777777777777" {
		t.Fatalf("expected random quiz to be served, got %q", promptState.QuizID)
	}

	if err := conn.WriteJSON(Message{
		Type: "game.quiz.answer",
		Data: map[string]any{"quiz_id": promptState.QuizID, "selected_index": 2},
	}); err != nil {
		t.Fatalf("WriteJSON quiz answer failed: %v", err)
	}
	result := readMessageOfType(t, conn, "game.quiz.result")
	body, err = json.Marshal(result.Data)
	if err != nil {
		t.Fatalf("Marshal quiz result failed: %v", err)
	}
	var resultState QuizResultState
	if err := json.Unmarshal(body, &resultState); err != nil {
		t.Fatalf("Unmarshal quiz result failed: %v", err)
	}
	if !resultState.Correct || resultState.Remaining != 1 {
		t.Fatalf("unexpected quiz result %+v", resultState)
	}
	if len(quizzes.quizzes.Quizzes) != 1 || quizzes.quizzes.Quizzes[0].ID != "66666666-6666-6666-6666-666666666666" {
		t.Fatalf("expected only the served quiz to be removed, got %+v", quizzes.quizzes.Quizzes)
	}
}

func TestWebsocketQuizRequestKeepsUnansweredQuizPending(t *testing.T) {
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
	quizzes := &fakeQuizCache{
		peekRandomIndex: 1,
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
					QuestionMarkdown: "First quiz?",
					OptionsMarkdown:  []string{"True", "False"},
					AnswerIndex:      0,
				},
				{
					ID:               "77777777-7777-7777-7777-777777777777",
					QuizIndex:        1,
					QuizType:         "mcq",
					QuestionMarkdown: "Second quiz?",
					OptionsMarkdown:  []string{"A", "B", "C"},
					AnswerIndex:      2,
				},
			},
		},
	}
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
	runGameSession(t, conn)

	if err := conn.WriteJSON(Message{Type: "game.quiz.request"}); err != nil {
		t.Fatalf("WriteJSON first quiz request failed: %v", err)
	}
	firstPrompt := readMessageOfType(t, conn, "game.quiz.presented")
	firstPromptState := decodeQuizPrompt(t, firstPrompt.Data)

	quizzes.peekRandomIndex = 0
	if err := conn.WriteJSON(Message{Type: "game.quiz.request"}); err != nil {
		t.Fatalf("WriteJSON second quiz request failed: %v", err)
	}
	secondPrompt := readMessageOfType(t, conn, "game.quiz.presented")
	secondPromptState := decodeQuizPrompt(t, secondPrompt.Data)

	if secondPromptState.QuizID != firstPromptState.QuizID {
		t.Fatalf("expected unanswered quiz to stay pending, first=%q second=%q", firstPromptState.QuizID, secondPromptState.QuizID)
	}

	if err := conn.WriteJSON(Message{
		Type: "game.quiz.answer",
		Data: map[string]any{"quiz_id": firstPromptState.QuizID, "selected_index": 2},
	}); err != nil {
		t.Fatalf("WriteJSON quiz answer failed: %v", err)
	}
	readMessageOfType(t, conn, "game.quiz.result")
	if quizzes.quizzes.CurrentQuizID != "" {
		t.Fatalf("expected current quiz to clear after answer, got %q", quizzes.quizzes.CurrentQuizID)
	}
}

func TestWebsocketQuizRequestCooldownRejectsFreshQuizAfterAnswer(t *testing.T) {
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
					QuestionMarkdown: "First quiz?",
					OptionsMarkdown:  []string{"True", "False"},
					AnswerIndex:      0,
				},
				{
					ID:               "77777777-7777-7777-7777-777777777777",
					QuizIndex:        1,
					QuizType:         "mcq",
					QuestionMarkdown: "Second quiz?",
					OptionsMarkdown:  []string{"A", "B", "C"},
					AnswerIndex:      2,
				},
			},
		},
	}
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
	runGameSession(t, conn)

	if err := conn.WriteJSON(Message{Type: "game.quiz.request"}); err != nil {
		t.Fatalf("WriteJSON quiz request failed: %v", err)
	}
	prompt := decodeQuizPrompt(t, readMessageOfType(t, conn, "game.quiz.presented").Data)
	if sessions.lastQuizStartedAt.IsZero() {
		t.Fatal("expected quiz start timestamp to be persisted")
	}

	if err := conn.WriteJSON(Message{
		Type: "game.quiz.answer",
		Data: map[string]any{"quiz_id": prompt.QuizID, "selected_index": 0},
	}); err != nil {
		t.Fatalf("WriteJSON quiz answer failed: %v", err)
	}
	readMessageOfType(t, conn, "game.quiz.result")

	if err := conn.WriteJSON(Message{Type: "game.quiz.request"}); err != nil {
		t.Fatalf("WriteJSON second quiz request failed: %v", err)
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
	if state.Reason != "quiz_cooldown" {
		t.Fatalf("expected quiz_cooldown, got %+v", state)
	}
	if state.RetryAfterSeconds <= 0 || state.RetryAfterSeconds > int(quizRequestCooldown.Seconds()) {
		t.Fatalf("unexpected retry_after_seconds %+v", state)
	}
	if quizzes.quizzes.CurrentQuizID != "" {
		t.Fatalf("cooldown rejection should not start another quiz, got current %q", quizzes.quizzes.CurrentQuizID)
	}

	gameState := decodeGameState(t, readMessageOfType(t, conn, "game.state").Data)
	if gameState.QuizCooldownRemainingSeconds <= 0 || gameState.QuizCooldownRemainingSeconds > int(quizRequestCooldown.Seconds()) {
		t.Fatalf("expected game.state cooldown remaining, got %+v", gameState)
	}
}

func TestWebsocketQuizIncorrectAnswerRecordsMistake(t *testing.T) {
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
	quizzes := &fakeQuizCache{
		quizzes: quizcache.LevelQuizzes{
			GenerationID: "generation-1",
			LevelID:      "11111111-1111-1111-1111-111111111111",
			UserID:       "22222222-2222-2222-2222-222222222222",
			SubChapterID: "55555555-5555-5555-5555-555555555555",
			Quizzes: []quizcache.CachedQuiz{
				{
					ID:               "77777777-7777-7777-7777-777777777777",
					QuizIndex:        2,
					QuizType:         "mcq",
					QuestionMarkdown: "Pick A",
					OptionsMarkdown:  []string{"A", "B", "C"},
					AnswerIndex:      0,
				},
			},
		},
	}
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

	if err := conn.WriteJSON(Message{
		Type: "game.quiz.answer",
		Data: map[string]any{"quiz_id": "77777777-7777-7777-7777-777777777777", "selected_index": 1},
	}); err != nil {
		t.Fatalf("WriteJSON quiz answer failed: %v", err)
	}
	result := readMessageOfType(t, conn, "game.quiz.result")
	body, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatalf("Marshal quiz result failed: %v", err)
	}
	var resultState QuizResultState
	if err := json.Unmarshal(body, &resultState); err != nil {
		t.Fatalf("Unmarshal quiz result failed: %v", err)
	}
	if resultState.Correct {
		t.Fatalf("expected incorrect result, got %+v", resultState)
	}
	if resultState.CorrectIndex != 0 || resultState.CorrectOptionMarkdown != "A" || resultState.SelectedOptionMarkdown != "B" {
		t.Fatalf("unexpected answer details %+v", resultState)
	}
	if len(quizzes.quizzes.Quizzes) != 0 {
		t.Fatalf("expected answered quiz to be removed, got %d quizzes", len(quizzes.quizzes.Quizzes))
	}
	mistakes := waitForMistakes(t, levels, 1)
	if mistakes[0].SelectedIndex != 1 || mistakes[0].QuizID != "77777777-7777-7777-7777-777777777777" {
		t.Fatalf("unexpected recorded mistake %+v", mistakes[0])
	}
}

func TestWebsocketQuizIncorrectAnswerRespondsBeforeMistakeSave(t *testing.T) {
	saveStarted := make(chan struct{})
	continueSave := make(chan struct{})
	levels := &fakeLevelRepository{
		bootstrap: repository.LevelBootstrap{
			LevelID:             "11111111-1111-1111-1111-111111111111",
			UserID:              "22222222-2222-2222-2222-222222222222",
			SubChapterID:        "55555555-5555-5555-5555-555555555555",
			GenerationID:        "generation-1",
			MapSeed:             12345,
			MapAlgorithmVersion: mapgen.Version,
		},
		saveMistakeStarted:  saveStarted,
		saveMistakeContinue: continueSave,
	}
	sessions := &fakeGameSessionStore{}
	quizzes := &fakeQuizCache{
		quizzes: quizcache.LevelQuizzes{
			GenerationID: "generation-1",
			LevelID:      "11111111-1111-1111-1111-111111111111",
			UserID:       "22222222-2222-2222-2222-222222222222",
			SubChapterID: "55555555-5555-5555-5555-555555555555",
			Quizzes: []quizcache.CachedQuiz{
				{
					ID:               "77777777-7777-7777-7777-777777777777",
					QuizIndex:        2,
					QuizType:         "mcq",
					QuestionMarkdown: "Pick A",
					OptionsMarkdown:  []string{"A", "B", "C"},
					AnswerIndex:      0,
				},
			},
		},
	}
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

	if err := conn.WriteJSON(Message{
		Type: "game.quiz.answer",
		Data: map[string]any{"quiz_id": "77777777-7777-7777-7777-777777777777", "selected_index": 1},
	}); err != nil {
		t.Fatalf("WriteJSON quiz answer failed: %v", err)
	}

	result := readMessageOfType(t, conn, "game.quiz.result")
	if result.Type != "game.quiz.result" {
		t.Fatalf("expected quiz result, got %s", result.Type)
	}

	select {
	case <-saveStarted:
	case <-time.After(time.Second):
		t.Fatal("mistake save did not start")
	}

	close(continueSave)
	mistakes := waitForMistakes(t, levels, 1)
	if mistakes[0].SelectedIndex != 1 {
		t.Fatalf("unexpected recorded mistake %+v", mistakes[0])
	}
}

func TestWebsocketGameExitDeletesSession(t *testing.T) {
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
					QuestionMarkdown: "Sky is blue?",
					OptionsMarkdown:  []string{"True", "False"},
					AnswerIndex:      0,
				},
			},
		},
	}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, levels, nil, quizzes, nil, nil, nil, sessions).Router()
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
		t.Fatalf("WriteJSON session start failed: %v", err)
	}
	readMessageOfType(t, conn, "game.session.started")

	if err := conn.WriteJSON(Message{Type: "game.exit"}); err != nil {
		t.Fatalf("WriteJSON game exit failed: %v", err)
	}

	exited := readMessageOfType(t, conn, "game.exited")
	body, err := json.Marshal(exited.Data)
	if err != nil {
		t.Fatalf("Marshal game exited failed: %v", err)
	}
	var exitedState GameExitedState
	if err := json.Unmarshal(body, &exitedState); err != nil {
		t.Fatalf("Unmarshal game exited failed: %v", err)
	}
	if !exitedState.Deleted {
		t.Fatalf("expected exited state to mark deletion, got %+v", exitedState)
	}
	if sessions.deletedSessionID != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Fatalf("expected deleted session id, got %q", sessions.deletedSessionID)
	}
	if quizzes.deletedGenerationID != "generation-1" {
		t.Fatalf("expected deleted quiz generation id, got %q", quizzes.deletedGenerationID)
	}
	if len(quizzes.quizzes.Quizzes) != 0 {
		t.Fatalf("expected quiz cache to be flushed, got %d quizzes", len(quizzes.quizzes.Quizzes))
	}
}

func TestWebsocketGameExitDeletesStoppedSessionByID(t *testing.T) {
	sessions := &fakeGameSessionStore{}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, &fakeLevelRepository{}, nil, nil, nil, nil, nil, sessions).Router()
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
		Type: "game.exit",
		Data: map[string]string{"session_id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"},
	}); err != nil {
		t.Fatalf("WriteJSON game exit failed: %v", err)
	}

	exited := readMessageOfType(t, conn, "game.exited")
	body, err := json.Marshal(exited.Data)
	if err != nil {
		t.Fatalf("Marshal game exited failed: %v", err)
	}
	var exitedState GameExitedState
	if err := json.Unmarshal(body, &exitedState); err != nil {
		t.Fatalf("Unmarshal game exited failed: %v", err)
	}
	if !exitedState.Deleted {
		t.Fatalf("expected exited state to mark deletion, got %+v", exitedState)
	}
	if sessions.deletedSessionID != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Fatalf("expected deleted session id, got %q", sessions.deletedSessionID)
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
				Stats:    gameobject.BirdStats{Damage: 10, ProjectileSpeed: gameobject.StandardProjectileSpeed, FireRate: 1, Range: 2.1, Cost: 50},
			},
		},
		enemies: []gamesession.StoredEnemy{
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
		t.Fatalf("WriteJSON session start failed: %v", err)
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
	if len(state.Enemies) != 1 {
		t.Fatalf("expected one restored enemy, got %d", len(state.Enemies))
	}
	if state.Enemies[0].ID != "cccccccc-cccc-cccc-cccc-cccccccccccc" || state.Enemies[0].Health != 20 {
		t.Fatalf("unexpected restored enemy %+v", state.Enemies[0])
	}
	if len(state.Projectiles) != 0 {
		t.Fatalf("expected obsolete stored projectiles to be ignored, got %d", len(state.Projectiles))
	}
}

func TestAdvanceRuntimeTickDamagesHealthWhenEnemyEscapes(t *testing.T) {
	runtime := runtimeSession{
		session: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			LevelID:   "11111111-1111-1111-1111-111111111111",
			Health:    10,
			Wave:      3,
			Tick:      240,
		},
		economy:           gamesession.NewEconomy(100),
		loopStarted:       true,
		waveStartedAtTick: 120,
		waveSpawned:       36,
		nextWaveTick:      120,
		path: []gameobject.Position{
			{X: 0, Y: 0},
			{X: 1, Y: 0},
		},
		enemies: []gameobject.Enemy{
			{ID: "enemy-1", Health: 10, Position: gameobject.Position{X: 1, Y: 0}, PathIndex: 1},
		},
	}

	events := advanceRuntimeTick(&runtime, time.Now().UTC())

	if runtime.session.Health != 0 {
		t.Fatalf("expected health to drop to 0, got %d", runtime.session.Health)
	}
	if len(runtime.enemies) != 0 {
		t.Fatalf("expected escaped enemy to be removed, got %d", len(runtime.enemies))
	}
	if len(events) == 0 || events[0].Type != "enemy.escaped" {
		t.Fatalf("expected enemy escaped event, got %+v", events)
	}
}

func TestAdvanceRuntimeTickWaitsThreeSecondsAfterWaveCleared(t *testing.T) {
	runtime := runtimeSession{
		session: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			LevelID:   "11111111-1111-1111-1111-111111111111",
			Health:    gamesession.InitialHealth,
			Wave:      1,
			Tick:      10,
		},
		economy:           gamesession.NewEconomy(100),
		loopStarted:       true,
		waveStartedAtTick: 1,
		waveSpawned:       currentWaveCountForTest(t, 1),
		nextWaveTick:      1,
		path: []gameobject.Position{
			{X: 0, Y: 0},
			{X: 1, Y: 0},
		},
	}

	events := advanceRuntimeTick(&runtime, time.Now().UTC())

	if runtime.session.Wave != 1 {
		t.Fatalf("expected wave 1 to remain current while waiting, got %d", runtime.session.Wave)
	}
	if runtime.nextWaveTick != 71 {
		t.Fatalf("expected wave 2 to be scheduled for tick 71, got %d", runtime.nextWaveTick)
	}
	if runtime.waveSpawned != 0 {
		t.Fatalf("expected spawned count reset for next wave, got %d", runtime.waveSpawned)
	}
	if len(runtime.enemies) != 0 {
		t.Fatalf("expected no enemies while waiting, got %d", len(runtime.enemies))
	}
	if len(events) != 1 || events[0].Type != "wave.cleared" {
		t.Fatalf("expected only wave cleared event, got %+v", events)
	}

	runtime.session.Tick = 69
	events = advanceRuntimeTick(&runtime, time.Now().UTC())
	if runtime.session.Wave != 1 || len(runtime.enemies) != 0 {
		t.Fatalf("wave 2 should not start before tick 71: wave=%d enemies=%d events=%+v", runtime.session.Wave, len(runtime.enemies), events)
	}

	events = advanceRuntimeTick(&runtime, time.Now().UTC())
	if runtime.session.Wave != 2 {
		t.Fatalf("expected wave 2 to start after delay, got %d", runtime.session.Wave)
	}
	if len(runtime.enemies) != 1 {
		t.Fatalf("expected wave 2 to spawn one enemy, got %d", len(runtime.enemies))
	}
	if len(events) < 2 || events[0].Type != "wave.started" || events[1].Type != "enemy.spawned" {
		t.Fatalf("expected wave started and enemy spawned events, got %+v", events)
	}
}

func currentWaveCountForTest(t *testing.T, waveNumber int) int {
	t.Helper()
	wave, ok := currentWaveDefinition(waveNumber)
	if !ok {
		t.Fatalf("wave %d not found", waveNumber)
	}
	return wave.Count()
}

func TestAdvanceRuntimeTickSpawnsEnemiesEverySecond(t *testing.T) {
	runtime := runtimeSession{
		session: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			LevelID:   "11111111-1111-1111-1111-111111111111",
			Health:    gamesession.InitialHealth,
		},
		economy:      gamesession.NewEconomy(100),
		loopStarted:  true,
		nextWaveTick: 1,
		path: []gameobject.Position{
			{X: 0, Y: 0},
			{X: 100, Y: 0},
		},
	}

	advanceRuntimeTick(&runtime, time.Now().UTC())
	if len(runtime.enemies) != 1 {
		t.Fatalf("expected first tick to spawn one enemy, got %d", len(runtime.enemies))
	}

	for i := 0; i < 21; i++ {
		advanceRuntimeTick(&runtime, time.Now().UTC())
	}
	if len(runtime.enemies) != 1 {
		t.Fatalf("expected no second enemy before 22 ticks, got %d", len(runtime.enemies))
	}

	advanceRuntimeTick(&runtime, time.Now().UTC())
	if len(runtime.enemies) != 2 {
		t.Fatalf("expected second enemy after 22 ticks, got %d", len(runtime.enemies))
	}
}

func TestEnemySpawnIntervalTicksForWaveReducesByWave(t *testing.T) {
	cases := []struct {
		wave     int
		interval int64
	}{
		{wave: 1, interval: 22},
		{wave: 2, interval: 19},
		{wave: 3, interval: 16},
		{wave: 4, interval: 13},
		{wave: 5, interval: 10},
		{wave: 6, interval: 8},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("wave_%d", tc.wave), func(t *testing.T) {
			if got := enemySpawnIntervalTicksForWave(tc.wave); got != tc.interval {
				t.Fatalf("expected interval %d, got %d", tc.interval, got)
			}
		})
	}
}

func TestAdvanceRuntimeTickSpawnsLaterWavesFaster(t *testing.T) {
	runtime := runtimeSession{
		session: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			LevelID:   "11111111-1111-1111-1111-111111111111",
			Health:    gamesession.InitialHealth,
			Wave:      1,
		},
		economy:      gamesession.NewEconomy(100),
		loopStarted:  true,
		nextWaveTick: 1,
		path: []gameobject.Position{
			{X: 0, Y: 0},
			{X: 100, Y: 0},
		},
	}

	advanceRuntimeTick(&runtime, time.Now().UTC())
	if runtime.session.Wave != 2 || len(runtime.enemies) != 1 {
		t.Fatalf("expected wave 2 first spawn, wave=%d enemies=%d", runtime.session.Wave, len(runtime.enemies))
	}

	for i := 0; i < 18; i++ {
		advanceRuntimeTick(&runtime, time.Now().UTC())
	}
	if len(runtime.enemies) != 1 {
		t.Fatalf("expected no second wave 2 enemy before 19 ticks, got %d", len(runtime.enemies))
	}

	advanceRuntimeTick(&runtime, time.Now().UTC())
	if len(runtime.enemies) != 2 {
		t.Fatalf("expected second wave 2 enemy after 19 ticks, got %d", len(runtime.enemies))
	}
}

func TestWaveDefinitionsMixEnemyTypesByWave(t *testing.T) {
	waves := waveDefinitions()
	type expectedGroup struct {
		enemyType string
		count     int
	}
	expectedGroups := map[int][]expectedGroup{
		1: {
			{enemyType: gameobject.EnemyTypeSmog, count: 20},
		},
		2: {
			{enemyType: gameobject.EnemyTypeSmog, count: 10},
			{enemyType: gameobject.EnemyTypeNoise, count: 10},
			{enemyType: gameobject.EnemyTypeSmog, count: 10},
			{enemyType: gameobject.EnemyTypeNoise, count: 10},
		},
		3: {
			{enemyType: gameobject.EnemyTypeJunk, count: 1},
			{enemyType: gameobject.EnemyTypeSmog, count: 10},
			{enemyType: gameobject.EnemyTypeNoise, count: 10},
			{enemyType: gameobject.EnemyTypeJunk, count: 1},
			{enemyType: gameobject.EnemyTypeSmog, count: 10},
			{enemyType: gameobject.EnemyTypeNoise, count: 10},
			{enemyType: gameobject.EnemyTypeJunk, count: 1},
			{enemyType: gameobject.EnemyTypeSmog, count: 10},
			{enemyType: gameobject.EnemyTypeNoise, count: 10},
		},
		4: {
			{enemyType: gameobject.EnemyTypeJunk, count: 1},
			{enemyType: gameobject.EnemyTypeSmog, count: 15},
			{enemyType: gameobject.EnemyTypeNoise, count: 20},
			{enemyType: gameobject.EnemyTypeJunk, count: 2},
			{enemyType: gameobject.EnemyTypeSmog, count: 15},
			{enemyType: gameobject.EnemyTypeNoise, count: 20},
			{enemyType: gameobject.EnemyTypeJunk, count: 2},
			{enemyType: gameobject.EnemyTypeSmog, count: 15},
			{enemyType: gameobject.EnemyTypeNoise, count: 20},
		},
		5: {
			{enemyType: gameobject.EnemyTypeJunk, count: 3},
			{enemyType: gameobject.EnemyTypeSmog, count: 20},
			{enemyType: gameobject.EnemyTypeNoise, count: 20},
			{enemyType: gameobject.EnemyTypeJunk, count: 3},
			{enemyType: gameobject.EnemyTypeSmog, count: 20},
			{enemyType: gameobject.EnemyTypeNoise, count: 20},
			{enemyType: gameobject.EnemyTypeJunk, count: 3},
			{enemyType: gameobject.EnemyTypeSmog, count: 20},
			{enemyType: gameobject.EnemyTypeNoise, count: 20},
		},
	}

	if len(waves) != len(expectedGroups) {
		t.Fatalf("expected %d waves, got %d", len(expectedGroups), len(waves))
	}
	for i, wave := range waves {
		if wave.Wave != i+1 {
			t.Fatalf("wave at index %d: expected wave number %d, got %d", i, i+1, wave.Wave)
		}
		if wave.Count() <= 0 {
			t.Fatalf("wave %d: expected at least one enemy, got %d", wave.Wave, wave.Count())
		}
		want := expectedGroups[wave.Wave]
		if len(wave.Groups) != len(want) {
			t.Fatalf("wave %d: expected %d groups, got %d", wave.Wave, len(want), len(wave.Groups))
		}
		for j, g := range wave.Groups {
			if g.Type != want[j].enemyType {
				t.Fatalf("wave %d group %d: expected type %q, got %q", wave.Wave, j, want[j].enemyType, g.Type)
			}
			if g.Count != want[j].count {
				t.Fatalf("wave %d group %d: expected count %d, got %d", wave.Wave, j, want[j].count, g.Count)
			}
			if g.Health <= 0 {
				t.Fatalf("wave %d %s: expected positive health, got %d", wave.Wave, g.Type, g.Health)
			}

			if g.Speed <= 0 || g.Speed > 10 {
				t.Fatalf("wave %d %s: speed %.2f outside sane range", wave.Wave, g.Type, g.Speed)
			}
		}
	}

	for _, enemyType := range gameobject.EnemyTypes() {
		if scaledEnemyHealth(2, enemyType) <= scaledEnemyHealth(1, enemyType) {
			t.Fatalf("expected wave 2 %s health to exceed wave 1", enemyType)
		}
		if scaledEnemySpeed(2, enemyType) <= scaledEnemySpeed(1, enemyType) {
			t.Fatalf("expected wave 2 %s speed to exceed wave 1", enemyType)
		}
	}
}

func TestWebsocketSendsVictoryWhenFinalWaveClears(t *testing.T) {
	waves := waveDefinitions()
	finalWave := waves[len(waves)-1]
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
			Essence:      gamesession.InitialEssence,
			Wave:         finalWave.Wave,
			Tick:         200,
			StartedAt:    time.Now().UTC(),
			UpdatedAt:    time.Now().UTC(),
		},
		waveStartedAtTick: 120,
		waveSpawned:       finalWave.Count(),
		nextWaveTick:      120,
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
		t.Fatalf("WriteJSON session start failed: %v", err)
	}
	readMessageOfType(t, conn, "game.session.started")
	runGameSession(t, conn)

	victory := readMessageOfType(t, conn, "game.victory")
	body, err := json.Marshal(victory.Data)
	if err != nil {
		t.Fatalf("Marshal victory failed: %v", err)
	}
	var victoryState GameVictoryState
	if err := json.Unmarshal(body, &victoryState); err != nil {
		t.Fatalf("Unmarshal victory failed: %v", err)
	}
	if victoryState.Reason != "all_waves_cleared" {
		t.Fatalf("unexpected victory reason %q", victoryState.Reason)
	}
	if victoryState.Wave != finalWave.Wave {
		t.Fatalf("expected victory on wave %d, got %d", finalWave.Wave, victoryState.Wave)
	}
}

func TestAdvanceRuntimeTickReportsBirdAttackAndEnemyDamage(t *testing.T) {
	bird, err := gameobject.NewBird("bird-1", gameobject.BirdTypeSparrow, gameobject.Position{X: 0, Y: 0})
	if err != nil {
		t.Fatalf("NewBird failed: %v", err)
	}
	runtime := runtimeSession{
		session: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			LevelID:   "11111111-1111-1111-1111-111111111111",
			Health:    gamesession.InitialHealth,
			Tick:      10,
		},
		economy:     gamesession.NewEconomy(100),
		loopStarted: true,
		birds:       []placedBird{{birdType: gameobject.BirdTypeSparrow, bird: bird}},
		enemies: []gameobject.Enemy{
			{ID: "enemy-1", Health: 30, Position: gameobject.Position{X: 0.1, Y: 0}},
		},
	}

	events := advanceRuntimeTick(&runtime, time.Now().UTC())

	if len(runtime.projectiles) != 0 {
		t.Fatalf("expected immediate attack to create no active projectiles, got %d active projectiles", len(runtime.projectiles))
	}
	if len(runtime.enemies) != 1 || runtime.enemies[0].Health != 10 {
		t.Fatalf("expected enemy health 10, got %+v", runtime.enemies)
	}
	var sawAttack, sawDamage bool
	for _, event := range events {
		if event.Type == "bird.attack" && event.BirdID == "bird-1" && event.EnemyID == "enemy-1" {
			sawAttack = true
		}
		if event.Type == "enemy.damage" && event.EnemyID == "enemy-1" && event.Health == 10 {
			sawDamage = true
		}
	}
	if !sawAttack || !sawDamage {
		t.Fatalf("expected attack and damage events, got %+v", events)
	}
}

func TestAdvanceRuntimeTickSplashAttackDamagesEnemiesInFeatherFan(t *testing.T) {
	bird, err := gameobject.NewBird("bird-1", gameobject.BirdTypePeacock, gameobject.Position{X: 0, Y: 0})
	if err != nil {
		t.Fatalf("NewBird failed: %v", err)
	}
	runtime := runtimeSession{
		session: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			LevelID:   "11111111-1111-1111-1111-111111111111",
			Health:    gamesession.InitialHealth,
			Tick:      10,
		},
		economy:     gamesession.NewEconomy(100),
		loopStarted: true,
		birds:       []placedBird{{birdType: gameobject.BirdTypePeacock, bird: bird}},
		enemies: []gameobject.Enemy{
			{ID: "target", Health: 30, Position: gameobject.Position{X: 1.0, Y: 0}, PathIndex: 2},
			{ID: "side-feather", Health: 30, Position: gameobject.Position{X: 1.93, Y: 0.52}, PathIndex: 1},
			{ID: "gap", Health: 30, Position: gameobject.Position{X: 1.98, Y: 0.26}, PathIndex: 0},
		},
	}

	events := advanceRuntimeTick(&runtime, time.Now().UTC())

	if len(runtime.projectiles) != 0 {
		t.Fatalf("expected splash attack to create no active projectiles, got %d", len(runtime.projectiles))
	}
	if runtime.enemies[0].Health != 10 || runtime.enemies[1].Health != 10 || runtime.enemies[2].Health != 30 {
		t.Fatalf("unexpected enemy health after splash: %+v", runtime.enemies)
	}
	damageEvents := 0
	for _, event := range events {
		if event.Type == "enemy.damage" {
			damageEvents++
		}
	}
	if damageEvents != 2 {
		t.Fatalf("expected two splash damage events, got %+v", events)
	}
}

func TestApplyAttackHitsAwardsEssenceForKilledEnemies(t *testing.T) {
	runtime := runtimeSession{
		session: gamesession.State{
			Essence: 100,
		},
		economy: gamesession.NewEconomy(100),
		enemies: []gameobject.Enemy{
			{ID: "smog", Type: gameobject.EnemyTypeSmog, Health: 5},
			{ID: "noise", Type: gameobject.EnemyTypeNoise, Health: 2},
			{ID: "junk", Type: gameobject.EnemyTypeJunk, Health: 15},
			{ID: "survivor", Type: gameobject.EnemyTypeSmog, Health: 20},
		},
	}

	events := applyAttackHits(&runtime, []gameobject.AttackHit{
		{EnemyID: "smog", Damage: 5},
		{EnemyID: "smog", Damage: 5},
		{EnemyID: "noise", Damage: 2},
		{EnemyID: "junk", Damage: 15},
		{EnemyID: "survivor", Damage: 10},
	})

	if runtime.economy.Essence != 137 || runtime.session.Essence != 137 {
		t.Fatalf("expected essence 137 after kill rewards, got economy=%d session=%d", runtime.economy.Essence, runtime.session.Essence)
	}
	if runtime.enemies[3].Health != 10 {
		t.Fatalf("expected survivor to remain at 10 health, got %+v", runtime.enemies[3])
	}
	if len(events) != 4 {
		t.Fatalf("expected four damage events, got %+v", events)
	}
}

func TestWebsocketPlaceTowerRejectsEnemyPath(t *testing.T) {
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
	maps := &fakeMapCache{
		cached: mapgen.GeneratedMap{
			Version:   mapgen.Version,
			Seed:      99,
			Width:     4,
			Height:    4,
			EnemyPath: []mapgen.PathTile{{X: 1, Y: 1, Kind: "straight"}},
		},
	}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, levels, maps, nil, nil, nil, nil, &fakeGameSessionStore{}).Router()
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
		t.Fatalf("WriteJSON session start failed: %v", err)
	}
	readMessageOfType(t, conn, "game.session.started")

	if err := conn.WriteJSON(Message{
		Type: "game.action.place_tower",
		Data: map[string]any{"bird_type": gameobject.BirdTypeSparrow, "x": 1, "y": 1},
	}); err != nil {
		t.Fatalf("WriteJSON place tower failed: %v", err)
	}

	rejected := readMessageOfType(t, conn, "game.action.rejected")
	body, err := json.Marshal(rejected.Data)
	if err != nil {
		t.Fatalf("Marshal rejected action failed: %v", err)
	}
	var rejectedData struct {
		Action string `json:"action"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(body, &rejectedData); err != nil {
		t.Fatalf("Unmarshal rejected action failed: %v", err)
	}
	if rejectedData.Action != placeTowerAction {
		t.Fatalf("unexpected rejected action %q", rejectedData.Action)
	}
	if rejectedData.Error != "tower cannot be placed on the enemy path" {
		t.Fatalf("unexpected rejection error %q", rejectedData.Error)
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

func TestQuizMistakesEndpointReturnsAuthenticatedLevelMistakes(t *testing.T) {
	levels := &fakeLevelRepository{
		mistakes: []repository.QuizMistakeInput{
			{
				UserID:           "22222222-2222-2222-2222-222222222222",
				LevelID:          "11111111-1111-1111-1111-111111111111",
				GenerationID:     "generation-1",
				QuizID:           "66666666-6666-6666-6666-666666666666",
				QuizIndex:        1,
				QuizType:         "mcq",
				QuestionMarkdown: "Pick A",
				OptionsMarkdown:  []string{"A", "B", "C"},
				AnswerIndex:      0,
				SelectedIndex:    2,
			},
			{
				UserID:           "33333333-3333-3333-3333-333333333333",
				LevelID:          "11111111-1111-1111-1111-111111111111",
				GenerationID:     "generation-2",
				QuizID:           "77777777-7777-7777-7777-777777777777",
				QuizIndex:        2,
				QuizType:         "true_false",
				QuestionMarkdown: "Other user's mistake",
				OptionsMarkdown:  []string{"True", "False"},
				AnswerIndex:      1,
				SelectedIndex:    0,
			},
		},
	}
	handler := New(config.Config{}, levels, &fakeMapCache{}).Router()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/quiz-mistakes?level_id=11111111-1111-1111-1111-111111111111", nil)
	request.Header.Set("X-Authenticated-User-Id", "22222222-2222-2222-2222-222222222222")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", response.Code, response.Body.String())
	}
	var summary QuizMistakeSummaryState
	if err := json.Unmarshal(response.Body.Bytes(), &summary); err != nil {
		t.Fatalf("Unmarshal mistake summary failed: %v", err)
	}
	if summary.LevelID != "11111111-1111-1111-1111-111111111111" || summary.Count != 1 {
		t.Fatalf("unexpected summary %+v", summary)
	}
	if len(summary.Mistakes) != 1 {
		t.Fatalf("expected one mistake, got %d", len(summary.Mistakes))
	}
	mistake := summary.Mistakes[0]
	if mistake.QuizID != "66666666-6666-6666-6666-666666666666" || mistake.SelectedIndex != 2 {
		t.Fatalf("unexpected mistake %+v", mistake)
	}
	if mistake.CorrectOptionMarkdown != "A" || mistake.SelectedOptionMarkdown != "C" {
		t.Fatalf("unexpected option text %+v", mistake)
	}
}

func TestQuizMistakesEndpointValidatesRequest(t *testing.T) {
	handler := New(config.Config{}, &fakeLevelRepository{}, &fakeMapCache{}).Router()

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/quiz-mistakes?level_id=11111111-1111-1111-1111-111111111111", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", unauthorized.Code)
	}

	missingLevel := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/quiz-mistakes", nil)
	request.Header.Set("X-Authenticated-User-Id", "22222222-2222-2222-2222-222222222222")
	handler.ServeHTTP(missingLevel, request)
	if missingLevel.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", missingLevel.Code)
	}
}

type fakeLevelRepository struct {
	bootstrap           repository.LevelBootstrap
	mistakeMu           sync.Mutex
	mistakeStartedOnce  sync.Once
	saveMistakeStarted  chan struct{}
	saveMistakeContinue chan struct{}
	mistakes            []repository.QuizMistakeInput
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

func (r *fakeLevelRepository) SaveQuizMistake(ctx context.Context, input repository.QuizMistakeInput) error {
	if r.saveMistakeStarted != nil {
		r.mistakeStartedOnce.Do(func() {
			close(r.saveMistakeStarted)
		})
	}
	if r.saveMistakeContinue != nil {
		select {
		case <-r.saveMistakeContinue:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	r.mistakeMu.Lock()
	defer r.mistakeMu.Unlock()
	r.mistakes = append(r.mistakes, input)
	return nil
}

func (r *fakeLevelRepository) ListQuizMistakes(_ context.Context, userID string, levelID string) ([]repository.QuizMistakeSummaryItem, error) {
	r.mistakeMu.Lock()
	defer r.mistakeMu.Unlock()

	items := make([]repository.QuizMistakeSummaryItem, 0, len(r.mistakes))
	for _, mistake := range r.mistakes {
		if mistake.UserID != userID || mistake.LevelID != levelID {
			continue
		}
		items = append(items, repository.QuizMistakeSummaryItem{
			ID:               mistake.QuizID,
			UserID:           mistake.UserID,
			LevelID:          mistake.LevelID,
			GenerationID:     mistake.GenerationID,
			QuizID:           mistake.QuizID,
			QuizIndex:        mistake.QuizIndex,
			QuizType:         mistake.QuizType,
			QuestionMarkdown: mistake.QuestionMarkdown,
			OptionsMarkdown:  append([]string(nil), mistake.OptionsMarkdown...),
			AnswerIndex:      mistake.AnswerIndex,
			SelectedIndex:    mistake.SelectedIndex,
		})
	}
	return items, nil
}

func (r *fakeLevelRepository) ClearQuizMistakes(_ context.Context, userID string, levelID string) error {
	r.mistakeMu.Lock()
	defer r.mistakeMu.Unlock()

	filtered := make([]repository.QuizMistakeInput, 0, len(r.mistakes))
	for _, mistake := range r.mistakes {
		if mistake.UserID == userID && mistake.LevelID == levelID {
			continue
		}
		filtered = append(filtered, mistake)
	}
	r.mistakes = filtered
	return nil
}

func (r *fakeLevelRepository) mistakesSnapshot() []repository.QuizMistakeInput {
	r.mistakeMu.Lock()
	defer r.mistakeMu.Unlock()
	return append([]repository.QuizMistakeInput(nil), r.mistakes...)
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

type fakeQuizCache struct {
	quizzes             quizcache.LevelQuizzes
	deletedGenerationID string
	peekRandomIndex     int
}

func (c *fakeQuizCache) Get(_ context.Context, generationID string) (quizcache.LevelQuizzes, error) {
	if c.quizzes.GenerationID != generationID {
		return quizcache.LevelQuizzes{}, quizcache.ErrQuizzesNotFound
	}
	return c.quizzes, nil
}

func (c *fakeQuizCache) Set(_ context.Context, generationID string, quizzes quizcache.LevelQuizzes) error {
	quizzes.GenerationID = generationID
	c.quizzes = quizzes
	return nil
}

func (c *fakeQuizCache) PeekNext(_ context.Context, generationID string) (quizcache.CachedQuiz, int, error) {
	if c.quizzes.GenerationID != generationID || len(c.quizzes.Quizzes) == 0 {
		return quizcache.CachedQuiz{}, 0, quizcache.ErrQuizzesNotFound
	}
	return c.quizzes.Quizzes[0], len(c.quizzes.Quizzes), nil
}

func (c *fakeQuizCache) PeekRandom(_ context.Context, generationID string) (quizcache.CachedQuiz, int, error) {
	if c.quizzes.GenerationID != generationID || len(c.quizzes.Quizzes) == 0 {
		return quizcache.CachedQuiz{}, 0, quizcache.ErrQuizzesNotFound
	}
	for _, quiz := range c.quizzes.Quizzes {
		if quiz.ID == c.quizzes.CurrentQuizID {
			return quiz, len(c.quizzes.Quizzes), nil
		}
	}
	index := c.peekRandomIndex
	if index < 0 || index >= len(c.quizzes.Quizzes) {
		index = 0
	}
	quiz := c.quizzes.Quizzes[index]
	c.quizzes.CurrentQuizID = quiz.ID
	return quiz, len(c.quizzes.Quizzes), nil
}

func (c *fakeQuizCache) Take(_ context.Context, generationID string, quizID string) (quizcache.CachedQuiz, int, error) {
	if c.quizzes.GenerationID != generationID {
		return quizcache.CachedQuiz{}, 0, quizcache.ErrQuizzesNotFound
	}
	for index, quiz := range c.quizzes.Quizzes {
		if quiz.ID != quizID {
			continue
		}
		c.quizzes.Quizzes = append(append([]quizcache.CachedQuiz{}, c.quizzes.Quizzes[:index]...), c.quizzes.Quizzes[index+1:]...)
		if c.quizzes.CurrentQuizID == quizID {
			c.quizzes.CurrentQuizID = ""
		}
		return quiz, len(c.quizzes.Quizzes), nil
	}
	return quizcache.CachedQuiz{}, len(c.quizzes.Quizzes), quizcache.ErrQuizNotFound
}

func (c *fakeQuizCache) Delete(_ context.Context, generationID string) error {
	c.deletedGenerationID = generationID
	if c.quizzes.GenerationID == generationID {
		c.quizzes = quizcache.LevelQuizzes{}
	}
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

func dialGameWebsocket(t *testing.T, serverURL string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/ws"
	header := http.Header{"X-Authenticated-User-Id": []string{"22222222-2222-2222-2222-222222222222"}}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline failed: %v", err)
	}
	return conn
}

func runGameSession(t *testing.T, conn interface {
	WriteJSON(any) error
}) {
	t.Helper()
	if err := conn.WriteJSON(Message{Type: "game.session.run"}); err != nil {
		t.Fatalf("WriteJSON game session run failed: %v", err)
	}
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
	options           gamesession.StartOptions
	state             gamesession.State
	economy           gamesession.Economy
	birds             []gamesession.StoredBird
	enemies           []gamesession.StoredEnemy
	projectiles       []gamesession.StoredProjectile
	waveStartedAtTick int64
	waveSpawned       int
	nextWaveTick      int64
	lastQuizStartedAt time.Time
	loopStarted       bool
	loopPaused        bool
	deletedSessionID  string
}

func (s *fakeGameSessionStore) Start(_ context.Context, options gamesession.StartOptions) (gamesession.State, error) {
	s.options = options
	now := time.Now().UTC()
	if s.state.SessionID != "" {
		return s.state, nil
	}
	s.state = gamesession.State{
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
	}
	s.nextWaveTick = 1
	return s.state, nil
}

func (s *fakeGameSessionStore) LoadRuntimeState(_ context.Context, _ string) (gamesession.RuntimeState, error) {
	return gamesession.RuntimeState{
		GenerationID:      s.state.GenerationID,
		Health:            s.state.Health,
		Essence:           s.state.Essence,
		Wave:              s.state.Wave,
		Tick:              s.state.Tick,
		LoopStarted:       s.loopStarted,
		LoopPaused:        s.loopPaused,
		WaveStartedAtTick: s.waveStartedAtTick,
		WaveSpawned:       s.waveSpawned,
		NextWaveTick:      s.nextWaveTick,
		LastQuizStartedAt: s.lastQuizStartedAt,
		Birds:             append([]gamesession.StoredBird{}, s.birds...),
		Enemies:           append([]gamesession.StoredEnemy{}, s.enemies...),
		Projectiles:       append([]gamesession.StoredProjectile{}, s.projectiles...),
	}, nil
}

func (s *fakeGameSessionStore) SaveRuntimeState(_ context.Context, _ string, runtime gamesession.RuntimeState) error {
	s.economy = gamesession.NewEconomy(runtime.Essence)
	s.birds = append([]gamesession.StoredBird{}, runtime.Birds...)
	s.enemies = append([]gamesession.StoredEnemy{}, runtime.Enemies...)
	s.projectiles = append([]gamesession.StoredProjectile{}, runtime.Projectiles...)
	s.state.Health = runtime.Health
	s.state.Essence = runtime.Essence
	s.state.Wave = runtime.Wave
	s.state.Tick = runtime.Tick
	s.loopStarted = runtime.LoopStarted
	s.loopPaused = runtime.LoopPaused
	s.waveStartedAtTick = runtime.WaveStartedAtTick
	s.waveSpawned = runtime.WaveSpawned
	s.nextWaveTick = runtime.NextWaveTick
	s.lastQuizStartedAt = runtime.LastQuizStartedAt
	return nil
}

func (s *fakeGameSessionStore) Delete(_ context.Context, sessionID string) error {
	s.deletedSessionID = sessionID
	s.state = gamesession.State{}
	s.economy = gamesession.Economy{}
	s.birds = nil
	s.enemies = nil
	s.projectiles = nil
	s.waveStartedAtTick = 0
	s.waveSpawned = 0
	s.nextWaveTick = 0
	return nil
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

func decodeQuizPrompt(t *testing.T, data any) QuizPromptState {
	t.Helper()
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Marshal quiz prompt failed: %v", err)
	}
	var state QuizPromptState
	if err := json.Unmarshal(body, &state); err != nil {
		t.Fatalf("Unmarshal quiz prompt failed: %v", err)
	}
	return state
}

func readMessageOfType(t *testing.T, conn *websocket.Conn, messageType string) Message {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	seen := make([]string, 0, 10)
	for time.Now().Before(deadline) {
		if err := conn.SetReadDeadline(deadline); err != nil {
			t.Fatalf("SetReadDeadline failed: %v", err)
		}
		var message Message
		if err := conn.ReadJSON(&message); err != nil {
			t.Fatalf("ReadJSON failed looking for %s after seeing %v: %v", messageType, seen, err)
		}
		if message.Type == messageType {
			return message
		}
		if len(seen) < cap(seen) {
			seen = append(seen, message.Type)
		}
	}
	t.Fatalf("did not receive message type %s after seeing %v", messageType, seen)
	return Message{}
}

func waitForMistakes(t *testing.T, levels *fakeLevelRepository, want int) []repository.QuizMistakeInput {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mistakes := levels.mistakesSnapshot()
		if len(mistakes) == want {
			return mistakes
		}
		time.Sleep(10 * time.Millisecond)
	}
	mistakes := levels.mistakesSnapshot()
	t.Fatalf("expected %d recorded mistakes, got %d", want, len(mistakes))
	return nil
}
