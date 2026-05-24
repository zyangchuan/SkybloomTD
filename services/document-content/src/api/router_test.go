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

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"skybloom/document-content-api/internal/api"
	"skybloom/document-content-api/internal/api/mocks"
	"skybloom/document-content-api/internal/config"
	"skybloom/document-content-api/internal/models"
)

var hexIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestHealth(t *testing.T) {
	router := newTestRouter(t, nil, nil)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{"status":"ok"}`, response.Body.String())
}

func TestUploadFilePublishesDocumentJob(t *testing.T) {
	publisher := mocks.NewMockPublisher(t)
	uploader := mocks.NewMockSourceUploader(t)
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
			mock.MatchedBy(isHexID),
			"lesson_1.pdf",
			"application/pdf",
		).
		Return(source, nil).
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
					job.TaskID == job.IndexTaskID &&
					job.UserID == "user_123" &&
					isHexID(job.DocumentID) &&
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

	router := newTestRouter(t, publisher, uploader)
	body, contentType := multipartBody(t, "lesson 1.pdf", "application/pdf", []byte("pdf bytes"))
	request := httptest.NewRequest(http.MethodPost, "/upload-file", body)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-Authenticated-User-Id", " user/123 ")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	var payload map[string]string
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "Upload file success", payload["message"])
	assert.Equal(t, "user_123", payload["user_id"])
	assert.Equal(t, publishedJob.DocumentID, payload["document_id"])
	assert.Equal(t, publishedJob.TaskID, payload["task_id"])
	assert.Equal(t, publishedJob.OCRTaskID, payload["ocr_task_id"])
	assert.Equal(t, publishedJob.UploadTaskID, payload["upload_task_id"])
	assert.Equal(t, publishedJob.IndexTaskID, payload["index_task_id"])
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
	source := models.SourceRef{Type: "s3", Bucket: "documents", Key: "source.pdf", Filename: "source.pdf"}

	uploader.
		On("UploadSource", mock.Anything, []byte("pdf bytes"), "user-1", mock.MatchedBy(isHexID), "lesson.pdf", "application/pdf").
		Return(source, nil).
		Once()
	publisher.
		On("Publish", mock.Anything, mock.MatchedBy(isHexID), mock.AnythingOfType("models.DocumentJob")).
		Return(errors.New("rabbitmq unavailable")).
		Once()

	router := newTestRouter(t, publisher, uploader)
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
		On("UploadSource", mock.Anything, []byte("pdf bytes"), "user-1", mock.MatchedBy(isHexID), "lesson.pdf", "application/pdf").
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

func TestUploadFileFallsBackToTempStorageWhenUploaderIsNil(t *testing.T) {
	tempDir := t.TempDir()
	publisher := mocks.NewMockPublisher(t)
	var publishedJob models.DocumentJob
	publisher.
		On("Publish", mock.Anything, mock.MatchedBy(isHexID), mock.AnythingOfType("models.DocumentJob")).
		Run(func(args mock.Arguments) {
			publishedJob = args.Get(2).(models.DocumentJob)
		}).
		Return(nil).
		Once()

	router := newTestRouterWithConfig(t, config.Config{TempDir: tempDir}, publisher, nil)
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
	content, err := os.ReadFile(sourcePath)
	require.NoError(t, err)
	assert.Equal(t, []byte("plain text"), content)
}

func newTestRouter(t *testing.T, publisher api.Publisher, uploader api.SourceUploader) *gin.Engine {
	return newTestRouterWithConfig(t, config.Config{TempDir: t.TempDir()}, publisher, uploader)
}

func newTestRouterWithConfig(t *testing.T, cfg config.Config, publisher api.Publisher, uploader api.SourceUploader) *gin.Engine {
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
	return api.NewRouter(cfg, publisher, uploader)
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
