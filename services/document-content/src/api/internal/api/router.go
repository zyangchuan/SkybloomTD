package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"skybloom/document-content-api/internal/config"
	"skybloom/document-content-api/internal/models"
)

const maxUploadBytes = 100 << 20

var safePathPartPattern = regexp.MustCompile(`[^A-Za-z0-9_.=-]+`)

type Server struct {
	config     config.Config
	publisher  Publisher
	storage    SourceUploader
	assets     DocumentAssetDeleter
	documents  DocumentStore
	taskStatus TaskStatusStore
}

type Publisher interface {
	Publish(ctx context.Context, messageID string, value any) error
}

type SourceUploader interface {
	UploadSource(
		ctx context.Context,
		content []byte,
		userID string,
		documentID string,
		filename string,
		contentType string,
	) (models.SourceRef, error)
	DeleteDocumentAssets(ctx context.Context, document models.Document) error
}

type DocumentStore interface {
	CreateQueuedDocument(ctx context.Context, document models.Document) error
	ListUserDocuments(ctx context.Context, userID uuid.UUID) ([]models.DocumentSummary, error)
	LoadUserDocument(ctx context.Context, documentID uuid.UUID, userID uuid.UUID) (models.Document, error)
	DeleteDocumentCascade(ctx context.Context, documentID uuid.UUID, userID uuid.UUID) error
}

type DocumentAssetDeleter interface {
	DeleteDocumentAssets(ctx context.Context, document models.Document) error
}

type TaskStatusStore interface {
	Set(ctx context.Context, status models.TaskStatus) error
	Get(ctx context.Context, taskID string) (models.TaskStatus, error)
}

type noopDocumentStore struct{}

type noopTaskStatusStore struct{}

func NewRouter(
	cfg config.Config,
	publisher Publisher,
	storageClient SourceUploader,
	documents DocumentStore,
	taskStatus TaskStatusStore,
) *gin.Engine {
	if documents == nil {
		documents = noopDocumentStore{}
	}
	if taskStatus == nil {
		taskStatus = noopTaskStatusStore{}
	}
	assetDeleter, _ := storageClient.(DocumentAssetDeleter)

	server := &Server{
		config:     cfg,
		publisher:  publisher,
		storage:    storageClient,
		assets:     assetDeleter,
		documents:  documents,
		taskStatus: taskStatus,
	}

	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		log.Fatalf("proxy configuration error: %v", err)
	}
	router.Use(gin.Logger(), gin.Recovery())

	router.GET("/health", server.health)
	router.POST("/upload-file", server.uploadFile)
	router.GET("/documents", server.listDocuments)
	router.DELETE("/documents/:document_id", server.deleteDocument)
	router.GET("/tasks/:task_id/status", server.getTaskStatus)
	return router
}

func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) getTaskStatus(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_id is required"})
		return
	}

	status, err := s.taskStatus.Get(c.Request.Context(), taskID)
	if errors.Is(err, models.ErrTaskStatusNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "task status not found"})
		return
	}
	if err != nil {
		log.Printf("redis task status read failed: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to read task status"})
		return
	}

	c.JSON(http.StatusOK, status)
}

func (s *Server) listDocuments(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}

	documents, err := s.documents.ListUserDocuments(c.Request.Context(), models.DatabaseUUID(userID, "user"))
	if err != nil {
		log.Printf("document list failed: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to list documents"})
		return
	}

	c.JSON(http.StatusOK, models.ListDocumentsResponse{Documents: documents})
}

func (s *Server) deleteDocument(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}
	dbUserID := models.DatabaseUUID(userID, "user")

	documentID, err := uuid.Parse(strings.TrimSpace(c.Param("document_id")))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid document_id"})
		return
	}

	document, err := s.documents.LoadUserDocument(c.Request.Context(), documentID, dbUserID)
	if errors.Is(err, models.ErrDocumentNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
		return
	}
	if err != nil {
		log.Printf("document load failed: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to load document"})
		return
	}

	if err := s.deleteDocumentAssets(c.Request.Context(), document); err != nil {
		log.Printf("document asset deletion failed: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to delete document assets"})
		return
	}

	if err := s.documents.DeleteDocumentCascade(c.Request.Context(), documentID, dbUserID); err != nil {
		if errors.Is(err, models.ErrDocumentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
			return
		}
		log.Printf("document deletion failed: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to delete document"})
		return
	}

	c.Status(http.StatusNoContent)
}

func (s *Server) uploadFile(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)
	if err := c.Request.ParseMultipartForm(maxUploadBytes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multipart upload"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read upload"})
		return
	}

	documentID := uuid.NewString()
	taskID := randomHexID()
	filename := safeFilename(header.Filename)
	contentType := header.Header.Get("Content-Type")
	source, err := s.sourceForUpload(c.Request.Context(), content, userID, documentID, filename, contentType)
	if err != nil {
		log.Printf("source upload failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store upload"})
		return
	}

	document, err := models.NewQueuedDocument(documentID, userID, taskID, filename, source)
	if err != nil {
		log.Printf("document row build failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create document"})
		return
	}
	if err := s.documents.CreateQueuedDocument(c.Request.Context(), document); err != nil {
		log.Printf("document create failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create document"})
		return
	}

	if err := s.taskStatus.Set(c.Request.Context(), models.NewTaskStatus(taskID, documentID, models.TaskStatusQueued, nil)); err != nil {
		log.Printf("redis task status write failed: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to record task status"})
		return
	}

	ocrTaskID := randomHexID()
	uploadTaskID := randomHexID()
	indexTaskID := randomHexID()
	job := models.DocumentJob{
		JobType:      "document.process",
		TaskID:       taskID,
		OCRTaskID:    ocrTaskID,
		UploadTaskID: uploadTaskID,
		IndexTaskID:  indexTaskID,
		UserID:       userID,
		DocumentID:   documentID,
		Filename:     filename,
		Source:       source,
	}

	if err := s.publisher.Publish(c.Request.Context(), job.TaskID, job); err != nil {
		log.Printf("rabbitmq publish failed: %v", err)
		errorMessage := err.Error()
		if statusErr := s.taskStatus.Set(c.Request.Context(), models.NewTaskStatus(taskID, documentID, models.TaskStatusFailed, &errorMessage)); statusErr != nil {
			log.Printf("redis task status failure write failed: %v", statusErr)
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to enqueue document job"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "Upload file success",
		"task_id":        job.TaskID,
		"ocr_task_id":    ocrTaskID,
		"upload_task_id": uploadTaskID,
		"index_task_id":  indexTaskID,
		"user_id":        userID,
		"document_id":    documentID,
		"is_ready":       false,
	})
}

func (s *Server) sourceForUpload(
	ctx context.Context,
	content []byte,
	userID string,
	documentID string,
	filename string,
	contentType string,
) (any, error) {
	if s.storage != nil {
		source, err := s.storage.UploadSource(ctx, content, userID, documentID, filename, contentType)
		if err != nil {
			return nil, err
		}
		return source, nil
	}

	path := filepath.Join(s.config.TempDir, userID, documentID, "input"+filepath.Ext(filename))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return nil, err
	}
	return path, nil
}

func (s *Server) deleteDocumentAssets(ctx context.Context, document models.Document) error {
	if s.assets != nil {
		return s.assets.DeleteDocumentAssets(ctx, document)
	}

	if documentHasS3Assets(document) {
		return fmt.Errorf("document asset deleter is not configured")
	}
	if document.SourcePath != nil && strings.TrimSpace(*document.SourcePath) != "" {
		return os.RemoveAll(filepath.Dir(*document.SourcePath))
	}
	return nil
}

func safePathPart(value string) string {
	cleaned := safePathPartPattern.ReplaceAllString(strings.TrimSpace(value), "_")
	cleaned = strings.Trim(cleaned, "._")
	if cleaned == "" {
		return "unknown"
	}
	return cleaned
}

func safeFilename(value string) string {
	base := filepath.Base(value)
	if base == "." || base == "/" || strings.TrimSpace(base) == "" {
		base = "input.pdf"
	}
	ext := safeFilenameExt(filepath.Ext(base))
	if ext == "" {
		ext = ".pdf"
	}
	stem := safePathPart(strings.TrimSuffix(base, filepath.Ext(base)))
	if stem == "unknown" {
		stem = "input"
	}
	return stem + ext
}

func safeFilenameExt(value string) string {
	value = safePathPartPattern.ReplaceAllString(strings.TrimSpace(value), "_")
	if !strings.HasPrefix(value, ".") {
		value = "." + value
	}
	if value == "." || value == "._" {
		return ""
	}
	return value
}

func randomHexID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("random source unavailable: %v", err))
	}
	return hex.EncodeToString(b[:])
}

func authenticatedUserID(c *gin.Context) (string, bool) {
	userID := strings.TrimSpace(c.GetHeader("X-Authenticated-User-Id"))
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return "", false
	}
	return safePathPart(userID), true
}

func documentHasS3Assets(document models.Document) bool {
	return nonEmptyString(document.SourceBucket) && nonEmptyString(document.SourceKey) ||
		nonEmptyString(document.S3Bucket) && nonEmptyString(document.S3Key)
}

func nonEmptyString(value *string) bool {
	return value != nil && strings.TrimSpace(*value) != ""
}

func (noopDocumentStore) CreateQueuedDocument(context.Context, models.Document) error {
	return nil
}

func (noopDocumentStore) ListUserDocuments(context.Context, uuid.UUID) ([]models.DocumentSummary, error) {
	return []models.DocumentSummary{}, nil
}

func (noopDocumentStore) LoadUserDocument(context.Context, uuid.UUID, uuid.UUID) (models.Document, error) {
	return models.Document{}, models.ErrDocumentNotFound
}

func (noopDocumentStore) DeleteDocumentCascade(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func (noopTaskStatusStore) Set(context.Context, models.TaskStatus) error {
	return nil
}

func (noopTaskStatusStore) Get(context.Context, string) (models.TaskStatus, error) {
	return models.TaskStatus{}, models.ErrTaskStatusNotFound
}
