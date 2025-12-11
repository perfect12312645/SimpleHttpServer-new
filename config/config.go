package config

import (
	"sync"
	"time"
)

// UploadStatus 分块上传状态（导出类型，供其他包使用）
type UploadStatus struct {
	TotalChunks    int       // 总分块数
	ReceivedChunks int       // 已接收分块数
	FilePath       string    // 文件存储路径
	LastUpdated    time.Time // 最后更新时间
}

// ResumeInfo 断点续传信息（导出类型，JSON标签保留）
type ResumeInfo struct {
	FileName       string `json:"file_name"`       // 文件名
	FileExists     bool   `json:"file_exists"`     // 文件是否已存在
	UploadedBytes  int64  `json:"uploaded_bytes"`  // 已上传字节数
	UploadedChunks int    `json:"uploaded_chunks"` // 已上传分块数
}

// ServerConfig 服务全局配置（导出类型）
type ServerConfig struct {
	Port        int64             // 服务端口
	MaxFileSize int64             // 最大文件大小(B)
	UploadDir   string            // 文件上传目录
	ChunkSize   int64             // 分块大小(B)
	FileIconMap map[string]string // 文件类型对应图标（修正字段名大写导出）
}

// 全局上传状态缓存
var UploadStatusCache sync.Map // 键：文件唯一标识，值：*UploadStatus

// 全局配置单例（导出变量，所有包可访问）
var GlobalConfig = &ServerConfig{
	// 初始化文件图标映射（直接在单例中赋值，更规范）
	FileIconMap: map[string]string{
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
	},
}
