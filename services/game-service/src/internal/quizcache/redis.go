package quizcache

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
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

type Store struct {
	client *redisclient.Client
	ttl    time.Duration
}

type LevelQuizzes struct {
	GenerationID string       `json:"generation_id"`
	LevelID      string       `json:"level_id"`
	UserID       string       `json:"user_id"`
	SubChapterID string       `json:"sub_chapter_id"`
	Quizzes      []CachedQuiz `json:"quizzes"`
}

type CachedQuiz struct {
	ID               string   `json:"id"`
	QuizIndex        int      `json:"quiz_index"`
	QuizType         string   `json:"quiz_type"`
	QuestionMarkdown string   `json:"question_markdown"`
	OptionsMarkdown  []string `json:"options_markdown"`
	AnswerIndex      int      `json:"answer_index"`
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
	quizzes, err := s.Get(ctx, generationID)
	if err != nil {
		return CachedQuiz{}, 0, err
	}
	if len(quizzes.Quizzes) == 0 {
		return CachedQuiz{}, 0, ErrQuizzesNotFound
	}
	index, err := randomIndex(len(quizzes.Quizzes))
	if err != nil {
		return CachedQuiz{}, 0, err
	}
	return quizzes.Quizzes[index], len(quizzes.Quizzes), nil
}

func (s *Store) Take(ctx context.Context, generationID string, quizID string) (CachedQuiz, int, error) {
	quizzes, err := s.Get(ctx, generationID)
	if err != nil {
		return CachedQuiz{}, 0, err
	}
	for index, quiz := range quizzes.Quizzes {
		if quiz.ID != quizID {
			continue
		}
		remaining := append([]CachedQuiz{}, quizzes.Quizzes[:index]...)
		remaining = append(remaining, quizzes.Quizzes[index+1:]...)
		quizzes.Quizzes = remaining
		if err := s.Set(ctx, generationID, quizzes); err != nil {
			return CachedQuiz{}, 0, err
		}
		return quiz, len(remaining), nil
	}
	return CachedQuiz{}, len(quizzes.Quizzes), ErrQuizNotFound
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

func randomIndex(length int) (int, error) {
	if length <= 0 {
		return 0, ErrQuizzesNotFound
	}
	value, err := rand.Int(rand.Reader, big.NewInt(int64(length)))
	if err != nil {
		return 0, err
	}
	return int(value.Int64()), nil
}

func sanitizeLevelQuizzes(quizzes *LevelQuizzes) {
	for index := range quizzes.Quizzes {
		quiz := &quizzes.Quizzes[index]
		quiz.QuestionMarkdown = quiztext.SanitizeMarkdown(quiz.QuestionMarkdown)
		quiz.OptionsMarkdown = quiztext.SanitizeMarkdownSlice(quiz.OptionsMarkdown)
	}
}
