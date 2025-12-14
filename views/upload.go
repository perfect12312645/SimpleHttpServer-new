package views

import (
	. "SimpleHttpServer/config"
	. "SimpleHttpServer/middleware" // 假设该包导出全局Zap Logger实例（Logger *zap.Logger）
	. "SimpleHttpServer/utils"
	"fmt"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap" // 导入zap包
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// UploadHandler 处理文件分片上传（优化版 + Zap日志）
func UploadHandler(c *gin.Context) {
	// ========== 1. 日志：记录请求开始 ==========
	Logger.Info("开始处理文件上传请求",
		zap.String("request_path", c.FullPath()),
		zap.String("client_ip", c.ClientIP()),
	)

	// ========== 2. 解析路由路径参数（适配 /upload/*path） ==========
	// 从路由获取子目录路径（/*path 匹配的部分）
	dirPath := c.Param("path")
	if dirPath == "" || dirPath == "/" || dirPath == "." {
		dirPath = ""
	}
	// 拼接最终上传目录（绝对路径）
	trimPath := filepath.Join(GlobalConfig.UploadDir, dirPath)

	// 规范化路径（处理 ../ 等非法路径）
	trimPath, err := filepath.Abs(trimPath)
	if err != nil {
		errMsg := fmt.Sprintf("规范化上传路径失败: %v", err)
		Logger.Error(errMsg,
			zap.String("trimPath", trimPath),
			zap.Error(err), // 携带原始错误
		)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": errMsg,
		})
		return
	}
	dirAbs, err := filepath.Abs(GlobalConfig.UploadDir)
	if err != nil {
		errMsg := fmt.Sprintf("获取root目录绝对路径失败: %v", err)
		Logger.Error("获取root目录绝对路径失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": errMsg,
		})
		return
	}
	// ========== 3. 校验上传目录是否存在且为目录 ==========
	uploadPathInfo, err := os.Stat(trimPath)
	if err != nil {
		if os.IsNotExist(err) {
			errMsg := fmt.Sprintf("上传目录不存在: %s", trimPath)
			Logger.Error(errMsg, zap.String("trimPath", trimPath))
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": errMsg,
			})
			return
		}
		errMsg := fmt.Sprintf("获取上传目录信息失败: %v", err)
		Logger.Error(errMsg,
			zap.String("trimPath", trimPath),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": errMsg,
		})
		return
	}
	if !uploadPathInfo.IsDir() {
		errMsg := fmt.Sprintf("路径%s不是目录", trimPath)
		Logger.Error(errMsg, zap.String("trimPath", trimPath))
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": errMsg,
		})
		return
	}

	// ========== 4. 解析并校验表单参数 ==========
	// 解析上传文件
	file, err := c.FormFile("file")
	if err != nil {
		errMsg := fmt.Sprintf("解析上传文件失败: %v", err)
		Logger.Error(errMsg, zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": errMsg,
		})
		return
	}

	// 解析基础参数
	fileName := c.PostForm("file_name")
	if fileName == "" {
		errMsg := "文件名称不能为空"
		Logger.Error(errMsg)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": errMsg,
		})
		return
	}

	// 解析分块索引（严格校验）
	chunkIndexStr := c.PostForm("chunk_index")
	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil || chunkIndex < 0 {
		errMsg := fmt.Sprintf("分块索引无效: %s（需为非负整数）", chunkIndexStr)
		Logger.Error(errMsg,
			zap.String("chunkIndexStr", chunkIndexStr),
			zap.Error(err),
		)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": errMsg,
		})
		return
	}

	// 解析总分块数（严格校验）
	totalChunksStr := c.PostForm("total_chunks")
	totalChunks, err := strconv.Atoi(totalChunksStr)
	if err != nil || totalChunks <= 0 {
		errMsg := fmt.Sprintf("总分块数无效: %s（需为正整数）", totalChunksStr)
		Logger.Error(errMsg,
			zap.String("totalChunksStr", totalChunksStr),
			zap.Error(err),
		)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": errMsg,
		})
		return
	}

	// 解析操作类型（校验合法值）
	action := c.PostForm("action")
	validActions := map[string]bool{"new": true, "overwrite": true, "resume": true}
	if !validActions[action] {
		errMsg := fmt.Sprintf("操作类型无效: %s（仅支持new/overwrite/resume）", action)
		Logger.Error(errMsg, zap.String("action", action))
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": errMsg,
		})
		return
	}

	// ========== 5. 校验文件大小限制 ==========
	chunkSize := int64(GlobalConfig.ChunkSize)
	if action == "new" || action == "overwrite" {
		// 计算文件总大小（单个分块大小 * 总分块数，最后一块可能更小）
		totalFileSize := file.Size * int64(totalChunks)
		if totalFileSize > GlobalConfig.MaxFileSize {
			errMsg := fmt.Sprintf("文件大小超过限制（最大支持%s，当前文件预估%s）",
				FormatSize(GlobalConfig.MaxFileSize), FormatSize(totalFileSize))
			Logger.Error(errMsg,
				zap.String("fileName", fileName),
				zap.Int64("totalFileSize", totalFileSize),
				zap.Int64("maxFileSize", GlobalConfig.MaxFileSize),
			)
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"status":  "error",
				"message": errMsg,
			})
			return
		}
	}

	// ========== 6. 拼接最终文件路径（规范化） ==========
	filePath := filepath.Join(trimPath, fileName)
	// 确保文件路径在上传目录内（防止路径穿越）
	if !strings.HasPrefix(filePath, dirAbs) {
		errMsg := fmt.Sprintf("文件路径非法，禁止跨目录上传: %s", filePath)
		Logger.Error(errMsg, zap.String("filePath", filePath))
		c.JSON(http.StatusForbidden, gin.H{
			"status":  "error",
			"message": errMsg,
		})
		return
	}

	// ========== 7. 处理上传状态（初始化/恢复） ==========
	var status *UploadStatus
	switch action {
	case "new", "overwrite":
		// 新文件/覆盖文件：初始化上传状态
		if chunkIndex == 0 {
			// 覆盖模式：先删除原有文件
			if action == "overwrite" {
				if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
					Logger.Warn("删除原有文件失败", // 警告级别
						zap.String("filePath", filePath),
						zap.Error(err),
					)
				}
			}
			// 初始化上传状态缓存
			status = &UploadStatus{
				TotalChunks:    totalChunks,
				ReceivedChunks: 0,
				FilePath:       filePath,
				LastUpdated:    time.Now(),
			}
			UploadStatusCache.Store(fileName, status)
			Logger.Info("初始化上传状态",
				zap.String("fileName", fileName),
				zap.Int("totalChunks", totalChunks),
				zap.String("filePath", filePath),
				zap.String("action", action),
			)
		}
	case "resume":
		// 续传：从缓存/磁盘恢复上传状态
		statusInterface, loaded := UploadStatusCache.Load(fileName)
		if !loaded {
			// 尝试从磁盘恢复
			fileStat, err := os.Stat(filePath)
			if err != nil {
				errMsg := fmt.Sprintf("无法恢复上传状态，文件不存在或无权限: %v", err)
				Logger.Error(errMsg,
					zap.String("fileName", fileName),
					zap.String("filePath", filePath),
					zap.Error(err),
				)
				c.JSON(http.StatusBadRequest, gin.H{
					"status":  "error",
					"message": errMsg,
				})
				return
			}
			// 计算已上传分块数
			uploadedChunks := int((fileStat.Size() + chunkSize - 1) / chunkSize)
			status = &UploadStatus{
				TotalChunks:    totalChunks,
				ReceivedChunks: uploadedChunks,
				FilePath:       filePath,
				LastUpdated:    time.Now(),
			}
			UploadStatusCache.Store(fileName, status)
			Logger.Info("从磁盘恢复上传状态",
				zap.String("fileName", fileName),
				zap.Int("uploadedChunks", uploadedChunks),
				zap.Int("totalChunks", totalChunks),
			)
		} else {
			status = statusInterface.(*UploadStatus)
		}
	}

	// ========== 8. 从缓存加载上传状态（兜底校验） ==========
	if status == nil {
		statusInterface, ok := UploadStatusCache.Load(fileName)
		if !ok {
			errMsg := fmt.Sprintf("上传状态不存在，无法继续上传: %s", fileName)
			Logger.Error(errMsg, zap.String("fileName", fileName))
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": errMsg,
			})
			return
		}
		status = statusInterface.(*UploadStatus)
	}

	// ========== 9. 校验分块索引（防止乱序上传） ==========
	if chunkIndex != status.ReceivedChunks {
		errMsg := fmt.Sprintf("分块索引错误，期望%d，收到%d", status.ReceivedChunks, chunkIndex)
		Logger.Error(errMsg,
			zap.String("fileName", fileName),
			zap.Int("expectedChunkIndex", status.ReceivedChunks),
			zap.Int("receivedChunkIndex", chunkIndex),
		)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": errMsg,
		})
		return
	}

	// ========== 10. 写入分块数据到文件 ==========
	// 打开文件（创建/追加写入）
	f, err := os.OpenFile(status.FilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		errMsg := fmt.Sprintf("打开文件失败: %v", err)
		Logger.Error(errMsg,
			zap.String("filePath", status.FilePath),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": errMsg,
		})
		return
	}
	defer f.Close() // 确保文件句柄释放

	// 打开上传分块
	src, err := file.Open()
	if err != nil {
		errMsg := fmt.Sprintf("读取上传分块失败: %v", err)
		Logger.Error(errMsg,
			zap.String("fileName", fileName),
			zap.Int("chunkIndex", chunkIndex),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": errMsg,
		})
		return
	}
	defer src.Close() // 确保分块句柄释放

	// 写入分块数据
	written, err := io.Copy(f, src)
	if err != nil {
		errMsg := fmt.Sprintf("写入分块数据失败: %v", err)
		Logger.Error(errMsg,
			zap.String("fileName", fileName),
			zap.Int("chunkIndex", chunkIndex),
			zap.Int64("writtenBytes", written),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": errMsg,
		})
		return
	}

	// ========== 11. 更新上传状态 ==========
	status.ReceivedChunks++
	status.LastUpdated = time.Now()
	receivedChunks := status.ReceivedChunks
	Logger.Info("分块上传成功",
		zap.String("fileName", fileName),
		zap.Int("chunkIndex", chunkIndex),
		zap.Int("receivedChunks", receivedChunks),
		zap.Int("totalChunks", totalChunks),
		zap.Int64("writtenBytes", written),
	)

	// ========== 12. 检查是否上传完成 ==========
	if receivedChunks == totalChunks {
		// 上传完成：清理缓存
		UploadStatusCache.Delete(fileName)
		Logger.Info("文件上传完成",
			zap.String("fileName", fileName),
			zap.String("filePath", status.FilePath),
			zap.String("totalSize", FormatSize(file.Size*int64(totalChunks))),
		)
		c.JSON(http.StatusOK, gin.H{
			"status":   "success",
			"message":  "文件上传完成",
			"filename": fileName,
			"filePath": status.FilePath,
		})
		return
	}

	// ========== 13. 分块上传成功（未完成） ==========
	c.JSON(http.StatusOK, gin.H{
		"status":     "success",
		"message":    fmt.Sprintf("分块 %d/%d 上传成功", receivedChunks, totalChunks),
		"next_chunk": receivedChunks,
	})
}

// ResumeInfoHandler 获取续传信息（优化版 + Zap日志）
func ResumeInfoHandler(c *gin.Context) {
	// ========== 1. 日志：记录请求开始 ==========
	Logger.Info("开始处理续传信息请求",
		zap.String("client_ip", c.ClientIP()),
		zap.String("request_path", c.FullPath()),
	)

	// ========== 2. 解析路由路径参数 ==========
	dirPath := c.Param("path")
	if dirPath == "" {
		dirPath = "."
	}
	// 拼接并规范化目录路径
	trimPath := filepath.Join(GlobalConfig.UploadDir, dirPath)
	trimPath, err := filepath.Abs(trimPath)
	if err != nil {
		errMsg := fmt.Sprintf("规范化目录路径失败: %v", err)
		Logger.Error(errMsg,
			zap.String("dirPath", dirPath),
			zap.Error(err),
		)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": errMsg,
		})
		return
	}
	dirAbs, err := filepath.Abs(GlobalConfig.UploadDir)
	if err != nil {
		errMsg := fmt.Sprintf("获取root目录绝对路径失败: %v", err)
		Logger.Error("获取root目录绝对路径失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": errMsg,
		})
		return
	}

	// ========== 3. 校验目录是否存在且为目录 ==========
	resumePathInfo, err := os.Stat(trimPath)
	if err != nil {
		if os.IsNotExist(err) {
			errMsg := fmt.Sprintf("目录不存在: %s", trimPath)
			Logger.Error(errMsg, zap.String("trimPath", trimPath))
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": errMsg,
			})
			return
		}
		errMsg := fmt.Sprintf("获取目录信息失败: %v", err)
		Logger.Error(errMsg,
			zap.String("trimPath", trimPath),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": errMsg,
		})
		return
	}
	if !resumePathInfo.IsDir() {
		errMsg := fmt.Sprintf("路径%s不是目录", trimPath)
		Logger.Error(errMsg, zap.String("trimPath", trimPath))
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": errMsg,
		})
		return
	}

	// ========== 4. 解析并校验文件名 ==========
	fileName := c.Query("file_name")
	if fileName == "" {
		errMsg := "文件名称不能为空"
		Logger.Error(errMsg)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": errMsg,
		})
		return
	}

	// ========== 5. 拼接文件路径并检查是否存在 ==========
	filePath := filepath.Join(trimPath, fileName)
	// 防止路径穿越
	if !strings.HasPrefix(filePath, dirAbs) {
		errMsg := fmt.Sprintf("文件路径非法: %s", filePath)
		Logger.Error(errMsg, zap.String("filePath", filePath))
		c.JSON(http.StatusForbidden, gin.H{
			"status":  "error",
			"message": errMsg,
		})
		return
	}

	// ========== 6. 构建续传信息 ==========
	info := ResumeInfo{
		FileName:   fileName,
		FileExists: false,
	}
	chunkSize := int64(GlobalConfig.ChunkSize)

	fileStat, err := os.Stat(filePath)
	if err == nil {
		info.FileExists = true
		info.UploadedBytes = fileStat.Size()
		info.UploadedChunks = int((fileStat.Size() + chunkSize - 1) / chunkSize)
		Logger.Info("获取续传信息成功",
			zap.String("fileName", fileName),
			zap.String("uploadedBytes", FormatSize(info.UploadedBytes)),
			zap.Int("uploadedChunks", info.UploadedChunks),
		)
	} else {
		Logger.Info("文件不存在，无需续传",
			zap.String("fileName", fileName),
			zap.String("filePath", filePath),
		)
	}

	// ========== 7. 返回续传信息 ==========
	c.JSON(http.StatusOK, info)
}
