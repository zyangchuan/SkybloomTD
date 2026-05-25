package main_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"skybloom/document-content-api/internal/api"
	"skybloom/document-content-api/internal/api/mocks"
	"skybloom/document-content-api/internal/config"
	"skybloom/document-content-api/internal/models"
)

var hexIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var uuidIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func TestHealth(t *testing.T) {
	router := newTestRouter(t, nil, nil)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{"status":"ok"}`, response.Body.String())
}

func TestGetTaskStatusReturnsRedisStatus(t *testing.T) {
	taskStatus := mocks.NewMockTaskStatusStore(t)
	expected := models.TaskStatus{
		TaskID:     "task-1",
		DocumentID: "document-1",
		Status:     models.TaskStatusProcessing,
		Error:      nil,
		UpdatedAt:  time.Date(2026, 5, 25, 12, 30, 0, 0, time.UTC),
	}
	taskStatus.
		On("Get", mock.Anything, "task-1").
		Return(expected, nil).
		Once()

	router := newTestRouterWithDeps(t, config.Config{TempDir: t.TempDir()}, nil, nil, nil, taskStatus)
	request := httptest.NewRequest(http.MethodGet, "/tasks/task-1/status", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	var payload models.TaskStatus
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, expected, payload)
}

func TestGetTaskStatusReturnsNotFoundWhenRedisStatusIsMissing(t *testing.T) {
	taskStatus := mocks.NewMockTaskStatusStore(t)
	taskStatus.
		On("Get", mock.Anything, "expired-task").
		Return(models.TaskStatus{}, models.ErrTaskStatusNotFound).
		Once()

	router := newTestRouterWithDeps(t, config.Config{TempDir: t.TempDir()}, nil, nil, nil, taskStatus)
	request := httptest.NewRequest(http.MethodGet, "/tasks/expired-task/status", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.JSONEq(t, `{"error":"task status not found"}`, response.Body.String())
}

func TestGetTaskStatusReturnsServiceUnavailableWhenRedisFails(t *testing.T) {
	taskStatus := mocks.NewMockTaskStatusStore(t)
	taskStatus.
		On("Get", mock.Anything, "task-1").
		Return(models.TaskStatus{}, errors.New("redis unavailable")).
		Once()

	router := newTestRouterWithDeps(t, config.Config{TempDir: t.TempDir()}, nil, nil, nil, taskStatus)
	request := httptest.NewRequest(http.MethodGet, "/tasks/task-1/status", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.JSONEq(t, `{"error":"failed to read task status"}`, response.Body.String())
}

func TestListDocumentsReturnsCurrentUserDocuments(t *testing.T) {
	documents := mocks.NewMockDocumentStore(t)
	createdAt := time.Date(2026, 5, 25, 12, 30, 0, 0, time.UTC)
	updatedAt := createdAt.Add(2 * time.Minute)
	expected := []models.DocumentSummary{
		{
			DocumentID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			Filename:   "lesson.pdf",
			IsReady:    false,
			TaskID:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			CreatedAt:  createdAt,
			UpdatedAt:  updatedAt,
		},
		{
			DocumentID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			Filename:   "notes.pdf",
			IsReady:    true,
			TaskID:     "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			CreatedAt:  createdAt.Add(-time.Hour),
			UpdatedAt:  updatedAt.Add(-time.Hour),
		},
	}

	documents.
		On("ListUserDocuments", mock.Anything, models.DatabaseUUID("user_123", "user")).
		Return(expected, nil).
		Once()

	router := newTestRouterWithDeps(t, config.Config{TempDir: t.TempDir()}, nil, nil, documents, nil)
	request := httptest.NewRequest(http.MethodGet, "/documents", nil)
	request.Header.Set("X-Authenticated-User-Id", " user/123 ")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	var payload models.ListDocumentsResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, expected, payload.Documents)
}

func TestListDocumentsRequiresAuthentication(t *testing.T) {
	router := newTestRouter(t, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/documents", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.JSONEq(t, `{"error":"Authentication required"}`, response.Body.String())
}

func TestListDocumentsReturnsServiceUnavailableWhenRepositoryFails(t *testing.T) {
	documents := mocks.NewMockDocumentStore(t)
	documents.
		On("ListUserDocuments", mock.Anything, models.DatabaseUUID("user-1", "user")).
		Return(nil, errors.New("database unavailable")).
		Once()

	router := newTestRouterWithDeps(t, config.Config{TempDir: t.TempDir()}, nil, nil, documents, nil)
	request := httptest.NewRequest(http.MethodGet, "/documents", nil)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.JSONEq(t, `{"error":"failed to list documents"}`, response.Body.String())
}

func TestUploadFilePublishesDocumentJob(t *testing.T) {
	publisher := mocks.NewMockPublisher(t)
	uploader := mocks.NewMockSourceUploader(t)
	documents := mocks.NewMockDocumentStore(t)
	taskStatus := mocks.NewMockTaskStatusStore(t)
	source := models.SourceRef{
		Type:        "s3",
		Bucket:      "documents",
		Key:         "users/user_123/documents/document/source/lesson_1.pdf",
		Filename:    "lesson_1.pdf",
		ContentType: "application/pdf",
	}

	uploader.
		On(
			"UploadSource",
			mock.Anything,
			[]byte("pdf bytes"),
			"user_123",
			mock.MatchedBy(isUUID),
			"lesson_1.pdf",
			"application/pdf",
		).
		Return(source, nil).
		Once()

	var storedDocument models.Document
	documents.
		On(
			"CreateQueuedDocument",
			mock.Anything,
			mock.MatchedBy(func(document models.Document) bool {
				return isUUID(document.ID.String()) &&
					document.UserID == models.DatabaseUUID("user_123", "user") &&
					document.Filename == "lesson_1.pdf" &&
					isHexID(document.TaskID) &&
					!document.IsReady &&
					document.SourceType == "s3" &&
					stringValue(document.SourceBucket) == source.Bucket &&
					stringValue(document.SourceKey) == source.Key &&
					stringValue(document.SourceContentType) == source.ContentType
			}),
		).
		Run(func(args mock.Arguments) {
			storedDocument = args.Get(1).(models.Document)
		}).
		Return(nil).
		Once()

	var queuedStatus models.TaskStatus
	taskStatus.
		On(
			"Set",
			mock.Anything,
			mock.MatchedBy(func(status models.TaskStatus) bool {
				return status.Status == models.TaskStatusQueued &&
					isHexID(status.TaskID) &&
					isUUID(status.DocumentID) &&
					status.Error == nil &&
					!status.UpdatedAt.IsZero()
			}),
		).
		Run(func(args mock.Arguments) {
			queuedStatus = args.Get(1).(models.TaskStatus)
		}).
		Return(nil).
		Once()

	var publishedJob models.DocumentJob
	publisher.
		On(
			"Publish",
			mock.Anything,
			mock.MatchedBy(isHexID),
			mock.MatchedBy(func(value any) bool {
				job, ok := value.(models.DocumentJob)
				if !ok {
					return false
				}
				return job.JobType == "document.process" &&
					isHexID(job.TaskID) &&
					isHexID(job.OCRTaskID) &&
					isHexID(job.UploadTaskID) &&
					isHexID(job.IndexTaskID) &&
					job.TaskID != job.IndexTaskID &&
					job.UserID == "user_123" &&
					isUUID(job.DocumentID) &&
					job.Filename == "lesson_1.pdf" &&
					assert.ObjectsAreEqual(source, job.Source)
			}),
		).
		Run(func(args mock.Arguments) {
			publishedJob = args.Get(2).(models.DocumentJob)
			assert.Equal(t, publishedJob.TaskID, args.String(1))
		}).
		Return(nil).
		Once()

	router := newTestRouterWithDeps(t, config.Config{TempDir: t.TempDir()}, publisher, uploader, documents, taskStatus)
	body, contentType := multipartBody(t, "lesson 1.pdf", "application/pdf", []byte("pdf bytes"))
	request := httptest.NewRequest(http.MethodPost, "/upload-file", body)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-Authenticated-User-Id", " user/123 ")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "Upload file success", payload["message"])
	assert.Equal(t, "user_123", payload["user_id"])
	assert.Equal(t, publishedJob.DocumentID, payload["document_id"])
	assert.Equal(t, publishedJob.TaskID, payload["task_id"])
	assert.Equal(t, publishedJob.OCRTaskID, payload["ocr_task_id"])
	assert.Equal(t, publishedJob.UploadTaskID, payload["upload_task_id"])
	assert.Equal(t, publishedJob.IndexTaskID, payload["index_task_id"])
	assert.Equal(t, false, payload["is_ready"])
	assert.Equal(t, storedDocument.ID.String(), publishedJob.DocumentID)
	assert.Equal(t, storedDocument.TaskID, publishedJob.TaskID)
	assert.Equal(t, queuedStatus.TaskID, publishedJob.TaskID)
	assert.Equal(t, queuedStatus.DocumentID, publishedJob.DocumentID)
}

func TestUploadFileRequiresAuthentication(t *testing.T) {
	router := newTestRouter(t, mocks.NewMockPublisher(t), mocks.NewMockSourceUploader(t))
	body, contentType := multipartBody(t, "lesson.pdf", "application/pdf", []byte("pdf bytes"))
	request := httptest.NewRequest(http.MethodPost, "/upload-file", body)
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.JSONEq(t, `{"error":"Authentication required"}`, response.Body.String())
}

func TestUploadFileRequiresFile(t *testing.T) {
	router := newTestRouter(t, mocks.NewMockPublisher(t), mocks.NewMockSourceUploader(t))
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.Close())
	request := httptest.NewRequest(http.MethodPost, "/upload-file", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.JSONEq(t, `{"error":"file is required"}`, response.Body.String())
}

func TestUploadFileReturnsServiceUnavailableWhenPublishFails(t *testing.T) {
	publisher := mocks.NewMockPublisher(t)
	uploader := mocks.NewMockSourceUploader(t)
	documents := mocks.NewMockDocumentStore(t)
	taskStatus := mocks.NewMockTaskStatusStore(t)
	source := models.SourceRef{Type: "s3", Bucket: "documents", Key: "source.pdf", Filename: "source.pdf"}

	uploader.
		On("UploadSource", mock.Anything, []byte("pdf bytes"), "user-1", mock.MatchedBy(isUUID), "lesson.pdf", "application/pdf").
		Return(source, nil).
		Once()
	documents.
		On("CreateQueuedDocument", mock.Anything, mock.MatchedBy(func(document models.Document) bool {
			return isUUID(document.ID.String()) &&
				document.UserID == models.DatabaseUUID("user-1", "user") &&
				document.Filename == "lesson.pdf" &&
				isHexID(document.TaskID) &&
				!document.IsReady
		})).
		Return(nil).
		Once()
	taskStatus.
		On("Set", mock.Anything, mock.MatchedBy(func(status models.TaskStatus) bool {
			return status.Status == models.TaskStatusQueued && status.Error == nil
		})).
		Return(nil).
		Once()
	publisher.
		On("Publish", mock.Anything, mock.MatchedBy(isHexID), mock.AnythingOfType("models.DocumentJob")).
		Return(errors.New("rabbitmq unavailable")).
		Once()
	taskStatus.
		On("Set", mock.Anything, mock.MatchedBy(func(status models.TaskStatus) bool {
			return status.Status == models.TaskStatusFailed &&
				status.Error != nil &&
				*status.Error == "rabbitmq unavailable"
		})).
		Return(nil).
		Once()

	router := newTestRouterWithDeps(t, config.Config{TempDir: t.TempDir()}, publisher, uploader, documents, taskStatus)
	body, contentType := multipartBody(t, "lesson.pdf", "application/pdf", []byte("pdf bytes"))
	request := httptest.NewRequest(http.MethodPost, "/upload-file", body)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.JSONEq(t, `{"error":"failed to enqueue document job"}`, response.Body.String())
}

func TestUploadFileReturnsInternalServerErrorWhenSourceUploadFails(t *testing.T) {
	publisher := mocks.NewMockPublisher(t)
	uploader := mocks.NewMockSourceUploader(t)
	uploader.
		On("UploadSource", mock.Anything, []byte("pdf bytes"), "user-1", mock.MatchedBy(isUUID), "lesson.pdf", "application/pdf").
		Return(models.SourceRef{}, errors.New("storage unavailable")).
		Once()

	router := newTestRouter(t, publisher, uploader)
	body, contentType := multipartBody(t, "lesson.pdf", "application/pdf", []byte("pdf bytes"))
	request := httptest.NewRequest(http.MethodPost, "/upload-file", body)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.JSONEq(t, `{"error":"failed to store upload"}`, response.Body.String())
}

func TestUploadFileDoesNotPublishWhenDocumentCreateFails(t *testing.T) {
	publisher := mocks.NewMockPublisher(t)
	uploader := mocks.NewMockSourceUploader(t)
	documents := mocks.NewMockDocumentStore(t)
	taskStatus := mocks.NewMockTaskStatusStore(t)
	source := models.SourceRef{Type: "s3", Bucket: "documents", Key: "source.pdf", Filename: "source.pdf"}

	uploader.
		On("UploadSource", mock.Anything, []byte("pdf bytes"), "user-1", mock.MatchedBy(isUUID), "lesson.pdf", "application/pdf").
		Return(source, nil).
		Once()
	documents.
		On("CreateQueuedDocument", mock.Anything, mock.AnythingOfType("models.Document")).
		Return(errors.New("database unavailable")).
		Once()

	router := newTestRouterWithDeps(t, config.Config{TempDir: t.TempDir()}, publisher, uploader, documents, taskStatus)
	body, contentType := multipartBody(t, "lesson.pdf", "application/pdf", []byte("pdf bytes"))
	request := httptest.NewRequest(http.MethodPost, "/upload-file", body)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.JSONEq(t, `{"error":"failed to create document"}`, response.Body.String())
}

func TestUploadFileDoesNotPublishWhenQueuedStatusWriteFails(t *testing.T) {
	publisher := mocks.NewMockPublisher(t)
	uploader := mocks.NewMockSourceUploader(t)
	documents := mocks.NewMockDocumentStore(t)
	taskStatus := mocks.NewMockTaskStatusStore(t)
	source := models.SourceRef{Type: "s3", Bucket: "documents", Key: "source.pdf", Filename: "source.pdf"}

	uploader.
		On("UploadSource", mock.Anything, []byte("pdf bytes"), "user-1", mock.MatchedBy(isUUID), "lesson.pdf", "application/pdf").
		Return(source, nil).
		Once()
	documents.
		On("CreateQueuedDocument", mock.Anything, mock.AnythingOfType("models.Document")).
		Return(nil).
		Once()
	taskStatus.
		On("Set", mock.Anything, mock.MatchedBy(func(status models.TaskStatus) bool {
			return status.Status == models.TaskStatusQueued && status.Error == nil
		})).
		Return(errors.New("redis unavailable")).
		Once()

	router := newTestRouterWithDeps(t, config.Config{TempDir: t.TempDir()}, publisher, uploader, documents, taskStatus)
	body, contentType := multipartBody(t, "lesson.pdf", "application/pdf", []byte("pdf bytes"))
	request := httptest.NewRequest(http.MethodPost, "/upload-file", body)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.JSONEq(t, `{"error":"failed to record task status"}`, response.Body.String())
}

func TestUploadFileFallsBackToTempStorageWhenUploaderIsNil(t *testing.T) {
	tempDir := t.TempDir()
	publisher := mocks.NewMockPublisher(t)
	documents := mocks.NewMockDocumentStore(t)
	taskStatus := mocks.NewMockTaskStatusStore(t)
	var publishedJob models.DocumentJob
	var storedDocument models.Document
	documents.
		On("CreateQueuedDocument", mock.Anything, mock.MatchedBy(func(document models.Document) bool {
			return isUUID(document.ID.String()) &&
				document.SourceType == "local" &&
				stringValue(document.SourcePath) == filepath.Join(tempDir, "user-1", document.ID.String(), "input.txt") &&
				!document.IsReady
		})).
		Run(func(args mock.Arguments) {
			storedDocument = args.Get(1).(models.Document)
		}).
		Return(nil).
		Once()
	taskStatus.
		On("Set", mock.Anything, mock.MatchedBy(func(status models.TaskStatus) bool {
			return status.Status == models.TaskStatusQueued && status.Error == nil
		})).
		Return(nil).
		Once()
	publisher.
		On("Publish", mock.Anything, mock.MatchedBy(isHexID), mock.AnythingOfType("models.DocumentJob")).
		Run(func(args mock.Arguments) {
			publishedJob = args.Get(2).(models.DocumentJob)
		}).
		Return(nil).
		Once()

	router := newTestRouterWithDeps(t, config.Config{TempDir: tempDir}, publisher, nil, documents, taskStatus)
	body, contentType := multipartBody(t, "notes.txt", "text/plain", []byte("plain text"))
	request := httptest.NewRequest(http.MethodPost, "/upload-file", body)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	sourcePath, ok := publishedJob.Source.(string)
	require.True(t, ok)
	assert.Equal(t, filepath.Join(tempDir, "user-1", publishedJob.DocumentID, "input.txt"), sourcePath)
	assert.Equal(t, storedDocument.ID.String(), publishedJob.DocumentID)
	content, err := os.ReadFile(sourcePath)
	require.NoError(t, err)
	assert.Equal(t, []byte("plain text"), content)
}

func newTestRouter(t *testing.T, publisher api.Publisher, uploader api.SourceUploader) *gin.Engine {
	return newTestRouterWithDeps(t, config.Config{TempDir: t.TempDir()}, publisher, uploader, nil, nil)
}

func newTestRouterWithDeps(
	t *testing.T,
	cfg config.Config,
	publisher api.Publisher,
	uploader api.SourceUploader,
	documents api.DocumentStore,
	taskStatus api.TaskStatusStore,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gin.DefaultWriter = io.Discard
	previousLogWriter := log.Writer()
	log.SetOutput(io.Discard)
	t.Cleanup(func() {
		log.SetOutput(previousLogWriter)
	})
	if cfg.TempDir == "" {
		cfg.TempDir = t.TempDir()
	}
	return api.NewRouter(cfg, publisher, uploader, documents, taskStatus)
}

func multipartBody(t *testing.T, filename string, contentType string, content []byte) (*bytes.Buffer, string) {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return body, writer.FormDataContentType()
}

func isHexID(value string) bool {
	return hexIDPattern.MatchString(value)
}

func isUUID(value string) bool {
	return uuidIDPattern.MatchString(value)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
