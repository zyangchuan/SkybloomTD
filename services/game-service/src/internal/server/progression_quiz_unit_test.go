package server

import (
	"math"
	"testing"
	"time"

	"skybloom/game-service/internal/gameobject"
	"skybloom/game-service/internal/quizcache"
)

func TestScaledEnemyHealthAndSpeed(t *testing.T) {
	tests := []struct {
		name       string
		wave       int
		enemyType  string
		wantHealth int
		wantSpeed  float64
	}{
		{name: "wave one uses base stats", wave: 1, enemyType: gameobject.EnemyTypeSmog, wantHealth: 20, wantSpeed: 0.8},
		{name: "wave five applies quadratic bonus", wave: 5, enemyType: gameobject.EnemyTypeNoise, wantHealth: 102, wantSpeed: 1.96},
		{name: "negative wave clamps to base stats", wave: -1, enemyType: gameobject.EnemyTypeJunk, wantHealth: 500, wantSpeed: 0.3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scaledEnemyHealth(tt.wave, tt.enemyType); got != tt.wantHealth {
				t.Fatalf("expected health %d, got %d", tt.wantHealth, got)
			}
			if got := scaledEnemySpeed(tt.wave, tt.enemyType); math.Abs(got-tt.wantSpeed) > 0.000001 {
				t.Fatalf("expected speed %f, got %f", tt.wantSpeed, got)
			}
		})
	}
}

func TestQuizPromptAvailable(t *testing.T) {
	quiz := quizcache.CachedQuiz{
		ID:               "quiz-1",
		QuizType:         "multiple_choice",
		QuestionMarkdown: "  What is **Go**?  ",
		OptionsMarkdown:  []string{" Language ", " Bird "},
	}

	got := quizPromptState(quiz, 3)

	if got.QuizID != quiz.ID ||
		got.QuizType != quiz.QuizType ||
		got.QuestionMarkdown != quiz.QuestionMarkdown ||
		len(got.OptionsMarkdown) != 2 ||
		got.OptionsMarkdown[0] != quiz.OptionsMarkdown[0] ||
		got.Remaining != 3 {
		t.Fatalf("unexpected quiz prompt state: %#v", got)
	}
}

func TestQuizRequestCooldown(t *testing.T) {
	startedAt := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	if got := quizCooldownRemainingSeconds(time.Time{}, startedAt); got != 0 {
		t.Fatalf("expected no cooldown before first quiz, got %d", got)
	}
	if got := quizCooldownRemainingSeconds(startedAt, startedAt.Add(5*time.Second)); got != 25 {
		t.Fatalf("expected 25 seconds remaining, got %d", got)
	}
	if got := quizCooldownRemainingSeconds(startedAt, startedAt.Add(quizRequestCooldown)); got != 0 {
		t.Fatalf("expected cooldown to expire, got %d", got)
	}
}

func TestQuizAnswerDecodeValidation(t *testing.T) {
	request, err := decodeQuizAnswer(map[string]any{
		"quiz_id":        "11111111-1111-1111-1111-111111111111",
		"selected_index": float64(2),
	})
	if err != nil {
		t.Fatalf("decodeQuizAnswer failed: %v", err)
	}
	if request.QuizID != "11111111-1111-1111-1111-111111111111" || request.SelectedIndex != 2 {
		t.Fatalf("unexpected quiz answer request: %#v", request)
	}

	if _, err := decodeQuizAnswer(map[string]any{
		"quiz_id":        "not-a-uuid",
		"selected_index": float64(0),
	}); err == nil {
		t.Fatalf("expected invalid quiz id to fail")
	}
}
