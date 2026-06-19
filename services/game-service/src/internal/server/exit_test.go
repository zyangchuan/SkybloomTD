package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"skybloom/game-service/internal/config"
	"skybloom/game-service/internal/mapgen"
	"skybloom/game-service/internal/quizcache"
	"skybloom/game-service/internal/repository"
)

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
