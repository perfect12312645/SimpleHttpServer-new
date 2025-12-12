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
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// FileInfo 封装文件/目录的核心信息，用于前端展示和分页处理
type FileInfo struct {
	Name      string    // 文件/目录的名称（搜索场景下为相对路径，普通场景下为原名称）
	Size      string    // 格式化后的大小展示值（目录显示"--"，文件显示如"1.2KB"）
	SizeBytes int64     // 文件原始字节数，用于判断是否生成二维码（<10KB）
	MTime     time.Time // 文件/目录的最后修改时间，用于排序
	IsDir     bool      // 标识是否为目录，用于前端展示不同图标和操作逻辑
	IsText    bool      // 标识是否为文本文件，用于前端判断是否展示预览功能
}

// IndexHandler 首页处理器，负责渲染文件列表首页
// 核心逻辑：解析分页/搜索参数 → 获取文件列表 → 计算上传目录绝对路径 → 渲染前端模板
func IndexHandler(c *gin.Context) {
	// 获取分页参数，默认值为第1页、每页10条数据
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("pageSize", "10")

	// 将分页参数解析为整数，确保前端模板可执行算术运算，非法值重置为默认值
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}
	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	// 获取上传目录的绝对路径，用于前端展示当前目录位置
	dirAbs, err := filepath.Abs(GlobalConfig.UploadDir)
	if err != nil {
		Logger.Error("获取上传目录绝对路径失败", zap.Error(err))
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "无法获取上传目录位置：" + err.Error(),
		})
		return
	}

	// 调用GetFileList获取根目录下的文件列表，空字符串表示根目录，传递搜索关键词和分页参数
	fileList, total, totalPage, err := GetFileList(dirAbs, c.Query("search"), strconv.Itoa(page), strconv.Itoa(pageSize))
	if err != nil {
		Logger.Error("读取文件列表失败", zap.Error(err))
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "无法读取文件列表：" + err.Error(),
		})
		return
	}

	// 渲染首页模板，传递文件列表、分页参数、系统配置等数据供前端使用
	c.HTML(http.StatusOK, "index.html", gin.H{
		"Files":         fileList,
		"Chunk_size":    GlobalConfig.ChunkSize,
		"Max_file_size": FormatSize(GlobalConfig.MaxFileSize),
		"dirAbs":        dirAbs,
		"dirRel":        "", // 当前目录相对路径，用于面包屑导航
		// 分页参数：传递整数类型，确保前端模板可执行加减运算
		"CurrentPage": page,              // 当前页码
		"PageSize":    pageSize,          // 每页展示数据条数
		"Total":       total,             // 符合条件的文件总数
		"TotalPage":   totalPage,         // 总页数
		"SearchKey":   c.Query("search"), // 搜索关键词，用于前端回显
	})
}

// recursiveSearchFiles 递归遍历目录的辅助函数
// 功能：从指定根目录和当前目录开始，递归收集符合搜索关键词的文件/目录信息
// 参数说明：
//
//	rootDir - 根上传目录，用于计算相对路径
//	currentDir - 当前遍历的目录
//	searchKey - 搜索关键词，用于过滤文件/目录
//	result - 收集结果的切片指针，避免值拷贝提升性能
func recursiveSearchFiles(rootDir, currentDir, searchKey string, result *[]FileInfo) {
	// 读取当前目录下的所有文件/目录条目
	entries, err := os.ReadDir(currentDir)
	if err != nil {
		Logger.Warn("递归读取目录失败", zap.String("dir", currentDir), zap.Error(err))
		return
	}

	for _, entry := range entries {
		// 跳过临时文件（.part后缀）和隐藏文件/目录（.开头），符合业务过滤规则
		if strings.HasSuffix(entry.Name(), ".part") || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		// 拼接当前条目绝对路径，用于后续获取文件信息和计算相对路径
		entryAbsPath := path.Join(currentDir, entry.Name())
		// 计算当前条目相对于根上传目录的相对路径，用于前端展示完整路径
		entryRelPath, err := filepath.Rel(rootDir, entryAbsPath)
		if err != nil {
			Logger.Warn("计算相对路径失败", zap.String("absPath", entryAbsPath), zap.Error(err))
			continue
		}

		// 获取当前条目的详细文件信息（大小、修改时间等）
		info, err := entry.Info()
		if err != nil {
			Logger.Warn("获取条目信息失败", zap.String("name", entry.Name()), zap.Error(err))
			continue
		}

		// 搜索过滤逻辑：仅匹配文件名（忽略大小写），非匹配项若为目录则递归遍历子目录
		match := strings.Contains(strings.ToLower(entry.Name()), strings.ToLower(searchKey))
		if !match {
			if entry.IsDir() {
				recursiveSearchFiles(rootDir, entryAbsPath, searchKey, result)
			}
			continue
		}

		// 构建FileInfo结构体，Name字段使用相对路径，其余字段按业务规则赋值
		fileInfo := FileInfo{
			Name:   entryRelPath,
			MTime:  info.ModTime(),
			IsDir:  entry.IsDir(),
			IsText: !entry.IsDir() && IsTextFile(entry.Name()),
		}

		// 区分目录和文件的大小展示逻辑：目录显示"--"，文件显示格式化后的大小
		if entry.IsDir() {
			fileInfo.Size = "--"
		} else {
			fileInfo.Size = FormatSize(info.Size())
			fileInfo.SizeBytes = info.Size()
		}

		// 将符合条件的FileInfo添加到结果切片中
		*result = append(*result, fileInfo)
	}
}

// GetFileList 获取指定目录下的文件列表，支持分页和递归搜索
// 参数说明：
//
//	dir - 目标目录（空字符串表示根上传目录）
//	searchKey - 搜索关键词（空字符串表示不搜索，仅读取当前目录）
//	pageStr - 页码字符串
//	pageSizeStr - 每页条数字符串
//
// 返回值：
//
//	[]FileInfo - 当前页的文件/目录列表
//	int - 符合条件的文件总数
//	int - 总页数
//	error - 错误信息（读取目录失败等）
func GetFileList(dir string, searchKey, pageStr, pageSizeStr string) ([]FileInfo, int, int, error) {
	var allFileList []FileInfo
	// 解析分页参数：非法值重置为默认值（页码≥1，每页条数1-100）
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	// 搜索逻辑分支：有搜索关键词则递归遍历目标目录所有子目录，无则仅读取当前目录
	if searchKey != "" {
		recursiveSearchFiles(dir, dir, searchKey, &allFileList)
	} else {
		// 无搜索关键词时，仅读取当前目录下的文件/目录，不递归
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("读取上传目录失败: %w", err)
		}

		// 遍历当前目录条目，构建FileInfo列表，过滤临时/隐藏文件
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".part") || strings.HasPrefix(entry.Name(), ".") {
				continue
			}

			info, err := entry.Info()
			if err != nil {
				Logger.Warn("获取条目信息失败",
					zap.String("name", entry.Name()),
					zap.Error(err))
				continue
			}

			// 构建FileInfo结构体，Name字段使用原文件名（非搜索场景）
			fileInfo := FileInfo{
				Name:   entry.Name(),
				MTime:  info.ModTime(),
				IsDir:  entry.IsDir(),
				IsText: !info.IsDir() && IsTextFile(info.Name()),
			}

			// 区分目录和文件的大小展示逻辑
			if entry.IsDir() {
				fileInfo.Size = "--"
			} else {
				fileInfo.Size = FormatSize(info.Size())
				fileInfo.SizeBytes = info.Size()
			}

			allFileList = append(allFileList, fileInfo)
		}
	}

	// 排序逻辑：所有文件/目录按修改时间降序排列，最新修改的条目在前
	sort.Slice(allFileList, func(i, j int) bool {
		return allFileList[i].MTime.After(allFileList[j].MTime)
	})

	// 分页处理逻辑：计算总条数、总页数，截取当前页数据
	total := len(allFileList)
	totalPage := (total + pageSize - 1) / pageSize
	start := (page - 1) * pageSize
	end := start + pageSize

	// 边界处理：起始索引超过总数时返回空列表
	if start >= total {
		return []FileInfo{}, total, totalPage, nil
	}
	// 结束索引超过总数时，截取到最后一条数据
	if end > total {
		end = total
	}
	// 截取当前页数据
	currentPageList := allFileList[start:end]

	return currentPageList, total, totalPage, nil
}

// ExploreDir 目录浏览处理器，支持访问指定子目录并展示其文件列表
// 核心逻辑：解析目录路径 → 安全校验 → 读取目录文件列表 → 渲染模板
func ExploreDir(c *gin.Context) {
	// 获取URL中/explore后的路径参数，即要访问的子目录相对路径
	relativePath := c.Param("path")

	// 获取根上传目录的绝对路径，作为目录访问权限校验的基准
	dirAbs, err := filepath.Abs(GlobalConfig.UploadDir)
	if err != nil {
		Logger.Error("获取上传目录绝对路径失败", zap.Error(err))
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "无法获取上传目录位置：" + err.Error(),
		})
		return
	}

	// 拼接目标目录绝对路径并清理（去除../等非法路径），防止路径遍历漏洞
	targetDir := filepath.Clean(filepath.Join(dirAbs, relativePath))
	// 安全校验：确保目标目录在根上传目录范围内，禁止访问外部目录
	if !strings.HasPrefix(targetDir, dirAbs) {
		c.String(http.StatusForbidden, "非法目录访问")
		return
	}
	// 校验目标路径是否存在且为目录
	fileInfo, err := os.Stat(targetDir)
	if err != nil {
		if os.IsNotExist(err) {
			c.String(http.StatusNotFound, "目录不存在")
			return
		}
		c.String(http.StatusInternalServerError, "获取目录信息失败：%v", err)
		return
	}
	if !fileInfo.IsDir() {
		c.String(http.StatusBadRequest, "目标路径不是目录")
		return
	}

	// 解析分页参数为整数，非法值重置为默认值，确保前端模板运算正常
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("pageSize", "10")
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}
	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	// 调用GetFileList获取目标目录下的文件列表，传递目录路径、搜索关键词和分页参数
	files, total, totalPage, err := GetFileList(targetDir, c.Query("search"), strconv.Itoa(page), strconv.Itoa(pageSize))
	if err != nil {
		c.String(http.StatusInternalServerError, "读取目录失败：%v", err)
		return
	}

	// 渲染模板，传递目录信息、文件列表、分页参数等数据供前端使用
	c.HTML(http.StatusOK, "index.html", gin.H{
		"dirAbs":        dirAbs,       // 当前目录绝对路径，用于前端展示
		"dirRel":        relativePath, // 当前目录相对路径，用于面包屑导航
		"Files":         files,        // 当前目录下的文件/目录列表
		"Chunk_size":    GlobalConfig.ChunkSize,
		"Max_file_size": FormatSize(GlobalConfig.MaxFileSize),
		"TotalPage":     totalPage,         // 总页数
		"Total":         total,             // 符合条件的文件总数
		"CurrentPage":   page,              // 当前页码
		"PageSize":      pageSize,          // 每页展示数据条数
		"SearchKey":     c.Query("search"), // 搜索关键词，用于前端回显
	})
}
