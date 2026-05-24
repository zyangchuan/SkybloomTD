package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/status"
)

type jsonCodec struct{}

func (jsonCodec) Name() string { return "json" }

func (jsonCodec) Marshal(value any) ([]byte, error) {
	return json.Marshal(value)
}

func (jsonCodec) Unmarshal(data []byte, value any) error {
	return json.Unmarshal(data, value)
}

type Config struct {
	DatabaseURL string
	Host        string
	Port        string
}

type Server struct {
	db     *sql.DB
	loader *MarkdownLoader
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

type documentRow struct {
	SubChapterID    uuid.UUID
	DocumentID      uuid.UUID
	ChapterID       uuid.UUID
	SubChapterIndex sql.NullInt32
	Title           sql.NullString
	StartLine       sql.NullInt32
	EndLine         sql.NullInt32
	S3Bucket        sql.NullString
	S3Key           sql.NullString
}

var (
	errBadRequest         = errors.New("bad request")
	errNotFound           = errors.New("not found")
	errContentUnavailable = errors.New("content unavailable")
)

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

	loader, err := NewMarkdownLoader()
	if err != nil {
		log.Fatalf("storage configuration error: %v", err)
	}

	listener, err := net.Listen("tcp", net.JoinHostPort(cfg.Host, cfg.Port))
	if err != nil {
		log.Fatalf("listen error: %v", err)
	}

	grpcServer := grpc.NewServer()
	RegisterDocumentContentService(grpcServer, &Server{db: db, loader: loader})
	log.Printf("document-content-grpc listening on %s", listener.Addr())
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("grpc server error: %v", err)
	}
}

func loadConfig() (Config, error) {
	databaseURL, err := databaseURLFromEnv()
	if err != nil {
		return Config{}, err
	}
	return Config{
		DatabaseURL: databaseURL,
		Host:        envOrDefault("CONTENT_GRPC_HOST", "0.0.0.0"),
		Port:        envOrDefault("CONTENT_GRPC_PORT", "50051"),
	}, nil
}

func databaseURLFromEnv() (string, error) {
	if raw := strings.TrimSpace(os.Getenv("DATABASE_URL")); raw != "" {
		return normalizePostgresURL(raw), nil
	}

	host := firstNonEmpty(os.Getenv("POSTGRES_HOST"), os.Getenv("AWS_RDS_POSTGRES_HOST"))
	port := firstNonEmpty(os.Getenv("POSTGRES_PORT"), "5432")
	dbName := os.Getenv("POSTGRES_DB")
	user := os.Getenv("POSTGRES_USER")
	password := os.Getenv("POSTGRES_PASSWORD")
	sslMode := firstNonEmpty(os.Getenv("POSTGRES_SSLMODE"), "require")
	if strings.TrimSpace(host) == "" || strings.TrimSpace(dbName) == "" ||
		strings.TrimSpace(user) == "" || password == "" {
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

type DocumentContentServiceServer interface {
	GetSubChapter(context.Context, *GetSubChapterRequest) (*GetSubChapterResponse, error)
}

func RegisterDocumentContentService(server *grpc.Server, service DocumentContentServiceServer) {
	server.RegisterService(&grpc.ServiceDesc{
		ServiceName: "document_content.v1.DocumentContentService",
		HandlerType: (*DocumentContentServiceServer)(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "GetSubChapter",
				Handler:    getSubChapterHandler,
			},
		},
		Streams:  []grpc.StreamDesc{},
		Metadata: "document_content.proto",
	}, service)
}

func getSubChapterHandler(service any, ctx context.Context, decode func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	request := new(GetSubChapterRequest)
	if err := decode(request); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return service.(DocumentContentServiceServer).GetSubChapter(ctx, request)
	}
	info := &grpc.UnaryServerInfo{
		Server:     service,
		FullMethod: "/document_content.v1.DocumentContentService/GetSubChapter",
	}
	handler := func(ctx context.Context, req any) (any, error) {
		return service.(DocumentContentServiceServer).GetSubChapter(ctx, req.(*GetSubChapterRequest))
	}
	return interceptor(ctx, request, info, handler)
}

func (s *Server) GetSubChapter(ctx context.Context, request *GetSubChapterRequest) (*GetSubChapterResponse, error) {
	content, err := s.fetchSubChapterContent(ctx, request.UserID, request.SubChapterID, request.MaxChars)
	if err != nil {
		switch {
		case errors.Is(err, errBadRequest):
			return nil, status.Error(codes.InvalidArgument, err.Error())
		case errors.Is(err, errNotFound):
			return nil, status.Error(codes.NotFound, err.Error())
		case errors.Is(err, errContentUnavailable):
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		default:
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	return &GetSubChapterResponse{SubChapter: content}, nil
}

func (s *Server) fetchSubChapterContent(ctx context.Context, requestedUserID string, subChapterID string, maxChars int32) (SubChapterContent, error) {
	if strings.TrimSpace(requestedUserID) == "" {
		return SubChapterContent{}, fmt.Errorf("%w: user_id is required", errBadRequest)
	}
	if maxChars < 0 {
		return SubChapterContent{}, fmt.Errorf("%w: max_chars must be zero or greater", errBadRequest)
	}

	subChapterUUID, err := uuid.Parse(subChapterID)
	if err != nil {
		return SubChapterContent{}, fmt.Errorf("%w: sub_chapter_id must be a valid UUID", errBadRequest)
	}
	userUUID := indexedUserUUID(requestedUserID)

	row, err := s.loadDocumentRow(ctx, subChapterUUID, userUUID)
	if err != nil {
		return SubChapterContent{}, err
	}
	if !row.S3Bucket.Valid || strings.TrimSpace(row.S3Bucket.String) == "" ||
		!row.S3Key.Valid || strings.TrimSpace(row.S3Key.String) == "" {
		return SubChapterContent{}, fmt.Errorf("%w: Document markdown S3 location is missing", errContentUnavailable)
	}

	startLine := row.StartLine.Int32
	endLine := row.EndLine.Int32
	markdown, err := s.loader.Download(ctx, row.S3Bucket.String, row.S3Key.String)
	if err != nil {
		return SubChapterContent{}, err
	}
	sourceText, err := sliceMarkdownLines(markdown, startLine, endLine)
	if err != nil {
		return SubChapterContent{}, err
	}

	sourceCharCount := int32(len([]rune(sourceText)))
	sourceTruncated := maxChars > 0 && sourceCharCount > maxChars
	if sourceTruncated {
		sourceText = string([]rune(sourceText)[:maxChars])
	}

	return SubChapterContent{
		NormalizedUserID:    userUUID.String(),
		RequestedUserID:     requestedUserID,
		SubChapterID:        row.SubChapterID.String(),
		DocumentID:          row.DocumentID.String(),
		ChapterID:           row.ChapterID.String(),
		SubChapterIndex:     row.SubChapterIndex.Int32,
		Title:               row.Title.String,
		StartLine:           startLine,
		EndLine:             endLine,
		SourceText:          sourceText,
		SourceChunkIDs:      []string{},
		ChunkCount:          0,
		CandidateChunkCount: 0,
		ChunkLookupStrategy: "s3_markdown_line_range",
		SourceCharCount:     sourceCharCount,
		SourceTruncated:     sourceTruncated,
		MarkdownCacheHit:    false,
		MarkdownCacheKey:    "",
		SourceContentHash:   sha256Hex(sourceText),
	}, nil
}

func (s *Server) loadDocumentRow(ctx context.Context, subChapterID uuid.UUID, userID uuid.UUID) (documentRow, error) {
	const query = `
		SELECT sc.id, sc.document_id, sc.chapter_id, sc.sub_chapter_index,
		       sc.title, sc.start_line, sc.end_line, d.s3_bucket, d.s3_key
		FROM sub_chapters sc
		JOIN documents d ON sc.document_id = d.id
		WHERE sc.id = $1 AND d.user_id = $2
	`
	var row documentRow
	err := s.db.QueryRowContext(ctx, query, subChapterID, userID).Scan(
		&row.SubChapterID,
		&row.DocumentID,
		&row.ChapterID,
		&row.SubChapterIndex,
		&row.Title,
		&row.StartLine,
		&row.EndLine,
		&row.S3Bucket,
		&row.S3Key,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return documentRow{}, fmt.Errorf("%w: No sub_chapter found for the provided user_id and sub_chapter_id", errNotFound)
	}
	if err != nil {
		return documentRow{}, err
	}
	return row, nil
}

func sliceMarkdownLines(markdown string, startLine int32, endLine int32) (string, error) {
	if startLine == 0 || endLine == 0 {
		return "", fmt.Errorf("%w: Sub-chapter line range is missing", errContentUnavailable)
	}
	if startLine < 1 {
		return "", fmt.Errorf("%w: Sub-chapter start_line must be at least 1", errContentUnavailable)
	}
	if endLine < startLine {
		return "", fmt.Errorf("%w: Sub-chapter end_line must be after start_line", errContentUnavailable)
	}

	normalized := strings.ReplaceAll(markdown, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	if int(startLine) > len(lines) {
		return "", fmt.Errorf("%w: Sub-chapter start_line is outside the markdown document", errContentUnavailable)
	}
	end := int(endLine)
	if end > len(lines) {
		end = len(lines)
	}
	selected := strings.TrimSpace(strings.Join(lines[int(startLine)-1:end], "\n"))
	if selected == "" {
		return "", fmt.Errorf("%w: No markdown text found for the sub_chapter_id", errContentUnavailable)
	}
	return selected, nil
}

type MarkdownLoader struct {
	client *s3.Client
}

func NewMarkdownLoader() (*MarkdownLoader, error) {
	region := envOrDefault("AWS_REGION", envOrDefault("AWS_DEFAULT_REGION", "us-east-1"))
	endpoint := strings.TrimRight(strings.TrimSpace(os.Getenv("AWS_S3_ENDPOINT_URL")), "/")
	accessKey := strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY_ID"))
	secretKey := strings.TrimSpace(os.Getenv("AWS_SECRET_ACCESS_KEY"))

	options := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}
	if accessKey != "" || secretKey != "" {
		options = append(options, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		))
	}
	if endpoint != "" {
		options = append(options, config.WithEndpointResolverWithOptions(
			aws.EndpointResolverWithOptionsFunc(
				func(service, region string, options ...any) (aws.Endpoint, error) {
					return aws.Endpoint{URL: endpoint, HostnameImmutable: true}, nil
				},
			),
		))
	}

	awsConfig, err := config.LoadDefaultConfig(context.Background(), options...)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		if endpoint != "" {
			options.UsePathStyle = true
		}
	})
	return &MarkdownLoader{client: client}, nil
}

func (m *MarkdownLoader) Download(ctx context.Context, bucket string, key string) (string, error) {
	output, err := m.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		return "", err
	}
	defer output.Body.Close()
	data, err := io.ReadAll(output.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func indexedUserUUID(value string) uuid.UUID {
	parsed, err := uuid.Parse(value)
	if err == nil {
		return parsed
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("ocr:user:"+value))
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
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
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}
