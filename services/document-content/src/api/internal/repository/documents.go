package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"skybloom/document-content-api/internal/models"
)

type DocumentRepository struct {
	db *gorm.DB
}

func NewDocumentRepository(db *gorm.DB) *DocumentRepository {
	return &DocumentRepository{db: db}
}

func (r *DocumentRepository) CreateQueuedDocument(ctx context.Context, document models.Document) error {
	return r.db.WithContext(ctx).Create(&document).Error
}

func (r *DocumentRepository) ListUserDocuments(ctx context.Context, userID uuid.UUID) ([]models.DocumentSummary, error) {
	var documents []models.Document
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&documents).Error; err != nil {
		return nil, err
	}

	summaries := make([]models.DocumentSummary, 0, len(documents))
	for _, document := range documents {
		summaries = append(summaries, models.NewDocumentSummary(document))
	}
	return summaries, nil
}
