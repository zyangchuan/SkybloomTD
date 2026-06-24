package database

import (
	"context"

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
	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, nil, err
	}

	return db, sqlDB.Close, nil
}
