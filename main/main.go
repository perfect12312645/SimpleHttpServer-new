package main

import (
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	port        int
	maxFileSize int64
	uploadDir   string
	chunkSizeMB int
	chunkSize   int64
	fileIconMap = map[string]string{
		"pdf":  "fa-file-pdf-o",
		"doc":  "fa-file-word-o",
		"docx": "fa-file-word-o",
		"xls":  "fa-file-excel-o",
		"xlsx": "fa-file-excel-o",
		"ppt":  "fa-file-powerpoint-o",
		"pptx": "fa-file-powerpoint-o",
		"jpg":  "fa-file-image-o",
		"jpeg": "fa-file-image-o",
		"png":  "fa-file-image-o",
		"gif":  "fa-file-image-o",
		"bmp":  "fa-file-image-o",
		"webp": "fa-file-image-o",
		"zip":  "fa-file-archive-o",
		"rar":  "fa-file-archive-o",
		"7z":   "fa-file-archive-o",
		"txt":  "fa-file-text-o",
	}
	uploadStatus sync.Map
)

type FileInfo struct {
	Name  string
	Size  string
	MTime time.Time
}

type UploadStatus struct {
	TotalChunks    int
	ReceivedChunks int
	FilePath       string
	LastUpdated    time.Time
}

type ResumeInfo struct {
	FileName       string `json:"file_name"`
	FileExists     bool   `json:"file_exists"`
	UploadedBytes  int64  `json:"uploaded_bytes"`
	UploadedChunks int    `json:"uploaded_chunks"`
}

func init() {
	flag.IntVar(&port, "p", 18181, "指定启动的端口，默认:18181")
	flag.Int64Var(&maxFileSize, "M", 20, "修改可上传的最大文件大小,单位:GB,默认:20")
	flag.StringVar(&uploadDir, "d", "uploads", "指定要上传的目录名，默认:uploads")
	flag.IntVar(&chunkSizeMB, "c", 5, "分块上传，每块儿的大小，大文件可取大一些，单位:MB，默认值:5")
}

func main() {
	flag.Parse()

	// 计算实际大小
	maxFileSize = maxFileSize * 1024 * 1024 * 1024
	chunkSize = int64(chunkSizeMB) * 1024 * 1024

	// 创建上传目录
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Fatalf("无法创建上传目录: %v", err)
	}

	r := gin.Default()

	// 自定义模板函数
	r.SetFuncMap(template.FuncMap{
		"datetimeformat": func(t time.Time) string {
			return t.Format("2006-01-02 15:04:05")
		},
		"getFileIconClass": func(filename string) string {
			parts := strings.Split(filename, ".")
			if len(parts) > 1 {
				ext := strings.ToLower(parts[len(parts)-1])
				if icon, ok := fileIconMap[ext]; ok {
					return icon
				}
			}
			return "fa-file-o"
		},
		"formatSize": formatSize,
	})

	r.LoadHTMLGlob("templates/*")

	// 路由设置
	r.GET("/", indexHandler)
	r.POST("/upload", uploadHandler)
	r.GET("/get_resume_info", resumeInfoHandler)
	r.GET("/download/:filename", downloadHandler)
	r.DELETE("/delete/:filename", deleteHandler)
	r.Static("/static", "./static")

	log.Printf("服务器运行在 http://0.0.0.0:%d", port)
	log.Printf("上传的文件将保存在 %s 目录", uploadDir)
	log.Printf("最大文件大小: %s", formatSize(maxFileSize))

	if err := r.Run(fmt.Sprintf(":%d", port)); err != nil {
		log.Fatal("服务器启动失败: ", err)
	}
}

func indexHandler(c *gin.Context) {
	files, err := getFileList()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "无法读取文件列表",
		})
		return
	}

	c.HTML(http.StatusOK, "index.html", gin.H{
		"Files":         files,
		"Chunk_size":    chunkSizeMB,
		"Max_file_size": formatSize(maxFileSize),
	})
}

func uploadHandler(c *gin.Context) {
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

	// 验证文件大小
	if action == "new" || action == "overwrite" {
		if file.Size*int64(totalChunks) > maxFileSize {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": fmt.Sprintf("文件大小超过限制 (%s)", formatSize(maxFileSize)),
			})
			return
		}
	}

	filePath := filepath.Join(uploadDir, fileName)

	// 处理上传操作
	switch action {
	case "new", "overwrite":
		if chunkIndex == 0 {
			if action == "overwrite" {
				os.Remove(filePath)
			}
			uploadStatus.Store(fileName, &UploadStatus{
				TotalChunks:    totalChunks,
				ReceivedChunks: 0,
				FilePath:       filePath,
				LastUpdated:    time.Now(),
			})
		}
	case "resume":
		if _, loaded := uploadStatus.Load(fileName); !loaded {
			// 尝试从磁盘恢复状态
			if info, err := os.Stat(filePath); err == nil {
				uploadedChunks := int((info.Size() + chunkSize - 1) / chunkSize)
				uploadStatus.Store(fileName, &UploadStatus{
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
	statusInterface, ok := uploadStatus.Load(fileName)
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
		uploadStatus.Delete(fileName)
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

func resumeInfoHandler(c *gin.Context) {
	fileName := c.Query("file_name")
	filePath := filepath.Join(uploadDir, fileName)

	info := ResumeInfo{
		FileName:   fileName,
		FileExists: false,
	}

	fileStat, err := os.Stat(filePath)
	if err == nil {
		info.FileExists = true
		info.UploadedBytes = fileStat.Size()
		info.UploadedChunks = int((fileStat.Size() + chunkSize - 1) / chunkSize)
	}

	c.JSON(http.StatusOK, info)
}

func downloadHandler(c *gin.Context) {
	filename := c.Param("filename")
	filePath := filepath.Join(uploadDir, filename)

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

func deleteHandler(c *gin.Context) {
	filename := c.Param("filename")
	filePath := filepath.Join(uploadDir, filename)
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
	uploadStatus.Delete(filename)

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "文件已删除"})
}

func getFileList() ([]FileInfo, error) {
	files := []FileInfo{}

	entries, err := os.ReadDir(uploadDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// 跳过临时文件
		if strings.HasSuffix(entry.Name(), ".part") {
			continue
		}

		files = append(files, FileInfo{
			Name:  entry.Name(),
			Size:  formatSize(info.Size()),
			MTime: info.ModTime(),
		})
	}

	// 按修改时间排序（最新的在前）
	for i, j := 0, len(files)-1; i < j; i, j = i+1, j-1 {
		files[i], files[j] = files[j], files[i]
	}

	return files, nil
}

func formatSize(size int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	unitIndex := 0
	floatSize := float64(size)

	for floatSize >= 1024 && unitIndex < len(units)-1 {
		floatSize /= 1024
		unitIndex++
	}

	return fmt.Sprintf("%.2f %s", floatSize, units[unitIndex])
}
