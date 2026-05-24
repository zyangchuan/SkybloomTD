package main_test

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"skybloom/level-generator-worker/internal/config"
	"skybloom/level-generator-worker/internal/generator"
	"skybloom/level-generator-worker/internal/models"
	"skybloom/level-generator-worker/internal/repository"
	"skybloom/level-generator-worker/internal/source"
	"skybloom/level-generator-worker/internal/worker"
	"skybloom/level-generator-worker/internal/worker/mocks"
)

func TestProcessJobFetchesGeneratesAndSavesLevel(t *testing.T) {
	levels := mocks.NewMockLevelRepository(t)
	sources := mocks.NewMockSourceFetcher(t)
	levelGenerator := mocks.NewMockLevelGenerator(t)
	workerService := newTestWorker(t, levels, sources, levelGenerator)

	job := models.LevelJob{
		TaskID:       "task-1",
		UserID:       "user-1",
		SubChapterID: "sub-chapter-1",
	}
	sourceContext := source.SourceContext{
		Status:       "retrieved",
		UserID:       "user-1",
		SubChapterID: "sub-chapter-1",
		SourceText:   "source text",
	}
	generation := generator.LevelGeneration{
		SummaryMarkdown: "summary",
		Quizzes: []generator.QuizItem{
			{
				QuizType:         "true_false",
				QuestionMarkdown: "Question?",
				OptionsMarkdown:  []string{"True", "False"},
				AnswerIndex:      0,
			},
		},
	}
	saved := repository.SavedLevel{
		LevelID:      "level-1",
		SubChapterID: "sub-chapter-1",
		DocumentID:   "document-1",
		QuizCount:    1,
		Model:        "test-model",
	}

	sources.
		On("FetchSubChapterContent", mock.Anything, "user-1", "sub-chapter-1").
		Return(sourceContext, nil).
		Once()
	levelGenerator.
		On("GenerateLevel", mock.Anything, sourceContext).
		Return(generation, nil).
		Once()
	levels.
		On("Insert", mock.Anything, sourceContext, generation, "test-model").
		Return(saved, nil).
		Once()

	err := workerService.ProcessJob(context.Background(), job)

	require.NoError(t, err)
}

func TestProcessJobStopsWhenSourceFetchFails(t *testing.T) {
	levels := mocks.NewMockLevelRepository(t)
	sources := mocks.NewMockSourceFetcher(t)
	levelGenerator := mocks.NewMockLevelGenerator(t)
	workerService := newTestWorker(t, levels, sources, levelGenerator)
	job := models.LevelJob{UserID: "user-1", SubChapterID: "sub-chapter-1"}

	sources.
		On("FetchSubChapterContent", mock.Anything, "user-1", "sub-chapter-1").
		Return(source.SourceContext{}, errors.New("source unavailable")).
		Once()

	err := workerService.ProcessJob(context.Background(), job)

	require.Error(t, err)
	assert.ErrorContains(t, err, "source unavailable")
	levelGenerator.AssertNotCalled(t, "GenerateLevel", mock.Anything, mock.Anything)
	levels.AssertNotCalled(t, "Insert", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestProcessJobStopsWhenGenerationFails(t *testing.T) {
	levels := mocks.NewMockLevelRepository(t)
	sources := mocks.NewMockSourceFetcher(t)
	levelGenerator := mocks.NewMockLevelGenerator(t)
	workerService := newTestWorker(t, levels, sources, levelGenerator)
	job := models.LevelJob{UserID: "user-1", SubChapterID: "sub-chapter-1"}
	sourceContext := source.SourceContext{Status: "retrieved", UserID: "user-1", SubChapterID: "sub-chapter-1"}

	sources.
		On("FetchSubChapterContent", mock.Anything, "user-1", "sub-chapter-1").
		Return(sourceContext, nil).
		Once()
	levelGenerator.
		On("GenerateLevel", mock.Anything, sourceContext).
		Return(generator.LevelGeneration{}, errors.New("generation failed")).
		Once()

	err := workerService.ProcessJob(context.Background(), job)

	require.Error(t, err)
	assert.ErrorContains(t, err, "generation failed")
	levels.AssertNotCalled(t, "Insert", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestProcessJobReturnsRepositoryError(t *testing.T) {
	levels := mocks.NewMockLevelRepository(t)
	sources := mocks.NewMockSourceFetcher(t)
	levelGenerator := mocks.NewMockLevelGenerator(t)
	workerService := newTestWorker(t, levels, sources, levelGenerator)
	job := models.LevelJob{UserID: "user-1", SubChapterID: "sub-chapter-1"}
	sourceContext := source.SourceContext{Status: "retrieved", UserID: "user-1", SubChapterID: "sub-chapter-1"}
	generation := generator.LevelGeneration{SummaryMarkdown: "summary"}

	sources.
		On("FetchSubChapterContent", mock.Anything, "user-1", "sub-chapter-1").
		Return(sourceContext, nil).
		Once()
	levelGenerator.
		On("GenerateLevel", mock.Anything, sourceContext).
		Return(generation, nil).
		Once()
	levels.
		On("Insert", mock.Anything, sourceContext, generation, "test-model").
		Return(repository.SavedLevel{}, errors.New("database unavailable")).
		Once()

	err := workerService.ProcessJob(context.Background(), job)

	require.Error(t, err)
	assert.ErrorContains(t, err, "database unavailable")
}

func newTestWorker(
	t *testing.T,
	levels worker.LevelRepository,
	sources worker.SourceFetcher,
	levelGenerator worker.LevelGenerator,
) *worker.Worker {
	t.Helper()
	previousLogWriter := log.Writer()
	log.SetOutput(io.Discard)
	t.Cleanup(func() {
		log.SetOutput(previousLogWriter)
	})
	return worker.New(config.Config{Model: "test-model"}, levels, sources, levelGenerator)
}
