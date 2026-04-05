package router

import (
	"net/http"
	"strings"

	"shieldlink-panel/internal/database"
	"shieldlink-panel/internal/handler"
	"shieldlink-panel/internal/middleware"
	"shieldlink-panel/internal/model"

	"github.com/gin-gonic/gin"
)

func Setup(r *gin.Engine) {
	r.Use(middleware.CORS())

	api := r.Group("/api/v1")

	// Public
	api.POST("/auth/login", handler.Login)

	// Authenticated
	auth := api.Group("", middleware.JWTAuth())
	{
		auth.GET("/auth/me", handler.GetMe)
		auth.PUT("/auth/password", handler.ChangePassword)

		auth.GET("/dashboard", handler.Dashboard)

		// Upstreams
		auth.GET("/upstreams", handler.ListUpstreams)
		auth.POST("/upstreams", handler.CreateUpstream)
		auth.PUT("/upstreams/:id", handler.UpdateUpstream)
		auth.DELETE("/upstreams/:id", handler.DeleteUpstream)

		// Decrypt servers
		auth.GET("/servers", handler.ListDecryptServers)
		auth.POST("/servers", handler.CreateDecryptServer)
		auth.PUT("/servers/:id", handler.UpdateDecryptServer)
		auth.DELETE("/servers/:id", handler.DeleteDecryptServer)
		auth.POST("/servers/:id/sync", handler.SyncDecryptServer)
		auth.GET("/servers/:id/status", handler.GetDecryptServerStatus)
		auth.POST("/servers/:id/check-install", handler.CheckInstallDecryptServer)
		auth.POST("/servers/check-install-all", handler.CheckInstallAllDecryptServers)

		// Merge servers
		auth.GET("/merge-servers", handler.ListMergeServers)
		auth.POST("/merge-servers", handler.CreateMergeServer)
		auth.PUT("/merge-servers/:id", handler.UpdateMergeServer)
		auth.DELETE("/merge-servers/:id", handler.DeleteMergeServer)
		auth.POST("/merge-servers/:id/check-install", handler.CheckInstallMergeServer)
		auth.POST("/merge-servers/check-install-all", handler.CheckInstallAllMergeServers)

		// Routes
		auth.GET("/routes", handler.ListRoutes)
		auth.POST("/routes", handler.CreateRoute)
		auth.PUT("/routes/:id", handler.UpdateRoute)
		auth.DELETE("/routes/:id", handler.DeleteRoute)
		auth.POST("/routes/sync-all", handler.SyncAllRoutes)

		// Decoration rules
		auth.GET("/rules", handler.ListRules)
		auth.POST("/rules", handler.CreateRule)
		auth.PUT("/rules/:id", handler.UpdateRule)
		auth.DELETE("/rules/:id", handler.DeleteRule)
	}

	// Serve frontend
	r.Static("/assets", "./web/dist/assets")
	r.StaticFile("/favicon.ico", "./web/dist/favicon.ico")

	// SPA fallback + subscription proxy
	r.NoRoute(func(c *gin.Context) {
		// Check if this request's Host matches an upstream domain FIRST
		// (subscription paths like /api/v1/client/subscribe must not be caught by panel API 404)
		host := c.Request.Host
		if idx := strings.LastIndex(host, ":"); idx != -1 {
			host = host[:idx]
		}
		var count int64
		database.DB.Model(&model.Upstream{}).Where("domain = ? AND enabled = ?", host, true).Count(&count)
		if count > 0 {
			handler.HandleSubscribe(c)
			return
		}

		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		// SPA fallback
		c.File("./web/dist/index.html")
	})
}
