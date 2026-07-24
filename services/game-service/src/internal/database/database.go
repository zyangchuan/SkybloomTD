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

	db, err := gorm.Open(postgres.New(postgres.Config{DSN: databaseURL, PreferSimpleProtocol: true}), &gorm.Config{})
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
	preflightStatements := []string{
		`CREATE SCHEMA IF NOT EXISTS private`,
		`REVOKE ALL ON SCHEMA private FROM PUBLIC`,
		`DO $$
		BEGIN
			IF to_regclass('private.level_generation_jobs') IS NULL AND to_regclass('public.level_generation_jobs') IS NOT NULL THEN
				ALTER TABLE public.level_generation_jobs SET SCHEMA private;
			END IF;
			IF to_regclass('private.levels') IS NULL AND to_regclass('public.levels') IS NOT NULL THEN
				ALTER TABLE public.levels SET SCHEMA private;
			END IF;
			IF to_regclass('private.quizzes') IS NULL AND to_regclass('public.quizzes') IS NOT NULL THEN
				ALTER TABLE public.quizzes SET SCHEMA private;
			END IF;
			IF to_regclass('private.quiz_mistakes') IS NULL AND to_regclass('public.quiz_mistakes') IS NOT NULL THEN
				ALTER TABLE public.quiz_mistakes SET SCHEMA private;
			END IF;
		END;
		$$;`,
	}
	for _, statement := range preflightStatements {
		if err := db.WithContext(ctx).Exec(statement).Error; err != nil {
			return err
		}
	}

	if err := db.WithContext(ctx).AutoMigrate(&models.LevelGenerationRecord{}, &models.Level{}, &models.Quiz{}); err != nil {
		return err
	}

	statements := []string{
		`DROP TABLE IF EXISTS private.quiz_mistakes`,
		`DROP TABLE IF EXISTS public.quiz_mistakes`,
		`ALTER TABLE private.level_generation_jobs ADD COLUMN IF NOT EXISTS idempotency_key TEXT`,
		`ALTER TABLE private.levels ADD COLUMN IF NOT EXISTS generation_id TEXT`,
		`ALTER TABLE private.levels ADD COLUMN IF NOT EXISTS map_seed BIGINT`,
		`ALTER TABLE private.levels ADD COLUMN IF NOT EXISTS map_algorithm_version INTEGER`,
		`ALTER TABLE private.levels ADD COLUMN IF NOT EXISTS source_chunk_ids JSONB NOT NULL DEFAULT '[]'::jsonb`,
		`ALTER TABLE private.levels ADD COLUMN IF NOT EXISTS source_metadata JSONB NOT NULL DEFAULT '{}'::jsonb`,
		`ALTER TABLE private.quizzes ADD COLUMN IF NOT EXISTS answer_index INTEGER`,
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'quizzes_answer_index_nonnegative_check'
				AND conrelid = 'private.quizzes'::regclass
			) THEN
				ALTER TABLE private.quizzes
				ADD CONSTRAINT quizzes_answer_index_nonnegative_check
				CHECK (answer_index >= 0);
			END IF;
		END;
		$$;`,
		`CREATE INDEX IF NOT EXISTS levels_user_id_idx ON private.levels(user_id)`,
		`CREATE INDEX IF NOT EXISTS levels_sub_chapter_id_idx ON private.levels(sub_chapter_id)`,
		`CREATE INDEX IF NOT EXISTS levels_document_id_idx ON private.levels(document_id)`,
		`CREATE INDEX IF NOT EXISTS levels_generation_id_idx ON private.levels(generation_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS levels_generation_id_unique_idx ON private.levels(generation_id) WHERE generation_id IS NOT NULL AND generation_id <> ''`,
		`CREATE INDEX IF NOT EXISTS quizzes_level_id_idx ON private.quizzes(level_id)`,
		`CREATE INDEX IF NOT EXISTS level_generation_jobs_user_id_idx ON private.level_generation_jobs(user_id)`,
		`CREATE INDEX IF NOT EXISTS level_generation_jobs_sub_chapter_id_idx ON private.level_generation_jobs(sub_chapter_id)`,
		`CREATE INDEX IF NOT EXISTS level_generation_jobs_level_id_idx ON private.level_generation_jobs(level_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS level_generation_jobs_idempotency_key_idx ON private.level_generation_jobs(idempotency_key) WHERE idempotency_key IS NOT NULL AND idempotency_key <> ''`,
		`UPDATE private.quizzes SET answer_index = 0 WHERE answer_index IS NULL`,
		`ALTER TABLE private.quizzes ALTER COLUMN answer_index SET NOT NULL`,
		`REVOKE ALL ON TABLE private.level_generation_jobs FROM PUBLIC`,
		`REVOKE ALL ON TABLE private.levels FROM PUBLIC`,
		`REVOKE ALL ON TABLE private.quizzes FROM PUBLIC`,
	}
	for _, statement := range statements {
		if err := db.WithContext(ctx).Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}
