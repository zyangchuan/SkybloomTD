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

func (r *DocumentRepository) ListDocumentChapters(ctx context.Context, documentID uuid.UUID, userID uuid.UUID) ([]models.ChapterSummary, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&models.Document{}).
		Where("id = ? AND user_id = ?", documentID, userID).
		Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, models.ErrDocumentNotFound
	}

	var chapters []models.ChapterSummary
	if err := r.db.WithContext(ctx).
		Table("chapters AS c").
		Select(`
			c.id AS chapter_id,
			c.document_id,
			c.chapter_index,
			c.title,
			c.start_line,
			c.end_line,
			c.created_at
		`).
		Where("c.document_id = ?", documentID).
		Order("c.chapter_index ASC NULLS LAST, c.created_at ASC").
		Find(&chapters).Error; err != nil {
		return nil, err
	}
	return chapters, nil
}

func (r *DocumentRepository) ListChapterSubChapters(ctx context.Context, chapterID uuid.UUID, userID uuid.UUID) ([]models.SubChapterSummary, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Table("chapters AS c").
		Joins("JOIN documents AS d ON d.id = c.document_id").
		Where("c.id = ? AND d.user_id = ?", chapterID, userID).
		Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, models.ErrChapterNotFound
	}

	var subChapters []models.SubChapterSummary
	if err := r.db.WithContext(ctx).
		Table("sub_chapters AS sc").
		Select(`
			sc.id AS sub_chapter_id,
			sc.document_id,
			sc.chapter_id,
			sc.sub_chapter_index,
			sc.title,
			sc.start_line,
			sc.end_line,
			sc.created_at
		`).
		Where("sc.chapter_id = ?", chapterID).
		Order("sc.sub_chapter_index ASC NULLS LAST, sc.created_at ASC").
		Find(&subChapters).Error; err != nil {
		return nil, err
	}
	return subChapters, nil
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
