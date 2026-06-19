package controllers

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"skybloom/document-content-api/internal/models"
)

func (s *Controller) GetTaskStatus(c *gin.Context) {
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
