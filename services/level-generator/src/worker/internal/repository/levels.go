package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"skybloom/level-generator-worker/internal/generator"
	"skybloom/level-generator-worker/internal/source"
)

type LevelRepository struct {
	db *sql.DB
}

type SavedLevel struct {
	LevelID      string
	SubChapterID string
	DocumentID   string
	QuizCount    int
	Model        string
}

func NewLevelRepository(db *sql.DB) *LevelRepository {
	return &LevelRepository{db: db}
}

func (r *LevelRepository) Insert(ctx context.Context, sourceContext source.SourceContext, generation generator.LevelGeneration, model string) (SavedLevel, error) {
	levelID, err := uuid.NewRandom()
	if err != nil {
		return SavedLevel{}, err
	}
	dbUserID, err := uuid.Parse(sourceContext.DBUserID)
	if err != nil {
		return SavedLevel{}, fmt.Errorf("db_user_id must be a valid UUID: %w", err)
	}
	documentID, err := uuid.Parse(sourceContext.DocumentID)
	if err != nil {
		return SavedLevel{}, fmt.Errorf("document_id must be a valid UUID: %w", err)
	}
	chapterID, err := uuid.Parse(sourceContext.ChapterID)
	if err != nil {
		return SavedLevel{}, fmt.Errorf("chapter_id must be a valid UUID: %w", err)
	}
	subChapterID, err := uuid.Parse(sourceContext.SubChapterID)
	if err != nil {
		return SavedLevel{}, fmt.Errorf("sub_chapter_id must be a valid UUID: %w", err)
	}

	chunkIDs, err := json.Marshal(sourceContext.ChunkIDs)
	if err != nil {
		return SavedLevel{}, err
	}
	sourceMetadata, err := json.Marshal(map[string]any{
		"sub_chapter_index":     sourceContext.SubChapterIndex,
		"sub_chapter_title":     sourceContext.SubChapterTitle,
		"start_line":            sourceContext.StartLine,
		"end_line":              sourceContext.EndLine,
		"chunk_count":           sourceContext.ChunkCount,
		"candidate_chunk_count": sourceContext.CandidateChunkCount,
		"chunk_lookup_strategy": sourceContext.ChunkLookupStrategy,
		"markdown_cache_hit":    sourceContext.MarkdownCacheHit,
		"markdown_cache_key":    sourceContext.MarkdownCacheKey,
		"source_char_count":     sourceContext.SourceCharCount,
		"source_truncated":      sourceContext.SourceTruncated,
		"source_content_hash":   sourceContext.SourceContentHash,
	})
	if err != nil {
		return SavedLevel{}, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return SavedLevel{}, err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO levels (
			id, user_id, document_id, chapter_id, sub_chapter_id,
			summary_markdown, source_chunk_ids, source_metadata, model
		) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, $9)`,
		levelID,
		dbUserID,
		documentID,
		chapterID,
		subChapterID,
		generation.SummaryMarkdown,
		string(chunkIDs),
		string(sourceMetadata),
		model,
	)
	if err != nil {
		return SavedLevel{}, err
	}

	for index, quiz := range generation.Quizzes {
		quizID, err := uuid.NewRandom()
		if err != nil {
			return SavedLevel{}, err
		}
		options, err := json.Marshal(quiz.OptionsMarkdown)
		if err != nil {
			return SavedLevel{}, err
		}
		_, err = tx.ExecContext(
			ctx,
			`INSERT INTO quizzes (
				id, level_id, quiz_index, quiz_type,
				question_markdown, options_markdown, answer_index
			) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)`,
			quizID,
			levelID,
			index,
			quiz.QuizType,
			quiz.QuestionMarkdown,
			string(options),
			quiz.AnswerIndex,
		)
		if err != nil {
			return SavedLevel{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return SavedLevel{}, err
	}
	return SavedLevel{
		LevelID:      levelID.String(),
		SubChapterID: sourceContext.SubChapterID,
		DocumentID:   sourceContext.DocumentID,
		QuizCount:    len(generation.Quizzes),
		Model:        model,
	}, nil
}
