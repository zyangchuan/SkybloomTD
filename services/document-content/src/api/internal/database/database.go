package database

import (
	"context"
	"errors"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"skybloom/document-content-api/internal/models"
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
	if err := db.WithContext(ctx).AutoMigrate(&models.Document{}); err != nil {
		return err
	}

	statements := []string{
		`ALTER TABLE documents ADD COLUMN IF NOT EXISTS task_id TEXT`,
		`ALTER TABLE documents ADD COLUMN IF NOT EXISTS is_ready BOOLEAN DEFAULT false`,
		`UPDATE documents SET is_ready = false WHERE is_ready IS NULL`,
		`ALTER TABLE documents ALTER COLUMN is_ready SET DEFAULT false`,
		`ALTER TABLE documents ALTER COLUMN is_ready SET NOT NULL`,
		`ALTER TABLE documents ADD COLUMN IF NOT EXISTS source_type TEXT`,
		`ALTER TABLE documents ADD COLUMN IF NOT EXISTS source_bucket TEXT`,
		`ALTER TABLE documents ADD COLUMN IF NOT EXISTS source_key TEXT`,
		`ALTER TABLE documents ADD COLUMN IF NOT EXISTS source_path TEXT`,
		`ALTER TABLE documents ADD COLUMN IF NOT EXISTS source_content_type TEXT`,
		`CREATE INDEX IF NOT EXISTS documents_task_id_idx ON documents(task_id)`,
		`CREATE INDEX IF NOT EXISTS documents_user_id_idx ON documents(user_id)`,
	}
	for _, statement := range statements {
		if err := db.WithContext(ctx).Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}
