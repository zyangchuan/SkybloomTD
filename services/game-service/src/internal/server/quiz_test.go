package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"skybloom/game-service/internal/config"
	"skybloom/game-service/internal/gamesession"
	"skybloom/game-service/internal/mapgen"
	"skybloom/game-service/internal/quizcache"
	"skybloom/game-service/internal/repository"
)

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
