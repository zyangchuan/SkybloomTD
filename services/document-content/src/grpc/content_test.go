package main_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"skybloom/document-content-grpc/internal/models"
	"skybloom/document-content-grpc/internal/repository"
	"skybloom/document-content-grpc/internal/service"
	"skybloom/document-content-grpc/internal/service/mocks"
)

func TestGetSubChapterReturnsMarkdownLineRange(t *testing.T) {
	documents := mocks.NewMockDocumentRepository(t)
	loader := mocks.NewMockMarkdownLoader(t)
	server := service.NewServer(documents, loader)

	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	subChapterID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	documentID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	chapterID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	row := repository.DocumentRow{
		SubChapterID:    subChapterID,
		DocumentID:      documentID,
		ChapterID:       chapterID,
		SubChapterIndex: ptr(int32(3)),
		Title:           ptr("Antiderivatives"),
		StartLine:       ptr(int32(2)),
		EndLine:         ptr(int32(4)),
		S3Bucket:        ptr("documents"),
		S3Key:           ptr("markdown/source.md"),
	}
	markdown := "line 1\nline 2\nline 3\nline 4\nline 5"
	expectedSource := "line 2\nline 3\nline 4"

	documents.
		On("LoadDocumentRow", mock.Anything, subChapterID, userID).
		Return(row, nil).
		Once()
	loader.
		On("Download", mock.Anything, "documents", "markdown/source.md").
		Return(markdown, nil).
		Once()

	response, err := server.GetSubChapter(context.Background(), &models.GetSubChapterRequest{
		UserID:       userID.String(),
		SubChapterID: subChapterID.String(),
	})

	require.NoError(t, err)
	require.NotNil(t, response)
	subChapter := response.SubChapter
	assert.Equal(t, userID.String(), subChapter.NormalizedUserID)
	assert.Equal(t, userID.String(), subChapter.RequestedUserID)
	assert.Equal(t, subChapterID.String(), subChapter.SubChapterID)
	assert.Equal(t, documentID.String(), subChapter.DocumentID)
	assert.Equal(t, chapterID.String(), subChapter.ChapterID)
	assert.Equal(t, int32(3), subChapter.SubChapterIndex)
	assert.Equal(t, "Antiderivatives", subChapter.Title)
	assert.Equal(t, int32(2), subChapter.StartLine)
	assert.Equal(t, int32(4), subChapter.EndLine)
	assert.Equal(t, expectedSource, subChapter.SourceText)
	assert.Equal(t, int32(len([]rune(expectedSource))), subChapter.SourceCharCount)
	assert.False(t, subChapter.SourceTruncated)
	assert.Equal(t, "s3_markdown_line_range", subChapter.ChunkLookupStrategy)
	assert.Equal(t, sha256Hex(expectedSource), subChapter.SourceContentHash)
}

func TestGetSubChapterTruncatesSourceText(t *testing.T) {
	documents := mocks.NewMockDocumentRepository(t)
	loader := mocks.NewMockMarkdownLoader(t)
	server := service.NewServer(documents, loader)

	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	subChapterID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	row := documentRow(subChapterID)
	documents.
		On("LoadDocumentRow", mock.Anything, subChapterID, userID).
		Return(row, nil).
		Once()
	loader.
		On("Download", mock.Anything, "documents", "markdown/source.md").
		Return("alpha\nbeta\ngamma", nil).
		Once()

	response, err := server.GetSubChapter(context.Background(), &models.GetSubChapterRequest{
		UserID:       userID.String(),
		SubChapterID: subChapterID.String(),
		MaxChars:     7,
	})

	require.NoError(t, err)
	assert.Equal(t, "alpha\nb", response.SubChapter.SourceText)
	assert.Equal(t, int32(len([]rune("alpha\nbeta\ngamma"))), response.SubChapter.SourceCharCount)
	assert.True(t, response.SubChapter.SourceTruncated)
	assert.Equal(t, sha256Hex("alpha\nb"), response.SubChapter.SourceContentHash)
}

func TestGetSubChapterRejectsInvalidRequests(t *testing.T) {
	server := service.NewServer(mocks.NewMockDocumentRepository(t), mocks.NewMockMarkdownLoader(t))

	_, err := server.GetSubChapter(context.Background(), &models.GetSubChapterRequest{
		UserID:       "",
		SubChapterID: "22222222-2222-2222-2222-222222222222",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestGetSubChapterMapsMissingRowsToNotFound(t *testing.T) {
	documents := mocks.NewMockDocumentRepository(t)
	loader := mocks.NewMockMarkdownLoader(t)
	server := service.NewServer(documents, loader)

	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	subChapterID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	documents.
		On("LoadDocumentRow", mock.Anything, subChapterID, userID).
		Return(repository.DocumentRow{}, fmt.Errorf("%w: missing", repository.ErrNotFound)).
		Once()

	_, err := server.GetSubChapter(context.Background(), &models.GetSubChapterRequest{
		UserID:       userID.String(),
		SubChapterID: subChapterID.String(),
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestGetSubChapterRequiresMarkdownLocation(t *testing.T) {
	documents := mocks.NewMockDocumentRepository(t)
	loader := mocks.NewMockMarkdownLoader(t)
	server := service.NewServer(documents, loader)

	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	subChapterID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	row := documentRow(subChapterID)
	row.S3Bucket = nil
	documents.
		On("LoadDocumentRow", mock.Anything, subChapterID, userID).
		Return(row, nil).
		Once()

	_, err := server.GetSubChapter(context.Background(), &models.GetSubChapterRequest{
		UserID:       userID.String(),
		SubChapterID: subChapterID.String(),
	})

	require.Error(t, err)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	loader.AssertNotCalled(t, "Download", mock.Anything, mock.Anything, mock.Anything)
}

func documentRow(subChapterID uuid.UUID) repository.DocumentRow {
	return repository.DocumentRow{
		SubChapterID:    subChapterID,
		DocumentID:      uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		ChapterID:       uuid.MustParse("44444444-4444-4444-4444-444444444444"),
		SubChapterIndex: ptr(int32(1)),
		Title:           ptr("Section"),
		StartLine:       ptr(int32(1)),
		EndLine:         ptr(int32(3)),
		S3Bucket:        ptr("documents"),
		S3Key:           ptr("markdown/source.md"),
	}
}

func ptr[T any](value T) *T {
	return &value
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
