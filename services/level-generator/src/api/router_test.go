package main_test

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"skybloom/level-generator-api/internal/api"
	"skybloom/level-generator-api/internal/api/mocks"
	"skybloom/level-generator-api/internal/models"
)

var hexIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestHealth(t *testing.T) {
	router := newTestRouter(t, nil)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{"status":"ok"}`, response.Body.String())
}

func TestGenerateLevelPublishesJob(t *testing.T) {
	publisher := mocks.NewMockPublisher(t)
	subChapterID := "22222222-2222-2222-2222-222222222222"
	var publishedJob models.LevelJob

	publisher.
		On(
			"Publish",
			mock.Anything,
			mock.MatchedBy(isHexID),
			mock.MatchedBy(func(value any) bool {
				job, ok := value.(models.LevelJob)
				if !ok {
					return false
				}
				return job.JobType == "level.generate" &&
					isHexID(job.TaskID) &&
					isHexID(job.FetchTaskID) &&
					job.TaskID == job.GenerateID &&
					job.UserID == "user-1" &&
					job.SubChapterID == subChapterID
			}),
		).
		Run(func(args mock.Arguments) {
			publishedJob = args.Get(2).(models.LevelJob)
			assert.Equal(t, publishedJob.TaskID, args.String(1))
		}).
		Return(nil).
		Once()

	router := newTestRouter(t, publisher)
	request := httptest.NewRequest(http.MethodPost, "/generate_level?sub_chapter_id="+subChapterID, nil)
	request.Header.Set("X-Authenticated-User-Id", " user-1 ")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	var payload map[string]string
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "Level generation started", payload["message"])
	assert.Equal(t, "user-1", payload["user_id"])
	assert.Equal(t, subChapterID, payload["sub_chapter_id"])
	assert.Equal(t, publishedJob.TaskID, payload["task_id"])
	assert.Equal(t, publishedJob.FetchTaskID, payload["fetch_task_id"])
	assert.Equal(t, publishedJob.GenerateID, payload["generate_task_id"])
}

func TestGenerateLevelRequiresAuthentication(t *testing.T) {
	router := newTestRouter(t, mocks.NewMockPublisher(t))
	request := httptest.NewRequest(http.MethodPost, "/generate_level?sub_chapter_id=22222222-2222-2222-2222-222222222222", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.JSONEq(t, `{"error":"Authentication required"}`, response.Body.String())
}

func TestGenerateLevelRequiresValidSubChapterID(t *testing.T) {
	router := newTestRouter(t, mocks.NewMockPublisher(t))
	request := httptest.NewRequest(http.MethodPost, "/generate_level?sub_chapter_id=not-a-uuid", nil)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnprocessableEntity, response.Code)
	assert.JSONEq(t, `{"error":"sub_chapter_id must be a valid UUID"}`, response.Body.String())
}

func TestGenerateLevelReturnsServiceUnavailableWhenPublishFails(t *testing.T) {
	publisher := mocks.NewMockPublisher(t)
	subChapterID := "22222222-2222-2222-2222-222222222222"
	publisher.
		On("Publish", mock.Anything, mock.MatchedBy(isHexID), mock.AnythingOfType("models.LevelJob")).
		Return(errors.New("rabbitmq unavailable")).
		Once()

	router := newTestRouter(t, publisher)
	request := httptest.NewRequest(http.MethodPost, "/generate_level?sub_chapter_id="+subChapterID, nil)
	request.Header.Set("X-Authenticated-User-Id", "user-1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.JSONEq(t, `{"error":"failed to enqueue level job"}`, response.Body.String())
}

func newTestRouter(t *testing.T, publisher api.Publisher) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gin.DefaultWriter = io.Discard
	previousLogWriter := log.Writer()
	log.SetOutput(io.Discard)
	t.Cleanup(func() {
		log.SetOutput(previousLogWriter)
	})
	return api.NewRouter(publisher)
}

func isHexID(value string) bool {
	return hexIDPattern.MatchString(value)
}
