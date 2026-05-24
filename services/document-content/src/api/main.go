package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const maxUploadBytes = 100 << 20

var safePathPartPattern = regexp.MustCompile(`[^A-Za-z0-9_.=-]+`)

type Config struct {
	Port                 string
	RabbitMQURL          string
	DocumentContentQueue string
	TempDir              string
}

type Server struct {
	config    Config
	publisher *Publisher
	storage   *Storage
}

type SourceRef struct {
	Type        string `json:"type"`
	Bucket      string `json:"bucket"`
	Key         string `json:"key"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
}

type DocumentJob struct {
	JobType      string `json:"job_type"`
	TaskID       string `json:"task_id"`
	OCRTaskID    string `json:"ocr_task_id"`
	UploadTaskID string `json:"upload_task_id"`
	IndexTaskID  string `json:"index_task_id"`
	UserID       string `json:"user_id"`
	DocumentID   string `json:"document_id"`
	Filename     string `json:"filename"`
	Source       any    `json:"source"`
}

func main() {
	cfg := loadConfig()

	publisher, err := NewPublisher(cfg.RabbitMQURL, cfg.DocumentContentQueue)
	if err != nil {
		log.Fatalf("rabbitmq connection error: %v", err)
	}
	defer publisher.Close()

	storage, err := NewStorageFromEnv()
	if err != nil {
		log.Fatalf("storage configuration error: %v", err)
	}

	server := &Server{
		config:    cfg,
		publisher: publisher,
		storage:   storage,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", server.health)
	mux.HandleFunc("POST /upload-file", server.uploadFile)

	addr := ":" + cfg.Port
	log.Printf("document-content-api listening on %s", addr)
	if err := http.ListenAndServe(addr, requestLogger(mux)); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func loadConfig() Config {
	return Config{
		Port:                 envOrDefault("DOCUMENT_CONTENT_API_PORT", envOrDefault("PORT", "8000")),
		RabbitMQURL:          envOrDefault("RABBITMQ_URL", "amqp://guest:guest@rabbitmq:5672/"),
		DocumentContentQueue: envOrDefault("DOCUMENT_CONTENT_QUEUE", "document.process"),
		TempDir:              envOrDefault("TEMP_DIR", "/temp"),
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) uploadFile(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get("X-Authenticated-User-Id"))
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
		return
	}
	userID = safePathPart(userID)

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart upload"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file is required"})
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read upload"})
		return
	}

	documentID := randomHexID()
	filename := safeFilename(header.Filename)
	contentType := header.Header.Get("Content-Type")
	source, err := s.sourceForUpload(r.Context(), content, userID, documentID, filename, contentType)
	if err != nil {
		log.Printf("source upload failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store upload"})
		return
	}

	ocrTaskID := randomHexID()
	uploadTaskID := randomHexID()
	indexTaskID := randomHexID()
	job := DocumentJob{
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

	if err := s.publisher.Publish(r.Context(), job.TaskID, job); err != nil {
		log.Printf("rabbitmq publish failed: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "failed to enqueue document job"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
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
