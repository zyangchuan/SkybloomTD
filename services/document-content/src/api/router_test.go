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

const defaultTestGameName = "Linear Algebra TD"

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

	router := newTestRouterWithDeps(t, config.Config{}, nil, nil, nil, taskStatus)
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

	router := newTestRouterWithDeps(t, config.Config{}, nil, nil, nil, taskStatus)
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

	router := newTestRouterWithDeps(t, config.Config{}, nil, nil, nil, taskStatus)
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
			GameName:   defaultTestGameName,
			IsReady:    false,
			TaskID:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			CreatedAt:  createdAt,
			UpdatedAt:  updatedAt,
		},
		{
			DocumentID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			Filename:   "notes.pdf",
			GameName:   "Calculus Arena",
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

	router := newTestRouterWithDeps(t, config.Config{}, nil, nil, documents, nil)
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

	router := newTestRouterWithDeps(t, config.Config{}, nil, nil, documents, nil)
	request := httptest.NewRequest(http.MethodGet, "/documents", nil)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.JSONEq(t, `{"error":"failed to list documents"}`, response.Body.String())
}

func TestListDocumentChaptersReturnsCurrentUserChapters(t *testing.T) {
	documents := mocks.NewMockDocumentStore(t)
	documentID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	userID := models.DatabaseUUID("user-1", "user")
	chapterIndex := 1
	title := "Vectors"
	startLine := 10
	endLine := 80
	expected := []models.ChapterSummary{
		{
			ChapterID:    uuid.MustParse("44444444-4444-4444-4444-444444444444"),
			DocumentID:   documentID,
			ChapterIndex: &chapterIndex,
			Title:        &title,
			StartLine:    &startLine,
			EndLine:      &endLine,
			CreatedAt:    time.Date(2026, 5, 25, 12, 30, 0, 0, time.UTC),
		},
	}

	documents.
		On("ListDocumentChapters", mock.Anything, documentID, userID).
		Return(expected, nil).
		Once()

	router := newTestRouterWithDeps(t, config.Config{}, nil, nil, documents, nil)
	request := httptest.NewRequest(http.MethodGet, "/documents/"+documentID.String()+"/chapters", nil)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	var payload models.ListChaptersResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, expected, payload.Chapters)
}

func TestListDocumentChaptersRejectsInvalidDocumentID(t *testing.T) {
	router := newTestRouter(t, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/documents/not-a-uuid/chapters", nil)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.JSONEq(t, `{"error":"invalid document_id"}`, response.Body.String())
}

func TestListDocumentChaptersReturnsNotFoundWhenDocumentDoesNotBelongToUser(t *testing.T) {
	documents := mocks.NewMockDocumentStore(t)
	documentID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	userID := models.DatabaseUUID("user-1", "user")
	documents.
		On("ListDocumentChapters", mock.Anything, documentID, userID).
		Return(nil, models.ErrDocumentNotFound).
		Once()

	router := newTestRouterWithDeps(t, config.Config{}, nil, nil, documents, nil)
	request := httptest.NewRequest(http.MethodGet, "/documents/"+documentID.String()+"/chapters", nil)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.JSONEq(t, `{"error":"document not found"}`, response.Body.String())
}

func TestListChapterSubChaptersReturnsCurrentUserSubChapters(t *testing.T) {
	documents := mocks.NewMockDocumentStore(t)
	chapterID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	documentID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	userID := models.DatabaseUUID("user-1", "user")
	subChapterIndex := 2
	title := "Basis"
	startLine := 20
	endLine := 40
	expected := []models.SubChapterSummary{
		{
			SubChapterID:    uuid.MustParse("55555555-5555-5555-5555-555555555555"),
			DocumentID:      documentID,
			ChapterID:       chapterID,
			SubChapterIndex: &subChapterIndex,
			Title:           &title,
			StartLine:       &startLine,
			EndLine:         &endLine,
			CreatedAt:       time.Date(2026, 5, 25, 12, 45, 0, 0, time.UTC),
		},
	}

	documents.
		On("ListChapterSubChapters", mock.Anything, chapterID, userID).
		Return(expected, nil).
		Once()

	router := newTestRouterWithDeps(t, config.Config{}, nil, nil, documents, nil)
	request := httptest.NewRequest(http.MethodGet, "/chapters/"+chapterID.String()+"/sub-chapters", nil)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	var payload models.ListSubChaptersResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, expected, payload.SubChapters)
}

func TestListChapterSubChaptersRejectsInvalidChapterID(t *testing.T) {
	router := newTestRouter(t, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/chapters/not-a-uuid/sub-chapters", nil)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.JSONEq(t, `{"error":"invalid chapter_id"}`, response.Body.String())
}

func TestListChapterSubChaptersReturnsNotFoundWhenChapterDoesNotBelongToUser(t *testing.T) {
	documents := mocks.NewMockDocumentStore(t)
	chapterID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	userID := models.DatabaseUUID("user-1", "user")
	documents.
		On("ListChapterSubChapters", mock.Anything, chapterID, userID).
		Return(nil, models.ErrChapterNotFound).
		Once()

	router := newTestRouterWithDeps(t, config.Config{}, nil, nil, documents, nil)
	request := httptest.NewRequest(http.MethodGet, "/chapters/"+chapterID.String()+"/sub-chapters", nil)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.JSONEq(t, `{"error":"chapter not found"}`, response.Body.String())
}

func TestDeleteDocumentDeletesAssetsAndDatabaseRows(t *testing.T) {
	documents := mocks.NewMockDocumentStore(t)
	uploader := mocks.NewMockSourceUploader(t)
	documentID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	userID := models.DatabaseUUID("user-1", "user")
	outputBucket := "documents"
	outputKey := "dev/users/user-1/documents/33333333-3333-3333-3333-333333333333/output.md"
	document := models.Document{
		ID:       documentID,
		UserID:   userID,
		Filename: "lesson.pdf",
		TaskID:   "cccccccccccccccccccccccccccccccc",
		S3Bucket: &outputBucket,
		S3Key:    &outputKey,
	}

	documents.
		On("LoadUserDocument", mock.Anything, documentID, userID).
		Return(document, nil).
		Once()
	uploader.
		On("DeleteDocumentFiles", mock.Anything, document).
		Return(nil).
		Once()
	documents.
		On("DeleteDocumentCascade", mock.Anything, documentID, userID).
		Return(nil).
		Once()

	router := newTestRouterWithDeps(t, config.Config{}, nil, uploader, documents, nil)
	request := httptest.NewRequest(http.MethodDelete, "/documents/"+documentID.String(), nil)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
	assert.Empty(t, response.Body.String())
}

func TestDeleteDocumentRequiresAuthentication(t *testing.T) {
	router := newTestRouter(t, nil, nil)
	request := httptest.NewRequest(http.MethodDelete, "/documents/33333333-3333-3333-3333-333333333333", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.JSONEq(t, `{"error":"Authentication required"}`, response.Body.String())
}

func TestDeleteDocumentRejectsInvalidDocumentID(t *testing.T) {
	router := newTestRouter(t, nil, nil)
	request := httptest.NewRequest(http.MethodDelete, "/documents/not-a-uuid", nil)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.JSONEq(t, `{"error":"invalid document_id"}`, response.Body.String())
}

func TestDeleteDocumentReturnsNotFoundWhenDocumentDoesNotBelongToUser(t *testing.T) {
	documents := mocks.NewMockDocumentStore(t)
	documentID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	userID := models.DatabaseUUID("user-1", "user")
	documents.
		On("LoadUserDocument", mock.Anything, documentID, userID).
		Return(models.Document{}, models.ErrDocumentNotFound).
		Once()

	router := newTestRouterWithDeps(t, config.Config{}, nil, nil, documents, nil)
	request := httptest.NewRequest(http.MethodDelete, "/documents/"+documentID.String(), nil)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.JSONEq(t, `{"error":"document not found"}`, response.Body.String())
}

func TestDeleteDocumentDoesNotDeleteRowsWhenAssetDeletionFails(t *testing.T) {
	documents := mocks.NewMockDocumentStore(t)
	uploader := mocks.NewMockSourceUploader(t)
	documentID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	userID := models.DatabaseUUID("user-1", "user")
	document := models.Document{
		ID:       documentID,
		UserID:   userID,
		Filename: "lesson.pdf",
		TaskID:   "cccccccccccccccccccccccccccccccc",
	}

	documents.
		On("LoadUserDocument", mock.Anything, documentID, userID).
		Return(document, nil).
		Once()
	uploader.
		On("DeleteDocumentFiles", mock.Anything, document).
		Return(errors.New("s3 unavailable")).
		Once()

	router := newTestRouterWithDeps(t, config.Config{}, nil, uploader, documents, nil)
	request := httptest.NewRequest(http.MethodDelete, "/documents/"+documentID.String(), nil)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.JSONEq(t, `{"error":"failed to delete document assets"}`, response.Body.String())
}

func TestDeleteDocumentReturnsServiceUnavailableWhenDatabaseDeleteFails(t *testing.T) {
	documents := mocks.NewMockDocumentStore(t)
	uploader := mocks.NewMockSourceUploader(t)
	documentID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	userID := models.DatabaseUUID("user-1", "user")
	document := models.Document{
		ID:       documentID,
		UserID:   userID,
		Filename: "lesson.pdf",
		TaskID:   "cccccccccccccccccccccccccccccccc",
	}

	documents.
		On("LoadUserDocument", mock.Anything, documentID, userID).
		Return(document, nil).
		Once()
	uploader.
		On("DeleteDocumentFiles", mock.Anything, document).
		Return(nil).
		Once()
	documents.
		On("DeleteDocumentCascade", mock.Anything, documentID, userID).
		Return(errors.New("database unavailable")).
		Once()

	router := newTestRouterWithDeps(t, config.Config{}, nil, uploader, documents, nil)
	request := httptest.NewRequest(http.MethodDelete, "/documents/"+documentID.String(), nil)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.JSONEq(t, `{"error":"failed to delete document"}`, response.Body.String())
}

func TestUploadFilePublishesDocumentJob(t *testing.T) {
	publisher := mocks.NewMockPublisher(t)
	uploader := mocks.NewMockSourceUploader(t)
	documents := mocks.NewMockDocumentStore(t)
	taskStatus := mocks.NewMockTaskStatusStore(t)
	source := models.SourceRef{
		Bucket:      "documents",
		Key:         "users/user_123/documents/document/source/lesson_1.pdf",
		Prefix:      "users/user_123/documents/document",
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
					document.GameName == defaultTestGameName &&
					isHexID(document.TaskID) &&
					!document.IsReady &&
					stringValue(document.S3Bucket) == source.Bucket &&
					stringValue(document.S3Prefix) == source.Prefix
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
					job.UserID == "user_123" &&
					isUUID(job.DocumentID) &&
					assert.ObjectsAreEqual(source, job.Source)
			}),
		).
		Run(func(args mock.Arguments) {
			publishedJob = args.Get(2).(models.DocumentJob)
			assert.Equal(t, publishedJob.TaskID, args.String(1))
		}).
		Return(nil).
		Once()

	router := newTestRouterWithDeps(t, config.Config{}, publisher, uploader, documents, taskStatus)
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
	assert.Equal(t, defaultTestGameName, payload["game_name"])
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

func TestUploadFileRequiresGameName(t *testing.T) {
	router := newTestRouter(t, mocks.NewMockPublisher(t), mocks.NewMockSourceUploader(t))
	body, contentType := multipartBodyWithFields(t, "lesson.pdf", "application/pdf", []byte("pdf bytes"), nil)
	request := httptest.NewRequest(http.MethodPost, "/upload-file", body)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.JSONEq(t, `{"error":"game_name is required"}`, response.Body.String())
}

func TestUploadFileReturnsServiceUnavailableWhenPublishFails(t *testing.T) {
	publisher := mocks.NewMockPublisher(t)
	uploader := mocks.NewMockSourceUploader(t)
	documents := mocks.NewMockDocumentStore(t)
	taskStatus := mocks.NewMockTaskStatusStore(t)
	source := models.SourceRef{Bucket: "documents", Key: "source.pdf", Filename: "lesson.pdf"}

	uploader.
		On("UploadSource", mock.Anything, []byte("pdf bytes"), "user-1", mock.MatchedBy(isUUID), "lesson.pdf", "application/pdf").
		Return(source, nil).
		Once()
	documents.
		On("CreateQueuedDocument", mock.Anything, mock.MatchedBy(func(document models.Document) bool {
			return isUUID(document.ID.String()) &&
				document.UserID == models.DatabaseUUID("user-1", "user") &&
				document.Filename == "lesson.pdf" &&
				document.GameName == defaultTestGameName &&
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

	router := newTestRouterWithDeps(t, config.Config{}, publisher, uploader, documents, taskStatus)
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
	source := models.SourceRef{Bucket: "documents", Key: "source.pdf", Filename: "source.pdf"}

	uploader.
		On("UploadSource", mock.Anything, []byte("pdf bytes"), "user-1", mock.MatchedBy(isUUID), "lesson.pdf", "application/pdf").
		Return(source, nil).
		Once()
	documents.
		On("CreateQueuedDocument", mock.Anything, mock.AnythingOfType("models.Document")).
		Return(errors.New("database unavailable")).
		Once()

	router := newTestRouterWithDeps(t, config.Config{}, publisher, uploader, documents, taskStatus)
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
	source := models.SourceRef{Bucket: "documents", Key: "source.pdf", Filename: "source.pdf"}

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

	router := newTestRouterWithDeps(t, config.Config{}, publisher, uploader, documents, taskStatus)
	body, contentType := multipartBody(t, "lesson.pdf", "application/pdf", []byte("pdf bytes"))
	request := httptest.NewRequest(http.MethodPost, "/upload-file", body)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.JSONEq(t, `{"error":"failed to record task status"}`, response.Body.String())
}

func newTestRouter(t *testing.T, publisher api.Publisher, storage api.StorageClient) *gin.Engine {
	return newTestRouterWithDeps(t, config.Config{}, publisher, storage, nil, nil)
}

func newTestRouterWithDeps(
	t *testing.T,
	cfg config.Config,
	publisher api.Publisher,
	storage api.StorageClient,
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
	return api.NewRouter(cfg, publisher, storage, documents, taskStatus)
}

func multipartBody(t *testing.T, filename string, contentType string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	return multipartBodyWithFields(t, filename, contentType, content, map[string]string{
		"game_name": defaultTestGameName,
	})
}

func multipartBodyWithFields(
	t *testing.T,
	filename string,
	contentType string,
	content []byte,
	fields map[string]string,
) (*bytes.Buffer, string) {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range fields {
		require.NoError(t, writer.WriteField(key, value))
	}
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
