package views

import (
	. "SimpleHttpServer/config"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"net/http"
)

// 登录处理
func LoginHandler(c *gin.Context) {
	if c.Request.Method == "GET" {
		// 检查是否已登录
		session := sessions.Default(c)
		user := session.Get("user")
		if user != nil {
			c.Redirect(http.StatusFound, "/")
			return
		}
		c.HTML(http.StatusOK, "login.html", gin.H{})
		return
	}

	// POST请求处理登录
	username := c.PostForm("username")
	password := c.PostForm("password")

	if username == GlobalConfig.UserName && password == GlobalConfig.Password {
		session := sessions.Default(c)
		session.Set("user", username)
		session.Save()
		c.Redirect(http.StatusFound, "/")
	} else {
		c.HTML(http.StatusOK, "login.html", gin.H{
			"error": "用户名或密码错误",
		})
	}
}

// 登出处理
func LogoutHandler(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()
	c.Redirect(http.StatusFound, "/login")
}
