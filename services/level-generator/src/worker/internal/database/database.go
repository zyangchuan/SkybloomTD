package database

import (
	"context"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"skybloom/level-generator-worker/internal/models"
)

func Open(ctx context.Context, databaseURL string) (*gorm.DB, func() error, error) {
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
	if err := db.WithContext(ctx).AutoMigrate(&models.Level{}, &models.Quiz{}); err != nil {
		return err
	}

	statements := []string{
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
		}
		$$`,
		`CREATE INDEX IF NOT EXISTS levels_user_id_idx ON levels(user_id)`,
		`CREATE INDEX IF NOT EXISTS levels_sub_chapter_id_idx ON levels(sub_chapter_id)`,
		`CREATE INDEX IF NOT EXISTS levels_document_id_idx ON levels(document_id)`,
		`CREATE INDEX IF NOT EXISTS quizzes_level_id_idx ON quizzes(level_id)`,
		`DO $$
		DECLARE
			constraint_name text;
		BEGIN
			FOR constraint_name IN
				SELECT conname
				FROM pg_constraint
				WHERE conrelid = 'levels'::regclass
				  AND contype = 'f'
			LOOP
				EXECUTE format('ALTER TABLE levels DROP CONSTRAINT IF EXISTS %I', constraint_name);
			END LOOP;
		END
		$$`,
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
