package worker

import (
	"context"
	"testing"

	"skybloom/game-service/internal/config"
	"skybloom/game-service/internal/generator"
	"skybloom/game-service/internal/mapgen"
	"skybloom/game-service/internal/models"
	"skybloom/game-service/internal/quizcache"
	"skybloom/game-service/internal/repository"
	"skybloom/game-service/internal/source"
)

func TestProcessMapJobStoresGeneratedMapAndMarksStatus(t *testing.T) {
	statuses := &fakeWorkerStatusStore{}
	maps := &fakeMapCache{}
	w := NewWithStoresAndQuizCache(config.Config{}, &fakeLevelRepository{}, &fakeSourceFetcher{}, &fakeLevelGenerator{}, statuses, maps, &fakeQuizCache{})
	job := models.LevelJob{
		JobType:             models.JobTypeMapGenerate,
		TaskID:              "task-1",
		GenerationID:        "generation-1",
		UserID:              "user-1",
		SubChapterID:        "subchapter-1",
		MapSeed:             42,
		MapAlgorithmVersion: mapgen.Version,
	}

	if err := w.ProcessJob(context.Background(), job); err != nil {
		t.Fatalf("ProcessJob failed: %v", err)
	}

	if maps.generationID != job.GenerationID ||
		maps.levelMap.Seed != job.MapSeed ||
		maps.levelMap.Version != job.MapAlgorithmVersion ||
		len(maps.levelMap.EnemyPath) == 0 {
		t.Fatalf("generated map was not stored correctly: %#v", maps)
	}
	assertWorkerStatusSteps(t, statuses.calls, "map", models.StepStatusRunning, models.StepStatusComplete)
}

func TestProcessQuizJobGeneratesLevelCachesQuizzesAndMarksStatus(t *testing.T) {
	levels := &fakeLevelRepository{
		reusableErr: models.ErrLevelNotFound,
		saved:       repository.SavedLevel{LevelID: "level-1", SubChapterID: "subchapter-1", QuizCount: 1},
		bootstrap: repository.LevelBootstrap{
			LevelID:      "level-1",
			UserID:       "user-1",
			GenerationID: "generation-1",
			Quizzes: []repository.QuizItem{{
				ID:               "quiz-1",
				QuizIndex:        0,
				QuizType:         "multiple_choice",
				QuestionMarkdown: "Question?",
				OptionsMarkdown:  []string{"A", "B"},
				AnswerIndex:      1,
			}},
		},
	}
	sources := &fakeSourceFetcher{source: source.SourceContext{
		Status:       "retrieved",
		UserID:       "user-1",
		SubChapterID: "subchapter-1",
		SourceText:   "lesson text",
	}}
	generatorClient := &fakeLevelGenerator{generation: generator.LevelGeneration{
		SummaryMarkdown: "Summary",
		Quizzes: []generator.QuizItem{{
			QuizType:              "multiple_choice",
			QuestionMarkdown:      "Question?",
			OptionsMarkdown:       []string{"A", "B"},
			AnswerIndex:           1,
			CorrectOptionMarkdown: "B",
		}},
	}}
	statuses := &fakeWorkerStatusStore{}
	quizzes := &fakeQuizCache{}
	w := NewWithStoresAndQuizCache(config.Config{Model: "gpt-test"}, levels, sources, generatorClient, statuses, &fakeMapCache{}, quizzes)
	job := models.LevelJob{
		JobType:             models.JobTypeQuizGenerate,
		TaskID:              "task-1",
		GenerationID:        "generation-1",
		UserID:              "user-1",
		SubChapterID:        "subchapter-1",
		MapSeed:             42,
		MapAlgorithmVersion: mapgen.Version,
	}

	if err := w.ProcessJob(context.Background(), job); err != nil {
		t.Fatalf("ProcessJob failed: %v", err)
	}

	if !sources.called || !generatorClient.called || levels.inserted.GenerationID != job.GenerationID {
		t.Fatalf("quiz generation pipeline did not call expected modules")
	}
	if quizzes.generationID != job.GenerationID || len(quizzes.quizzes.Quizzes) != 1 {
		t.Fatalf("quizzes were not cached correctly: %#v", quizzes)
	}
	assertWorkerStatusSteps(t, statuses.calls, "quiz", models.StepStatusRunning, models.StepStatusComplete)
}

func TestProcessQuizRefillCachesBeforeDatabaseAppend(t *testing.T) {
	levels := &fakeLevelRepository{
		bootstrap: repository.LevelBootstrap{
			LevelID:      "level-1",
			UserID:       "user-1",
			GenerationID: "generation-1",
			Quizzes: []repository.QuizItem{{
				ID:               "quiz-1",
				QuizIndex:        0,
				QuizType:         "mcq",
				QuestionMarkdown: "Existing?",
				OptionsMarkdown:  []string{"A", "B", "C"},
				AnswerIndex:      1,
			}},
		},
		appendErr: context.Canceled,
	}
	sources := &fakeSourceFetcher{source: source.SourceContext{
		Status:       "retrieved",
		UserID:       "user-1",
		SubChapterID: "subchapter-1",
		SourceText:   "lesson text",
	}}
	generatorClient := &fakeLevelGenerator{generation: generator.LevelGeneration{
		SummaryMarkdown: "Summary",
		Quizzes: []generator.QuizItem{{
			QuizType:              "mcq",
			QuestionMarkdown:      "Refill?",
			OptionsMarkdown:       []string{"A", "B", "C"},
			AnswerIndex:           1,
			CorrectOptionMarkdown: "B",
		}},
	}}
	quizzes := &fakeQuizCache{}
	w := NewWithStoresAndQuizCache(config.Config{}, levels, sources, generatorClient, &fakeWorkerStatusStore{}, &fakeMapCache{}, quizzes)
	job := models.LevelJob{
		JobType:      models.JobTypeQuizRefill,
		TaskID:       "refill-1",
		GenerationID: "generation-1",
		LevelID:      "level-1",
		UserID:       "user-1",
		SubChapterID: "subchapter-1",
	}

	if err := w.ProcessJob(context.Background(), job); err != nil {
		t.Fatalf("ProcessJob failed: %v", err)
	}

	if quizzes.generationID != job.GenerationID || len(quizzes.quizzes.Quizzes) != 1 {
		t.Fatalf("refill quizzes were not cached before db append: %#v", quizzes)
	}
	if quizzes.quizzes.Quizzes[0].QuestionMarkdown != "Refill?" {
		t.Fatalf("unexpected cached refill quiz: %#v", quizzes.quizzes.Quizzes[0])
	}
}

func assertWorkerStatusSteps(t *testing.T, calls []workerStatusCall, step string, statuses ...string) {
	t.Helper()

	if len(calls) != len(statuses) {
		t.Fatalf("expected %d status calls, got %#v", len(statuses), calls)
	}
	for index, want := range statuses {
		if calls[index].step != step || calls[index].status != want {
			t.Fatalf("expected %s/%s at index %d, got %#v", step, want, index, calls[index])
		}
	}
}

type workerStatusCall struct {
	step   string
	status string
	update models.StatusUpdate
}

type fakeWorkerStatusStore struct {
	calls []workerStatusCall
}

func (s *fakeWorkerStatusStore) MarkStep(_ context.Context, _ models.LevelJob, step string, stepStatus string, update models.StatusUpdate) error {
	s.calls = append(s.calls, workerStatusCall{step: step, status: stepStatus, update: update})
	return nil
}

type fakeMapCache struct {
	generationID string
	levelMap     mapgen.GeneratedMap
}

func (c *fakeMapCache) Set(_ context.Context, generationID string, levelMap mapgen.GeneratedMap) error {
	c.generationID = generationID
	c.levelMap = levelMap
	return nil
}

type fakeQuizCache struct {
	generationID string
	quizzes      quizcache.LevelQuizzes
}

func (c *fakeQuizCache) Get(_ context.Context, generationID string) (quizcache.LevelQuizzes, error) {
	if c.generationID == generationID && len(c.quizzes.Quizzes) > 0 {
		return c.quizzes, nil
	}
	return quizcache.LevelQuizzes{}, quizcache.ErrQuizzesNotFound
}

func (c *fakeQuizCache) Set(_ context.Context, generationID string, quizzes quizcache.LevelQuizzes) error {
	c.generationID = generationID
	c.quizzes = quizzes
	return nil
}

func (c *fakeQuizCache) Append(_ context.Context, generationID string, quizzes []quizcache.CachedQuiz) (int, error) {
	c.generationID = generationID
	c.quizzes.Quizzes = append(c.quizzes.Quizzes, quizzes...)
	return len(c.quizzes.Quizzes), nil
}

func (c *fakeQuizCache) ReleaseRefillLease(_ context.Context, _ string, _ string) error {
	return nil
}

type fakeLevelRepository struct {
	reusableErr error
	saved       repository.SavedLevel
	bootstrap   repository.LevelBootstrap
	inserted    repository.LevelInsertOptions
	appendErr   error
}

func (r *fakeLevelRepository) AttachGenerationToLevel(_ context.Context, _ string, _ string, options repository.LevelInsertOptions) (repository.SavedLevel, error) {
	r.inserted = options
	return r.saved, nil
}

func (r *fakeLevelRepository) AppendQuizzes(_ context.Context, levelID string, _ string, generation generator.LevelGeneration, _ int) (repository.QuizAppendResult, error) {
	if r.appendErr != nil {
		return repository.QuizAppendResult{}, r.appendErr
	}
	items := make([]repository.QuizItem, 0, len(generation.Quizzes))
	for index, quiz := range generation.Quizzes {
		items = append(items, repository.QuizItem{
			ID:               "refill-quiz",
			QuizIndex:        index,
			QuizType:         quiz.QuizType,
			QuestionMarkdown: quiz.QuestionMarkdown,
			OptionsMarkdown:  quiz.OptionsMarkdown,
			AnswerIndex:      quiz.AnswerIndex,
		})
	}
	return repository.QuizAppendResult{
		LevelID:      levelID,
		Appended:     len(items),
		TotalQuizzes: len(items),
		Quizzes:      items,
	}, nil
}

func (r *fakeLevelRepository) FindReusableLevelWithQuizzes(_ context.Context, _ string, _ string) (models.ReusableLevel, error) {
	if r.reusableErr != nil {
		return models.ReusableLevel{}, r.reusableErr
	}
	return models.ReusableLevel{}, models.ErrLevelNotFound
}

func (r *fakeLevelRepository) GetBootstrap(_ context.Context, _ string, _ string) (repository.LevelBootstrap, error) {
	return r.bootstrap, nil
}

func (r *fakeLevelRepository) Insert(_ context.Context, _ source.SourceContext, _ generator.LevelGeneration, _ string, options repository.LevelInsertOptions) (repository.SavedLevel, error) {
	r.inserted = options
	return r.saved, nil
}

type fakeSourceFetcher struct {
	source source.SourceContext
	err    error
	called bool
}

func (f *fakeSourceFetcher) FetchSubChapterContent(_ context.Context, _ string, _ string) (source.SourceContext, error) {
	f.called = true
	if f.err != nil {
		return source.SourceContext{}, f.err
	}
	return f.source, nil
}

type fakeLevelGenerator struct {
	generation generator.LevelGeneration
	err        error
	called     bool
}

func (g *fakeLevelGenerator) GenerateLevel(_ context.Context, _ source.SourceContext) (generator.LevelGeneration, error) {
	g.called = true
	if g.err != nil {
		return generator.LevelGeneration{}, g.err
	}
	return g.generation, nil
}

func (g *fakeLevelGenerator) GenerateQuizRefill(_ context.Context, _ source.SourceContext, _ []generator.ExistingQuiz) (generator.LevelGeneration, error) {
	return g.GenerateLevel(context.Background(), source.SourceContext{})
}
