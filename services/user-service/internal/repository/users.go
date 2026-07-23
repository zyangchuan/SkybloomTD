package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"skybloom/user-service/internal/models"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (r *UserRepository) GetByID(ctx context.Context, userID uuid.UUID) (models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).First(&user, "id = ?", userID).Error
	if err != nil {
		return models.User{}, err
	}
	return user, nil
}

func (r *UserRepository) Upsert(ctx context.Context, user models.User) (models.User, error) {
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"email":      user.Email,
				"user_name":  user.UserName,
				"metadata":   user.Metadata,
				"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
			}),
		}).
		Create(&user).Error
	if err != nil {
		return models.User{}, err
	}

	var saved models.User
	err = r.db.WithContext(ctx).First(&saved, "id = ?", user.ID).Error
	if err != nil {
		return models.User{}, err
	}
	return saved, nil
}
