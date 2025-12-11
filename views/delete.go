package views

import (
	. "SimpleHttpServer/config"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"os"
	"path/filepath"
)

func DeleteHandler(c *gin.Context) {
	filename := c.Param("filename")
	filePath := filepath.Join(GlobalConfig.UploadDir, filename)
	tempFilePath := filePath + ".part"

	// 删除主文件
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("删除文件失败: %v", err)})
		return
	}

	// 删除临时文件
	if err := os.Remove(tempFilePath); err != nil && !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("删除临时文件失败: %v", err)})
		return
	}

	// 删除上传状态
	UploadStatusCache.Delete(filename)

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "文件已删除"})
}
