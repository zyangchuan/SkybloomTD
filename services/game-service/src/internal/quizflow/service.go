package quizflow

import (
	"context"
	"errors"
	"math"
	"time"

	"skybloom/game-service/internal/quizcache"
	"skybloom/game-service/internal/quiztext"
)

const (
	UnavailableGameNotStarted     = "game_not_started"
	UnavailableNoQuizzesRemaining = "no_quizzes_remaining"
	UnavailableQuizCooldown       = "quiz_cooldown"
)

type Store interface {
	Get(ctx context.Context, generationID string) (quizcache.LevelQuizzes, error)
	PeekRandom(ctx context.Context, generationID string) (quizcache.CachedQuiz, int, error)
	Take(ctx context.Context, generationID string, quizID string) (quizcache.CachedQuiz, int, error)
}

type Service struct {
	store    Store
	cooldown time.Duration
}

type Prompt struct {
	QuizID           string   `json:"quiz_id"`
	QuizType         string   `json:"quiz_type"`
	QuestionMarkdown string   `json:"question_markdown"`
	OptionsMarkdown  []string `json:"options_markdown"`
	Remaining        int      `json:"remaining"`
}

type Unavailable struct {
	Reason            string `json:"reason"`
	RetryAfterSeconds int    `json:"retry_after_seconds,omitempty"`
}

type PresentRequest struct {
	GenerationID      string
	CurrentQuizID     string
	LastQuizStartedAt time.Time
	Now               time.Time
	RequireCooldown   bool
}

type PresentResult struct {
	Prompt        Prompt
	Unavailable   Unavailable
	CurrentQuizID string
	Remaining     int
}

type AnswerRequest struct {
	GenerationID  string
	CurrentQuizID string
	QuizID        string
	SelectedIndex int
}

type AnswerResult struct {
	Quiz                   quizcache.CachedQuiz
	Correct                bool
	SelectedIndex          int
	CorrectIndex           int
	SelectedOptionMarkdown string
	CorrectOptionMarkdown  string
	Remaining              int
	Unavailable            Unavailable
}

func NewService(store Store, cooldown time.Duration) *Service {
	return &Service{store: store, cooldown: cooldown}
}

func (s *Service) Present(ctx context.Context, request PresentRequest) (PresentResult, error) {
	if s == nil || s.store == nil {
		return PresentResult{}, errors.New("quiz cache is not configured")
	}
	now := request.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}

	quizzes, err := s.store.Get(ctx, request.GenerationID)
	if errors.Is(err, quizcache.ErrQuizzesNotFound) {
		return PresentResult{Unavailable: Unavailable{Reason: UnavailableNoQuizzesRemaining}}, nil
	}
	if err != nil {
		return PresentResult{}, errors.New("failed to load quiz")
	}
	if len(quizzes.Quizzes) == 0 {
		return PresentResult{Unavailable: Unavailable{Reason: UnavailableNoQuizzesRemaining}}, nil
	}

	if quiz, ok := cachedQuizByID(quizzes.Quizzes, request.CurrentQuizID); ok {
		return PresentResult{
			Prompt:        PromptState(quiz, len(quizzes.Quizzes)),
			CurrentQuizID: quiz.ID,
			Remaining:     len(quizzes.Quizzes),
		}, nil
	}

	if quiz, ok := cachedQuizByID(quizzes.Quizzes, quizzes.CurrentQuizID); ok {
		return PresentResult{
			Prompt:        PromptState(quiz, len(quizzes.Quizzes)),
			CurrentQuizID: quiz.ID,
			Remaining:     len(quizzes.Quizzes),
		}, nil
	}

	if request.RequireCooldown {
		if retryAfter := CooldownRemainingSeconds(s.cooldown, request.LastQuizStartedAt, now); retryAfter > 0 {
			return PresentResult{
				Unavailable: Unavailable{
					Reason:            UnavailableQuizCooldown,
					RetryAfterSeconds: retryAfter,
				},
			}, nil
		}
	}

	quiz, remaining, err := s.store.PeekRandom(ctx, request.GenerationID)
	if errors.Is(err, quizcache.ErrQuizzesNotFound) {
		return PresentResult{Unavailable: Unavailable{Reason: UnavailableNoQuizzesRemaining}}, nil
	}
	if err != nil {
		return PresentResult{}, errors.New("failed to load quiz")
	}
	return PresentResult{
		Prompt:        PromptState(quiz, remaining),
		CurrentQuizID: quiz.ID,
		Remaining:     remaining,
	}, nil
}

func (s *Service) Answer(ctx context.Context, request AnswerRequest) (AnswerResult, error) {
	if s == nil || s.store == nil {
		return AnswerResult{}, errors.New("quiz cache is not configured")
	}
	quizzes, err := s.store.Get(ctx, request.GenerationID)
	if errors.Is(err, quizcache.ErrQuizzesNotFound) {
		return AnswerResult{Unavailable: Unavailable{Reason: UnavailableNoQuizzesRemaining}}, nil
	}
	if err != nil {
		return AnswerResult{}, errors.New("failed to load quiz")
	}

	expectedQuizID := request.CurrentQuizID
	if expectedQuizID == "" {
		expectedQuizID = quizzes.CurrentQuizID
	}
	if expectedQuizID == "" && len(quizzes.Quizzes) > 0 {
		expectedQuizID = quizzes.Quizzes[0].ID
	}
	if expectedQuizID != "" && expectedQuizID != request.QuizID {
		return AnswerResult{}, errors.New("quiz_id is not the current quiz")
	}

	currentQuiz, ok := cachedQuizByID(quizzes.Quizzes, request.QuizID)
	if !ok {
		return AnswerResult{}, errors.New("quiz not found")
	}
	if request.SelectedIndex < 0 || request.SelectedIndex >= len(currentQuiz.OptionsMarkdown) {
		return AnswerResult{}, errors.New("selected_index is out of range")
	}

	answeredQuiz, remaining, err := s.store.Take(ctx, request.GenerationID, request.QuizID)
	if errors.Is(err, quizcache.ErrQuizNotFound) || errors.Is(err, quizcache.ErrQuizzesNotFound) {
		return AnswerResult{}, errors.New("quiz not found")
	}
	if err != nil {
		return AnswerResult{}, errors.New("failed to remove answered quiz")
	}

	return AnswerResult{
		Quiz:                   answeredQuiz,
		Correct:                request.SelectedIndex == answeredQuiz.AnswerIndex,
		SelectedIndex:          request.SelectedIndex,
		CorrectIndex:           answeredQuiz.AnswerIndex,
		SelectedOptionMarkdown: optionMarkdown(answeredQuiz.OptionsMarkdown, request.SelectedIndex),
		CorrectOptionMarkdown:  optionMarkdown(answeredQuiz.OptionsMarkdown, answeredQuiz.AnswerIndex),
		Remaining:              remaining,
	}, nil
}

func PromptState(quiz quizcache.CachedQuiz, remaining int) Prompt {
	return Prompt{
		QuizID:           quiz.ID,
		QuizType:         quiz.QuizType,
		QuestionMarkdown: quiztext.SanitizeMarkdown(quiz.QuestionMarkdown),
		OptionsMarkdown:  quiztext.SanitizeMarkdownSlice(quiz.OptionsMarkdown),
		Remaining:        remaining,
	}
}

func CooldownRemainingSeconds(cooldown time.Duration, lastQuizStartedAt time.Time, now time.Time) int {
	if cooldown <= 0 || lastQuizStartedAt.IsZero() {
		return 0
	}
	remaining := cooldown - now.UTC().Sub(lastQuizStartedAt.UTC())
	if remaining <= 0 {
		return 0
	}
	return int(math.Ceil(remaining.Seconds()))
}

func cachedQuizByID(quizzes []quizcache.CachedQuiz, quizID string) (quizcache.CachedQuiz, bool) {
	for _, quiz := range quizzes {
		if quiz.ID == quizID {
			return quiz, true
		}
	}
	return quizcache.CachedQuiz{}, false
}

func optionMarkdown(options []string, index int) string {
	if index < 0 || index >= len(options) {
		return ""
	}
	return quiztext.SanitizeMarkdown(options[index])
}
