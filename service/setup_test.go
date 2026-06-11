package service

import (
	"context"
	"fmt"
	"log"
	"testing"

	"edu_market/config"
	"edu_market/database"
	"edu_market/model"
	"edu_market/utils"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const testDBName = "edu_market_test"

// createTestDB 创建测试专用数据库（如不存在）
func createTestDB() {
	cfg := config.App.Database
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/?charset=%s&parseTime=True&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Charset)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("连接 MySQL 失败: %v", err)
	}

	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	if err := db.Exec("CREATE DATABASE IF NOT EXISTS " + testDBName +
		" DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci").Error; err != nil {
		log.Fatalf("创建测试数据库失败: %v", err)
	}
	log.Printf("测试数据库 %s 就绪", testDBName)
}

// cleanAllTestData 清空测试库所有数据（按外键顺序删除）
func cleanAllTestData() {
	database.DB.Exec("SET FOREIGN_KEY_CHECKS = 0")
	database.DB.Where("1=1").Delete(&model.Review{})
	database.DB.Where("1=1").Delete(&model.Order{})
	database.DB.Where("1=1").Delete(&model.Message{})
	database.DB.Where("1=1").Delete(&model.Session{})
	database.DB.Where("1=1").Delete(&model.DocumentChunk{})
	database.DB.Where("1=1").Delete(&model.Document{})
	database.DB.Where("1=1").Delete(&model.Material{})
	database.DB.Where("1=1").Delete(&model.Course{})
	database.DB.Where("1=1").Delete(&model.Category{})
	database.DB.Where("1=1").Delete(&model.User{})
	database.DB.Exec("SET FOREIGN_KEY_CHECKS = 1")
}

// TestMain 初始化测试环境
func TestMain(m *testing.M) {
	// 手动加载配置
	config.App = &config.Config{
		Server: config.ServerConfig{Port: 8080, Mode: "test"},
		Database: config.DatabaseConfig{
			Host: "127.0.0.1", Port: 3306, User: "root", Password: "123456",
			DBName: testDBName, Charset: "utf8mb4", MaxIdleConns: 5, MaxOpenConns: 10,
		},
		Redis: config.RedisConfig{
			Addr: "127.0.0.1:6379", Password: "", DB: 2,
		},
		JWT: config.JWTConfig{Secret: "test-secret-key", AccessTTLMin: 30, RefreshTTLHours: 24},
		Captcha: config.CaptchaConfig{Length: 6, ExpireSeconds: 300, ResendSeconds: 1},
		Agent:   config.AgentConfig{MaxToolRounds: 7, ContextMaxMsg: 20, ChunkSize: 500, ChunkOverlap: 50},
		Document: config.DocumentConfig{AutoSaveDelay: 2, RagSync: true, MaxUploadSize: 20 << 20, AllowedFormats: []string{".pdf", ".pptx", ".docx", ".md", ".txt"}},
	}

	// 初始化 Redis（测试环境不可用时继续运行，captcha 相关测试会处理）
	if err := database.InitRedis(); err != nil {
		log.Printf("警告: Redis 未连接 (%v)，验证码相关测试将跳过", err)
	}

	// 初始化验证码
	utils.InitCaptcha()

	// 创建测试数据库
	createTestDB()

	// 初始化数据库（连接测试库 + AutoMigrate）
	database.Init()

	// 清理上次残留（防御性）
	cleanAllTestData()
	log.Println("测试环境初始化完成")

	// 运行所有测试
	code := m.Run()

	// 清空测试数据库
	cleanAllTestData()
	log.Println("测试数据已清空")

	// 归还 Redis 测试库（如有）
	if database.RDB != nil {
		database.RDB.FlushDB(context.Background()).Err()
	}

	if code != 0 {
		log.Printf("测试失败，退出码: %d", code)
	}
}
