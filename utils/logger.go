package utils

import (
	"crypto/rand"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"

	"edu_market/config"

	"gopkg.in/natefinch/lumberjack.v2"
)

// InitLogger 初始化结构化日志（控制台 + 文件双写）
func InitLogger() {
	// 确保 logs 目录存在
	logDir := "logs"
	os.MkdirAll(logDir, 0755)

	// 文件滚动配置
	fileWriter := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, "app.log"),
		MaxSize:    10,   // 10MB 切分
		MaxBackups: 30,   // 保留 30 个旧文件
		MaxAge:     7,    // 保留 7 天
		Compress:   true, // gzip 压缩旧日志
	}

	// 开发模式：控制台彩色 + 文件；生产模式：JSON 写文件
	var handler slog.Handler
	if config.App.Server.Mode == "release" {
		// 生产：JSON 格式只写文件
		handler = slog.NewJSONHandler(fileWriter, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	} else {
		// 开发：文本格式同时输出到控制台和文件
		multi := io.MultiWriter(os.Stdout, fileWriter)
		handler = slog.NewTextHandler(multi, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	// 把标准库 log 也重定向到文件（兼容旧代码的 log.Printf）
	log.SetOutput(io.MultiWriter(os.Stdout, fileWriter))
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	slog.Info("日志系统初始化完成", "mode", config.App.Server.Mode, "logDir", logDir)
}

// NewRequestID 生成短请求 ID（8位随机字符串）
func NewRequestID() string {
	const chars = "0123456789abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, 8)
	rand.Read(b)
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b)
}
