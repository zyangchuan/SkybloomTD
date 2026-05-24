package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"skybloom/document-content-api/internal/config"
	"skybloom/document-content-api/internal/messaging"
	"skybloom/document-content-api/internal/models"
	"skybloom/document-content-api/internal/storage"
)

const maxUploadBytes = 100 << 20

var safePathPartPattern = regexp.MustCompile(`[^A-Za-z0-9_.=-]+`)

type Server struct {
	config    config.Config
	publisher *messaging.Publisher
	storage   *storage.Storage
}

func NewRouter(cfg config.Config, publisher *messaging.Publisher, storageClient *storage.Storage) *gin.Engine {
	server := &Server{
		config:    cfg,
		publisher: publisher,
		storage:   storageClient,
	}

	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		log.Fatalf("proxy configuration error: %v", err)
	}
	router.Use(gin.Logger(), gin.Recovery())

	router.GET("/health", server.health)
	router.POST("/upload-file", server.uploadFile)
	return router
}

func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) uploadFile(c *gin.Context) {
	userID := strings.TrimSpace(c.GetHeader("X-Authenticated-User-Id"))
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}
	userID = safePathPart(userID)

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

	documentID := randomHexID()
	filename := safeFilename(header.Filename)
	contentType := header.Header.Get("Content-Type")
	source, err := s.sourceForUpload(c.Request.Context(), content, userID, documentID, filename, contentType)
	if err != nil {
		log.Printf("source upload failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store upload"})
		return
	}

	ocrTaskID := randomHexID()
	uploadTaskID := randomHexID()
	indexTaskID := randomHexID()
	job := models.DocumentJob{
		JobType:      "document.process",
		TaskID:       indexTaskID,
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
