package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Config struct {
	Port                string
	RabbitMQURL         string
	LevelGeneratorQueue string
}

type Server struct {
	publisher *Publisher
}

type LevelJob struct {
	JobType      string `json:"job_type"`
	TaskID       string `json:"task_id"`
	FetchTaskID  string `json:"fetch_task_id"`
	GenerateID   string `json:"generate_task_id"`
	UserID       string `json:"user_id"`
	SubChapterID string `json:"sub_chapter_id"`
}

func main() {
	cfg := loadConfig()
	publisher, err := NewPublisher(cfg.RabbitMQURL, cfg.LevelGeneratorQueue)
	if err != nil {
		log.Fatalf("rabbitmq connection error: %v", err)
	}
	defer publisher.Close()

	server := &Server{publisher: publisher}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", server.health)
	mux.HandleFunc("GET /generate_level", server.generateLevel)
	mux.HandleFunc("POST /generate_level", server.generateLevel)

	addr := ":" + cfg.Port
	log.Printf("level-generator-api listening on %s", addr)
	if err := http.ListenAndServe(addr, requestLogger(mux)); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func loadConfig() Config {
	return Config{
		Port:                envOrDefault("LEVEL_GENERATOR_API_PORT", envOrDefault("PORT", "8000")),
		RabbitMQURL:         envOrDefault("RABBITMQ_URL", "amqp://guest:guest@rabbitmq:5672/"),
		LevelGeneratorQueue: envOrDefault("LEVEL_GENERATOR_QUEUE", "level.generate"),
	}
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
	job := LevelJob{
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

type Publisher struct {
	conn  *amqp.Connection
	ch    *amqp.Channel
	queue string
}

func NewPublisher(url string, queue string) (*Publisher, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, err
	}
	if _, err := ch.QueueDeclare(queue, true, false, false, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, err
	}
	return &Publisher{conn: conn, ch: ch, queue: queue}, nil
}

func (p *Publisher) Publish(ctx context.Context, messageID string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return p.ch.PublishWithContext(ctx, "", p.queue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    messageID,
		Timestamp:    time.Now().UTC(),
		Body:         body,
	})
}

func (p *Publisher) Close() {
	if p.ch != nil {
		_ = p.ch.Close()
	}
	if p.conn != nil {
		_ = p.conn.Close()
	}
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

func envOrDefault(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
