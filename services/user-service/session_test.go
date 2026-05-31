package main_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"skybloom/user-service/internal/auth"
)

// ---------------------------------------------------------------------------
// Authentication — edge cases not covered by user_service_test.go
// ---------------------------------------------------------------------------

func TestVerifyAuthReturnUnauthorizedWithNoCookie(t *testing.T) {
	router := newTestRouter(t, nil)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/auth/verify", nil)
	// Deliberately no cookie set.
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.JSONEq(t, `{"error":"missing access token cookie"}`, response.Body.String())
}

func TestVerifyAuthReturnUnauthorizedWithEmptyCookieValue(t *testing.T) {
	router := newTestRouter(t, nil)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/auth/verify", nil)
	request.AddCookie(&http.Cookie{Name: "skybloom_access_token", Value: "   "})

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.JSONEq(t, `{"error":"missing access token cookie"}`, response.Body.String())
}

func TestVerifyAuthReturnUnauthorizedWithMalformedToken(t *testing.T) {
	router := newTestRouter(t, nil)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/auth/verify", nil)
	request.AddCookie(&http.Cookie{Name: "skybloom_access_token", Value: "not.a.valid.jwt"})

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.JSONEq(t, `{"error":"invalid jwt"}`, response.Body.String())
}

func TestVerifyAuthReturnUnauthorizedWithTokenMissingSubject(t *testing.T) {
	// Build a JWT that has no "sub" claim.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, auth.SupabaseClaims{
		Email: "no-subject@example.com",
		// Intentionally no Subject in RegisteredClaims.
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	rawToken, err := token.SignedString([]byte(strings.Repeat("x", 32)))
	require.NoError(t, err)

	router := newTestRouter(t, nil)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/auth/verify", nil)
	request.AddCookie(&http.Cookie{Name: "skybloom_access_token", Value: rawToken})

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.JSONEq(t, `{"error":"jwt subject is required"}`, response.Body.String())
}

func TestVerifyAuthReturnUnauthorizedWithTokenSubjectNotUUID(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, auth.SupabaseClaims{
		Email: "user@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "not-a-uuid",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	rawToken, err := token.SignedString([]byte(strings.Repeat("x", 32)))
	require.NoError(t, err)

	router := newTestRouter(t, nil)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/auth/verify", nil)
	request.AddCookie(&http.Cookie{Name: "skybloom_access_token", Value: rawToken})

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.JSONEq(t, `{"error":"jwt subject must be a UUID"}`, response.Body.String())
}

func TestVerifyAuthHeadMethodReturnsNoContent(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	token := signedTestToken(t, userID, "head@example.com")
	router := newTestRouter(t, nil)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodHead, "/auth/verify", nil)
	request.AddCookie(&http.Cookie{Name: "skybloom_access_token", Value: token})
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
	assert.Equal(t, userID.String(), response.Header().Get("X-User-Id"))
}

// ---------------------------------------------------------------------------
// Session persistence — the server is stateless; a valid cookie = active session
// ---------------------------------------------------------------------------

func TestVerifyAuthAcceptsValidTokenRepeatedly(t *testing.T) {
	// Simulates page refresh: same token, multiple requests.
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	token := signedTestToken(t, userID, "student@example.com")
	router := newTestRouter(t, nil)

	for i := 0; i < 3; i++ {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/auth/verify", nil)
		request.AddCookie(&http.Cookie{Name: "skybloom_access_token", Value: token})
		router.ServeHTTP(response, request)

		assert.Equal(t, http.StatusNoContent, response.Code, "request %d should succeed", i+1)
		assert.Equal(t, userID.String(), response.Header().Get("X-User-Id"), "user id should be stable across requests")
	}
}

// ---------------------------------------------------------------------------
// Logout — clearing the cookie makes the session invalid
// ---------------------------------------------------------------------------

func TestVerifyAuthReturnUnauthorizedAfterCookieIsCleared(t *testing.T) {
	// After logout the client removes the cookie. The server should return 401
	// on the next verification attempt with no cookie.
	router := newTestRouter(t, nil)

	// Step 1: authenticated request succeeds.
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	token := signedTestToken(t, userID, "student@example.com")

	authed := httptest.NewRecorder()
	authedReq := httptest.NewRequest(http.MethodGet, "/auth/verify", nil)
	authedReq.AddCookie(&http.Cookie{Name: "skybloom_access_token", Value: token})
	router.ServeHTTP(authed, authedReq)
	require.Equal(t, http.StatusNoContent, authed.Code)

	// Step 2: after "logout" (cookie removed), the same endpoint returns 401.
	loggedOut := httptest.NewRecorder()
	loggedOutReq := httptest.NewRequest(http.MethodGet, "/auth/verify", nil)
	// No cookie — simulates the browser clearing it on logout.
	router.ServeHTTP(loggedOut, loggedOutReq)

	assert.Equal(t, http.StatusUnauthorized, loggedOut.Code)
}

// ---------------------------------------------------------------------------
// Protected endpoint authentication
// ---------------------------------------------------------------------------

func TestUpsertUserReturnUnauthorizedWithNoCookie(t *testing.T) {
	router := newTestRouter(t, nil)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/users/me", nil)
	// No cookie set.
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestUpsertUserReturnUnauthorizedWithMalformedToken(t *testing.T) {
	router := newTestRouter(t, nil)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/users/me", nil)
	request.AddCookie(&http.Cookie{Name: "skybloom_access_token", Value: "garbage.token.value"})

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
}
