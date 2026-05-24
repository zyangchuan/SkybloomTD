package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type User struct {
	ID        uuid.UUID         `gorm:"type:uuid;primaryKey" json:"id"`
	Email     *string           `gorm:"type:text;index:users_email_idx" json:"email,omitempty"`
	UserName  string            `gorm:"column:user_name;type:text;not null" json:"user_name"`
	Metadata  datatypes.JSONMap `gorm:"type:jsonb;not null;default:'{}'::jsonb" json:"metadata"`
	CreatedAt time.Time         `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time         `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (User) TableName() string {
	return "private.users"
}
