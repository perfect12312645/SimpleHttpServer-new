package views

import (
	. "SimpleHttpServer/config"
	"fmt"
	"github.com/gin-gonic/gin"
	"mime"
	"net/http"
	"os"
	"path/filepath"
)

func DownloadHandler(c *gin.Context) {
	filename := c.Param("filename")
	filePath := filepath.Join(GlobalConfig.UploadDir, filename)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}

	// 设置正确的MIME类型
	ext := filepath.Ext(filename)
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	c.Header("Content-Type", mimeType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	c.File(filePath)
}
