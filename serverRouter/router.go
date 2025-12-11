package serverRouter

import (
	"SimpleHttpServer/views"
	"github.com/gin-gonic/gin"
)

func RouterInit(r *gin.Engine) {
	// 路由设置
	r.GET("/", views.IndexHandler)
	r.POST("/upload", views.UploadHandler)
	r.GET("/get_resume_info", views.ResumeInfoHandler)
	r.GET("/download/:filename", views.DownloadHandler)
	r.DELETE("/delete/:filename", views.DeleteHandler)
	r.Static("/static", "./static")
}
