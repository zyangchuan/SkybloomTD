package api

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"

	"skybloom/user-service/internal/auth"
	"skybloom/user-service/internal/config"
	"skybloom/user-service/internal/models"
	"skybloom/user-service/internal/repository"
)

type Server struct {
	config     config.Config
	users      *repository.UserRepository
	authParser *auth.Parser
}

type UpsertUserRequest struct {
	ID       string         `json:"id"`
	Email    string         `json:"email"`
	UserName string         `json:"user_name" binding:"required"`
	Metadata map[string]any `json:"metadata"`
}

func NewRouter(cfg config.Config, users *repository.UserRepository) *gin.Engine {
	server := &Server{
		config: cfg,
		users:  users,
		authParser: &auth.Parser{
			JWTSecret:          cfg.SupabaseJWTSecret,
			JWTAudience:        cfg.SupabaseJWTAudience,
			JWKSURL:            cfg.SupabaseJWKSURL,
			AllowUnverifiedJWT: cfg.AllowUnverifiedJWT,
		},
	}

	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		log.Fatalf("proxy configuration error: %v", err)
	}
	router.Use(gin.Logger(), gin.Recovery())
	router.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORSAllowedOrigins,
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	router.GET("/health", server.health)
	router.GET("/ready", server.ready)
	router.GET("/auth/verify", server.verifyAuth)
	router.HEAD("/auth/verify", server.verifyAuth)

	protected := router.Group("/")
	protected.Use(auth.Middleware(cfg))
	protected.POST("/users", server.upsertUser)
	protected.POST("/users/me", server.upsertUser)

	return router
}

func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) ready(c *gin.Context) {
	if err := s.users.Ping(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

func (s *Server) verifyAuth(c *gin.Context) {
	claims, err := s.authParser.ParseAuthorizationHeader(c.GetHeader("Authorization"))
	if err != nil {
		log.Printf("auth verify failed: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "jwt subject must be a UUID"})
		return
	}

	c.Header("X-User-Id", userID.String())
	if strings.TrimSpace(claims.Email) != "" {
		c.Header("X-User-Email", strings.TrimSpace(claims.Email))
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) upsertUser(c *gin.Context) {
	userID, ok := auth.UserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authenticated user"})
		return
	}

	claims, ok := auth.Claims(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing jwt claims"})
		return
	}

	var request UpsertUserRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if strings.TrimSpace(request.ID) != "" && request.ID != userID.String() {
		c.JSON(http.StatusForbidden, gin.H{"error": "request id does not match authenticated user"})
		return
	}

	userName := strings.TrimSpace(request.UserName)
	if userName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_name is required"})
		return
	}

	email := firstNonEmpty(request.Email, claims.Email)
	var emailPtr *string
	if strings.TrimSpace(email) != "" {
		normalized := strings.TrimSpace(email)
		emailPtr = &normalized
	}

	metadata := request.Metadata
	if metadata == nil {
		metadata = claims.UserMetadata
	}
	if metadata == nil {
		metadata = map[string]any{}
	}

	user, err := s.users.Upsert(c.Request.Context(), models.User{
		ID:       userID,
		Email:    emailPtr,
		UserName: userName,
		Metadata: datatypes.JSONMap(metadata),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store user"})
		return
	}

	c.JSON(http.StatusOK, user)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
