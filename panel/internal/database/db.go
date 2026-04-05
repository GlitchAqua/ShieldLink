package database

import (
	"log"

	"shieldlink-panel/internal/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"github.com/glebarez/sqlite"
)

var DB *gorm.DB

func Init(dbPath string) {
	var err error
	DB, err = gorm.Open(sqlite.Open(dbPath+"?_journal_mode=WAL&_busy_timeout=5000"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("Failed to connect database: %v", err)
	}

	DB.AutoMigrate(
		&model.User{},
		&model.Upstream{},
		&model.DecryptServer{},
		&model.MergeServer{},
		&model.Route{},
		&model.DecorationRule{},
	)

	ensureAdmin(DB)
}

func ensureAdmin(db *gorm.DB) {
	var count int64
	db.Model(&model.User{}).Count(&count)
	if count > 0 {
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	db.Create(&model.User{
		Username: "admin",
		Password: string(hash),
		Role:     "admin",
		Enabled:  true,
	})
	log.Println("Created default admin user (admin / admin123)")
}
