package views

import (
	. "SimpleHttpServer/config"
	. "SimpleHttpServer/utils"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func UploadHandler(c *gin.Context) {
	// 解析表单
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的文件上传"})
		return
	}

	fileName := c.PostForm("file_name")
	chunkIndex, _ := strconv.Atoi(c.PostForm("chunk_index"))
	totalChunks, _ := strconv.Atoi(c.PostForm("total_chunks"))
	action := c.PostForm("action")
	chunkSize := GlobalConfig.ChunkSize
	// 验证文件大小
	if action == "new" || action == "overwrite" {
		if file.Size*int64(totalChunks) > GlobalConfig.MaxFileSize {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": fmt.Sprintf("文件大小超过限制 (%s)", FormatSize(GlobalConfig.MaxFileSize)),
			})
			return
		}
	}

	filePath := filepath.Join(GlobalConfig.UploadDir, fileName)

	// 处理上传操作
	switch action {
	case "new", "overwrite":
		if chunkIndex == 0 {
			if action == "overwrite" {
				os.Remove(filePath)
			}
			UploadStatusCache.Store(fileName, &UploadStatus{
				TotalChunks:    totalChunks,
				ReceivedChunks: 0,
				FilePath:       filePath,
				LastUpdated:    time.Now(),
			})
		}
	case "resume":
		if _, loaded := UploadStatusCache.Load(fileName); !loaded {
			// 尝试从磁盘恢复状态
			if info, err := os.Stat(filePath); err == nil {
				uploadedChunks := int((info.Size() + chunkSize - 1) / chunkSize)
				UploadStatusCache.Store(fileName, &UploadStatus{
					TotalChunks:    totalChunks,
					ReceivedChunks: uploadedChunks,
					FilePath:       filePath,
					LastUpdated:    time.Now(),
				})
			} else {
				c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "无法恢复上传状态"})
				return
			}
		}
	}

	// 获取上传状态
	statusInterface, ok := UploadStatusCache.Load(fileName)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "上传状态不存在"})
		return
	}

	status := statusInterface.(*UploadStatus)

	// 验证分块索引
	if chunkIndex != status.ReceivedChunks {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": fmt.Sprintf("错误的分块索引, 期望 %d, 收到 %d", status.ReceivedChunks, chunkIndex),
		})
		return
	}

	// 打开文件
	f, err := os.OpenFile(status.FilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("无法打开文件: %v", err)})
		return
	}
	defer f.Close()

	// 读取分块内容
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("无法读取上传文件: %v", err)})
		return
	}
	defer src.Close()

	// 写入文件
	if _, err := io.Copy(f, src); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("写入文件失败: %v", err)})
		return
	}

	// 更新状态
	status.ReceivedChunks++
	status.LastUpdated = time.Now()
	receivedChunks := status.ReceivedChunks

	// 检查是否完成
	if receivedChunks == totalChunks {
		UploadStatusCache.Delete(fileName)
		c.JSON(http.StatusOK, gin.H{
			"status":   "success",
			"message":  "文件上传完成",
			"filename": fileName,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":     "success",
		"message":    fmt.Sprintf("分块 %d/%d 上传成功", receivedChunks, totalChunks),
		"next_chunk": receivedChunks,
	})
}

func ResumeInfoHandler(c *gin.Context) {
	fileName := c.Query("file_name")
	filePath := filepath.Join(GlobalConfig.UploadDir, fileName)

	info := ResumeInfo{
		FileName:   fileName,
		FileExists: false,
	}
	chunkSize := GlobalConfig.ChunkSize

	fileStat, err := os.Stat(filePath)
	if err == nil {
		info.FileExists = true
		info.UploadedBytes = fileStat.Size()
		info.UploadedChunks = int((fileStat.Size() + chunkSize - 1) / chunkSize)
	}

	c.JSON(http.StatusOK, info)
}
