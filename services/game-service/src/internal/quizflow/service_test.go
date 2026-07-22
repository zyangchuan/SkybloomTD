package quizflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"skybloom/game-service/internal/quizcache"
)

type fakeStore struct {
	quizzes quizcache.LevelQuizzes
	err     error
	takenID string
}

func (s *fakeStore) Get(context.Context, string) (quizcache.LevelQuizzes, error) {
	if s.err != nil {
		return quizcache.LevelQuizzes{}, s.err
	}
	return s.quizzes, nil
}

func (s *fakeStore) PeekRandom(context.Context, string) (quizcache.CachedQuiz, int, error) {
	if s.err != nil {
		return quizcache.CachedQuiz{}, 0, s.err
	}
	if len(s.quizzes.Quizzes) == 0 {
		return quizcache.CachedQuiz{}, 0, quizcache.ErrQuizzesNotFound
	}
	quiz := s.quizzes.Quizzes[0]
	s.quizzes.CurrentQuizID = quiz.ID
	return quiz, len(s.quizzes.Quizzes), nil
}

func (s *fakeStore) Take(_ context.Context, _ string, quizID string) (quizcache.CachedQuiz, int, error) {
	if s.err != nil {
		return quizcache.CachedQuiz{}, 0, s.err
	}
	for index, quiz := range s.quizzes.Quizzes {
		if quiz.ID != quizID {
			continue
		}
		s.takenID = quizID
		s.quizzes.Quizzes = append(s.quizzes.Quizzes[:index], s.quizzes.Quizzes[index+1:]...)
		if s.quizzes.CurrentQuizID == quizID {
			s.quizzes.CurrentQuizID = ""
		}
		return quiz, len(s.quizzes.Quizzes), nil
	}
	return quizcache.CachedQuiz{}, len(s.quizzes.Quizzes), quizcache.ErrQuizNotFound
}

func TestPresentUsesCurrentQuizWithoutCooldown(t *testing.T) {
	store := &fakeStore{quizzes: quizcache.LevelQuizzes{
		CurrentQuizID: "quiz-2",
		Quizzes: []quizcache.CachedQuiz{
			quiz("quiz-1", 0),
			quiz("quiz-2", 1),
		},
	}}
	service := NewService(store, 20*time.Second)

	result, err := service.Present(context.Background(), PresentRequest{
		GenerationID:      "generation-1",
		LastQuizStartedAt: time.Now().UTC(),
		Now:               time.Now().UTC(),
		RequireCooldown:   true,
	})
	if err != nil {
		t.Fatalf("Present failed: %v", err)
	}
	if result.Prompt.QuizID != "quiz-2" || result.CurrentQuizID != "quiz-2" || result.Remaining != 2 {
		t.Fatalf("unexpected current quiz result: %#v", result)
	}
}

func TestPresentPrefersRequestCurrentQuiz(t *testing.T) {
	store := &fakeStore{quizzes: quizcache.LevelQuizzes{
		CurrentQuizID: "quiz-2",
		Quizzes: []quizcache.CachedQuiz{
			quiz("quiz-1", 0),
			quiz("quiz-2", 1),
		},
	}}
	service := NewService(store, 20*time.Second)

	result, err := service.Present(context.Background(), PresentRequest{
		GenerationID:  "generation-1",
		CurrentQuizID: "quiz-1",
	})
	if err != nil {
		t.Fatalf("Present failed: %v", err)
	}
	if result.Prompt.QuizID != "quiz-1" || result.CurrentQuizID != "quiz-1" {
		t.Fatalf("unexpected request current quiz result: %#v", result)
	}
}

func TestPresentReturnsCooldownBeforeFreshQuiz(t *testing.T) {
	startedAt := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{quizzes: quizcache.LevelQuizzes{Quizzes: []quizcache.CachedQuiz{quiz("quiz-1", 0)}}}
	service := NewService(store, 20*time.Second)

	result, err := service.Present(context.Background(), PresentRequest{
		GenerationID:      "generation-1",
		LastQuizStartedAt: startedAt,
		Now:               startedAt.Add(5 * time.Second),
		RequireCooldown:   true,
	})
	if err != nil {
		t.Fatalf("Present failed: %v", err)
	}
	if result.Unavailable.Reason != UnavailableQuizCooldown || result.Unavailable.RetryAfterSeconds != 15 {
		t.Fatalf("unexpected cooldown result: %#v", result)
	}
}

func TestAnswerValidatesCurrentQuizAndRemovesAnsweredQuiz(t *testing.T) {
	store := &fakeStore{quizzes: quizcache.LevelQuizzes{
		CurrentQuizID: "quiz-1",
		Quizzes:       []quizcache.CachedQuiz{quiz("quiz-1", 1), quiz("quiz-2", 0)},
	}}
	service := NewService(store, 20*time.Second)

	result, err := service.Answer(context.Background(), AnswerRequest{
		GenerationID:  "generation-1",
		CurrentQuizID: "quiz-1",
		QuizID:        "quiz-1",
		SelectedIndex: 1,
	})
	if err != nil {
		t.Fatalf("Answer failed: %v", err)
	}
	if !result.Correct || result.Remaining != 1 || store.takenID != "quiz-1" {
		t.Fatalf("unexpected answer result: %#v taken=%s", result, store.takenID)
	}
}

func TestAnswerRejectsNonCurrentQuiz(t *testing.T) {
	store := &fakeStore{quizzes: quizcache.LevelQuizzes{
		CurrentQuizID: "quiz-1",
		Quizzes:       []quizcache.CachedQuiz{quiz("quiz-1", 1), quiz("quiz-2", 0)},
	}}
	service := NewService(store, 20*time.Second)

	_, err := service.Answer(context.Background(), AnswerRequest{
		GenerationID:  "generation-1",
		CurrentQuizID: "quiz-1",
		QuizID:        "quiz-2",
		SelectedIndex: 0,
	})
	if err == nil {
		t.Fatalf("expected non-current quiz to fail")
	}
}

func TestPresentMapsMissingCacheToUnavailable(t *testing.T) {
	service := NewService(&fakeStore{err: quizcache.ErrQuizzesNotFound}, 20*time.Second)

	result, err := service.Present(context.Background(), PresentRequest{GenerationID: "generation-1"})
	if err != nil {
		t.Fatalf("Present failed: %v", err)
	}
	if result.Unavailable.Reason != UnavailableNoQuizzesRemaining {
		t.Fatalf("unexpected missing cache result: %#v", result)
	}
}

func TestPresentReturnsLoadErrors(t *testing.T) {
	service := NewService(&fakeStore{err: errors.New("redis down")}, 20*time.Second)

	if _, err := service.Present(context.Background(), PresentRequest{GenerationID: "generation-1"}); err == nil {
		t.Fatalf("expected load error")
	}
}

func quiz(id string, answerIndex int) quizcache.CachedQuiz {
	return quizcache.CachedQuiz{
		ID:               id,
		QuizType:         "multiple_choice",
		QuestionMarkdown: "Question?",
		OptionsMarkdown:  []string{"A", "B"},
		AnswerIndex:      answerIndex,
	}
}
