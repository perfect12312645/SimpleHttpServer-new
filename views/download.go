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
import (
	"net/url"
	"strings"
)

// DownloadHandler 适配多级路径的文件下载接口
// 路由建议：r.GET("/download/*path", DownloadHandler)（*path匹配多级路径）
func DownloadHandler(c *gin.Context) {
	// 1. 获取URL中的编码后的完整路径参数（替换原filename）
	encodedPath := c.Param("path")
	if encodedPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未指定要下载的文件路径"})
		return
	}

	// 2. 去掉路径开头的/ + URL解码（处理空格/中文/特殊字符）
	encodedPath = strings.TrimPrefix(encodedPath, "/")
	fileFullPath, err := url.QueryUnescape(encodedPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("路径解析失败：%v", err)})
		return
	}

	// 3. 路径安全校验（防止../../等路径遍历攻击）
	rootUploadDir := filepath.Clean(GlobalConfig.UploadDir)
	targetFilePath := filepath.Clean(filepath.Join(rootUploadDir, fileFullPath))
	// 校验：目标文件必须在根上传目录内
	if !strings.HasPrefix(targetFilePath, rootUploadDir) {
		c.JSON(http.StatusForbidden, gin.H{"error": "禁止下载上传目录外的文件（路径遍历攻击）"})
		return
	}

	// 4. 检查文件是否存在（保留原有逻辑）
	if _, err := os.Stat(targetFilePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}

	// 5. 提取最终的文件名（关键：多级路径下只取最后一段作为下载文件名）
	// 比如 fileFullPath = "xxx/yyy/a.txt" → fileName = "a.txt"
	fileName := filepath.Base(fileFullPath)
	// 对文件名做URL编码（防止中文/空格导致下载文件名乱码）
	encodedFileName := url.QueryEscape(fileName)

	// 6. 设置MIME类型（保留原有逻辑）
	ext := filepath.Ext(fileName)
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	// 7. 设置下载响应头（修复中文文件名乱码问题）
	c.Header("Content-Type", mimeType)
	// RFC 5987 标准：支持中文文件名，兼容各浏览器
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", fileName, encodedFileName))
	// 可选：设置文件大小（提升下载体验）
	if fileInfo, err := os.Stat(targetFilePath); err == nil {
		c.Header("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))
	}

	// 8. 输出文件（保留原有逻辑）
	c.File(targetFilePath)
}
