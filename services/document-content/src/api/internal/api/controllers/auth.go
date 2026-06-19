package controllers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func authenticatedUserID(c *gin.Context) (string, bool) {
	userID := strings.TrimSpace(c.GetHeader("X-Authenticated-User-Id"))
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return "", false
	}
	return safePathPart(userID), true
}
