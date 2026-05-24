package database

import (
	"context"
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"skybloom/user-service/internal/models"
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
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("CREATE SCHEMA IF NOT EXISTS private").Error; err != nil {
			return fmt.Errorf("create private schema: %w", err)
		}
		if err := tx.Exec("REVOKE ALL ON SCHEMA private FROM PUBLIC").Error; err != nil {
			return fmt.Errorf("revoke private schema grants: %w", err)
		}
		if err := tx.AutoMigrate(&models.User{}); err != nil {
			return fmt.Errorf("migrate users table: %w", err)
		}
		if err := tx.Exec("REVOKE ALL ON TABLE private.users FROM PUBLIC").Error; err != nil {
			return fmt.Errorf("revoke users table grants: %w", err)
		}
		return nil
	})
}
