package views

import (
	. "SimpleHttpServer/config"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// DeleteHandler 适配多级路径的文件删除接口
// 路由建议：r.DELETE("/delete/*path", DeleteHandler)（*path匹配多级路径）
func DeleteHandler(c *gin.Context) {
	// 1. 获取URL中的编码后的完整路径参数（替换原filename）
	encodedPath := c.Param("path")
	if encodedPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "未指定要删除的文件路径",
		})
		return
	}

	// 2. 去掉路径开头的/（避免拼接后出现//）+ URL解码（处理空格/中文/特殊字符）
	encodedPath = strings.TrimPrefix(encodedPath, "/")
	fileFullPath, err := url.QueryUnescape(encodedPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": fmt.Sprintf("路径解析失败：%v", err),
		})
		return
	}

	// 3. 路径安全校验（防止../../等路径遍历攻击，核心！）
	rootUploadDir := filepath.Clean(GlobalConfig.UploadDir) // 根目录清理
	targetFilePath := filepath.Clean(filepath.Join(rootUploadDir, fileFullPath))
	// 校验：目标文件必须在根上传目录内
	if !strings.HasPrefix(targetFilePath, rootUploadDir) {
		c.JSON(http.StatusForbidden, gin.H{
			"status":  "error",
			"message": "禁止删除上传目录外的文件（路径遍历攻击）",
		})
		return
	}

	// 4. 拼接临时文件路径（保留原有逻辑）
	tempFilePath := targetFilePath + ".part"

	// 5. 删除主文件（保留原有逻辑，兼容文件不存在的情况）
	if err := os.Remove(targetFilePath); err != nil && !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": fmt.Sprintf("删除文件失败: %v", err),
		})
		return
	}

	// 6. 删除临时文件（保留原有逻辑）
	if err := os.Remove(tempFilePath); err != nil && !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": fmt.Sprintf("删除临时文件失败: %v", err),
		})
		return
	}

	// 7. 删除上传状态缓存（key改为完整路径，和前端传递的一致）
	UploadStatusCache.Delete(fileFullPath)

	// 8. 返回成功（格式和前端JS匹配）
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "文件已删除",
	})
}
