package repository

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"skybloom/document-content-api/internal/models"
)

type DocumentRepository struct {
	db *gorm.DB
}

const gameLibraryPageSize = 10
const (
	documentsTable    = "private.documents"
	starredGamesTable = "private.starred_games"
	chaptersTable     = "private.chapters"
	subChaptersTable  = "private.sub_chapters"
	levelsTable       = "private.levels"
	quizzesTable      = "private.quizzes"
)

type gameCursor struct {
	SortTime   time.Time
	DocumentID uuid.UUID
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

func (r *DocumentRepository) SetDocumentVisibility(ctx context.Context, documentID uuid.UUID, userID uuid.UUID, isPublic bool) (models.DocumentSummary, error) {
	var document models.Document
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.Document{}).
			Where("id = ? AND user_id = ?", documentID, userID).
			Update("is_public", isPublic)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return models.ErrDocumentNotFound
		}
		return tx.Where("id = ? AND user_id = ?", documentID, userID).Take(&document).Error
	})
	if err != nil {
		return models.DocumentSummary{}, err
	}
	return models.NewDocumentSummary(document), nil
}

func (r *DocumentRepository) ListPublicGames(ctx context.Context, userID uuid.UUID, cursor string) (models.ListGameLibraryResponse, error) {
	var rows []gameLibraryRow
	query := r.db.WithContext(ctx).
		Table(documentsTable+" AS d").
		Select(`
			d.id AS document_id,
			d.user_id,
			d.source_filename,
			d.game_name,
			d.is_ready,
			d.is_public,
			d.created_at,
			d.updated_at,
			sg.user_id IS NOT NULL AS starred_by_me
		`).
		Joins("LEFT JOIN "+starredGamesTable+" AS sg ON sg.document_id = d.id AND sg.user_id = ?", userID).
		Where("d.is_public = true AND d.is_ready = true")

	if cursor != "" {
		decoded, err := decodeGameCursor(cursor)
		if err != nil {
			return models.ListGameLibraryResponse{}, err
		}
		query = query.Where("(d.created_at < ? OR (d.created_at = ? AND d.id < ?))", decoded.SortTime, decoded.SortTime, decoded.DocumentID)
	}

	if err := query.
		Order("d.created_at DESC, d.id DESC").
		Limit(gameLibraryPageSize + 1).
		Find(&rows).Error; err != nil {
		return models.ListGameLibraryResponse{}, err
	}

	return gameLibraryResponse(rows, func(row gameLibraryRow) time.Time { return row.CreatedAt }), nil
}

func (r *DocumentRepository) ListStarredGames(ctx context.Context, userID uuid.UUID, cursor string) (models.ListGameLibraryResponse, error) {
	var rows []gameLibraryRow
	query := r.db.WithContext(ctx).
		Table(starredGamesTable+" AS sg").
		Select(`
			d.id AS document_id,
			d.user_id,
			d.source_filename,
			d.game_name,
			d.is_ready,
			d.is_public,
			d.created_at,
			d.updated_at,
			true AS starred_by_me,
			sg.created_at AS starred_at
		`).
		Joins("JOIN "+documentsTable+" AS d ON d.id = sg.document_id").
		Where("sg.user_id = ?", userID).
		Where("d.user_id = ? OR (d.is_public = true AND d.is_ready = true)", userID)

	if cursor != "" {
		decoded, err := decodeGameCursor(cursor)
		if err != nil {
			return models.ListGameLibraryResponse{}, err
		}
		query = query.Where("(sg.created_at < ? OR (sg.created_at = ? AND d.id < ?))", decoded.SortTime, decoded.SortTime, decoded.DocumentID)
	}

	if err := query.
		Order("sg.created_at DESC, d.id DESC").
		Limit(gameLibraryPageSize + 1).
		Find(&rows).Error; err != nil {
		return models.ListGameLibraryResponse{}, err
	}

	return gameLibraryResponse(rows, func(row gameLibraryRow) time.Time { return row.StarredAt }), nil
}

func (r *DocumentRepository) StarGame(ctx context.Context, documentID uuid.UUID, userID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.Document{}).
			Where("id = ? AND (user_id = ? OR (is_public = true AND is_ready = true))", documentID, userID).
			Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return models.ErrDocumentNotFound
		}
		return tx.Exec(`
			INSERT INTO private.starred_games (user_id, document_id)
			VALUES (?, ?)
			ON CONFLICT (user_id, document_id) DO NOTHING
		`, userID, documentID).Error
	})
}

func (r *DocumentRepository) UnstarGame(ctx context.Context, documentID uuid.UUID, userID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Exec("DELETE FROM private.starred_games WHERE user_id = ? AND document_id = ?", userID, documentID).
		Error
}

type gameLibraryRow struct {
	DocumentID     uuid.UUID
	UserID         uuid.UUID
	SourceFilename string
	GameName       string
	IsReady        bool
	IsPublic       bool
	StarredByMe    bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
	StarredAt      time.Time
}

func gameLibraryResponse(rows []gameLibraryRow, sortTime func(gameLibraryRow) time.Time) models.ListGameLibraryResponse {
	nextCursor := ""
	if len(rows) > gameLibraryPageSize {
		nextRow := rows[gameLibraryPageSize-1]
		nextCursor = encodeGameCursor(gameCursor{SortTime: sortTime(nextRow), DocumentID: nextRow.DocumentID})
		rows = rows[:gameLibraryPageSize]
	}

	games := make([]models.GameLibrarySummary, 0, len(rows))
	for _, row := range rows {
		games = append(games, models.GameLibrarySummary{
			DocumentID:     row.DocumentID,
			UserID:         row.UserID,
			SourceFilename: row.SourceFilename,
			GameName:       row.GameName,
			IsReady:        row.IsReady,
			IsPublic:       row.IsPublic,
			StarredByMe:    row.StarredByMe,
			CreatedAt:      row.CreatedAt,
			UpdatedAt:      row.UpdatedAt,
		})
	}

	return models.ListGameLibraryResponse{Games: games, NextCursor: nextCursor}
}

func encodeGameCursor(cursor gameCursor) string {
	value := fmt.Sprintf("%s|%s", cursor.SortTime.UTC().Format(time.RFC3339Nano), cursor.DocumentID.String())
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeGameCursor(value string) (gameCursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return gameCursor{}, models.ErrInvalidCursor
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 2 {
		return gameCursor{}, models.ErrInvalidCursor
	}
	sortTime, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return gameCursor{}, models.ErrInvalidCursor
	}
	documentID, err := uuid.Parse(parts[1])
	if err != nil {
		return gameCursor{}, models.ErrInvalidCursor
	}
	return gameCursor{SortTime: sortTime, DocumentID: documentID}, nil
}

func (r *DocumentRepository) ListDocumentChapters(ctx context.Context, documentID uuid.UUID, userID uuid.UUID) ([]models.ChapterSummary, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&models.Document{}).
		Where("id = ? AND (user_id = ? OR (is_public = true AND is_ready = true))", documentID, userID).
		Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, models.ErrDocumentNotFound
	}

	var chapters []models.ChapterSummary
	if err := r.db.WithContext(ctx).
		Table(chaptersTable+" AS c").
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
		Table(chaptersTable+" AS c").
		Joins("JOIN "+documentsTable+" AS d ON d.id = c.document_id").
		Where("c.id = ? AND (d.user_id = ? OR (d.is_public = true AND d.is_ready = true))", chapterID, userID).
		Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, models.ErrChapterNotFound
	}

	var subChapters []models.SubChapterSummary
	if err := r.db.WithContext(ctx).
		Table(subChaptersTable+" AS sc").
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
			`DELETE FROM `+quizzesTable+`
			WHERE level_id IN (
				SELECT id FROM `+levelsTable+` WHERE document_id = ?
			)`,
			documentID,
		).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM `+levelsTable+` WHERE document_id = ?`, documentID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM `+starredGamesTable+` WHERE document_id = ?`, documentID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM `+subChaptersTable+` WHERE document_id = ?`, documentID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM `+chaptersTable+` WHERE document_id = ?`, documentID).Error; err != nil {
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
