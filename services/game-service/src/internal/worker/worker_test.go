package worker

import (
	"context"
	"fmt"
	"testing"

	"skybloom/game-service/internal/config"
	"skybloom/game-service/internal/generator"
	"skybloom/game-service/internal/models"
	"skybloom/game-service/internal/quizcache"
	"skybloom/game-service/internal/repository"
	"skybloom/game-service/internal/source"
)

func TestProcessQuizJobReusesExistingQuizzes(t *testing.T) {
	ctx := context.Background()
	repo := &fakeLevelRepository{
		reusable: &models.ReusableLevel{
			LevelID:      "11111111-1111-1111-1111-111111111111",
			UserID:       "user-1",
			SubChapterID: "sub-1",
			QuizCount:    10,
		},
	}
	sources := &fakeSourceFetcher{}
	generatorClient := &fakeLevelGenerator{}
	statuses := &fakeGenerationStatusStore{}
	quizCache := &fakeQuizCache{}
	worker := NewWithStoresAndQuizCache(config.Config{}, repo, sources, generatorClient, statuses, NoopMapCache{}, quizCache)

	job := quizJob()
	if err := worker.ProcessQuizJob(ctx, job); err != nil {
		t.Fatalf("ProcessQuizJob failed: %v", err)
	}

	if sources.fetchCount != 0 {
		t.Fatalf("expected no source fetch, got %d", sources.fetchCount)
	}
	if generatorClient.generateCount != 0 {
		t.Fatalf("expected no quiz generation, got %d", generatorClient.generateCount)
	}
	if repo.insertCount != 0 {
		t.Fatalf("expected no level insert, got %d", repo.insertCount)
	}
	if repo.attachCount != 1 {
		t.Fatalf("expected one generation attach, got %d", repo.attachCount)
	}
	if statuses.lastStep != "quiz" || statuses.lastStatus != models.StepStatusComplete {
		t.Fatalf("expected quiz step complete, got step=%q status=%q", statuses.lastStep, statuses.lastStatus)
	}
	if statuses.lastUpdate.LevelID == nil || *statuses.lastUpdate.LevelID != repo.reusable.LevelID {
		t.Fatalf("expected status level_id %q, got %v", repo.reusable.LevelID, statuses.lastUpdate.LevelID)
	}
	if quizCache.setCount != 1 {
		t.Fatalf("expected quiz cache write, got %d", quizCache.setCount)
	}
	if quizCache.last.GenerationID != job.GenerationID {
		t.Fatalf("expected cached generation id %q, got %q", job.GenerationID, quizCache.last.GenerationID)
	}
}

func TestProcessQuizJobGeneratesQuizzesWhenNoneExist(t *testing.T) {
	ctx := context.Background()
	repo := &fakeLevelRepository{}
	sources := &fakeSourceFetcher{
		result: source.SourceContext{
			Status:       "retrieved",
			DBUserID:     "user-1",
			SubChapterID: "sub-1",
			DocumentID:   "document-1",
			ChapterID:    "chapter-1",
			SourceText:   "Lesson content",
		},
	}
	generatorClient := &fakeLevelGenerator{
		result: generator.LevelGeneration{
			SummaryMarkdown: "Summary",
			Quizzes: []generator.QuizItem{
				{
					QuizType:         "true_false",
					QuestionMarkdown: "Question",
					OptionsMarkdown:  []string{"True", "False"},
					AnswerIndex:      0,
				},
			},
		},
	}
	statuses := &fakeGenerationStatusStore{}
	quizCache := &fakeQuizCache{}
	worker := NewWithStoresAndQuizCache(config.Config{}, repo, sources, generatorClient, statuses, NoopMapCache{}, quizCache)

	job := quizJob()
	if err := worker.ProcessQuizJob(ctx, job); err != nil {
		t.Fatalf("ProcessQuizJob failed: %v", err)
	}

	if sources.fetchCount != 1 {
		t.Fatalf("expected one source fetch, got %d", sources.fetchCount)
	}
	if generatorClient.generateCount != 1 {
		t.Fatalf("expected one quiz generation, got %d", generatorClient.generateCount)
	}
	if repo.insertCount != 1 {
		t.Fatalf("expected one level insert, got %d", repo.insertCount)
	}
	if repo.attachCount != 0 {
		t.Fatalf("expected no generation attach, got %d", repo.attachCount)
	}
	if statuses.lastStep != "quiz" || statuses.lastStatus != models.StepStatusComplete {
		t.Fatalf("expected quiz step complete, got step=%q status=%q", statuses.lastStep, statuses.lastStatus)
	}
	if quizCache.setCount != 1 {
		t.Fatalf("expected quiz cache write, got %d", quizCache.setCount)
	}
	if len(quizCache.last.Quizzes) != len(generatorClient.result.Quizzes) {
		t.Fatalf("expected %d cached quizzes, got %d", len(generatorClient.result.Quizzes), len(quizCache.last.Quizzes))
	}
	if quizCache.last.GenerationID != job.GenerationID {
		t.Fatalf("expected cached generation id %q, got %q", job.GenerationID, quizCache.last.GenerationID)
	}
}

func quizJob() models.LevelJob {
	return models.LevelJob{
		JobType:             models.JobTypeQuizGenerate,
		TaskID:              "task-1",
		GenerationID:        "generation-1",
		UserID:              "user-1",
		SubChapterID:        "sub-1",
		MapSeed:             123,
		MapAlgorithmVersion: 1,
	}
}

type fakeLevelRepository struct {
	reusable    *models.ReusableLevel
	attachCount int
	insertCount int
	bootstraps  map[string]repository.LevelBootstrap
}

func (r *fakeLevelRepository) FindReusableLevelWithQuizzes(_ context.Context, userID string, subChapterID string) (models.ReusableLevel, error) {
	if r.reusable == nil || r.reusable.UserID != userID || r.reusable.SubChapterID != subChapterID {
		return models.ReusableLevel{}, models.ErrLevelNotFound
	}
	return *r.reusable, nil
}

func (r *fakeLevelRepository) AttachGenerationToLevel(_ context.Context, generationID string, levelID string, _ repository.LevelInsertOptions) (repository.SavedLevel, error) {
	r.attachCount++
	r.ensureBootstraps()
	if _, ok := r.bootstraps[levelID]; !ok {
		r.bootstraps[levelID] = repository.LevelBootstrap{
			LevelID:             levelID,
			UserID:              r.reusable.UserID,
			DocumentID:          r.reusable.DocumentID,
			SubChapterID:        r.reusable.SubChapterID,
			GenerationID:        generationID,
			MapSeed:             123,
			MapAlgorithmVersion: 1,
			Quizzes:             fakeRepositoryQuizzes(r.reusable.QuizCount),
		}
	}
	return repository.SavedLevel{
		LevelID:      levelID,
		SubChapterID: "sub-1",
		DocumentID:   "document-1",
		QuizCount:    10,
	}, nil
}

func (r *fakeLevelRepository) Insert(_ context.Context, sourceContext source.SourceContext, generation generator.LevelGeneration, model string, options repository.LevelInsertOptions) (repository.SavedLevel, error) {
	r.insertCount++
	levelID := "22222222-2222-2222-2222-222222222222"
	r.ensureBootstraps()
	quizzes := make([]repository.QuizItem, 0, len(generation.Quizzes))
	for index, quiz := range generation.Quizzes {
		quizzes = append(quizzes, repository.QuizItem{
			ID:               fmt.Sprintf("quiz-%d", index),
			QuizIndex:        index,
			QuizType:         quiz.QuizType,
			QuestionMarkdown: quiz.QuestionMarkdown,
			OptionsMarkdown:  quiz.OptionsMarkdown,
			AnswerIndex:      quiz.AnswerIndex,
		})
	}
	r.bootstraps[levelID] = repository.LevelBootstrap{
		LevelID:             levelID,
		UserID:              sourceContext.DBUserID,
		DocumentID:          sourceContext.DocumentID,
		ChapterID:           sourceContext.ChapterID,
		SubChapterID:        sourceContext.SubChapterID,
		GenerationID:        options.GenerationID,
		MapSeed:             options.MapSeed,
		MapAlgorithmVersion: options.MapAlgorithmVersion,
		SummaryMarkdown:     generation.SummaryMarkdown,
		Quizzes:             quizzes,
	}
	return repository.SavedLevel{
		LevelID:      levelID,
		SubChapterID: sourceContext.SubChapterID,
		DocumentID:   sourceContext.DocumentID,
		QuizCount:    len(generation.Quizzes),
		Model:        model,
	}, nil
}

func (r *fakeLevelRepository) GetBootstrap(_ context.Context, levelID string, userID string) (repository.LevelBootstrap, error) {
	r.ensureBootstraps()
	bootstrap, ok := r.bootstraps[levelID]
	if !ok || bootstrap.UserID != userID {
		return repository.LevelBootstrap{}, models.ErrLevelNotFound
	}
	return bootstrap, nil
}

func (r *fakeLevelRepository) ensureBootstraps() {
	if r.bootstraps == nil {
		r.bootstraps = map[string]repository.LevelBootstrap{}
	}
}

func fakeRepositoryQuizzes(count int) []repository.QuizItem {
	quizzes := make([]repository.QuizItem, 0, count)
	for index := 0; index < count; index++ {
		quizzes = append(quizzes, repository.QuizItem{
			ID:               fmt.Sprintf("quiz-%d", index),
			QuizIndex:        index,
			QuizType:         "true_false",
			QuestionMarkdown: "Question",
			OptionsMarkdown:  []string{"True", "False"},
			AnswerIndex:      index % 2,
		})
	}
	return quizzes
}

type fakeSourceFetcher struct {
	result     source.SourceContext
	fetchCount int
}

func (f *fakeSourceFetcher) FetchSubChapterContent(context.Context, string, string) (source.SourceContext, error) {
	f.fetchCount++
	return f.result, nil
}

type fakeLevelGenerator struct {
	result        generator.LevelGeneration
	generateCount int
}

func (g *fakeLevelGenerator) GenerateLevel(context.Context, source.SourceContext) (generator.LevelGeneration, error) {
	g.generateCount++
	return g.result, nil
}

type fakeGenerationStatusStore struct {
	lastStep   string
	lastStatus string
	lastUpdate models.StatusUpdate
}

func (s *fakeGenerationStatusStore) MarkStep(_ context.Context, _ models.LevelJob, step string, stepStatus string, update models.StatusUpdate) error {
	s.lastStep = step
	s.lastStatus = stepStatus
	s.lastUpdate = update
	return nil
}

type fakeQuizCache struct {
	setCount int
	last     quizcache.LevelQuizzes
}

func (c *fakeQuizCache) Set(_ context.Context, _ string, quizzes quizcache.LevelQuizzes) error {
	c.setCount++
	c.last = quizzes
	return nil
}
