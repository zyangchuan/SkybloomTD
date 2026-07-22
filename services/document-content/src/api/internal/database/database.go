package database

import (
	"context"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Open(ctx context.Context, databaseURL string) (*gorm.DB, func() error, error) {

	db, err := gorm.Open(postgres.New(postgres.Config{DSN: databaseURL, PreferSimpleProtocol: true}), &gorm.Config{})
	if err != nil {
		return nil, nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, err
	}

	// Database connection health check
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, nil, err
	}

	return db, sqlDB.Close, nil
}

func Migrate(ctx context.Context, db *gorm.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS documents (
			id UUID PRIMARY KEY,
			user_id UUID,
			s3_bucket TEXT,
			source_filename TEXT,
			game_name TEXT NOT NULL DEFAULT 'Untitled Game',
			task_id TEXT,
			is_ready BOOLEAN NOT NULL DEFAULT false,
			is_public BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMPTZ,
			updated_at TIMESTAMPTZ
		)`,
		`ALTER TABLE documents ADD COLUMN IF NOT EXISTS source_filename TEXT`,
		`ALTER TABLE documents ADD COLUMN IF NOT EXISTS game_name TEXT`,
		`UPDATE documents SET game_name = COALESCE(NULLIF(game_name, ''), NULLIF(source_filename, ''), 'Untitled Game') WHERE game_name IS NULL OR game_name = ''`,
		`ALTER TABLE documents ALTER COLUMN game_name SET DEFAULT 'Untitled Game'`,
		`ALTER TABLE documents ALTER COLUMN game_name SET NOT NULL`,
		`ALTER TABLE documents ADD COLUMN IF NOT EXISTS task_id TEXT`,
		`ALTER TABLE documents ADD COLUMN IF NOT EXISTS is_ready BOOLEAN DEFAULT false`,
		`UPDATE documents SET is_ready = false WHERE is_ready IS NULL`,
		`ALTER TABLE documents ALTER COLUMN is_ready SET DEFAULT false`,
		`ALTER TABLE documents ALTER COLUMN is_ready SET NOT NULL`,
		`ALTER TABLE documents ADD COLUMN IF NOT EXISTS is_public BOOLEAN DEFAULT true`,
		`UPDATE documents SET is_public = true WHERE is_public IS NULL`,
		`ALTER TABLE documents ALTER COLUMN is_public SET DEFAULT true`,
		`ALTER TABLE documents ALTER COLUMN is_public SET NOT NULL`,
		`CREATE TABLE IF NOT EXISTS starred_games (
			user_id UUID NOT NULL,
			document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, document_id)
		)`,
		`CREATE TABLE IF NOT EXISTS chapters (
			id UUID PRIMARY KEY,
			document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
			chapter_index INTEGER,
			title TEXT,
			start_line INTEGER,
			end_line INTEGER,
			created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS sub_chapters (
			id UUID PRIMARY KEY,
			document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
			chapter_id UUID NOT NULL REFERENCES chapters(id) ON DELETE CASCADE,
			sub_chapter_index INTEGER,
			title TEXT,
			start_line INTEGER,
			end_line INTEGER,
			created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS documents_task_id_idx ON documents(task_id)`,
		`CREATE INDEX IF NOT EXISTS documents_user_id_idx ON documents(user_id)`,
		`CREATE INDEX IF NOT EXISTS documents_public_ready_created_idx ON documents(is_public, is_ready, created_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS documents_user_public_idx ON documents(user_id, is_public)`,
		`CREATE INDEX IF NOT EXISTS starred_games_user_created_idx ON starred_games(user_id, created_at DESC, document_id DESC)`,
		`CREATE INDEX IF NOT EXISTS chapters_document_id_idx ON chapters(document_id)`,
		`CREATE INDEX IF NOT EXISTS sub_chapters_document_id_idx ON sub_chapters(document_id)`,
		`CREATE INDEX IF NOT EXISTS sub_chapters_chapter_id_idx ON sub_chapters(chapter_id)`,
	}
	for _, statement := range statements {
		if err := db.WithContext(ctx).Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}
