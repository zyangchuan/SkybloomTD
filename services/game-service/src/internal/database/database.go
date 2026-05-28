package database

import (
	"context"
	"errors"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"skybloom/game-service/internal/models"
)

func Open(ctx context.Context, databaseURL string) (*gorm.DB, func() error, error) {
	if databaseURL == "" {
		return nil, nil, errors.New("database URL is required")
	}

	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		return nil, nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, err
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, nil, err
	}

	return db, sqlDB.Close, nil
}

func Migrate(ctx context.Context, db *gorm.DB) error {
	if err := db.WithContext(ctx).AutoMigrate(&models.LevelGenerationRecord{}, &models.Level{}, &models.Quiz{}, &models.QuizMistake{}); err != nil {
		return err
	}

	statements := []string{
		`ALTER TABLE level_generation_jobs ADD COLUMN IF NOT EXISTS idempotency_key TEXT`,
		`ALTER TABLE levels ADD COLUMN IF NOT EXISTS generation_id TEXT`,
		`ALTER TABLE levels ADD COLUMN IF NOT EXISTS map_seed BIGINT`,
		`ALTER TABLE levels ADD COLUMN IF NOT EXISTS map_algorithm_version INTEGER`,
		`ALTER TABLE levels ADD COLUMN IF NOT EXISTS source_chunk_ids JSONB NOT NULL DEFAULT '[]'::jsonb`,
		`ALTER TABLE levels ADD COLUMN IF NOT EXISTS source_metadata JSONB NOT NULL DEFAULT '{}'::jsonb`,
		`ALTER TABLE quizzes ADD COLUMN IF NOT EXISTS answer_index INTEGER`,
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'quizzes_answer_index_nonnegative_check'
			) THEN
				ALTER TABLE quizzes
				ADD CONSTRAINT quizzes_answer_index_nonnegative_check
				CHECK (answer_index >= 0);
			END IF;
		END;
		$$;`,
		`CREATE INDEX IF NOT EXISTS levels_user_id_idx ON levels(user_id)`,
		`CREATE INDEX IF NOT EXISTS levels_sub_chapter_id_idx ON levels(sub_chapter_id)`,
		`CREATE INDEX IF NOT EXISTS levels_document_id_idx ON levels(document_id)`,
		`CREATE INDEX IF NOT EXISTS levels_generation_id_idx ON levels(generation_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS levels_generation_id_unique_idx ON levels(generation_id) WHERE generation_id IS NOT NULL AND generation_id <> ''`,
		`CREATE INDEX IF NOT EXISTS quizzes_level_id_idx ON quizzes(level_id)`,
		`CREATE INDEX IF NOT EXISTS quiz_mistakes_user_id_idx ON quiz_mistakes(user_id)`,
		`CREATE INDEX IF NOT EXISTS quiz_mistakes_level_id_idx ON quiz_mistakes(level_id)`,
		`CREATE INDEX IF NOT EXISTS quiz_mistakes_generation_id_idx ON quiz_mistakes(generation_id)`,
		`CREATE INDEX IF NOT EXISTS level_generation_jobs_user_id_idx ON level_generation_jobs(user_id)`,
		`CREATE INDEX IF NOT EXISTS level_generation_jobs_sub_chapter_id_idx ON level_generation_jobs(sub_chapter_id)`,
		`CREATE INDEX IF NOT EXISTS level_generation_jobs_level_id_idx ON level_generation_jobs(level_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS level_generation_jobs_idempotency_key_idx ON level_generation_jobs(idempotency_key) WHERE idempotency_key IS NOT NULL AND idempotency_key <> ''`,
		`UPDATE quizzes SET answer_index = 0 WHERE answer_index IS NULL`,
		`ALTER TABLE quizzes ALTER COLUMN answer_index SET NOT NULL`,
	}
	for _, statement := range statements {
		if err := db.WithContext(ctx).Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}
