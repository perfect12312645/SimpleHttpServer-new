package utils

import (
	"SimpleHttpServer/config"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func FormatSize(size int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	unitIndex := 0
	floatSize := float64(size)

	for floatSize >= 1024 && unitIndex < len(units)-1 {
		floatSize /= 1024
		unitIndex++
	}

	return fmt.Sprintf("%.2f %s", floatSize, units[unitIndex])
}

// IsTextFile 后缀判断版：高效、无IO，适合海量文件场景
// 核心逻辑：提取后缀→转小写→匹配预定义列表
func IsTextFile(filename string) bool {
	// 1. 提取文件后缀（带.）
	ext := filepath.Ext(filename)
	if ext == "" {
		// 无后缀：判定为非文本（可根据业务调整，比如返回false/让用户手动标记）
		return false
	}

	// 2. 去掉前缀.，转小写（兼容大写后缀如.TXT/.JSON）
	extLower := strings.ToLower(ext[1:])

	// 3. 匹配文本后缀列表
	return config.TextFileExts[extLower]
}

// 计算文件的 MD5 值
func FileMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
