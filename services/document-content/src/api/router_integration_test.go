package main_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	api "skybloom/document-content-api/internal/api"
	"skybloom/document-content-api/internal/api/mocks"
	"skybloom/document-content-api/internal/config"
	"skybloom/document-content-api/internal/models"
)

const testUserID = "user-1"

type apiMocks struct {
	publisher  *mocks.Publisher
	storage    *mocks.StorageClient
	documents  *mocks.DocumentStore
	taskStatus *mocks.TaskStatusStore
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func TestUploadFileHappyPath(t *testing.T) {
	deps := newAPIMocks(t)
	router := newTestRouter(deps)
	source := models.SourceRef{S3Bucket: "documents", SourceFilename: "rulebook.pdf"}
	storageUserID := models.DatabaseUUID(testUserID, "user").String()

	deps.storage.On(
		"UploadSource",
		mock.Anything,
		[]byte("pdf bytes"),
		storageUserID,
		mock.MatchedBy(isUUIDString),
		"rulebook.pdf",
		"application/octet-stream",
	).Return(source, nil).Once()
	deps.documents.On("CreateQueuedDocument", mock.Anything, mock.MatchedBy(func(document models.Document) bool {
		return document.ID != uuid.Nil &&
			document.UserID.String() == storageUserID &&
			document.S3Bucket != nil &&
			*document.S3Bucket == source.S3Bucket &&
			document.SourceFilename == source.SourceFilename &&
			document.GameName == "SkybloomTD" &&
			document.TaskID != "" &&
			!document.IsReady
	})).Return(nil).Once()
	deps.taskStatus.On("Set", mock.Anything, mock.MatchedBy(func(status models.TaskStatus) bool {
		return status.TaskID != "" &&
			isUUIDString(status.DocumentID) &&
			status.Status == models.TaskStatusQueued &&
			status.Error == nil
	})).Return(nil).Once()
	deps.publisher.On("Publish", mock.Anything, mock.MatchedBy(nonEmptyString), mock.MatchedBy(func(value any) bool {
		job, ok := value.(models.DocumentJob)
		return ok &&
			job.JobType == "document.process" &&
			job.TaskID != "" &&
			job.UserID == storageUserID &&
			isUUIDString(job.DocumentID) &&
			assert.ObjectsAreEqual(source, job.Source)
	})).Return(nil).Once()

	response := performRequest(router, newUploadRequest(t, "SkybloomTD", "rulebook.pdf", "pdf bytes"))

	require.Equal(t, http.StatusOK, response.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	assert.Equal(t, "Upload file success", body["message"])
	assert.Equal(t, storageUserID, body["user_id"])
	assert.Equal(t, "SkybloomTD", body["game_name"])
	assert.Equal(t, false, body["is_ready"])
	assert.NotEmpty(t, body["task_id"])
	assert.NotEmpty(t, body["document_id"])
}

func TestUploadFilePublisherFailureMarksTaskFailed(t *testing.T) {
	deps := newAPIMocks(t)
	router := newTestRouter(deps)
	source := models.SourceRef{S3Bucket: "documents", SourceFilename: "rulebook.pdf"}
	publishErr := errors.New("rabbitmq unavailable")
	storageUserID := models.DatabaseUUID(testUserID, "user").String()

	deps.storage.On("UploadSource", mock.Anything, []byte("pdf bytes"), storageUserID, mock.MatchedBy(isUUIDString), "rulebook.pdf", "application/octet-stream").Return(source, nil).Once()
	deps.documents.On("CreateQueuedDocument", mock.Anything, mock.AnythingOfType("models.Document")).Return(nil).Once()
	deps.taskStatus.On("Set", mock.Anything, mock.MatchedBy(func(status models.TaskStatus) bool {
		return status.Status == models.TaskStatusQueued && status.Error == nil
	})).Return(nil).Once()
	deps.publisher.On("Publish", mock.Anything, mock.MatchedBy(nonEmptyString), mock.AnythingOfType("models.DocumentJob")).Return(publishErr).Once()
	deps.taskStatus.On("Set", mock.Anything, mock.MatchedBy(func(status models.TaskStatus) bool {
		return status.Status == models.TaskStatusFailed &&
			status.Error != nil &&
			*status.Error == publishErr.Error()
	})).Return(nil).Once()

	response := performRequest(router, newUploadRequest(t, "SkybloomTD", "rulebook.pdf", "pdf bytes"))

	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.JSONEq(t, `{"error":"failed to enqueue document job"}`, response.Body.String())
}

func TestDeleteDocumentHappyPath(t *testing.T) {
	deps := newAPIMocks(t)
	router := newTestRouter(deps)
	documentID := uuid.New()
	userUUID := models.DatabaseUUID(testUserID, "user")
	bucket := "documents"
	document := models.Document{
		ID:             documentID,
		UserID:         userUUID,
		S3Bucket:       &bucket,
		SourceFilename: "rulebook.pdf",
		GameName:       "SkybloomTD",
		TaskID:         "task-1",
	}

	deps.documents.On("LoadUserDocument", mock.Anything, documentID, userUUID).Return(document, nil).Once()
	deps.storage.On("DeleteDocumentFiles", mock.Anything, document).Return(nil).Once()
	deps.documents.On("DeleteDocumentCascade", mock.Anything, documentID, userUUID).Return(nil).Once()

	request := httptest.NewRequest(http.MethodDelete, "/documents/"+documentID.String(), nil)
	request.Header.Set("X-Authenticated-User-Id", testUserID)
	response := performRequest(router, request)

	require.Equal(t, http.StatusNoContent, response.Code)
	assert.Empty(t, response.Body.String())
}

func TestTaskStatusFoundAndMissing(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		deps := newAPIMocks(t)
		router := newTestRouter(deps)
		status := models.TaskStatus{
			TaskID:     "task-1",
			DocumentID: uuid.NewString(),
			Status:     models.TaskStatusSuccessful,
		}
		deps.taskStatus.On("Get", mock.Anything, "task-1").Return(status, nil).Once()

		response := performRequest(router, httptest.NewRequest(http.MethodGet, "/tasks/task-1/status", nil))

		require.Equal(t, http.StatusOK, response.Code)
		var body models.TaskStatus
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
		assert.Equal(t, status.TaskID, body.TaskID)
		assert.Equal(t, status.DocumentID, body.DocumentID)
		assert.Equal(t, status.Status, body.Status)
	})

	t.Run("missing", func(t *testing.T) {
		deps := newAPIMocks(t)
		router := newTestRouter(deps)
		deps.taskStatus.On("Get", mock.Anything, "missing-task").Return(models.TaskStatus{}, models.ErrTaskStatusNotFound).Once()

		response := performRequest(router, httptest.NewRequest(http.MethodGet, "/tasks/missing-task/status", nil))

		require.Equal(t, http.StatusNotFound, response.Code)
		assert.JSONEq(t, `{"error":"task status not found"}`, response.Body.String())
	})
}

func newAPIMocks(t *testing.T) apiMocks {
	return apiMocks{
		publisher:  mocks.NewPublisher(t),
		storage:    mocks.NewStorageClient(t),
		documents:  mocks.NewDocumentStore(t),
		taskStatus: mocks.NewTaskStatusStore(t),
	}
}

func newTestRouter(deps apiMocks) http.Handler {
	return api.NewRouter(config.Config{}, deps.publisher, deps.storage, deps.documents, deps.taskStatus)
}

func newUploadRequest(t *testing.T, gameName string, filename string, content string) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("game_name", gameName))

	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	request := httptest.NewRequest(http.MethodPost, "/upload-file", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-Authenticated-User-Id", testUserID)
	return request
}

func performRequest(handler http.Handler, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func isUUIDString(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func nonEmptyString(value string) bool {
	return value != ""
}
