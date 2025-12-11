package views

//
//import (
//	"bytes"
//	"encoding/base64"
//	"fmt"
//	"io"
//	"net/http"
//	"strings"
//	"unicode/utf8"
//
//	"github.com/gin-gonic/gin"
//)
//
//// 文件分片信息结构
//type QRChunk struct {
//	Index     int    `json:"index"`     // 分片索引 (从1开始)
//	Total     int    `json:"total"`     // 总分片数
//	Content   string `json:"content"`   // 分片内容
//	IsBase64  bool   `json:"isBase64"`  // 是否为Base64编码
//	QRCodePNG string `json:"qrCodePNG"` // 二维码Base64图片
//}
//
//// 主要Gin视图函数
//func HandleFileToQR(c *gin.Context) {
//	// 1. 获取上传的文件
//	file, header, err := c.Request.FormFile("file")
//	if err != nil {
//		c.JSON(http.StatusBadRequest, gin.H{
//			"error":  "请上传文件",
//			"detail": err.Error(),
//		})
//		return
//	}
//	defer file.Close()
//
//	// 2. 读取文件内容
//	fileBytes, err := io.ReadAll(file)
//	if err != nil {
//		c.JSON(http.StatusInternalServerError, gin.H{
//			"error":  "读取文件失败",
//			"detail": err.Error(),
//		})
//		return
//	}
//
//	fileName := header.Filename
//	fileSize := len(fileBytes)
//
//	// 3. 检查文件大小 (10KB限制)
//	maxSize := 10 * 1024 // 10KB
//	if fileSize > maxSize {
//		c.JSON(http.StatusBadRequest, gin.H{
//			"error":      "文件过大",
//			"detail":     fmt.Sprintf("文件大小: %d 字节, 最大支持: %d 字节", fileSize, maxSize),
//			"suggestion": "请使用小于10KB的文件",
//		})
//		return
//	}
//
//	// 4. 智能判断文件类型 (不依赖文件名)
//	isTextFile, detectedType := detectFileType(fileBytes, fileName)
//	chunkSize := 2 * 1024 // 每个分片2KB
//
//	// 5. 根据文件类型处理并分片
//	var chunks []QRChunk
//	if isTextFile {
//		// 文本文件 - 安全分割 (避免切坏中文字符)
//		chunks = splitTextToChunks(fileBytes, chunkSize, false)
//	} else {
//		// 二进制文件 - 转Base64后分割
//		chunks = splitBinaryToChunks(fileBytes, chunkSize)
//	}
//
//	// 6. 为每个分片生成二维码
//	qrChunks := generateQRCodeForChunks(chunks)
//
//	// 7. 返回结果给前端
//	c.HTML(http.StatusOK, "qr_display.html", gin.H{
//		"fileName":    fileName,
//		"fileSize":    formatFileSize(fileSize),
//		"fileType":    detectedType,
//		"isTextFile":  isTextFile,
//		"chunkSize":   chunkSize,
//		"totalChunks": len(qrChunks),
//		"chunks":      qrChunks,
//		"rawFileSize": fileSize,
//	})
//}
//
//// 检测文件类型 (不依赖后缀名)
//func detectFileType(content []byte, filename string) (bool, string) {
//	// 方法1: 检查是否为有效的UTF-8文本
//	if utf8.Valid(content) {
//		// 进一步检查是否主要为可打印字符
//		printableRatio := calculatePrintableRatio(content)
//		if printableRatio > 0.8 { // 80%以上为可打印字符
//			return true, "UTF-8文本"
//		}
//	}
//
//	// 方法2: 通过HTTP DetectContentType检测
//	contentType := http.DetectContentType(content)
//	if strings.HasPrefix(contentType, "text/") {
//		return true, contentType
//	}
//
//	// 方法3: 检查常见文本文件特征
//	if isLikelyText(content) {
//		return true, "文本/配置文件"
//	}
//
//	// 默认为二进制文件
//	return false, "二进制文件"
//}
//
//// 计算可打印字符比例
//func calculatePrintableRatio(content []byte) float64 {
//	if len(content) == 0 {
//		return 0
//	}
//
//	printableCount := 0
//	for _, b := range content {
//		// ASCII可打印字符范围 (包括空格、换行、制表符)
//		if (b >= 32 && b <= 126) || b == 9 || b == 10 || b == 13 {
//			printableCount++
//		}
//	}
//
//	return float64(printableCount) / float64(len(content))
//}
//
//// 检查是否为可能的文本文件
//func isLikelyText(content []byte) bool {
//	// 检查空字节 - 二进制文件通常会有
//	for _, b := range content {
//		if b == 0 && len(content) > 10 {
//			return false
//		}
//	}
//
//	// 检查常见文本文件头
//	textHeaders := [][]byte{
//		[]byte("#!"), // Shebang脚本
//		[]byte("{"),  // JSON开始
//		[]byte("<"),  // XML/HTML开始
//		[]byte("<?xml"),
//		[]byte("---"), // YAML
//	}
//
//	for _, header := range textHeaders {
//		if bytes.HasPrefix(content, header) {
//			return true
//		}
//	}
//
//	return false
//}
//
//// 分割文本文件 (安全处理中文字符)
//func splitTextToChunks(content []byte, chunkSize int, forceBase64 bool) []QRChunk {
//	var chunks []QRChunk
//
//	// 将字节转换为rune，确保字符完整性
//	runes := []rune(string(content))
//	totalRunes := len(runes)
//
//	// 计算需要多少分片 (按字符数计算)
//	runesPerChunk := chunkSize / 3 // 保守估计，每个中文字符约3字节
//	if runesPerChunk < 100 {
//		runesPerChunk = 100 // 最小100字符
//	}
//
//	totalChunks := (totalRunes + runesPerChunk - 1) / runesPerChunk
//
//	for i := 0; i < totalChunks; i++ {
//		start := i * runesPerChunk
//		end := start + runesPerChunk
//		if end > totalRunes {
//			end = totalRunes
//		}
//
//		chunkRunes := runes[start:end]
//		chunkStr := string(chunkRunes)
//
//		// 检查是否需要Base64编码
//		isBase64 := forceBase64 || !isPureASCII(chunkStr)
//		var chunkContent string
//
//		if isBase64 {
//			chunkContent = base64.StdEncoding.EncodeToString([]byte(chunkStr))
//		} else {
//			chunkContent = chunkStr
//		}
//
//		chunks = append(chunks, QRChunk{
//			Index:    i + 1,
//			Total:    totalChunks,
//			Content:  chunkContent,
//			IsBase64: isBase64,
//		})
//	}
//
//	return chunks
//}
//
//// 分割二进制文件 (转为Base64后分割)
//func splitBinaryToChunks(content []byte, chunkSize int) []QRChunk {
//	// 先将整个文件转为Base64
//	base64Str := base64.StdEncoding.EncodeToString(content)
//
//	// Base64字符串分割 (注意: Base64字符是ASCII，可直接按长度分割)
//	totalLen := len(base64Str)
//	charsPerChunk := chunkSize * 4 / 3 // Base64编码后大约增加33%
//
//	totalChunks := (totalLen + charsPerChunk - 1) / charsPerChunk
//	var chunks []QRChunk
//
//	for i := 0; i < totalChunks; i++ {
//		start := i * charsPerChunk
//		end := start + charsPerChunk
//		if end > totalLen {
//			end = totalLen
//		}
//
//		chunkContent := base64Str[start:end]
//
//		chunks = append(chunks, QRChunk{
//			Index:    i + 1,
//			Total:    totalChunks,
//			Content:  chunkContent,
//			IsBase64: true, // 二进制文件始终是Base64
//		})
//	}
//
//	return chunks
//}
//
//// 检查是否为纯ASCII
//func isPureASCII(s string) bool {
//	for i := 0; i < len(s); i++ {
//		if s[i] > 127 {
//			return false
//		}
//	}
//	return true
//}
//
//// 为每个分片生成二维码图片
//func generateQRCodeForChunks(chunks []QRChunk) []QRChunk {
//	var qrChunks []QRChunk
//
//	for _, chunk := range chunks {
//		// 构建分片信息字符串
//		info := fmt.Sprintf("PART %d/%d\n", chunk.Index, chunk.Total)
//		if chunk.IsBase64 {
//			info += "[BASE64]\n"
//		}
//		info += chunk.Content
//
//		// 生成二维码
//		qr, err := qrcode.New(info, qrcode.Medium)
//		if err != nil {
//			// 如果出错，继续处理下一个
//			continue
//		}
//
//		// 获取二维码PNG字节
//		pngBytes, err := qr.PNG(256)
//		if err != nil {
//			continue
//		}
//
//		// 转为Base64字符串用于前端显示
//		qrBase64 := base64.StdEncoding.EncodeToString(pngBytes)
//
//		newChunk := chunk
//		newChunk.QRCodePNG = "data:image/png;base64," + qrBase64
//		qrChunks = append(qrChunks, newChunk)
//	}
//
//	return qrChunks
//}
//
//// 格式化文件大小显示
//func formatFileSize(size int) string {
//	if size < 1024 {
//		return fmt.Sprintf("%d B", size)
//	} else if size < 1024*1024 {
//		return fmt.Sprintf("%.1f KB", float64(size)/1024)
//	}
//	return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
//}
