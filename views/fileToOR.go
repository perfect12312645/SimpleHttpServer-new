package views

import (
	// 点导入middleware包，直接调用包内函数（如Logger）
	. "SimpleHttpServer/middleware"

	"SimpleHttpServer/config"
	"SimpleHttpServer/utils"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/skip2/go-qrcode" // 二维码生成依赖
	"go.uber.org/zap"            // 若Logger返回zap.Logger，需引入（根据实际情况调整）
)

// 文件分片信息结构
type QRChunk struct {
	Index     int    `json:"index"`     // 分片索引 (从1开始)
	Total     int    `json:"total"`     // 总分片数
	Content   string `json:"content"`   // 分片内容
	IsBase64  bool   `json:"isBase64"`  // 是否为Base64编码
	QRCodePNG string `json:"qrCodePNG"` // 二维码Base64图片
}

// 主要Gin视图函数
func HandleFileToQR(c *gin.Context) {
	// 1. 获取上传的文件路径参数
	path := c.Param("path")
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		Logger.Error("生成二维码失败：请求路径为空", zap.String("request_url", c.Request.URL.String()))
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "file path is required (e.g. /qrcode/run.sh)",
		})
		return
	}

	// 获取上传目录绝对路径
	dirAbs, err := filepath.Abs(config.GlobalConfig.UploadDir)
	if err != nil {
		Logger.Error("生成二维码失败：获取上传目录绝对路径失败", zap.Error(err), zap.String("upload_dir", config.GlobalConfig.UploadDir))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "获取文件目录失败",
			"detail": err.Error(),
		})
		return
	}

	// 安全拼接文件路径（避免分隔符问题，如dirAbs无/结尾导致路径错误）
	fullPath := filepath.Join(dirAbs, path)

	// 校验文件是否存在、是否为文件、大小限制
	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		Logger.Error("生成二维码失败：打开文件失败", zap.String("full_path", fullPath), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "打开文件失败",
			"detail": err.Error(),
		})
		return
	}

	if fileInfo.IsDir() {
		Logger.Warn("生成二维码失败：请求路径是目录而非文件", zap.String("full_path", fullPath))
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求必须是文件（不能是目录）",
		})
		return
	}

	if fileInfo.Size() == 0 {
		Logger.Warn("生成二维码失败：请求文件为空", zap.String("full_path", fullPath))
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "文件为空",
		})
		return
	}

	// 提前校验文件大小
	maxSize := 10 * 1024 // 10KB
	if fileInfo.Size() >= int64(maxSize) {
		Logger.Warn("生成二维码失败：文件大小超过限制", zap.String("full_path", fullPath), zap.Int64("file_size", fileInfo.Size()), zap.Int64("max_size", int64(maxSize)))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "文件过大",
			"detail":     fmt.Sprintf("文件大小: %d 字节, 最大支持: %d 字节", fileInfo.Size(), maxSize),
			"suggestion": "请使用小于10KB的文件",
		})
		return
	}

	// 2. 读取文件内容（使用拼接后的完整路径）
	fileBytes, err := os.ReadFile(fullPath)
	if err != nil {
		Logger.Error("生成二维码失败：读取文件失败", zap.String("full_path", fullPath), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "读取文件失败",
			"detail": err.Error(),
		})
		return
	}

	// 变量定义
	fileName := fileInfo.Name()
	fileSize := len(fileBytes)
	Logger.Info("成功读取文件，开始分片处理", zap.String("file_name", fileName), zap.Int("file_size", fileSize))

	// 3. 分片处理（新增错误返回）
	chunkSize := 2048 // 每个分片2KB
	var chunks []QRChunk
	var splitErr error
	isText := isTextFile(fileBytes)
	if isText {
		// 文本文件分片（新增错误返回）
		chunks, splitErr = splitTextToChunks(fileBytes, chunkSize)
	} else {
		// 二进制文件分片（新增错误返回）
		chunks, splitErr = splitBinaryToChunks(fileBytes, chunkSize)
	}

	if splitErr != nil {
		Logger.Error("生成二维码失败：文件分片失败", zap.String("file_name", fileName), zap.Error(splitErr))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "文件分片失败",
			"detail": splitErr.Error(),
		})
		return
	}

	if len(chunks) == 0 {
		Logger.Error("生成二维码失败：未生成任何分片", zap.String("file_name", fileName))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "未生成任何分片",
		})
		return
	}

	Logger.Info("文件分片成功", zap.String("file_name", fileName), zap.Int("total_chunks", len(chunks)), zap.Bool("is_text_file", isText))

	// 4. 生成二维码（新增错误返回）
	qrChunks, qrErr := generateQRCodeForChunks(chunks)
	if qrErr != nil {
		Logger.Error("生成二维码失败：二维码生成失败", zap.String("file_name", fileName), zap.Error(qrErr))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "生成二维码失败",
			"detail": qrErr.Error(),
		})
		return
	}
	md5, err := utils.FileMD5(fullPath)
	if err != nil {
		Logger.Error("md5计算失败", zap.String("file_name", fileName), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "md5计算失败",
			"detail": err.Error(),
		})
		return
	}
	// 5. 返回JSON结果
	Logger.Info("二维码生成成功", zap.String("file_name", fileName), zap.Int("total_qr_chunks", len(qrChunks)))
	c.JSON(http.StatusOK, gin.H{
		"fileName":    fileName,
		"fileSize":    utils.FormatSize(int64(fileSize)),
		"isTextFile":  isText,
		"chunkSize":   chunkSize,
		"totalChunks": len(qrChunks),
		"chunks":      qrChunks, // 每个chunk包含QRCodePNG（DataURI）
		"rawFileSize": fileSize,
		"md5":         md5,
	})
}

// 检测文件是否为文本文件
func isTextFile(content []byte) bool {
	if utf8.Valid(content) {
		printableRatio := calculatePrintableRatio(content)
		if printableRatio > 0.8 {
			return true
		}
	}

	contentType := http.DetectContentType(content)
	if strings.HasPrefix(contentType, "text/") {
		return true
	}

	return false
}

// 计算可打印字符比例
func calculatePrintableRatio(content []byte) float64 {
	if len(content) == 0 {
		return 0
	}

	printableCount := 0
	for _, b := range content {
		if (b >= 32 && b <= 126) || b == 9 || b == 10 || b == 13 {
			printableCount++
		}
	}

	return float64(printableCount) / float64(len(content))
}

// 分割文本文件 (新增错误返回，移除无用逻辑)
func splitTextToChunks(content []byte, chunkSize int) ([]QRChunk, error) {
	Logger.Info("开始文本文件分片", zap.Int("总字节数", len(content)), zap.Int("目标分片字节数", chunkSize))

	// 空内容校验
	if len(content) == 0 {
		Logger.Error("文本内容为空，无法分片")
		return nil, fmt.Errorf("文本内容为空，无法分片")
	}

	// 转为rune（用于后续保证字符不被截断）
	runes := []rune(string(content))
	totalRunes := len(runes)
	if totalRunes == 0 {
		Logger.Error("文本转换为字符后为空")
		return nil, fmt.Errorf("文本转换为字符后为空")
	}

	// 核心修复：按字节数计算分块数（而非字符数）
	var chunks []QRChunk
	currentBytePos := 0 // 当前字节位置
	currentRunePos := 0 // 当前字符位置
	chunkIndex := 1     // 分片索引
	totalBytes := len(content)

	// 循环分片（保证字符不被截断）
	for currentBytePos < totalBytes {
		// 计算当前分片的目标字节结束位置
		targetEndByte := currentBytePos + chunkSize
		if targetEndByte > totalBytes {
			targetEndByte = totalBytes
		}

		// 找到目标字节位置对应的字符位置（避免截断多字节字符）
		endRunePos := currentRunePos
		tempBytePos := currentBytePos
		for tempBytePos < targetEndByte && endRunePos < totalRunes {
			// 获取当前字符的字节长度
			charLen := len(string(runes[endRunePos]))
			// 如果当前字符会超出目标字节位置，停止（保证字符完整）
			if tempBytePos+charLen > targetEndByte {
				break
			}
			tempBytePos += charLen
			endRunePos += 1
		}

		// 截取当前分片的字符→转回字节
		chunkRunes := runes[currentRunePos:endRunePos]
		chunkStr := string(chunkRunes)

		// 添加分片
		chunks = append(chunks, QRChunk{
			Index:    chunkIndex,
			Total:    0, // 先占位，后续统一赋值
			Content:  chunkStr,
			IsBase64: false,
		})

		// 更新位置
		currentBytePos = len(string(runes[:endRunePos])) // 累计已处理字节数
		currentRunePos = endRunePos
		chunkIndex += 1
	}

	// 统一设置总分块数
	totalChunks := len(chunks)
	for i := range chunks {
		chunks[i].Total = totalChunks
	}

	Logger.Info("文本文件分片完成", zap.Int("实际总分块数", totalChunks), zap.Int("目标分片字节数", chunkSize))
	return chunks, nil
}

// 分割二进制文件 (新增错误返回，移除无用逻辑)
func splitBinaryToChunks(content []byte, chunkSize int) ([]QRChunk, error) {
	// 空内容校验
	if len(content) == 0 {
		Logger.Error("二进制分片失败：二进制内容为空")
		return nil, fmt.Errorf("二进制内容为空，无法分片")
	}

	// 二进制转Base64
	base64Str := base64.StdEncoding.EncodeToString(content)
	if base64Str == "" {
		Logger.Error("二进制分片失败：Base64编码失败")
		return nil, fmt.Errorf("二进制内容Base64编码失败")
	}

	// 计算分片数
	totalLen := len(base64Str)
	totalChunks := (totalLen + chunkSize - 1) / chunkSize

	var chunks []QRChunk
	for i := 0; i < totalChunks; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > totalLen {
			end = totalLen
		}

		chunkContent := base64Str[start:end]

		chunks = append(chunks, QRChunk{
			Index:    i + 1,
			Total:    totalChunks,
			Content:  chunkContent, // 仅返回Base64内容，无额外头部
			IsBase64: true,         // 二进制文件标记为Base64
		})
	}

	Logger.Info("二进制文件分片完成", zap.Int("total_chunks", totalChunks), zap.Int("base64_str_len", totalLen))
	return chunks, nil
}

// 为每个分片生成二维码图片（核心改造：新增错误返回+移除分片头部）
func generateQRCodeForChunks(chunks []QRChunk) ([]QRChunk, error) {
	// 空分片校验
	if len(chunks) == 0 {
		Logger.Error("二维码生成失败：分片列表为空")
		return nil, fmt.Errorf("分片列表为空，无法生成二维码")
	}

	var qrChunks []QRChunk
	for _, chunk := range chunks {
		// 核心修改：移除所有额外头部，仅用分片内容生成二维码
		info := chunk.Content // 直接使用原分片内容，无PART/BASE64标记

		// 生成二维码
		qr, err := qrcode.New(info, qrcode.Medium)
		if err != nil {
			Logger.Error("二维码生成失败：生成分片二维码失败", zap.Int("chunk_index", chunk.Index), zap.Error(err))
			return nil, fmt.Errorf("生成分片%d二维码失败: %w", chunk.Index, err)
		}
		qr.DisableBorder = false // 显示边框提升扫码成功率

		// 生成PNG字节
		pngBytes, err := qr.PNG(1024)
		if err != nil {
			Logger.Error("二维码生成失败：生成分片PNG失败", zap.Int("chunk_index", chunk.Index), zap.Error(err))
			return nil, fmt.Errorf("生成分片%d二维码PNG失败: %w", chunk.Index, err)
		}

		// 转为Base64供前端渲染
		qrBase64 := base64.StdEncoding.EncodeToString(pngBytes)
		qrDataURI := "data:image/png;base64," + qrBase64

		newChunk := chunk
		newChunk.QRCodePNG = qrDataURI
		qrChunks = append(qrChunks, newChunk)
	}

	Logger.Info("二维码生成完成", zap.Int("total_qr_chunks", len(qrChunks)))
	return qrChunks, nil
}
