package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
)

const systemPrompt = `You are an educational level generator.

Use only the source text provided by the user. Produce a complete level made of:
- a markdown summary for the level
- at least 10 quizzes for the level

Each quiz can be either:
- mcq: exactly 3 markdown option strings
- true_false: exactly two markdown option strings, True and False

Every question_markdown value and every options_markdown item must be a markdown
string. answer_index must be the zero-based integer index of the correct option
in options_markdown. Do not include facts that are not supported by the source
text.`

type jsonCodec struct{}

func (jsonCodec) Name() string { return "json" }

func (jsonCodec) Marshal(value any) ([]byte, error) {
	return json.Marshal(value)
}

func (jsonCodec) Unmarshal(data []byte, value any) error {
	return json.Unmarshal(data, value)
}

type Config struct {
	RabbitMQURL                string
	Queue                      string
	DatabaseURL                string
	OpenAIAPIKey               string
	OpenAIBaseURL              string
	Model                      string
	Temperature                float64
	Timeout                    time.Duration
	MaxRetries                 int
	LevelSourceMaxChars        int32
	DocumentContentGRPCAddr    string
	DocumentContentGRPCTimeout time.Duration
}

type Worker struct {
	config     Config
	db         *sql.DB
	httpClient *http.Client
}

type LevelJob struct {
	TaskID       string `json:"task_id"`
	FetchTaskID  string `json:"fetch_task_id"`
	GenerateID   string `json:"generate_task_id"`
	UserID       string `json:"user_id"`
	SubChapterID string `json:"sub_chapter_id"`
}

type GetSubChapterRequest struct {
	UserID       string `json:"user_id"`
	SubChapterID string `json:"sub_chapter_id"`
	MaxChars     int32  `json:"max_chars"`
}

type GetSubChapterResponse struct {
	SubChapter SubChapterContent `json:"sub_chapter"`
}

type SubChapterContent struct {
	NormalizedUserID    string   `json:"normalized_user_id"`
	RequestedUserID     string   `json:"requested_user_id"`
	SubChapterID        string   `json:"sub_chapter_id"`
	DocumentID          string   `json:"document_id"`
	ChapterID           string   `json:"chapter_id"`
	SubChapterIndex     int32    `json:"sub_chapter_index"`
	Title               string   `json:"title"`
	StartLine           int32    `json:"start_line"`
	EndLine             int32    `json:"end_line"`
	SourceText          string   `json:"source_text"`
	SourceChunkIDs      []string `json:"source_chunk_ids"`
	ChunkCount          int32    `json:"chunk_count"`
	CandidateChunkCount int32    `json:"candidate_chunk_count"`
	ChunkLookupStrategy string   `json:"chunk_lookup_strategy"`
	SourceCharCount     int32    `json:"source_char_count"`
	SourceTruncated     bool     `json:"source_truncated"`
	MarkdownCacheHit    bool     `json:"markdown_cache_hit"`
	MarkdownCacheKey    string   `json:"markdown_cache_key"`
	SourceContentHash   string   `json:"source_content_hash"`
}

type SourceContext struct {
	Status              string
	UserID              string
	DBUserID            string
	SubChapterID        string
	DocumentID          string
	ChapterID           string
	SubChapterIndex     int32
	SubChapterTitle     string
	StartLine           int32
	EndLine             int32
	ChunkIDs            []string
	ChunkCount          int32
	CandidateChunkCount int32
	ChunkLookupStrategy string
	MarkdownCacheHit    bool
	MarkdownCacheKey    string
	SourceCharCount     int32
	SourceTruncated     bool
	SourceContentHash   string
	SourceText          string
}

type LevelGeneration struct {
	SummaryMarkdown string     `json:"summary_markdown"`
	Quizzes         []QuizItem `json:"quizzes"`
}

type QuizItem struct {
	QuizType         string   `json:"quiz_type"`
	QuestionMarkdown string   `json:"question_markdown"`
	OptionsMarkdown  []string `json:"options_markdown"`
	AnswerIndex      int      `json:"answer_index"`
}

func main() {
	encoding.RegisterCodec(jsonCodec{})

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database open error: %v", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("database connection error: %v", err)
	}
	if err := ensureSchema(ctx, db); err != nil {
		log.Fatalf("database migration error: %v", err)
	}

	worker := &Worker{
		config: cfg,
		db:     db,
		httpClient: &http.Client{
			Timeout: cfg.Timeout + 15*time.Second,
		},
	}
	if err := worker.consume(); err != nil {
		log.Fatalf("worker error: %v", err)
	}
}

func loadConfig() (Config, error) {
	databaseURL, err := databaseURLFromEnv()
	if err != nil {
		return Config{}, err
	}
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return Config{}, errors.New("OPENAI_API_KEY is required")
	}
	return Config{
		RabbitMQURL:                envOrDefault("RABBITMQ_URL", "amqp://guest:guest@rabbitmq:5672/"),
		Queue:                      envOrDefault("LEVEL_GENERATOR_QUEUE", "level.generate"),
		DatabaseURL:                databaseURL,
		OpenAIAPIKey:               apiKey,
		OpenAIBaseURL:              strings.TrimRight(envOrDefault("OPENAI_BASE_URL", "https://api.openai.com/v1"), "/"),
		Model:                      envOrDefault("LEVEL_LLM_MODEL", "gpt-4o-mini"),
		Temperature:                envFloat("LEVEL_LLM_TEMPERATURE", 0.2),
		Timeout:                    time.Duration(envFloat("LEVEL_LLM_TIMEOUT_SECONDS", 60)) * time.Second,
		MaxRetries:                 envInt("LEVEL_LLM_MAX_RETRIES", 1),
		LevelSourceMaxChars:        int32(envInt("LEVEL_SOURCE_MAX_CHARS", 24000)),
		DocumentContentGRPCAddr:    envOrDefault("DOCUMENT_CONTENT_GRPC_ADDR", envOrDefault("OCR_CONTENT_GRPC_ADDR", "localhost:50051")),
		DocumentContentGRPCTimeout: time.Duration(envFloat("DOCUMENT_CONTENT_GRPC_TIMEOUT_SECONDS", envFloat("OCR_CONTENT_GRPC_TIMEOUT_SECONDS", 30))) * time.Second,
	}, nil
}

func (w *Worker) consume() error {
	conn, err := amqp.Dial(w.config.RabbitMQURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if _, err := ch.QueueDeclare(w.config.Queue, true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.Qos(1, 0, false); err != nil {
		return err
	}
	deliveries, err := ch.Consume(w.config.Queue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	log.Printf("level-generator worker consuming queue=%s", w.config.Queue)
	for delivery := range deliveries {
		var job LevelJob
		if err := json.Unmarshal(delivery.Body, &job); err != nil {
			log.Printf("invalid level job: %v", err)
			_ = delivery.Nack(false, false)
			continue
		}
		if err := w.processJob(context.Background(), job); err != nil {
			log.Printf("level job failed task_id=%s sub_chapter_id=%s: %v", job.TaskID, job.SubChapterID, err)
			_ = delivery.Nack(false, false)
			continue
		}
		_ = delivery.Ack(false)
	}
	return nil
}

func (w *Worker) processJob(ctx context.Context, job LevelJob) error {
	sourceContext, err := w.fetchSubChapterContent(ctx, job.UserID, job.SubChapterID)
	if err != nil {
		return err
	}
	generation, err := w.generateLevel(ctx, sourceContext)
	if err != nil {
		return err
	}
	saved, err := insertLevel(ctx, w.db, sourceContext, generation, w.config.Model)
	if err != nil {
		return err
	}
	log.Printf(
		"level job complete task_id=%s level_id=%s sub_chapter_id=%s quiz_count=%d",
		job.TaskID,
		saved.LevelID,
		saved.SubChapterID,
		saved.QuizCount,
	)
	return nil
}

func (w *Worker) fetchSubChapterContent(ctx context.Context, userID string, subChapterID string) (SourceContext, error) {
	callCtx, cancel := context.WithTimeout(ctx, w.config.DocumentContentGRPCTimeout)
	defer cancel()

	conn, err := grpc.NewClient(
		w.config.DocumentContentGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.ForceCodec(jsonCodec{}),
			grpc.CallContentSubtype("json"),
		),
	)
	if err != nil {
		return SourceContext{}, err
	}
	defer conn.Close()

	request := &GetSubChapterRequest{
		UserID:       userID,
		SubChapterID: subChapterID,
		MaxChars:     w.config.LevelSourceMaxChars,
	}
	var response GetSubChapterResponse
	if err := conn.Invoke(callCtx, "/document_content.v1.DocumentContentService/GetSubChapter", request, &response); err != nil {
		return SourceContext{}, err
	}

	subChapter := response.SubChapter
	return SourceContext{
		Status:              "retrieved",
		UserID:              userID,
		DBUserID:            subChapter.NormalizedUserID,
		SubChapterID:        subChapter.SubChapterID,
		DocumentID:          subChapter.DocumentID,
		ChapterID:           subChapter.ChapterID,
		SubChapterIndex:     subChapter.SubChapterIndex,
		SubChapterTitle:     subChapter.Title,
		StartLine:           subChapter.StartLine,
		EndLine:             subChapter.EndLine,
		ChunkIDs:            subChapter.SourceChunkIDs,
		ChunkCount:          subChapter.ChunkCount,
		CandidateChunkCount: subChapter.CandidateChunkCount,
		ChunkLookupStrategy: subChapter.ChunkLookupStrategy,
		MarkdownCacheHit:    subChapter.MarkdownCacheHit,
		MarkdownCacheKey:    subChapter.MarkdownCacheKey,
		SourceCharCount:     subChapter.SourceCharCount,
		SourceTruncated:     subChapter.SourceTruncated,
		SourceContentHash:   subChapter.SourceContentHash,
		SourceText:          subChapter.SourceText,
	}, nil
}

func (w *Worker) generateLevel(ctx context.Context, source SourceContext) (LevelGeneration, error) {
	if source.Status != "retrieved" {
		return LevelGeneration{}, errors.New("source context was not retrieved successfully")
	}

	prompt := fmt.Sprintf(
		"Generate the level for this sub-chapter.\n\nSub-chapter title: %s\nSub-chapter id: %s\n\nSource text:\n%s",
		firstNonEmpty(source.SubChapterTitle, "Untitled"),
		source.SubChapterID,
		source.SourceText,
	)

	var generation LevelGeneration
	var lastErr error
	attempts := w.config.MaxRetries + 1
	for attempt := 1; attempt <= attempts; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, w.config.Timeout)
		err := w.callOpenAI(callCtx, prompt, &generation)
		cancel()
		if err == nil {
			return generation, validateGeneration(generation)
		}
		lastErr = err
		if attempt < attempts {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return LevelGeneration{}, lastErr
}

func (w *Worker) callOpenAI(ctx context.Context, prompt string, generation *LevelGeneration) error {
	requestBody := map[string]any{
		"model":       w.config.Model,
		"temperature": w.config.Temperature,
		"input": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": prompt},
		},
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "level_generation",
				"strict": true,
				"schema": levelGenerationSchema(),
			},
		},
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, w.config.OpenAIBaseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+w.config.OpenAIAPIKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := w.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("openai response status %d: %s", response.StatusCode, string(responseBody))
	}

	outputText, err := responseOutputText(responseBody)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(outputText), generation); err != nil {
		return fmt.Errorf("decode level generation: %w", err)
	}
	return nil
}

func responseOutputText(data []byte) (string, error) {
	var response struct {
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type    string `json:"type"`
				Text    string `json:"text"`
				Refusal string `json:"refusal"`
			} `json:"content"`
		} `json:"output"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return "", err
	}
	if response.Error != nil && response.Error.Message != "" {
		return "", errors.New(response.Error.Message)
	}
	for _, output := range response.Output {
		if output.Type != "message" {
			continue
		}
		for _, content := range output.Content {
			if content.Type == "refusal" {
				return "", fmt.Errorf("model refusal: %s", content.Refusal)
			}
			if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
				return content.Text, nil
			}
		}
	}
	return "", errors.New("OpenAI response did not include output_text")
}

func levelGenerationSchema() map[string]any {
	quizSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"quiz_type", "question_markdown", "options_markdown", "answer_index"},
		"properties": map[string]any{
			"quiz_type": map[string]any{
				"type": "string",
				"enum": []string{"mcq", "true_false"},
			},
			"question_markdown": map[string]any{"type": "string"},
			"options_markdown": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"answer_index": map[string]any{
				"type":    "integer",
				"minimum": 0,
			},
		},
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"summary_markdown", "quizzes"},
		"properties": map[string]any{
			"summary_markdown": map[string]any{"type": "string"},
			"quizzes": map[string]any{
				"type":     "array",
				"minItems": 10,
				"items":    quizSchema,
			},
		},
	}
}

func validateGeneration(generation LevelGeneration) error {
	if strings.TrimSpace(generation.SummaryMarkdown) == "" {
		return errors.New("summary_markdown cannot be empty")
	}
	if len(generation.Quizzes) < 10 {
		return errors.New("generate at least 10 quizzes")
	}
	for index, quiz := range generation.Quizzes {
		if strings.TrimSpace(quiz.QuestionMarkdown) == "" {
			return fmt.Errorf("quiz %d question_markdown cannot be empty", index)
		}
		if quiz.QuizType != "mcq" && quiz.QuizType != "true_false" {
			return fmt.Errorf("quiz %d has invalid quiz_type", index)
		}
		if quiz.QuizType == "mcq" && len(quiz.OptionsMarkdown) != 3 {
			return fmt.Errorf("quiz %d mcq must include exactly three options", index)
		}
		if quiz.QuizType == "true_false" {
			if len(quiz.OptionsMarkdown) != 2 {
				return fmt.Errorf("quiz %d true_false must include exactly two options", index)
			}
			options := map[string]bool{}
			for _, option := range quiz.OptionsMarkdown {
				options[strings.ToLower(strings.TrimSpace(option))] = true
			}
			if !options["true"] || !options["false"] || len(options) != 2 {
				return fmt.Errorf("quiz %d true_false options must be True and False", index)
			}
		}
		if quiz.AnswerIndex < 0 || quiz.AnswerIndex >= len(quiz.OptionsMarkdown) {
			return fmt.Errorf("quiz %d answer_index must point to an option", index)
		}
	}
	return nil
}

type SavedLevel struct {
	LevelID      string
	SubChapterID string
	DocumentID   string
	QuizCount    int
	Model        string
}

func ensureSchema(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS levels (
			id UUID PRIMARY KEY,
			user_id UUID,
			document_id UUID NOT NULL,
			chapter_id UUID NOT NULL,
			sub_chapter_id UUID NOT NULL,
			summary_markdown TEXT NOT NULL,
			source_chunk_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
			source_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			model TEXT,
			created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS quizzes (
			id UUID PRIMARY KEY,
			level_id UUID NOT NULL REFERENCES levels(id) ON DELETE CASCADE,
			quiz_index INTEGER NOT NULL,
			quiz_type TEXT NOT NULL
				CONSTRAINT quizzes_quiz_type_check
				CHECK (quiz_type IN ('mcq', 'true_false')),
			question_markdown TEXT NOT NULL,
			options_markdown JSONB NOT NULL,
			answer_index INTEGER NOT NULL,
			created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT quizzes_level_id_quiz_index_key UNIQUE (level_id, quiz_index)
		)`,
		`ALTER TABLE quizzes ADD COLUMN IF NOT EXISTS answer_index INTEGER`,
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'quizzes_answer_index_nonnegative_check'
			) THEN
				ALTER TABLE quizzes
				ADD CONSTRAINT quizzes_answer_index_nonnegative_check
				CHECK (answer_index >= 0);
			END IF;
		}
		$$`,
		`CREATE INDEX IF NOT EXISTS levels_user_id_idx ON levels(user_id)`,
		`CREATE INDEX IF NOT EXISTS levels_sub_chapter_id_idx ON levels(sub_chapter_id)`,
		`CREATE INDEX IF NOT EXISTS levels_document_id_idx ON levels(document_id)`,
		`CREATE INDEX IF NOT EXISTS quizzes_level_id_idx ON quizzes(level_id)`,
		`DO $$
		DECLARE
			constraint_name text;
		BEGIN
			FOR constraint_name IN
				SELECT conname
				FROM pg_constraint
				WHERE conrelid = 'levels'::regclass
				  AND contype = 'f'
			LOOP
				EXECUTE format('ALTER TABLE levels DROP CONSTRAINT IF EXISTS %I', constraint_name);
			END LOOP;
		END
		$$`,
		`UPDATE quizzes SET answer_index = 0 WHERE answer_index IS NULL`,
		`ALTER TABLE quizzes ALTER COLUMN answer_index SET NOT NULL`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func insertLevel(ctx context.Context, db *sql.DB, source SourceContext, generation LevelGeneration, model string) (SavedLevel, error) {
	levelID, err := uuid.NewRandom()
	if err != nil {
		return SavedLevel{}, err
	}
	dbUserID, err := uuid.Parse(source.DBUserID)
	if err != nil {
		return SavedLevel{}, fmt.Errorf("db_user_id must be a valid UUID: %w", err)
	}
	documentID, err := uuid.Parse(source.DocumentID)
	if err != nil {
		return SavedLevel{}, fmt.Errorf("document_id must be a valid UUID: %w", err)
	}
	chapterID, err := uuid.Parse(source.ChapterID)
	if err != nil {
		return SavedLevel{}, fmt.Errorf("chapter_id must be a valid UUID: %w", err)
	}
	subChapterID, err := uuid.Parse(source.SubChapterID)
	if err != nil {
		return SavedLevel{}, fmt.Errorf("sub_chapter_id must be a valid UUID: %w", err)
	}

	chunkIDs, err := json.Marshal(source.ChunkIDs)
	if err != nil {
		return SavedLevel{}, err
	}
	sourceMetadata, err := json.Marshal(map[string]any{
		"sub_chapter_index":     source.SubChapterIndex,
		"sub_chapter_title":     source.SubChapterTitle,
		"start_line":            source.StartLine,
		"end_line":              source.EndLine,
		"chunk_count":           source.ChunkCount,
		"candidate_chunk_count": source.CandidateChunkCount,
		"chunk_lookup_strategy": source.ChunkLookupStrategy,
		"markdown_cache_hit":    source.MarkdownCacheHit,
		"markdown_cache_key":    source.MarkdownCacheKey,
		"source_char_count":     source.SourceCharCount,
		"source_truncated":      source.SourceTruncated,
		"source_content_hash":   source.SourceContentHash,
	})
	if err != nil {
		return SavedLevel{}, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return SavedLevel{}, err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO levels (
			id, user_id, document_id, chapter_id, sub_chapter_id,
			summary_markdown, source_chunk_ids, source_metadata, model
		) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, $9)`,
		levelID,
		dbUserID,
		documentID,
		chapterID,
		subChapterID,
		generation.SummaryMarkdown,
		string(chunkIDs),
		string(sourceMetadata),
		model,
	)
	if err != nil {
		return SavedLevel{}, err
	}

	for index, quiz := range generation.Quizzes {
		quizID, err := uuid.NewRandom()
		if err != nil {
			return SavedLevel{}, err
		}
		options, err := json.Marshal(quiz.OptionsMarkdown)
		if err != nil {
			return SavedLevel{}, err
		}
		_, err = tx.ExecContext(
			ctx,
			`INSERT INTO quizzes (
				id, level_id, quiz_index, quiz_type,
				question_markdown, options_markdown, answer_index
			) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)`,
			quizID,
			levelID,
			index,
			quiz.QuizType,
			quiz.QuestionMarkdown,
			string(options),
			quiz.AnswerIndex,
		)
		if err != nil {
			return SavedLevel{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return SavedLevel{}, err
	}
	return SavedLevel{
		LevelID:      levelID.String(),
		SubChapterID: source.SubChapterID,
		DocumentID:   source.DocumentID,
		QuizCount:    len(generation.Quizzes),
		Model:        model,
	}, nil
}

func databaseURLFromEnv() (string, error) {
	if raw := strings.TrimSpace(firstNonEmpty(os.Getenv("LEVEL_DATABASE_URL"), os.Getenv("DATABASE_URL"), os.Getenv("POSTGRES_DSN"))); raw != "" {
		return normalizePostgresURL(raw), nil
	}

	host := firstNonEmpty(os.Getenv("LEVEL_POSTGRES_HOST"), os.Getenv("POSTGRES_HOST"), os.Getenv("AWS_RDS_POSTGRES_HOST"))
	port := firstNonEmpty(os.Getenv("LEVEL_POSTGRES_PORT"), os.Getenv("POSTGRES_PORT"), "5432")
	dbName := firstNonEmpty(os.Getenv("LEVEL_POSTGRES_DB"), os.Getenv("POSTGRES_DB"))
	user := firstNonEmpty(os.Getenv("LEVEL_POSTGRES_USER"), os.Getenv("POSTGRES_USER"))
	password := firstNonEmpty(os.Getenv("LEVEL_POSTGRES_PASSWORD"), os.Getenv("POSTGRES_PASSWORD"))
	sslMode := firstNonEmpty(os.Getenv("LEVEL_POSTGRES_SSLMODE"), os.Getenv("POSTGRES_SSLMODE"), "require")
	if host == "" || dbName == "" || user == "" || password == "" {
		return "", errors.New("set DATABASE_URL or POSTGRES_HOST, POSTGRES_DB, POSTGRES_USER, and POSTGRES_PASSWORD")
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + dbName,
	}
	query := u.Query()
	if sslMode != "" {
		query.Set("sslmode", sslMode)
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func normalizePostgresURL(raw string) string {
	if strings.HasPrefix(raw, "postgresql+psycopg://") {
		return "postgres://" + strings.TrimPrefix(raw, "postgresql+psycopg://")
	}
	return raw
}

func randomHexID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("random source unavailable: %v", err))
	}
	return hex.EncodeToString(b[:])
}

func envOrDefault(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	var value int
	if _, err := fmt.Sscanf(raw, "%d", &value); err != nil {
		return fallback
	}
	return value
}

func envFloat(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	var value float64
	if _, err := fmt.Sscanf(raw, "%f", &value); err != nil {
		return fallback
	}
	return value
}
