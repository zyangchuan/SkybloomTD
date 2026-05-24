package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type Level struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey"`
	UserID          *uuid.UUID     `gorm:"type:uuid;index"`
	DocumentID      uuid.UUID      `gorm:"type:uuid;not null;index"`
	ChapterID       uuid.UUID      `gorm:"type:uuid;not null"`
	SubChapterID    uuid.UUID      `gorm:"type:uuid;not null;index"`
	SummaryMarkdown string         `gorm:"type:text;not null"`
	SourceChunkIDs  datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'::jsonb"`
	SourceMetadata  datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'::jsonb"`
	Model           string         `gorm:"type:text"`
	CreatedAt       time.Time      `gorm:"default:CURRENT_TIMESTAMP"`
}

func (Level) TableName() string {
	return "levels"
}

type Quiz struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey"`
	LevelID          uuid.UUID      `gorm:"type:uuid;not null;index;uniqueIndex:quizzes_level_id_quiz_index_key"`
	Level            *Level         `gorm:"constraint:OnDelete:CASCADE;"`
	QuizIndex        int            `gorm:"not null;uniqueIndex:quizzes_level_id_quiz_index_key"`
	QuizType         string         `gorm:"type:text;not null;check:quizzes_quiz_type_check,quiz_type IN ('mcq','true_false')"`
	QuestionMarkdown string         `gorm:"type:text;not null"`
	OptionsMarkdown  datatypes.JSON `gorm:"type:jsonb;not null"`
	AnswerIndex      int            `gorm:"not null;check:quizzes_answer_index_nonnegative_check,answer_index >= 0"`
	CreatedAt        time.Time      `gorm:"default:CURRENT_TIMESTAMP"`
}

func (Quiz) TableName() string {
	return "quizzes"
}
