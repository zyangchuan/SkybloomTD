package quizcache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"skybloom/game-service/internal/redisclient"
	"skybloom/game-service/internal/repository"
)

var ErrQuizzesNotFound = errors.New("level quizzes not found")

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
			QuestionMarkdown: quiz.QuestionMarkdown,
			OptionsMarkdown:  quiz.OptionsMarkdown,
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
	return quizzes, nil
}

func (s *Store) Set(ctx context.Context, generationID string, quizzes LevelQuizzes) error {
	_ = ctx
	quizzes.GenerationID = generationID
	body, err := json.Marshal(quizzes)
	if err != nil {
		return err
	}
	_, err = s.client.Do("SET", key(generationID), string(body), "EX", strconv.Itoa(int(s.ttl.Seconds())))
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
