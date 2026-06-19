package controllers

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"skybloom/document-content-api/internal/models"
)

func (s *Controller) ListDocuments(c *gin.Context) {
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

func (s *Controller) ListDocumentChapters(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}

	documentID, err := uuid.Parse(strings.TrimSpace(c.Param("document_id")))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid document_id"})
		return
	}

	chapters, err := s.documents.ListDocumentChapters(c.Request.Context(), documentID, models.DatabaseUUID(userID, "user"))
	if errors.Is(err, models.ErrDocumentNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
		return
	}
	if err != nil {
		log.Printf("chapter list failed: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to list chapters"})
		return
	}

	c.JSON(http.StatusOK, models.ListChaptersResponse{Chapters: chapters})
}

func (s *Controller) ListChapterSubChapters(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}

	chapterID, err := uuid.Parse(strings.TrimSpace(c.Param("chapter_id")))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter_id"})
		return
	}

	subChapters, err := s.documents.ListChapterSubChapters(c.Request.Context(), chapterID, models.DatabaseUUID(userID, "user"))
	if errors.Is(err, models.ErrChapterNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "chapter not found"})
		return
	}
	if err != nil {
		log.Printf("sub-chapter list failed: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to list sub_chapters"})
		return
	}

	c.JSON(http.StatusOK, models.ListSubChaptersResponse{SubChapters: subChapters})
}

func (s *Controller) DeleteDocument(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}
	dbUserID := models.DatabaseUUID(userID, "user")

	documentID, err := uuid.Parse(c.Param("document_id"))
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

	if err := s.storage.DeleteDocumentFiles(c.Request.Context(), document); err != nil {
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
