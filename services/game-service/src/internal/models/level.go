package models

import (
	"errors"
	"time"
)

var ErrLevelNotFound = errors.New("level not found")

type Level struct {
	ID                  string     `gorm:"type:uuid;primaryKey"`
	UserID              *string    `gorm:"type:uuid;index"`
	DocumentID          string     `gorm:"type:uuid;not null;index"`
	ChapterID           string     `gorm:"type:uuid;not null"`
	SubChapterID        string     `gorm:"type:uuid;not null;index"`
	GenerationID        *string    `gorm:"type:text;index"`
	MapSeed             *int64     `gorm:"type:bigint"`
	MapAlgorithmVersion *int       `gorm:"column:map_algorithm_version"`
	SummaryMarkdown     string     `gorm:"type:text;not null"`
	SourceChunkIDs      []byte     `gorm:"type:jsonb;not null;default:'[]'::jsonb"`
	SourceMetadata      []byte     `gorm:"type:jsonb;not null;default:'{}'::jsonb"`
	Model               string     `gorm:"type:text"`
	CreatedAt           *time.Time `gorm:"default:CURRENT_TIMESTAMP"`
}

func (Level) TableName() string {
	return "levels"
}

type Quiz struct {
	ID               string     `gorm:"type:uuid;primaryKey"`
	LevelID          string     `gorm:"type:uuid;not null;index;uniqueIndex:quizzes_level_id_quiz_index_key"`
	QuizIndex        int        `gorm:"not null;uniqueIndex:quizzes_level_id_quiz_index_key"`
	QuizType         string     `gorm:"type:text;not null;check:quizzes_quiz_type_check,quiz_type IN ('mcq','true_false')"`
	QuestionMarkdown string     `gorm:"type:text;not null"`
	OptionsMarkdown  []byte     `gorm:"type:jsonb;not null"`
	AnswerIndex      int        `gorm:"not null;check:quizzes_answer_index_nonnegative_check,answer_index >= 0"`
	CreatedAt        *time.Time `gorm:"default:CURRENT_TIMESTAMP"`
}

func (Quiz) TableName() string {
	return "quizzes"
}

type QuizMistake struct {
	ID               string     `gorm:"type:uuid;primaryKey"`
	UserID           string     `gorm:"type:uuid;not null;index"`
	LevelID          string     `gorm:"type:uuid;not null;index"`
	GenerationID     string     `gorm:"type:text;not null;index"`
	QuizID           string     `gorm:"type:uuid;not null;index"`
	QuizIndex        int        `gorm:"not null"`
	QuizType         string     `gorm:"type:text;not null"`
	QuestionMarkdown string     `gorm:"type:text;not null"`
	OptionsMarkdown  []byte     `gorm:"type:jsonb;not null"`
	AnswerIndex      int        `gorm:"not null"`
	SelectedIndex    int        `gorm:"not null"`
	CreatedAt        *time.Time `gorm:"default:CURRENT_TIMESTAMP"`
}

func (QuizMistake) TableName() string {
	return "quiz_mistakes"
}

type ReusableLevel struct {
	LevelID                string
	UserID                 string
	DocumentID             string
	SubChapterID           string
	GenerationID           *string
	MapSeed                *int64
	MapAlgorithmVersion    *int
	GenerationRecordExists bool
	QuizCount              int
}

func (l ReusableLevel) HasPlayableMap() bool {
	return l.GenerationID != nil &&
		*l.GenerationID != "" &&
		l.MapSeed != nil &&
		l.MapAlgorithmVersion != nil &&
		l.GenerationRecordExists
}
