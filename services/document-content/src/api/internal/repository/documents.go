package repository

import (
	"context"
	"errors"

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

func (r *DocumentRepository) LoadUserDocument(ctx context.Context, documentID uuid.UUID, userID uuid.UUID) (models.Document, error) {
	var document models.Document
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", documentID, userID).
		Take(&document).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.Document{}, models.ErrDocumentNotFound
	}
	if err != nil {
		return models.Document{}, err
	}
	return document, nil
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

func (r *DocumentRepository) DeleteDocumentCascade(ctx context.Context, documentID uuid.UUID, userID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			`DELETE FROM quizzes
			WHERE level_id IN (
				SELECT id FROM levels WHERE document_id = ?
			)`,
			documentID,
		).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM levels WHERE document_id = ?`, documentID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM chunks WHERE document_id = ?`, documentID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM sub_chapters WHERE document_id = ?`, documentID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM chapters WHERE document_id = ?`, documentID).Error; err != nil {
			return err
		}

		result := tx.Delete(&models.Document{}, "id = ? AND user_id = ?", documentID, userID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return models.ErrDocumentNotFound
		}
		return nil
	})
}
