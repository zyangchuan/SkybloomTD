package controllers

import (
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"unicode/utf8"

	"crypto/rand"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"skybloom/document-content-api/internal/models"
)

func (s *Controller) UploadFile(c *gin.Context) {
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

	// Game name validation
	gameName := normalizeGameName(c.Request.FormValue(gameNameField))
	if gameName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gameNameMissingErr})
		return
	}
	if utf8.RuneCountInString(gameName) > maxGameNameLength {
		c.JSON(http.StatusBadRequest, gin.H{"error": "game_name must be 120 characters or fewer"})
		return
	}

	content, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read upload"})
		return
	}

	documentID := uuid.NewString()
	taskID := randomHexID()
	filename := safeFilename(header.Filename)
	contentType := header.Header.Get("Content-Type")

	// Upload file to the S3 bucket
	source, err := s.storage.UploadSource(c.Request.Context(), content, userID, documentID, filename, contentType)
	if err != nil {
		log.Printf("source upload failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store upload"})
		return
	}

	// Create database row
	document, err := models.NewQueuedDocument(documentID, userID, taskID, gameName, source)
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

	// Initialise a new document processing task status in Redis
	if err := s.taskStatus.Set(c.Request.Context(), models.NewTaskStatus(taskID, documentID, models.TaskStatusQueued, nil)); err != nil {
		log.Printf("redis task status write failed: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to record task status"})
		return
	}

	// Publish the message for ocr worker
	job := models.DocumentJob{
		JobType:    "document.process",
		TaskID:     taskID,
		UserID:     userID,
		DocumentID: documentID,
		Source:     source,
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
		"message":     "Upload file success",
		"task_id":     job.TaskID,
		"user_id":     userID,
		"document_id": documentID,
		"game_name":   gameName,
		"is_ready":    false,
	})
}

func randomHexID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("random source unavailable: %v", err))
	}
	return hex.EncodeToString(b[:])
}
