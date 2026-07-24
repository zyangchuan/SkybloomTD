package api

import (
	"log"

	"github.com/gin-gonic/gin"

	"skybloom/document-content-api/internal/api/controllers"
	"skybloom/document-content-api/internal/config"
)

type Publisher = controllers.Publisher
type StorageClient = controllers.StorageClient
type DocumentStore = controllers.DocumentStore
type TaskStatusStore = controllers.TaskStatusStore

func NewRouter(
	cfg config.Config,
	publisher Publisher,
	storageClient StorageClient,
	documentStore DocumentStore,
	taskStatusStore TaskStatusStore,
) *gin.Engine {
	controller := controllers.NewController(cfg, publisher, storageClient, documentStore, taskStatusStore)

	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		log.Fatalf("proxy configuration error: %v", err)
	}
	router.Use(gin.Logger(), gin.Recovery())

	router.GET("/health", controller.Health)
	router.POST("/upload-file", controller.UploadFile)
	router.GET("/documents", controller.ListDocuments)
	router.PATCH("/documents/:document_id/visibility", controller.UpdateDocumentVisibility)
	router.GET("/documents/:document_id/chapters", controller.ListDocumentChapters)
	router.DELETE("/documents/:document_id", controller.DeleteDocument)
	router.GET("/library/games", controller.ListPublicGames)
	router.GET("/library/starred", controller.ListStarredGames)
	router.POST("/library/games/:document_id/star", controller.StarGame)
	router.DELETE("/library/games/:document_id/star", controller.UnstarGame)
	router.GET("/chapters/:chapter_id/sub-chapters", controller.ListChapterSubChapters)
	router.GET("/tasks/:task_id/status", controller.GetTaskStatus)
	return router
}
