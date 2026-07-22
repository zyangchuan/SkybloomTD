package controllers

import (
	"context"

	"github.com/google/uuid"

	"skybloom/document-content-api/internal/config"
	"skybloom/document-content-api/internal/models"
)

type Controller struct {
	config     config.Config
	publisher  Publisher
	storage    StorageClient
	documents  DocumentStore
	taskStatus TaskStatusStore
}

type Publisher interface {
	Publish(ctx context.Context, messageID string, value any) error
}

type StorageClient interface {
	UploadSource(
		ctx context.Context,
		content []byte,
		userID string,
		documentID string,
		filename string,
		contentType string,
	) (models.SourceRef, error)
	DeleteDocumentFiles(ctx context.Context, document models.Document) error
}

type DocumentStore interface {
	CreateQueuedDocument(ctx context.Context, document models.Document) error
	ListUserDocuments(ctx context.Context, userID uuid.UUID) ([]models.DocumentSummary, error)
	ListPublicGames(ctx context.Context, userID uuid.UUID, cursor string) (models.ListGameLibraryResponse, error)
	ListStarredGames(ctx context.Context, userID uuid.UUID, cursor string) (models.ListGameLibraryResponse, error)
	SetDocumentVisibility(ctx context.Context, documentID uuid.UUID, userID uuid.UUID, isPublic bool) (models.DocumentSummary, error)
	StarGame(ctx context.Context, documentID uuid.UUID, userID uuid.UUID) error
	UnstarGame(ctx context.Context, documentID uuid.UUID, userID uuid.UUID) error
	ListDocumentChapters(ctx context.Context, documentID uuid.UUID, userID uuid.UUID) ([]models.ChapterSummary, error)
	ListChapterSubChapters(ctx context.Context, chapterID uuid.UUID, userID uuid.UUID) ([]models.SubChapterSummary, error)
	LoadUserDocument(ctx context.Context, documentID uuid.UUID, userID uuid.UUID) (models.Document, error)
	DeleteDocumentCascade(ctx context.Context, documentID uuid.UUID, userID uuid.UUID) error
}

type TaskStatusStore interface {
	Set(ctx context.Context, status models.TaskStatus) error
	Get(ctx context.Context, taskID string) (models.TaskStatus, error)
}

func NewController(
	cfg config.Config,
	publisher Publisher,
	storageClient StorageClient,
	documentStore DocumentStore,
	taskStatusStore TaskStatusStore,
) *Controller {
	return &Controller{
		config:     cfg,
		publisher:  publisher,
		storage:    storageClient,
		documents:  documentStore,
		taskStatus: taskStatusStore,
	}
}
