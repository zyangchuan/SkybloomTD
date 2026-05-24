package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"skybloom/level-generator-api/internal/models"
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Server struct {
	publisher Publisher
}

type Publisher interface {
	Publish(ctx context.Context, messageID string, value any) error
}

func NewRouter(publisher Publisher) *gin.Engine {
	server := &Server{publisher: publisher}

	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		log.Fatalf("proxy configuration error: %v", err)
	}
	router.Use(gin.Logger(), gin.Recovery())

	router.GET("/health", server.health)
	router.GET("/generate_level", server.generateLevel)
	router.POST("/generate_level", server.generateLevel)
	return router
}

func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) generateLevel(c *gin.Context) {
	userID := strings.TrimSpace(c.GetHeader("X-Authenticated-User-Id"))
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	subChapterID := strings.TrimSpace(c.Query("sub_chapter_id"))
	if !uuidPattern.MatchString(subChapterID) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "sub_chapter_id must be a valid UUID"})
		return
	}

	fetchTaskID := randomHexID()
	generateTaskID := randomHexID()
	job := models.LevelJob{
		JobType:      "level.generate",
		TaskID:       generateTaskID,
		FetchTaskID:  fetchTaskID,
		GenerateID:   generateTaskID,
		UserID:       userID,
		SubChapterID: subChapterID,
	}
	if err := s.publisher.Publish(c.Request.Context(), job.TaskID, job); err != nil {
		log.Printf("rabbitmq publish failed: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to enqueue level job"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":          "Level generation started",
		"task_id":          job.TaskID,
		"fetch_task_id":    fetchTaskID,
		"generate_task_id": generateTaskID,
		"user_id":          userID,
		"sub_chapter_id":   subChapterID,
	})
}

func randomHexID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("random source unavailable: %v", err))
	}
	return hex.EncodeToString(b[:])
}
