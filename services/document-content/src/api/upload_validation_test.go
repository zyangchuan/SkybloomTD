package main_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"skybloom/document-content-api/internal/api/mocks"
	"skybloom/document-content-api/internal/config"
	"skybloom/document-content-api/internal/models"
)

// ---------------------------------------------------------------------------
// Game-name field validation
// ---------------------------------------------------------------------------

func TestUploadFileRejectsGameNameExceedingMaxLength(t *testing.T) {
	router := newTestRouter(t, mocks.NewMockPublisher(t), mocks.NewMockSourceUploader(t))
	longName := strings.Repeat("a", 121) // 121 characters — one over the 120-char limit
	body, contentType := multipartBodyWithFields(t, "notes.pdf", "application/pdf", []byte("pdf bytes"), map[string]string{
		"game_name": longName,
	})

	request := httptest.NewRequest(http.MethodPost, "/upload-file", body)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.JSONEq(t, `{"error":"game_name must be 120 characters or fewer"}`, response.Body.String())
}

func TestUploadFileAcceptsGameNameAtMaxLength(t *testing.T) {
	publisher := mocks.NewMockPublisher(t)
	uploader := mocks.NewMockSourceUploader(t)
	documents := mocks.NewMockDocumentStore(t)
	taskStatus := mocks.NewMockTaskStatusStore(t)

	exactName := strings.Repeat("a", 120) // exactly 120 characters

	uploader.
		On("UploadSource", mock.Anything, mock.Anything, "user-1", mock.MatchedBy(isUUID), "notes.pdf", "application/pdf").
		Return(models.SourceRef{Bucket: "documents", Key: "source.pdf", Filename: "notes.pdf"}, nil).
		Once()
	documents.
		On("CreateQueuedDocument", mock.Anything, mock.MatchedBy(func(doc models.Document) bool {
			return doc.GameName == exactName
		})).
		Return(nil).
		Once()
	taskStatus.
		On("Set", mock.Anything, mock.Anything).
		Return(nil).
		Once()
	publisher.
		On("Publish", mock.Anything, mock.Anything, mock.Anything).
		Return(nil).
		Once()

	router := newTestRouterWithDeps(t, config.Config{}, publisher, uploader, documents, taskStatus)
	body, contentType := multipartBodyWithFields(t, "notes.pdf", "application/pdf", []byte("pdf bytes"), map[string]string{
		"game_name": exactName,
	})
	request := httptest.NewRequest(http.MethodPost, "/upload-file", body)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
}

func TestUploadFileRejectsEmptyGameName(t *testing.T) {
	router := newTestRouter(t, mocks.NewMockPublisher(t), mocks.NewMockSourceUploader(t))
	body, contentType := multipartBodyWithFields(t, "notes.pdf", "application/pdf", []byte("pdf bytes"), map[string]string{
		"game_name": "",
	})

	request := httptest.NewRequest(http.MethodPost, "/upload-file", body)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Contains(t, response.Body.String(), "game_name")
}

// ---------------------------------------------------------------------------
// Filename sanitisation
// ---------------------------------------------------------------------------

func TestUploadFileSanitizesPathTraversalInFilename(t *testing.T) {
	publisher := mocks.NewMockPublisher(t)
	uploader := mocks.NewMockSourceUploader(t)
	documents := mocks.NewMockDocumentStore(t)
	taskStatus := mocks.NewMockTaskStatusStore(t)

	// The server must strip leading path components from the filename before
	// forwarding it to storage and the database.
	uploader.
		On(
			"UploadSource",
			mock.Anything,
			mock.Anything,
			"user-1",
			mock.MatchedBy(isUUID),
			mock.MatchedBy(func(filename string) bool {
				// Must NOT contain ".." or directory separators in the stored name.
				return !strings.Contains(filename, "..") && !strings.Contains(filename, "/")
			}),
			mock.Anything,
		).
		Return(models.SourceRef{Bucket: "documents", Key: "source.pdf", Filename: "passwd.pdf"}, nil).
		Once()
	documents.
		On("CreateQueuedDocument", mock.Anything, mock.Anything).
		Return(nil).
		Once()
	taskStatus.
		On("Set", mock.Anything, mock.Anything).
		Return(nil).
		Once()
	publisher.
		On("Publish", mock.Anything, mock.Anything, mock.Anything).
		Return(nil).
		Once()

	router := newTestRouterWithDeps(t, config.Config{}, publisher, uploader, documents, taskStatus)
	body, contentType := multipartBodyWithFields(
		t,
		"../../../etc/passwd.pdf",
		"application/pdf",
		[]byte("pdf bytes"),
		map[string]string{"game_name": defaultTestGameName},
	)
	request := httptest.NewRequest(http.MethodPost, "/upload-file", body)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	// The request itself should succeed; the sanitised name is an implementation detail
	// verified via the mock expectation above.
	require.Equal(t, http.StatusOK, response.Code)
}

func TestUploadFileSanitizesWindowsPathSeparator(t *testing.T) {
	publisher := mocks.NewMockPublisher(t)
	uploader := mocks.NewMockSourceUploader(t)
	documents := mocks.NewMockDocumentStore(t)
	taskStatus := mocks.NewMockTaskStatusStore(t)

	uploader.
		On(
			"UploadSource",
			mock.Anything,
			mock.Anything,
			"user-1",
			mock.MatchedBy(isUUID),
			mock.MatchedBy(func(filename string) bool {
				return !strings.Contains(filename, "\\") && !strings.Contains(filename, "..")
			}),
			mock.Anything,
		).
		Return(models.SourceRef{Bucket: "documents", Key: "source.pdf", Filename: "notes.pdf"}, nil).
		Once()
	documents.
		On("CreateQueuedDocument", mock.Anything, mock.Anything).
		Return(nil).
		Once()
	taskStatus.
		On("Set", mock.Anything, mock.Anything).
		Return(nil).
		Once()
	publisher.
		On("Publish", mock.Anything, mock.Anything, mock.Anything).
		Return(nil).
		Once()

	router := newTestRouterWithDeps(t, config.Config{}, publisher, uploader, documents, taskStatus)
	body, contentType := multipartBodyWithFields(
		t,
		`C:\Users\attacker\notes.pdf`,
		"application/pdf",
		[]byte("pdf bytes"),
		map[string]string{"game_name": defaultTestGameName},
	)
	request := httptest.NewRequest(http.MethodPost, "/upload-file", body)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
}

// ---------------------------------------------------------------------------
// Level access — document and chapter list endpoints
// ---------------------------------------------------------------------------

func TestListDocumentChaptersRequiresAuthentication(t *testing.T) {
	router := newTestRouter(t, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/documents/33333333-3333-3333-3333-333333333333/chapters", nil)
	// No auth header.
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestListChapterSubChaptersRequiresAuthentication(t *testing.T) {
	router := newTestRouter(t, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/chapters/44444444-4444-4444-4444-444444444444/sub-chapters", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
}

// ---------------------------------------------------------------------------
// Task status — error cases
// ---------------------------------------------------------------------------

func TestGetTaskStatusReturnsNotFoundForUnknownTask(t *testing.T) {
	taskStatus := mocks.NewMockTaskStatusStore(t)
	taskStatus.
		On("Get", mock.Anything, "unknown-task").
		Return(models.TaskStatus{}, models.ErrTaskStatusNotFound).
		Once()

	router := newTestRouterWithDeps(t, config.Config{}, nil, nil, nil, taskStatus)
	request := httptest.NewRequest(http.MethodGet, "/tasks/unknown-task/status", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.JSONEq(t, `{"error":"task status not found"}`, response.Body.String())
}
