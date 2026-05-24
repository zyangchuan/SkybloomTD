package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"skybloom/level-generator-api/internal/messaging"
	"skybloom/level-generator-api/internal/models"
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Server struct {
	publisher *messaging.Publisher
}

func NewRouter(publisher *messaging.Publisher) http.Handler {
	server := &Server{publisher: publisher}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", server.health)
	mux.HandleFunc("GET /generate_level", server.generateLevel)
	mux.HandleFunc("POST /generate_level", server.generateLevel)
	return requestLogger(mux)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) generateLevel(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get("X-Authenticated-User-Id"))
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
		return
	}

	subChapterID := strings.TrimSpace(r.URL.Query().Get("sub_chapter_id"))
	if !uuidPattern.MatchString(subChapterID) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "sub_chapter_id must be a valid UUID"})
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
	if err := s.publisher.Publish(r.Context(), job.TaskID, job); err != nil {
		log.Printf("rabbitmq publish failed: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "failed to enqueue level job"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write response failed: %v", err)
	}
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
