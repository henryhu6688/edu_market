package service

import (
	"log"
	"testing"

	"edu_market/config"
	"edu_market/database"
	"edu_market/utils"
)

// TestMain 初始化测试环境（数据库 + 验证码 + JWT配置）
func TestMain(m *testing.M) {
	// 手动加载配置（测试从 service/ 目录运行，无法走 config.Load 的文件路径）
	config.App = &config.Config{
		Server: config.ServerConfig{Port: 8080, Mode: "test"},
		Database: config.DatabaseConfig{
			Host: "127.0.0.1", Port: 3306, User: "root", Password: "123456",
			DBName: "edu_market", Charset: "utf8mb4", MaxIdleConns: 5, MaxOpenConns: 10,
		},
		Redis: config.RedisConfig{
			Addr: "127.0.0.1:6379", Password: "", DB: 2,
		},
		JWT: config.JWTConfig{Secret: "test-secret-key", ExpireHours: 24},
		Captcha: config.CaptchaConfig{Length: 6, ExpireSeconds: 300, ResendSeconds: 1},
	}

	// 初始化 Redis
	database.InitRedis()

	// 初始化验证码
	utils.InitCaptcha()

	// 初始化数据库
	database.Init()

	log.Println("测试环境初始化完成")
	m.Run()
}
