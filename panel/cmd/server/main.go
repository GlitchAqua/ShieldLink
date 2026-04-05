package main

import (
	"log"
	"os"

	"shieldlink-panel/internal/database"
	"shieldlink-panel/internal/middleware"
	"shieldlink-panel/internal/router"
	"shieldlink-panel/internal/service"

	"github.com/gin-gonic/gin"
)

func main() {
	dbPath := envOr("DB_PATH", "./data/shieldlink_panel.db")
	jwtSecret := envOr("JWT_SECRET", "shieldlink-panel-secret-change-me")
	listen := envOr("LISTEN", ":8888")

	middleware.InitJWT(jwtSecret)
	database.Init(dbPath)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	router.Setup(r)

	service.StartCheckScheduler()

	log.Printf("ShieldLink Panel starting on %s", listen)
	log.Printf("Default admin credentials: admin / admin123")
	if err := r.Run(listen); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
