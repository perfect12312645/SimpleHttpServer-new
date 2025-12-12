package views

import (
	"SimpleHttpServer/config"
	"SimpleHttpServer/middleware"
	"SimpleHttpServer/utils"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"strings"
)

// PreviewFile 文本文件预览接口
// 路由：/preview/*path
// 核心逻辑：
// 1. 路径安全校验（防止路径遍历）
// 2. 文件基础校验（存在、是普通文件）
// 3. 大小校验（0 < 大小 ≤ 10MB）
// 4. 文本后缀校验（基于后缀判断）
// 5. 读取内容并渲染预览，异常场景返回error.html
func PreviewFile(c *gin.Context) {
	// 1. 获取URL中的文件相对路径（*path会包含/，比如/xxx/xxx/a.txt）
	relPath := c.Param("path")
	if relPath == "" {
		renderError(c, "预览失败：未指定文件路径")
		return
	}
	// 去掉路径开头的/（避免拼接后出现//）
	relPath = strings.TrimPrefix(relPath, "/")

	// 2. 拼接文件绝对路径 + 路径安全校验（防止../../等路径遍历）
	rootUploadDir := config.GlobalConfig.UploadDir // 你的根上传目录（确保是绝对路径）
	absFilePath := filepath.Join(rootUploadDir, relPath)
	// 关键：校验拼接后的路径是否在根上传目录内（防止路径遍历攻击）
	if !strings.HasPrefix(filepath.Clean(absFilePath), filepath.Clean(rootUploadDir)) {
		renderError(c, "预览失败：非法文件路径（禁止访问上传目录外的文件）")
		middleware.Logger.Warn("非法文件路径访问", zap.String("absFilePath", absFilePath), zap.String("rootDir", rootUploadDir))
		return
	}

	// 3. 文件基础属性校验
	fileInfo, err := os.Stat(absFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			renderError(c, "预览失败：文件不存在")
		} else {
			renderError(c, "预览失败：获取文件信息失败："+err.Error())
		}
		middleware.Logger.Warn("文件基础校验失败", zap.String("filePath", absFilePath), zap.Error(err))
		return
	}
	// 校验是否为普通文件（排除目录、设备文件等）
	if !fileInfo.Mode().IsRegular() {
		renderError(c, "预览失败：当前路径不是普通文件（可能是目录/设备文件）")
		return
	}

	// 4. 文件大小校验（阈值：0 < 大小 ≤ 10MB）
	const (
		maxPreviewSize = 10 * 1024 * 1024 // 10MB（文本预览合理上限）
		minPreviewSize = 1                // 空文件不允许预览
	)
	fileSize := fileInfo.Size()
	if fileSize < minPreviewSize {
		renderError(c, "预览失败：文件为空（大小为0字节）")
		return
	}
	if fileSize > maxPreviewSize {
		renderError(c, "预览失败：文件过大（仅支持预览≤10MB的文本文件）")
		return
	}

	// 5. 文本后缀校验（复用之前的IsTextFile函数）
	if !utils.IsTextFile(absFilePath) {
		renderError(c, "预览失败：仅支持预览文本文件（后缀不符）")
		return
	}

	// 6. 读取文件内容（10MB以内，内存可控）
	content, err := os.ReadFile(absFilePath)
	if err != nil {
		renderError(c, "预览失败：读取文件内容失败："+err.Error())
		middleware.Logger.Warn("读取文件内容失败", zap.String("filePath", absFilePath), zap.Error(err))
		return
	}

	// 7. 渲染预览页面（传递文件名、路径、内容）
	c.HTML(200, "preview.html", gin.H{
		"fileName": filepath.Base(absFilePath), // 文件名（如a.txt）
		"fileSize": fileSize,                   // 文件大小（字节）
		"content":  string(content),            // 文件内容
	})
}

// renderError 统一渲染错误页面
func renderError(c *gin.Context, msg string) {
	c.HTML(500, "error.html", gin.H{
		"error": msg,
	})
}
