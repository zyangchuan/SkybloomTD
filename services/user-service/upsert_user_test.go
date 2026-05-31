package main_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"skybloom/user-service/internal/api/mocks"
	"skybloom/user-service/internal/models"
)

func TestUpsertUserReturnsForbiddenWhenIDMismatchesToken(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	token := signedTestToken(t, userID, "user@example.com")
	router := newTestRouter(t, nil)

	body := bytes.NewBufferString(`{"id":"99999999-9999-9999-9999-999999999999","user_name":"Alice"}`)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/users/me", body)
	request.AddCookie(&http.Cookie{Name: "skybloom_access_token", Value: token})
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.JSONEq(t, `{"error":"request id does not match authenticated user"}`, response.Body.String())
}

func TestUpsertUserReturnsBadRequestForEmptyUserName(t *testing.T) {
	// An empty user_name is caught by Gin's binding:"required" tag, which fires
	// before the explicit TrimSpace check in the handler.
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	token := signedTestToken(t, userID, "user@example.com")
	router := newTestRouter(t, nil)

	body := bytes.NewBufferString(`{"user_name":""}`)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/users/me", body)
	request.AddCookie(&http.Cookie{Name: "skybloom_access_token", Value: token})
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	// Gin's binding:"required" tag fires before the TrimSpace check and uses the
	// struct field name in its message, so we match on "UserName" and "required".
	assert.Contains(t, response.Body.String(), "UserName")
	assert.Contains(t, response.Body.String(), "required")
}

func TestUpsertUserReturnsBadRequestForWhitespaceOnlyUserName(t *testing.T) {
	// TrimSpace("   ") == "" so whitespace-only is treated the same as empty.
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	token := signedTestToken(t, userID, "user@example.com")
	router := newTestRouter(t, nil)

	body := bytes.NewBufferString(`{"user_name":"   "}`)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/users/me", body)
	request.AddCookie(&http.Cookie{Name: "skybloom_access_token", Value: token})
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.JSONEq(t, `{"error":"user_name is required"}`, response.Body.String())
}

func TestUpsertUserReturnsInternalServerErrorWhenStoreFails(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	token := signedTestToken(t, userID, "user@example.com")
	users := mocks.NewMockUserStore(t)
	users.
		On("Upsert", mock.Anything, mock.AnythingOfType("models.User")).
		Return(models.User{}, errors.New("database unavailable")).
		Once()
	router := newTestRouter(t, users)

	body := bytes.NewBufferString(`{"user_name":"Alice"}`)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/users/me", body)
	request.AddCookie(&http.Cookie{Name: "skybloom_access_token", Value: token})
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.JSONEq(t, `{"error":"failed to store user"}`, response.Body.String())
}

func TestUpsertUserUsesJWTEmailWhenRequestEmailIsAbsent(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	// JWT contains the email; the request body does not.
	token := signedTestToken(t, userID, "jwt@example.com")
	users := mocks.NewMockUserStore(t)

	saved := models.User{ID: userID}
	users.
		On("Upsert", mock.Anything, mock.MatchedBy(func(u models.User) bool {
			return u.ID == userID &&
				u.Email != nil &&
				*u.Email == "jwt@example.com" &&
				u.UserName == "Alice"
		})).
		Return(saved, nil).
		Once()
	router := newTestRouter(t, users)

	// No "email" field in the request body — server should fall back to JWT claim.
	body := bytes.NewBufferString(`{"user_name":"Alice"}`)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/users/me", body)
	request.AddCookie(&http.Cookie{Name: "skybloom_access_token", Value: token})
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	var payload models.User
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, userID, payload.ID)
}

func TestUpsertUserTrimsUserNameWhitespace(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	token := signedTestToken(t, userID, "user@example.com")
	users := mocks.NewMockUserStore(t)

	saved := models.User{ID: userID}
	users.
		On("Upsert", mock.Anything, mock.MatchedBy(func(u models.User) bool {
			return u.UserName == "Alice" // leading/trailing space stripped
		})).
		Return(saved, nil).
		Once()
	router := newTestRouter(t, users)

	body := bytes.NewBufferString(`{"user_name":"  Alice  "}`)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/users/me", body)
	request.AddCookie(&http.Cookie{Name: "skybloom_access_token", Value: token})
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
}

func TestVerifyAuthEmailHeaderOmittedWhenClaimEmailIsBlank(t *testing.T) {
	// When the JWT contains no email (or whitespace-only email), the
	// X-User-Email header must not be set on the response.
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	token := signedTestToken(t, userID, "   ") // whitespace-only email
	router := newTestRouter(t, nil)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/auth/verify", nil)
	request.AddCookie(&http.Cookie{Name: "skybloom_access_token", Value: token})
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
	assert.Empty(t, strings.TrimSpace(response.Header().Get("X-User-Email")),
		"X-User-Email should be absent when JWT email is blank")
}
