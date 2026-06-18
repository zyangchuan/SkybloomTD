package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"skybloom/document-content-grpc/internal/models"
	"skybloom/document-content-grpc/internal/repository"
)

type DocumentRepository interface {
	LoadDocumentRow(ctx context.Context, subChapterID uuid.UUID, userID uuid.UUID) (repository.DocumentRow, error)
}

type MarkdownLoader interface {
	Download(ctx context.Context, bucket string, key string) (string, error)
}

type Server struct {
	documents DocumentRepository
	loader    MarkdownLoader
}

var (
	errBadRequest         = errors.New("bad request")
	errContentUnavailable = errors.New("content unavailable")
)

func NewServer(documents DocumentRepository, loader MarkdownLoader) *Server {
	return &Server{documents: documents, loader: loader}
}

func (s *Server) GetSubChapter(ctx context.Context, request *models.GetSubChapterRequest) (*models.GetSubChapterResponse, error) {
	content, err := s.fetchSubChapterContent(ctx, request.UserID, request.SubChapterID, request.MaxChars)
	if err != nil {
		switch {
		case errors.Is(err, errBadRequest):
			return nil, status.Error(codes.InvalidArgument, err.Error())
		case errors.Is(err, repository.ErrNotFound):
			return nil, status.Error(codes.NotFound, err.Error())
		case errors.Is(err, errContentUnavailable):
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		default:
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	return &models.GetSubChapterResponse{SubChapter: content}, nil
}

func (s *Server) fetchSubChapterContent(ctx context.Context, requestedUserID string, subChapterID string, maxChars int32) (models.SubChapterContent, error) {
	if strings.TrimSpace(requestedUserID) == "" {
		return models.SubChapterContent{}, fmt.Errorf("%w: user_id is required", errBadRequest)
	}
	if maxChars < 0 {
		return models.SubChapterContent{}, fmt.Errorf("%w: max_chars must be zero or greater", errBadRequest)
	}

	subChapterUUID, err := uuid.Parse(subChapterID)
	if err != nil {
		return models.SubChapterContent{}, fmt.Errorf("%w: sub_chapter_id must be a valid UUID", errBadRequest)
	}
	userUUID := indexedUserUUID(requestedUserID)

	row, err := s.documents.LoadDocumentRow(ctx, subChapterUUID, userUUID)
	if err != nil {
		return models.SubChapterContent{}, err
	}
	if row.S3Bucket == nil || strings.TrimSpace(*row.S3Bucket) == "" {
		return models.SubChapterContent{}, fmt.Errorf("%w: Document markdown S3 location is missing", errContentUnavailable)
	}

	startLine := int32Value(row.StartLine)
	endLine := int32Value(row.EndLine)
	markdownKey := fmt.Sprintf("%s/%s/output.md", row.UserID.String(), row.DocumentID.String())
	markdown, err := s.loader.Download(ctx, *row.S3Bucket, markdownKey)
	if err != nil {
		return models.SubChapterContent{}, err
	}
	sourceText, err := sliceMarkdownLines(markdown, startLine, endLine)
	if err != nil {
		return models.SubChapterContent{}, err
	}

	sourceCharCount := int32(len([]rune(sourceText)))
	sourceTruncated := maxChars > 0 && sourceCharCount > maxChars
	if sourceTruncated {
		sourceText = string([]rune(sourceText)[:maxChars])
	}

	return models.SubChapterContent{
		NormalizedUserID:    userUUID.String(),
		RequestedUserID:     requestedUserID,
		SubChapterID:        row.SubChapterID.String(),
		DocumentID:          row.DocumentID.String(),
		ChapterID:           row.ChapterID.String(),
		SubChapterIndex:     int32Value(row.SubChapterIndex),
		Title:               stringValue(row.Title),
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

func int32Value(value *int32) int32 {
	if value == nil {
		return 0
	}
	return *value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
