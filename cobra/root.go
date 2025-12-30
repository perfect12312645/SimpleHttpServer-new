package cobra

import (
	. "SimpleHttpServer/config"
	. "SimpleHttpServer/middleware"
	"SimpleHttpServer/serverRouter"
	"SimpleHttpServer/utils"
	"fmt"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"html/template"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	maxFileSizeGB   int64
	chunkSizeMB     int64
	fileToORMaxZize int64
	username        string
	password        string
)

var rootCmd = &cobra.Command{
	Use:   "SimpleHttpServer",
	Short: "文件上传服务",
	Run: func(cmd *cobra.Command, args []string) {

		// 1. 计算实际文件大小（带日志输出结构化参数）
		GlobalConfig.MaxFileSize = maxFileSizeGB * 1024 * 1024 * 1024
		GlobalConfig.ChunkSize = chunkSizeMB * 1024 * 1024
		if fileToORMaxZize > 30*1024 || fileToORMaxZize <= 2048 {
			fmt.Println("强制限制最大转OR体积大于2KB，小于30KB!!!")
			os.Exit(1)
		}
		GlobalConfig.FileToORMaxZize = fileToORMaxZize
		GlobalConfig.UserName = username
		GlobalConfig.Password = password
		Logger.Info("配置参数解析完成",
			zap.Int64("port", GlobalConfig.Port),
			zap.Int64("max_file_size_gb", maxFileSizeGB),
			zap.Int64("max_file_size_bytes", GlobalConfig.MaxFileSize),
			zap.String("upload_dir", GlobalConfig.UploadDir),
			zap.Int64("chunk_size_mb", GlobalConfig.ChunkSize),
		)

		// 2. 创建上传目录（带错误日志）
		if err := os.MkdirAll(GlobalConfig.UploadDir, 0755); err != nil {
			Logger.Fatal("创建上传目录失败",
				zap.String("dir", GlobalConfig.UploadDir),
				zap.Error(err),
			)
			os.Exit(1)
		}
		Logger.Debug("上传目录创建/检查成功", zap.String("dir", GlobalConfig.UploadDir))

		// 3. 初始化Gin引擎（修复原代码混用r和router的问题）
		r := gin.New() // 改用gin.New()，手动添加必要中间件，避免Default()的默认日志
		setupSession(r)
		// 核心中间件：恢复panic + zap日志 + 日志格式化
		r.Use(
			gin.Recovery(), // 基础panic恢复（与RecoveryWithZap配合）
			ginzap.Ginzap(Logger, time.RFC3339, true), // 请求日志
			ginzap.RecoveryWithZap(Logger, true),      // panic恢复日志（带堆栈）
			//gin.Logger(),                              // 可选：保留gin默认访问日志（若需要）
		)

		// 4. 自定义模板函数（保留原有逻辑，补充日志）
		r.SetFuncMap(template.FuncMap{
			"datetimeformat": func(t time.Time) string {
				return t.Format("2006-01-02 15:04:05")
			},
			"getFileIconClass": func(filename string) string {
				parts := strings.Split(filename, ".")
				if len(parts) > 1 {
					ext := strings.ToLower(parts[len(parts)-1])
					if icon, ok := GlobalConfig.FileIconMap[ext]; ok {
						return icon
					}
				}
				return "fa-file-o"
			},
			"formatSize": utils.FormatSize,
			"add": func(a, b int) int { // 新增
				return a + b
			},
			"sub": func(a, b int) int { // 新增
				return a - b
			},
			"split":      strings.Split,
			"trimPrefix": strings.TrimPrefix,
			"replace": func(s, old, new string) string {
				// n=-1 表示替换所有匹配项（模板中替换//为/需要替换所有）
				return strings.Replace(s, old, new, -1)
			},
		})

		// 5. 加载模板 + 初始化路由（修复原代码路由未挂载的问题）
		r.LoadHTMLGlob("templates/*")
		serverRouter.RouterInit(r) // 路由挂载到实际启动的r引擎

		// 6. 启动前日志（结构化输出）
		serverAddr := fmt.Sprintf("http://0.0.0.0:%d", GlobalConfig.Port)
		Logger.Info("Gin服务启动中",
			zap.String("server_addr", serverAddr),
			zap.String("upload_dir", GlobalConfig.UploadDir),
			zap.Int64("max_file_size", maxFileSizeGB),
		)

		// 7. 启动服务（带错误日志）
		if err := r.Run(fmt.Sprintf(":%d", GlobalConfig.Port)); err != nil {
			Logger.Fatal("服务器启动失败", zap.Error(err))
		}
	},
}

// Execute 执行根命令（入口函数）
func Execute() {
	// 确保程序退出时刷新日志缓冲区
	defer func() {
		if err := Logger.Sync(); err != nil {
			fmt.Printf("日志缓冲区刷新失败: %v\n", err)
		}
	}()

	if err := rootCmd.Execute(); err != nil {
		Logger.Error("命令执行失败", zap.Error(err))
		os.Exit(1)
	}
}

// 初始化session中间件
func setupSession(router *gin.Engine) {
	store := cookie.NewStore([]byte("32-byte-secret-key-1234567890abcdef"))
	router.Use(sessions.Sessions("upload-session", store))
	store.Options(sessions.Options{
		Path:     "/",                  // Cookie作用于全站路径
		MaxAge:   86400 * 1,            // Cookie有效期7天（避免频繁登录）
		HttpOnly: true,                 // 防止XSS攻击（必须开启）
		SameSite: http.SameSiteLaxMode, // 兼容跨站请求（IP/域名都能携带Cookie）
		// 关键：不设置Domain，让浏览器自动适配请求的Host（IP/域名）
		// Domain: "",  // 留空，自动适配（127.0.0.1/192.168.1.100/your-domain.com）
		// Secure: false // 先设为false，后续通过中间件动态调整
	})
}

// init 初始化：先初始化日志，再定义命令行参数
func init() {
	// 1. 优先初始化zap日志（必须在所有日志输出前执行）
	var err error
	InitZapLogger() // 修正：接收InitZapLogger的返回值
	if err != nil {
		fmt.Printf("日志初始化失败: %v\n", err)
		os.Exit(1)
	}
	Logger.Debug("zap日志初始化完成")

	// 2. 定义全局持久化参数（所有子命令都可继承）
	rootCmd.PersistentFlags().Int64VarP(
		&GlobalConfig.Port,
		"port", "P",
		18181,
		"指定启动的端口，默认:18181",
	)
	rootCmd.PersistentFlags().Int64VarP(
		&maxFileSizeGB,
		"max-size", "M",
		20,
		"可上传的最大文件大小(GB)，默认:20",
	)
	rootCmd.PersistentFlags().StringVarP(
		&GlobalConfig.UploadDir,
		"dir", "d",
		"uploads",
		"上传目录名，默认:uploads",
	)
	rootCmd.PersistentFlags().Int64VarP(
		&chunkSizeMB,
		"chunk", "c",
		5,
		"分块大小(MB)，默认:5",
	)
	rootCmd.PersistentFlags().Int64VarP(
		&fileToORMaxZize,
		"--or-size", "o",
		10*1024,
		"可以转换为二维码的最大文件，不建议增大，大文件识别困难（单位：B），默认:10KB",
	)
	rootCmd.PersistentFlags().StringVarP(
		&username,
		"username", "u",
		"admin",
		"默认登陆用户名",
	)
	rootCmd.PersistentFlags().StringVarP(
		&password,
		"password", "p",
		"admin@123",
		"默认登陆密码",
	)

}
