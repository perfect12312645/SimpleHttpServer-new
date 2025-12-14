package serverRouter

import (
	"SimpleHttpServer/middleware"
	"SimpleHttpServer/views"
	"github.com/gin-gonic/gin"
)

func RouterInit(r *gin.Engine) {
	// ========== 1. 公开路由分组（不需要登录） ==========
	public := r.Group("/")
	{
		// 登录接口（只有这个接口不用登录）
		public.GET("/login", views.LoginHandler) // 你的登录处理函数（需要自己实现）
		public.POST("/login", views.LoginHandler)
		public.GET("/download/*path", views.DownloadHandler) // 文件下载

		// 静态资源（比如前端页面、css/js，不需要登录）
		public.Static("/static", "./static")
	}
	// ========== 2. 受保护路由分组（必须登录） ==========
	protected := r.Group("/")
	protected.Use(middleware.AuthRequired()) // 加登录认证「安检门」
	{
		// 你的核心业务接口（全部需要登录）
		protected.GET("/", views.IndexHandler)                           // 首页
		protected.POST("/upload", views.UploadHandler)                   // 根目录上传
		protected.POST("/upload/*path", views.UploadHandler)             // 子目录上传
		protected.GET("/get_resume_info", views.ResumeInfoHandler)       // 根目录续传
		protected.GET("/get_resume_info/*path", views.ResumeInfoHandler) // 子目录续传
		protected.DELETE("/delete/*path", views.DeleteHandler)           // 文件删除
		protected.GET("/explore/*path", views.ExploreDir)                // 目录浏览
		protected.GET("/preview/*path", views.PreviewFile)               // 文件预览
		protected.GET("/qrcode/*path", views.HandleFileToQR)             // 生成二维码

		// 登出接口（必须登录后才能登出）
		protected.GET("/logout", views.LogoutHandler) // 你的登出处理函数（需要自己实现）
	}
}
