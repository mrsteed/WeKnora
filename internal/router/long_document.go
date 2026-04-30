package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

func RegisterLongDocumentTaskRoutes(r *gin.RouterGroup, handler *handler.LongDocumentTaskHandler) {
	group := r.Group("/long-document-tasks")
	{
		group.POST("", handler.CreateTask)
		group.GET("", handler.ListTasksBySession)
		group.GET("/:id", handler.GetTask)
		group.GET("/:id/batches", handler.ListBatches)
		group.GET("/:id/artifact", handler.GetArtifact)
		group.GET("/:id/download", handler.DownloadArtifact)
		group.GET("/:id/events", handler.StreamEvents)
		group.POST("/:id/retry", handler.RetryTask)
		group.POST("/:id/cancel", handler.CancelTask)
	}
}
