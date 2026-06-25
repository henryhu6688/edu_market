package database

import (
	"log"
	"time"

	"edu_market/config"
	"edu_market/model"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB 全局数据库实例
var DB *gorm.DB

// InitLogLevel 初始化时的 GORM 日志级别（REPL/调测可设为 logger.Silent）
var InitLogLevel = logger.Info

// Init 初始化数据库连接并自动迁移
func Init() {
	dsn := config.App.Database.DSN()

	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(InitLogLevel),
	})
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		log.Fatalf("获取数据库实例失败: %v", err)
	}

	// 连接池配置
	sqlDB.SetMaxIdleConns(config.App.Database.MaxIdleConns)
	sqlDB.SetMaxOpenConns(config.App.Database.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// 自动迁移（开发环境）
	if err := autoMigrate(); err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}

	log.Println("数据库初始化成功")
}

// SilenceLog 关闭 GORM SQL 日志（REPL/调测用）
func SilenceLog() {
	DB.Logger = logger.Default.LogMode(logger.Silent)
}

// autoMigrate 自动迁移所有模型
func autoMigrate() error {
	return DB.AutoMigrate(
		&model.User{},
		&model.Category{},
		&model.Course{},
		&model.Order{},
		&model.Review{},
		&model.Session{},
		&model.Message{},
		&model.DocumentChunk{},
		&model.Material{},
		&model.Document{},
		&model.FAQ{},
		&model.UserMemory{},
	)
}
