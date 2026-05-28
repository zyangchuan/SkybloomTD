package main_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"skybloom/user-service/internal/api"
	"skybloom/user-service/internal/api/mocks"
	"skybloom/user-service/internal/auth"
	"skybloom/user-service/internal/config"
	"skybloom/user-service/internal/models"
)

func TestHealth(t *testing.T) {
	router := newTestRouter(t, nil)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{"status":"ok"}`, response.Body.String())
}

func TestReadyPingsUserStore(t *testing.T) {
	users := mocks.NewMockUserStore(t)
	users.On("Ping", mock.Anything).Return(nil).Once()
	router := newTestRouter(t, users)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{"status":"ready"}`, response.Body.String())
}

func TestReadyReturnsServiceUnavailableWhenStoreIsDown(t *testing.T) {
	users := mocks.NewMockUserStore(t)
	users.On("Ping", mock.Anything).Return(errors.New("database down")).Once()
	router := newTestRouter(t, users)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.JSONEq(t, `{"error":"database unavailable"}`, response.Body.String())
}

func TestVerifyAuthReturnsUserHeaders(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	token := signedTestToken(t, userID, "student@example.com")
	router := newTestRouter(t, nil)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/auth/verify", nil)
	request.AddCookie(&http.Cookie{Name: "skybloom_access_token", Value: token})
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
	assert.Equal(t, userID.String(), response.Header().Get("X-User-Id"))
	assert.Equal(t, "student@example.com", response.Header().Get("X-User-Email"))
}

func TestVerifyAuthRejectsAuthorizationHeader(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	token := signedTestToken(t, userID, "student@example.com")
	router := newTestRouter(t, nil)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/auth/verify", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestUpsertUserStoresAuthenticatedUser(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	token := signedTestToken(t, userID, "student@example.com")
	users := mocks.NewMockUserStore(t)
	router := newTestRouter(t, users)

	saved := models.User{
		ID:       userID,
		Email:    ptr("override@example.com"),
		UserName: "Student",
		Metadata: datatypes.JSONMap{"grade": float64(3)},
	}
	users.
		On("Upsert", mock.Anything, mock.MatchedBy(func(user models.User) bool {
			return user.ID == userID &&
				user.Email != nil &&
				*user.Email == "override@example.com" &&
				user.UserName == "Student" &&
				user.Metadata["grade"] == float64(3)
		})).
		Return(saved, nil).
		Once()

	body := bytes.NewBufferString(`{"email":"override@example.com","user_name":" Student ","metadata":{"grade":3}}`)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/users/me", body)
	request.AddCookie(&http.Cookie{Name: "skybloom_access_token", Value: token})
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	var payload models.User
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, userID, payload.ID)
	assert.NotNil(t, payload.Email)
	assert.Equal(t, "override@example.com", *payload.Email)
	assert.Equal(t, "Student", payload.UserName)
}

func newTestRouter(t *testing.T, users api.UserStore) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gin.DefaultWriter = io.Discard
	previousLogWriter := log.Writer()
	log.SetOutput(io.Discard)
	t.Cleanup(func() {
		log.SetOutput(previousLogWriter)
	})
	return api.NewRouter(config.Config{
		AllowUnverifiedJWT: true,
		CORSAllowedOrigins: []string{"*"},
	}, users)
}

func signedTestToken(t *testing.T, userID uuid.UUID, email string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, auth.SupabaseClaims{
		Email: email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	rawToken, err := token.SignedString([]byte(strings.Repeat("x", 32)))
	require.NoError(t, err)
	return rawToken
}

func ptr[T any](value T) *T {
	return &value
}
