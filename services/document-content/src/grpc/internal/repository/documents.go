package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("not found")

type DocumentRow struct {
	SubChapterID    uuid.UUID
	DocumentID      uuid.UUID
	ChapterID       uuid.UUID
	SubChapterIndex sql.NullInt32
	Title           sql.NullString
	StartLine       sql.NullInt32
	EndLine         sql.NullInt32
	S3Bucket        sql.NullString
	S3Key           sql.NullString
}

type DocumentRepository struct {
	db *sql.DB
}

func NewDocumentRepository(db *sql.DB) *DocumentRepository {
	return &DocumentRepository{db: db}
}

func (r *DocumentRepository) LoadDocumentRow(ctx context.Context, subChapterID uuid.UUID, userID uuid.UUID) (DocumentRow, error) {
	const query = `
		SELECT sc.id, sc.document_id, sc.chapter_id, sc.sub_chapter_index,
		       sc.title, sc.start_line, sc.end_line, d.s3_bucket, d.s3_key
		FROM sub_chapters sc
		JOIN documents d ON sc.document_id = d.id
		WHERE sc.id = $1 AND d.user_id = $2
	`
	var row DocumentRow
	err := r.db.QueryRowContext(ctx, query, subChapterID, userID).Scan(
		&row.SubChapterID,
		&row.DocumentID,
		&row.ChapterID,
		&row.SubChapterIndex,
		&row.Title,
		&row.StartLine,
		&row.EndLine,
		&row.S3Bucket,
		&row.S3Key,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return DocumentRow{}, fmt.Errorf("%w: No sub_chapter found for the provided user_id and sub_chapter_id", ErrNotFound)
	}
	if err != nil {
		return DocumentRow{}, err
	}
	return row, nil
}
