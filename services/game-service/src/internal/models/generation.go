package models

import (
	"errors"
	"time"
)

const (
	JobTypeMapGenerate  = "map.generate"
	JobTypeQuizGenerate = "quiz.generate"

	GenerationStatusPending  = "pending"
	GenerationStatusComplete = "complete"
	GenerationStatusFailed   = "failed"

	StepStatusPending  = "pending"
	StepStatusRunning  = "running"
	StepStatusComplete = "complete"
	StepStatusFailed   = "failed"
)

var (
	ErrGenerationNotFound = errors.New("level generation not found")
	ErrStatusNotFound     = errors.New("level generation status not found")
)

type LevelJob struct {
	JobType             string `json:"job_type"`
	TaskID              string `json:"task_id"`
	GenerationID        string `json:"generation_id"`
	UserID              string `json:"user_id"`
	SubChapterID        string `json:"sub_chapter_id"`
	MapSeed             int64  `json:"map_seed"`
	MapAlgorithmVersion int    `json:"map_algorithm_version"`
}

type LevelGenerationRecord struct {
	ID                  string    `gorm:"type:text;primaryKey" json:"generation_id"`
	IdempotencyKey      string    `gorm:"type:text;uniqueIndex" json:"-"`
	UserID              string    `gorm:"type:text;not null;index" json:"user_id"`
	SubChapterID        string    `gorm:"type:text;not null;index" json:"sub_chapter_id"`
	MapSeed             int64     `gorm:"type:bigint;not null" json:"map_seed"`
	MapAlgorithmVersion int       `gorm:"not null" json:"map_algorithm_version"`
	LevelID             *string   `gorm:"type:text;index" json:"level_id,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (LevelGenerationRecord) TableName() string {
	return "level_generation_jobs"
}

type GenerationStatus struct {
	GenerationID        string    `json:"generation_id"`
	UserID              string    `json:"user_id"`
	SubChapterID        string    `json:"sub_chapter_id"`
	Status              string    `json:"status"`
	MapStatus           string    `json:"map_status"`
	QuizStatus          string    `json:"quiz_status"`
	MapSeed             int64     `json:"map_seed"`
	MapAlgorithmVersion int       `json:"map_algorithm_version"`
	LevelID             *string   `json:"level_id,omitempty"`
	Error               *string   `json:"error,omitempty"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type StatusUpdate struct {
	LevelID             *string
	MapSeed             *int64
	MapAlgorithmVersion *int
	Error               *string
}
