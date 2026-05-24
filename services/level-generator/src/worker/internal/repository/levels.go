package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"skybloom/level-generator-worker/internal/generator"
	"skybloom/level-generator-worker/internal/models"
	"skybloom/level-generator-worker/internal/source"
)

type LevelRepository struct {
	db *gorm.DB
}

type SavedLevel struct {
	LevelID      string
	SubChapterID string
	DocumentID   string
	QuizCount    int
	Model        string
}

func NewLevelRepository(db *gorm.DB) *LevelRepository {
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

	level := models.Level{
		ID:              levelID,
		UserID:          &dbUserID,
		DocumentID:      documentID,
		ChapterID:       chapterID,
		SubChapterID:    subChapterID,
		SummaryMarkdown: generation.SummaryMarkdown,
		SourceChunkIDs:  datatypes.JSON(chunkIDs),
		SourceMetadata:  datatypes.JSON(sourceMetadata),
		Model:           model,
	}
	quizzes := make([]models.Quiz, 0, len(generation.Quizzes))
	for index, quiz := range generation.Quizzes {
		quizID, err := uuid.NewRandom()
		if err != nil {
			return SavedLevel{}, err
		}
		options, err := json.Marshal(quiz.OptionsMarkdown)
		if err != nil {
			return SavedLevel{}, err
		}
		quizzes = append(quizzes, models.Quiz{
			ID:               quizID,
			LevelID:          levelID,
			QuizIndex:        index,
			QuizType:         quiz.QuizType,
			QuestionMarkdown: quiz.QuestionMarkdown,
			OptionsMarkdown:  datatypes.JSON(options),
			AnswerIndex:      quiz.AnswerIndex,
		})
	}

	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&level).Error; err != nil {
			return err
		}
		if len(quizzes) > 0 {
			return tx.Omit("Level").Create(&quizzes).Error
		}
		return nil
	})
	if err != nil {
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
