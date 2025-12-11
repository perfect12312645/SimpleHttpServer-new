package middleware

import (
	"gopkg.in/natefinch/lumberjack.v2"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	loggerOnce sync.Once // 单例初始化锁
	Logger     *zap.Logger
)

// 初始化Zap日志（支持输出到文件和控制台，按大小切割日志）
func InitZapLogger() {
	loggerOnce.Do(func() {
		// 日志文件配置（按大小切割，保留30天）
		fileWriter := &lumberjack.Logger{
			Filename:   "./logs/http-server.log", // 日志文件路径
			MaxSize:    100,                      // 单个文件最大100MB
			MaxBackups: 30,                       // 最多保留30个备份
			MaxAge:     30,                       // 保留30天
			Compress:   true,                     // 压缩旧日志
		}
		// 将fileWriter包装为zap可识别的WriteSyncer
		fileSyncer := zapcore.AddSync(fileWriter)
		//// 控制台输出
		consoleSyncer := zapcore.AddSync(os.Stdout)
		// 定义日志级别（生产环境用Info，开发环境用Debug）
		atomicLevel := zap.NewAtomicLevel()
		if os.Getenv("ENV") == "dev" {
			atomicLevel.SetLevel(zap.DebugLevel)
		} else {
			atomicLevel.SetLevel(zap.InfoLevel)
		}

		encoderConfig := zapcore.EncoderConfig{
			TimeKey:        "time",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller", // 显示调用文件和行号
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.CapitalLevelEncoder, // 级别大写（INFO/ERROR）
			EncodeTime:     customTimeEncoder,           // 自定义时间格式
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder, // 短路径调用者（如pkg/server/api/views/template.go:23）
		}
		// 创建JSON编码器
		encoder := zapcore.NewJSONEncoder(encoderConfig)
		// 5. 构建Zap核心：绑定输出目标、级别、编码器
		core := zapcore.NewCore(
			encoder,
			zapcore.NewMultiWriteSyncer(fileSyncer, consoleSyncer), // 同时输出到文件和控制台
			atomicLevel,
		)
		// 6. 添加调用者信息和堆栈跟踪（开发环境用）
		logger := zap.New(core)
		if os.Getenv("ENV") == "dev" {
			logger = logger.WithOptions(zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel))
		} else {
			// 生产环境只在错误级别添加堆栈跟踪
			logger = logger.WithOptions(zap.AddCaller(), zap.AddStacktrace(zap.PanicLevel))
		}

		Logger = logger
	})
}

// 自定义时间格式（如 2023-10-01 15:04:05.000）
func customTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006-01-02 15:04:05.000"))
}
