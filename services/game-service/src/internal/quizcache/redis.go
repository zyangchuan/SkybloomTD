package quizcache

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"skybloom/game-service/internal/quiztext"
	"skybloom/game-service/internal/redisclient"
	"skybloom/game-service/internal/repository"
)

var (
	ErrQuizzesNotFound = errors.New("level quizzes not found")
	ErrQuizNotFound    = errors.New("quiz not found")
)

//go:embed lua/release_refill_lease.lua
var releaseRefillLeaseScript string

//go:embed lua/append_quizzes.lua
var appendQuizzesScript string

//go:embed lua/peek_random.lua
var peekRandomScript string

//go:embed lua/take_quiz.lua
var takeQuizScript string

type Store struct {
	client *redisclient.Client
	ttl    time.Duration
}

type LevelQuizzes struct {
	GenerationID  string       `json:"generation_id"`
	LevelID       string       `json:"level_id"`
	UserID        string       `json:"user_id"`
	SubChapterID  string       `json:"sub_chapter_id"`
	CurrentQuizID string       `json:"current_quiz_id,omitempty"`
	Quizzes       []CachedQuiz `json:"quizzes"`
}

type CachedQuiz struct {
	ID               string   `json:"id"`
	QuizIndex        int      `json:"quiz_index"`
	QuizType         string   `json:"quiz_type"`
	QuestionMarkdown string   `json:"question_markdown"`
	OptionsMarkdown  []string `json:"options_markdown"`
	AnswerIndex      int      `json:"answer_index"`
}

type cachedQuizScriptResult struct {
	Found     bool       `json:"found"`
	Quiz      CachedQuiz `json:"quiz"`
	Remaining int        `json:"remaining"`
}

func New(redisURL string, ttl time.Duration) (*Store, error) {
	if strings.TrimSpace(redisURL) == "" {
		return nil, nil
	}
	client, err := redisclient.New(redisURL)
	if err != nil {
		return nil, err
	}
	return &Store{
		client: client,
		ttl:    ttl,
	}, nil
}

func FromLevelBootstrap(level repository.LevelBootstrap, generationID string) LevelQuizzes {
	quizzes := make([]CachedQuiz, 0, len(level.Quizzes))
	for _, quiz := range level.Quizzes {
		quizzes = append(quizzes, CachedQuiz{
			ID:               quiz.ID,
			QuizIndex:        quiz.QuizIndex,
			QuizType:         quiz.QuizType,
			QuestionMarkdown: quiztext.SanitizeMarkdown(quiz.QuestionMarkdown),
			OptionsMarkdown:  quiztext.SanitizeMarkdownSlice(quiz.OptionsMarkdown),
			AnswerIndex:      quiz.AnswerIndex,
		})
	}
	return LevelQuizzes{
		GenerationID: generationID,
		LevelID:      level.LevelID,
		UserID:       level.UserID,
		SubChapterID: level.SubChapterID,
		Quizzes:      quizzes,
	}
}

func FromRepositoryQuizzes(quizzes []repository.QuizItem) []CachedQuiz {
	cached := make([]CachedQuiz, 0, len(quizzes))
	for _, quiz := range quizzes {
		cached = append(cached, CachedQuiz{
			ID:               quiz.ID,
			QuizIndex:        quiz.QuizIndex,
			QuizType:         quiz.QuizType,
			QuestionMarkdown: quiztext.SanitizeMarkdown(quiz.QuestionMarkdown),
			OptionsMarkdown:  quiztext.SanitizeMarkdownSlice(quiz.OptionsMarkdown),
			AnswerIndex:      quiz.AnswerIndex,
		})
	}
	return cached
}

func (s *Store) Get(ctx context.Context, generationID string) (LevelQuizzes, error) {
	_ = ctx
	if s == nil || s.client == nil {
		return LevelQuizzes{}, errors.New("quiz cache is not configured")
	}
	raw, err := s.client.Do("GET", key(generationID))
	if errors.Is(err, redisclient.ErrNil) {
		return LevelQuizzes{}, ErrQuizzesNotFound
	}
	if err != nil {
		return LevelQuizzes{}, err
	}

	body, ok := raw.(string)
	if !ok {
		return LevelQuizzes{}, fmt.Errorf("unexpected redis quiz payload type %T", raw)
	}

	var quizzes LevelQuizzes
	if err := json.Unmarshal([]byte(body), &quizzes); err != nil {
		return LevelQuizzes{}, err
	}
	sanitizeLevelQuizzes(&quizzes)
	return quizzes, nil
}

func (s *Store) Set(ctx context.Context, generationID string, quizzes LevelQuizzes) error {
	_ = ctx
	quizzes.GenerationID = generationID
	sanitizeLevelQuizzes(&quizzes)
	if s == nil || s.client == nil {
		return errors.New("quiz cache is not configured")
	}
	body, err := json.Marshal(quizzes)
	if err != nil {
		return err
	}
	_, err = s.client.Do("SET", key(generationID), string(body), "EX", strconv.Itoa(int(s.ttl.Seconds())))
	return err
}

func (s *Store) Append(ctx context.Context, generationID string, quizzes []CachedQuiz) (int, error) {
	_ = ctx
	if s == nil || s.client == nil {
		return 0, errors.New("quiz cache is not configured")
	}
	body, err := json.Marshal(quizzes)
	if err != nil {
		return 0, err
	}
	raw, err := s.client.Do("EVAL", appendQuizzesScript, "1", key(generationID), string(body), strconv.Itoa(int(s.ttl.Seconds())))
	if errors.Is(err, redisclient.ErrNil) {
		return 0, ErrQuizzesNotFound
	}
	if err != nil {
		return 0, err
	}
	return intValue(raw, 0), nil
}

func (s *Store) AcquireRefillLease(ctx context.Context, generationID string, leaseValue string, ttl time.Duration) (bool, error) {
	_ = ctx
	if s == nil || s.client == nil {
		return false, errors.New("quiz cache is not configured")
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	raw, err := s.client.Do("SET", refillLeaseKey(generationID), leaseValue, "NX", "EX", strconv.Itoa(int(ttl.Seconds())))
	if errors.Is(err, redisclient.ErrNil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.EqualFold(fmt.Sprint(raw), "OK"), nil
}

func (s *Store) ReleaseRefillLease(ctx context.Context, generationID string, leaseValue string) error {
	_ = ctx
	if s == nil || s.client == nil {
		return errors.New("quiz cache is not configured")
	}
	_, err := s.client.Do("EVAL", releaseRefillLeaseScript, "1", refillLeaseKey(generationID), leaseValue)
	return err
}

func (s *Store) PeekNext(ctx context.Context, generationID string) (CachedQuiz, int, error) {
	quizzes, err := s.Get(ctx, generationID)
	if err != nil {
		return CachedQuiz{}, 0, err
	}
	if len(quizzes.Quizzes) == 0 {
		return CachedQuiz{}, 0, ErrQuizzesNotFound
	}
	return quizzes.Quizzes[0], len(quizzes.Quizzes), nil
}

func (s *Store) PeekRandom(ctx context.Context, generationID string) (CachedQuiz, int, error) {
	_ = ctx
	if s == nil || s.client == nil {
		return CachedQuiz{}, 0, errors.New("quiz cache is not configured")
	}
	raw, err := s.client.Do("EVAL", peekRandomScript, "1", key(generationID), strconv.Itoa(int(s.ttl.Seconds())))
	if errors.Is(err, redisclient.ErrNil) {
		return CachedQuiz{}, 0, ErrQuizzesNotFound
	}
	if err != nil {
		return CachedQuiz{}, 0, err
	}
	return parseCachedQuizScriptResult(raw)
}

func (s *Store) Take(ctx context.Context, generationID string, quizID string) (CachedQuiz, int, error) {
	_ = ctx
	if s == nil || s.client == nil {
		return CachedQuiz{}, 0, errors.New("quiz cache is not configured")
	}
	raw, err := s.client.Do("EVAL", takeQuizScript, "1", key(generationID), quizID, strconv.Itoa(int(s.ttl.Seconds())))
	if errors.Is(err, redisclient.ErrNil) {
		return CachedQuiz{}, 0, ErrQuizzesNotFound
	}
	if err != nil {
		return CachedQuiz{}, 0, err
	}
	quiz, remaining, err := parseCachedQuizScriptResult(raw)
	if errors.Is(err, ErrQuizNotFound) {
		return CachedQuiz{}, remaining, err
	}
	return quiz, remaining, err
}

func (s *Store) Delete(ctx context.Context, generationID string) error {
	_ = ctx
	if s == nil || s.client == nil {
		return errors.New("quiz cache is not configured")
	}
	_, err := s.client.Do("DEL", key(generationID))
	return err
}

func (s *Store) Close() error {
	if s.client == nil {
		return nil
	}
	return s.client.Close()
}

func key(generationID string) string {
	return fmt.Sprintf("level-quizzes:v1:generation:%s", generationID)
}

func refillLeaseKey(generationID string) string {
	return fmt.Sprintf("level-quizzes:v1:generation:%s:refill-lease", generationID)
}

func parseCachedQuizScriptResult(raw any) (CachedQuiz, int, error) {
	body, ok := raw.(string)
	if !ok || strings.TrimSpace(body) == "" {
		return CachedQuiz{}, 0, fmt.Errorf("unexpected redis quiz script payload type %T", raw)
	}
	var result cachedQuizScriptResult
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return CachedQuiz{}, 0, err
	}
	if !result.Found {
		return CachedQuiz{}, result.Remaining, ErrQuizNotFound
	}
	sanitizeCachedQuiz(&result.Quiz)
	return result.Quiz, result.Remaining, nil
}

func intValue(raw any, fallback int) int {
	switch value := raw.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case string:
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func sanitizeCachedQuiz(quiz *CachedQuiz) {
	quiz.QuestionMarkdown = quiztext.SanitizeMarkdown(quiz.QuestionMarkdown)
	quiz.OptionsMarkdown = quiztext.SanitizeMarkdownSlice(quiz.OptionsMarkdown)
}

func cachedQuizByID(quizzes []CachedQuiz, quizID string) (CachedQuiz, bool) {
	for _, quiz := range quizzes {
		if quiz.ID == quizID {
			sanitizeCachedQuiz(&quiz)
			return quiz, true
		}
	}
	return CachedQuiz{}, false
}

func sanitizeLevelQuizzes(quizzes *LevelQuizzes) {
	for index := range quizzes.Quizzes {
		sanitizeCachedQuiz(&quizzes.Quizzes[index])
	}
}
