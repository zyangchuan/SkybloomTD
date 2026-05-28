package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"skybloom/game-service/internal/generator"
	"skybloom/game-service/internal/models"
	"skybloom/game-service/internal/source"
)

type LevelRepository struct {
	db *gorm.DB
}

type LevelBootstrap struct {
	LevelID             string     `json:"level_id"`
	UserID              string     `json:"user_id"`
	DocumentID          string     `json:"document_id"`
	ChapterID           string     `json:"chapter_id"`
	SubChapterID        string     `json:"sub_chapter_id"`
	GenerationID        string     `json:"generation_id"`
	MapSeed             int64      `json:"map_seed"`
	MapAlgorithmVersion int        `json:"map_algorithm_version"`
	SummaryMarkdown     string     `json:"summary_markdown"`
	Quizzes             []QuizItem `json:"quizzes"`
}

type QuizItem struct {
	ID               string   `json:"id"`
	QuizIndex        int      `json:"quiz_index"`
	QuizType         string   `json:"quiz_type"`
	QuestionMarkdown string   `json:"question_markdown"`
	OptionsMarkdown  []string `json:"options_markdown"`
	AnswerIndex      int      `json:"answer_index"`
}

type QuizMistakeInput struct {
	UserID           string
	LevelID          string
	GenerationID     string
	QuizID           string
	QuizIndex        int
	QuizType         string
	QuestionMarkdown string
	OptionsMarkdown  []string
	AnswerIndex      int
	SelectedIndex    int
}

type SavedLevel struct {
	LevelID      string
	SubChapterID string
	DocumentID   string
	QuizCount    int
	Model        string
}

type LevelInsertOptions struct {
	GenerationID        string
	MapSeed             int64
	MapAlgorithmVersion int
}

func NewLevelRepository(db *gorm.DB) *LevelRepository {
	return &LevelRepository{db: db}
}

func (r *LevelRepository) GetBootstrap(ctx context.Context, levelID string, userID string) (LevelBootstrap, error) {
	var level models.Level
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", levelID, userID).
		First(&level).
		Error
	if err == gorm.ErrRecordNotFound {
		return LevelBootstrap{}, models.ErrLevelNotFound
	}
	if err != nil {
		return LevelBootstrap{}, err
	}
	if level.UserID == nil ||
		level.GenerationID == nil ||
		level.MapSeed == nil ||
		level.MapAlgorithmVersion == nil {
		return LevelBootstrap{}, models.ErrLevelNotFound
	}

	var quizzes []models.Quiz
	if err := r.db.WithContext(ctx).
		Where("level_id = ?", level.ID).
		Order("quiz_index ASC").
		Find(&quizzes).
		Error; err != nil {
		return LevelBootstrap{}, err
	}

	items := make([]QuizItem, 0, len(quizzes))
	for _, quiz := range quizzes {
		var options []string
		if len(quiz.OptionsMarkdown) > 0 {
			if err := json.Unmarshal(quiz.OptionsMarkdown, &options); err != nil {
				return LevelBootstrap{}, err
			}
		}
		items = append(items, QuizItem{
			ID:               quiz.ID,
			QuizIndex:        quiz.QuizIndex,
			QuizType:         quiz.QuizType,
			QuestionMarkdown: quiz.QuestionMarkdown,
			OptionsMarkdown:  options,
			AnswerIndex:      quiz.AnswerIndex,
		})
	}

	return LevelBootstrap{
		LevelID:             level.ID,
		UserID:              *level.UserID,
		DocumentID:          level.DocumentID,
		ChapterID:           level.ChapterID,
		SubChapterID:        level.SubChapterID,
		GenerationID:        *level.GenerationID,
		MapSeed:             *level.MapSeed,
		MapAlgorithmVersion: *level.MapAlgorithmVersion,
		SummaryMarkdown:     level.SummaryMarkdown,
		Quizzes:             items,
	}, nil
}

func (r *LevelRepository) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (r *LevelRepository) SaveQuizMistake(ctx context.Context, input QuizMistakeInput) error {
	optionsJSON, err := json.Marshal(input.OptionsMarkdown)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(&models.QuizMistake{
		ID:               uuid.NewString(),
		UserID:           input.UserID,
		LevelID:          input.LevelID,
		GenerationID:     input.GenerationID,
		QuizID:           input.QuizID,
		QuizIndex:        input.QuizIndex,
		QuizType:         input.QuizType,
		QuestionMarkdown: input.QuestionMarkdown,
		OptionsMarkdown:  optionsJSON,
		AnswerIndex:      input.AnswerIndex,
		SelectedIndex:    input.SelectedIndex,
	}).Error
}

func (r *LevelRepository) CreateGeneration(ctx context.Context, generation models.LevelGenerationRecord) error {
	return r.db.WithContext(ctx).Create(&generation).Error
}

func (r *LevelRepository) GetGeneration(ctx context.Context, generationID string) (models.LevelGenerationRecord, error) {
	var generation models.LevelGenerationRecord
	err := r.db.WithContext(ctx).
		Where("id = ?", generationID).
		First(&generation).
		Error
	if err == gorm.ErrRecordNotFound {
		return models.LevelGenerationRecord{}, models.ErrGenerationNotFound
	}
	if err != nil {
		return models.LevelGenerationRecord{}, err
	}
	return generation, nil
}

func (r *LevelRepository) GetGenerationByIdempotencyKey(ctx context.Context, idempotencyKey string) (models.LevelGenerationRecord, error) {
	var generation models.LevelGenerationRecord
	err := r.db.WithContext(ctx).
		Where("idempotency_key = ?", idempotencyKey).
		First(&generation).
		Error
	if err == gorm.ErrRecordNotFound {
		return models.LevelGenerationRecord{}, models.ErrGenerationNotFound
	}
	if err != nil {
		return models.LevelGenerationRecord{}, err
	}
	return generation, nil
}

func (r *LevelRepository) FindReusableLevelWithQuizzes(ctx context.Context, userID string, subChapterID string) (models.ReusableLevel, error) {
	var row struct {
		LevelID                string
		UserID                 string
		DocumentID             string
		SubChapterID           string
		GenerationID           *string
		MapSeed                *int64
		MapAlgorithmVersion    *int
		GenerationRecordExists bool
		QuizCount              int
	}
	result := r.db.WithContext(ctx).
		Table("levels").
		Select(`
			levels.id AS level_id,
			levels.user_id AS user_id,
			levels.document_id AS document_id,
			levels.sub_chapter_id AS sub_chapter_id,
			levels.generation_id AS generation_id,
			levels.map_seed AS map_seed,
			levels.map_algorithm_version AS map_algorithm_version,
			level_generation_jobs.id IS NOT NULL AS generation_record_exists,
			(SELECT COUNT(*) FROM quizzes WHERE quizzes.level_id = levels.id) AS quiz_count
		`).
		Joins("LEFT JOIN level_generation_jobs ON level_generation_jobs.id = levels.generation_id").
		Where("levels.user_id = ? AND levels.sub_chapter_id = ?", userID, subChapterID).
		Where("EXISTS (SELECT 1 FROM quizzes WHERE quizzes.level_id = levels.id)").
		Order("levels.created_at DESC NULLS LAST, levels.id DESC").
		Limit(1).
		Scan(&row)
	if result.Error != nil {
		return models.ReusableLevel{}, result.Error
	}
	if result.RowsAffected == 0 || row.QuizCount == 0 {
		return models.ReusableLevel{}, models.ErrLevelNotFound
	}
	return models.ReusableLevel{
		LevelID:                row.LevelID,
		UserID:                 row.UserID,
		DocumentID:             row.DocumentID,
		SubChapterID:           row.SubChapterID,
		GenerationID:           row.GenerationID,
		MapSeed:                row.MapSeed,
		MapAlgorithmVersion:    row.MapAlgorithmVersion,
		GenerationRecordExists: row.GenerationRecordExists,
		QuizCount:              row.QuizCount,
	}, nil
}

func (r *LevelRepository) AttachGenerationToLevel(ctx context.Context, generationID string, levelID string, options LevelInsertOptions) (SavedLevel, error) {
	var saved SavedLevel
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var generationRecord models.LevelGenerationRecord
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", generationID).
			First(&generationRecord).
			Error
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("level generation %s not found", generationID)
		}
		if err != nil {
			return err
		}
		if generationRecord.LevelID != nil && *generationRecord.LevelID != "" {
			existing, err := savedLevel(tx, *generationRecord.LevelID)
			if err != nil {
				return err
			}
			saved = existing
			return nil
		}

		var level models.Level
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", levelID).
			First(&level).
			Error
		if err == gorm.ErrRecordNotFound {
			return models.ErrLevelNotFound
		}
		if err != nil {
			return err
		}
		if level.UserID == nil ||
			*level.UserID != generationRecord.UserID ||
			level.SubChapterID != generationRecord.SubChapterID {
			return models.ErrLevelNotFound
		}

		hasValidMapBinding := false
		if level.GenerationID != nil &&
			*level.GenerationID != "" &&
			level.MapSeed != nil &&
			level.MapAlgorithmVersion != nil {
			var generationCount int64
			if err := tx.Model(&models.LevelGenerationRecord{}).
				Where("id = ?", *level.GenerationID).
				Count(&generationCount).
				Error; err != nil {
				return err
			}
			hasValidMapBinding = generationCount > 0
		}
		if !hasValidMapBinding {
			updates := map[string]any{
				"generation_id":         generationID,
				"map_seed":              options.MapSeed,
				"map_algorithm_version": options.MapAlgorithmVersion,
			}
			if err := tx.Model(&models.Level{}).Where("id = ?", level.ID).Updates(updates).Error; err != nil {
				return err
			}
		}

		generationUpdates := map[string]any{"level_id": level.ID}
		if hasValidMapBinding {
			generationUpdates["map_seed"] = *level.MapSeed
			generationUpdates["map_algorithm_version"] = *level.MapAlgorithmVersion
		}
		result := tx.Model(&models.LevelGenerationRecord{}).
			Where("id = ?", generationID).
			Updates(generationUpdates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("level generation %s not found", generationID)
		}

		existing, err := savedLevel(tx, level.ID)
		if err != nil {
			return err
		}
		saved = existing
		return nil
	})
	if err != nil {
		return SavedLevel{}, err
	}
	return saved, nil
}

func (r *LevelRepository) Insert(
	ctx context.Context,
	sourceContext source.SourceContext,
	generation generator.LevelGeneration,
	model string,
	options LevelInsertOptions,
) (SavedLevel, error) {
	levelID := uuid.NewString()

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
		UserID:          &sourceContext.DBUserID,
		DocumentID:      sourceContext.DocumentID,
		ChapterID:       sourceContext.ChapterID,
		SubChapterID:    sourceContext.SubChapterID,
		SummaryMarkdown: generation.SummaryMarkdown,
		SourceChunkIDs:  chunkIDs,
		SourceMetadata:  sourceMetadata,
		Model:           model,
	}
	if options.GenerationID != "" {
		level.GenerationID = &options.GenerationID
	}
	if options.MapSeed != 0 || options.MapAlgorithmVersion != 0 {
		level.MapSeed = &options.MapSeed
		level.MapAlgorithmVersion = &options.MapAlgorithmVersion
	}

	quizzes := make([]models.Quiz, 0, len(generation.Quizzes))
	for index, quiz := range generation.Quizzes {
		optionsJSON, err := json.Marshal(quiz.OptionsMarkdown)
		if err != nil {
			return SavedLevel{}, err
		}
		quizzes = append(quizzes, models.Quiz{
			ID:               uuid.NewString(),
			LevelID:          levelID,
			QuizIndex:        index,
			QuizType:         quiz.QuizType,
			QuestionMarkdown: quiz.QuestionMarkdown,
			OptionsMarkdown:  optionsJSON,
			AnswerIndex:      quiz.AnswerIndex,
		})
	}

	var saved SavedLevel
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if options.GenerationID != "" {
			var generationRecord models.LevelGenerationRecord
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("id = ?", options.GenerationID).
				First(&generationRecord).
				Error
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("level generation %s not found", options.GenerationID)
			}
			if err != nil {
				return err
			}
			if generationRecord.LevelID != nil && *generationRecord.LevelID != "" {
				existing, err := savedLevel(tx, *generationRecord.LevelID)
				if err != nil {
					return err
				}
				saved = existing
				return nil
			}
		}

		if err := tx.Create(&level).Error; err != nil {
			return err
		}
		if len(quizzes) > 0 {
			if err := tx.Create(&quizzes).Error; err != nil {
				return err
			}
		}
		if options.GenerationID != "" {
			result := tx.Model(&models.LevelGenerationRecord{}).
				Where("id = ?", options.GenerationID).
				Updates(map[string]any{"level_id": levelID})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return fmt.Errorf("level generation %s not found", options.GenerationID)
			}
		}
		saved = SavedLevel{
			LevelID:      levelID,
			SubChapterID: sourceContext.SubChapterID,
			DocumentID:   sourceContext.DocumentID,
			QuizCount:    len(generation.Quizzes),
			Model:        model,
		}
		return nil
	})
	if err != nil {
		return SavedLevel{}, err
	}
	return saved, nil
}

func savedLevel(db *gorm.DB, levelID string) (SavedLevel, error) {
	var level models.Level
	if err := db.Where("id = ?", levelID).First(&level).Error; err != nil {
		return SavedLevel{}, err
	}
	var quizCount int64
	if err := db.Model(&models.Quiz{}).Where("level_id = ?", levelID).Count(&quizCount).Error; err != nil {
		return SavedLevel{}, err
	}
	return SavedLevel{
		LevelID:      level.ID,
		SubChapterID: level.SubChapterID,
		DocumentID:   level.DocumentID,
		QuizCount:    int(quizCount),
		Model:        level.Model,
	}, nil
}
