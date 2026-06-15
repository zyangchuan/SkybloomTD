package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Controller) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
