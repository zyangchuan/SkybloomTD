package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrNotFound = errors.New("not found")

type DocumentRow struct {
	SubChapterID    uuid.UUID
	DocumentID      uuid.UUID
	ChapterID       uuid.UUID
	SubChapterIndex *int32
	Title           *string
	StartLine       *int32
	EndLine         *int32
	UserID          uuid.UUID
	S3Bucket        *string
}

type DocumentRepository struct {
	db *gorm.DB
}

func NewDocumentRepository(db *gorm.DB) *DocumentRepository {
	return &DocumentRepository{db: db}
}

func (r *DocumentRepository) LoadDocumentRow(ctx context.Context, subChapterID uuid.UUID, userID uuid.UUID) (DocumentRow, error) {
	var row DocumentRow
	err := r.db.WithContext(ctx).
		Table("sub_chapters AS sc").
		Select(`
			sc.id AS sub_chapter_id,
			sc.document_id,
			sc.chapter_id,
			sc.sub_chapter_index,
			sc.title,
			sc.start_line,
			sc.end_line,
			d.user_id,
			d.s3_bucket
		`).
		Joins("JOIN documents AS d ON sc.document_id = d.id").
		Where("sc.id = ? AND d.user_id = ?", subChapterID, userID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DocumentRow{}, fmt.Errorf("%w: No sub_chapter found for the provided user_id and sub_chapter_id", ErrNotFound)
	}
	if err != nil {
		return DocumentRow{}, err
	}
	return row, nil
}
