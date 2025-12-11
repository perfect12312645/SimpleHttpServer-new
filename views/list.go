package views

import (
	. "SimpleHttpServer/config"
	. "SimpleHttpServer/middleware"
	. "SimpleHttpServer/utils"
	"fmt"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// FileInfo 文件信息结构体（保留原有字段，新增Type标识）
type FileInfo struct {
	Name      string    // 文件名/目录名
	Size      string    // 格式化后的大小（如1.2KB、文件夹）
	SizeBytes int64     // 原始字节数（用于判断<10KB）
	MTime     time.Time // 修改时间
	IsDir     bool      // 是否为目录
	IsText    bool      // 是否为文本文件
}

// IndexHandler 首页处理器 - 新增分页参数接收
func IndexHandler(c *gin.Context) {
	// 1. 获取分页参数（默认第1页，每页10条）
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("pageSize", "10")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1 // 页码非法默认第1页
	}

	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 10 // 每页数量非法默认10条，最大限制100条
	}

	// 2. 获取文件列表（带分页）
	fileList, total, err := GetFileList(page, pageSize)
	if err != nil {
		Logger.Error("读取文件列表失败", zap.Error(err))
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "无法读取文件列表：" + err.Error(),
		})
		return
	}

	// 3. 获取上传目录绝对路径
	dirAbs, err := filepath.Abs(GlobalConfig.UploadDir)
	if err != nil {
		Logger.Error("获取上传目录绝对路径失败", zap.Error(err))
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "无法获取上传目录位置：" + err.Error(),
		})
		return
	}
	// 4. 计算分页相关参数
	totalPage := (total + pageSize - 1) / pageSize // 总页数（向上取整）

	// 5. 渲染页面
	c.HTML(http.StatusOK, "index.html", gin.H{
		"Files":         fileList,
		"Chunk_size":    GlobalConfig.ChunkSize,
		"Max_file_size": FormatSize(GlobalConfig.MaxFileSize),
		"dirAbs":        dirAbs,
		// 分页参数
		"CurrentPage": page,      // 当前页码
		"PageSize":    pageSize,  // 每页数量
		"Total":       total,     // 总条数
		"TotalPage":   totalPage, // 总页数
	})
}

// GetFileList 获取文件列表（支持分页）
// 参数：page-页码（从1开始），pageSize-每页条数
// 返回：当前页列表、总条数、错误
func GetFileList(page, pageSize int) ([]FileInfo, int, error) {
	var allFileList []FileInfo
	uploadDir := GlobalConfig.UploadDir

	// 1. 读取目录所有条目（文件+目录）
	entries, err := os.ReadDir(uploadDir)
	if err != nil {
		return nil, 0, fmt.Errorf("读取上传目录失败: %w", err)
	}

	// 2. 遍历所有条目，构建完整列表
	for _, entry := range entries {
		// 跳过临时文件/隐藏文件（目录和文件都适用）
		if strings.HasSuffix(entry.Name(), ".part") || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		// 获取文件/目录的详细信息
		info, err := entry.Info()
		if err != nil {
			Logger.Warn("获取条目信息失败",
				zap.String("name", entry.Name()),
				zap.Error(err))
			continue
		}

		// 构建通用的FileInfo结构体
		fileInfo := FileInfo{
			Name: entry.Name(),

			MTime:  info.ModTime(),
			IsDir:  entry.IsDir(),
			IsText: !info.IsDir() && isTextFile(info.Name()),
		}

		// 区分文件/目录的特殊字段
		if entry.IsDir() {
			fileInfo.Size = "--" // 目录不显示大小

		} else {
			fileInfo.Size = FormatSize(info.Size()) // 文件显示格式化后的大小
			fileInfo.SizeBytes = info.Size()        // 原始字节数
		}

		allFileList = append(allFileList, fileInfo)
	}

	// 3. 排序规则：统一按修改时间（创建时间）降序排列（最新的在前）
	sort.Slice(allFileList, func(i, j int) bool {
		// 按MTime降序，无论文件/目录类型
		return allFileList[i].MTime.After(allFileList[j].MTime)
	})

	// 4. 分页处理
	total := len(allFileList)
	// 计算分页起始/结束索引
	start := (page - 1) * pageSize
	end := start + pageSize

	// 边界处理：起始索引超过总数，返回空列表
	if start >= total {
		return []FileInfo{}, total, nil
	}
	// 结束索引超过总数，取到最后一条
	if end > total {
		end = total
	}
	// 截取当前页数据
	currentPageList := allFileList[start:end]

	return currentPageList, total, nil
}

// 补充IsText判断逻辑（示例）
func isTextFile(filename string) bool {
	// 文本文件后缀列表，可根据需求扩展
	textExts := map[string]bool{
		"txt": true, "md": true, "json": true, "xml": true,
		"yml": true, "yaml": true, "ini": true, "conf": true,
		"log": true, "csv": true, "tsv": true,
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return false
	}
	return textExts[ext[1:]] // 去掉前缀"."
}
