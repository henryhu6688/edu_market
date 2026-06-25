package utils

import (
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"edu_market/config"
)

// dailyWriter 按天滚动的 io.Writer（每天一个日志文件，保留最近 N 天）
type dailyWriter struct {
	mu       sync.Mutex
	dir      string
	prefix   string
	maxAge   int           // 保留天数
	curDate  string        // 当前日期 "2006-01-02"
	curFile  *os.File      // 当天文件句柄
}

func newDailyWriter(dir, prefix string, maxAge int) *dailyWriter {
	return &dailyWriter{dir: dir, prefix: prefix, maxAge: maxAge}
}

func (w *dailyWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if w.curDate != today {
		// 切换日期：关旧文件、开新文件
		if w.curFile != nil {
			w.curFile.Close()
		}
		os.MkdirAll(w.dir, 0755)
		f, err := os.OpenFile(
			filepath.Join(w.dir, fmt.Sprintf("%s-%s.log", w.prefix, today)),
			os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644,
		)
		if err != nil {
			return 0, err
		}
		w.curFile = f
		w.curDate = today

		// 清理过期日志
		w.cleanOld()
	}
	return w.curFile.Write(p)
}

func (w *dailyWriter) cleanOld() {
	cutoff := time.Now().Add(-time.Duration(w.maxAge) * 24 * time.Hour)
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		// 匹配 prefix-YYYY-MM-DD.log 格式
		name := e.Name()
		if len(name) != len(w.prefix)+1+10+4 || name[:len(w.prefix)] != w.prefix {
			continue
		}
		datePart := name[len(w.prefix)+1 : len(w.prefix)+1+10]
		t, err := time.Parse("2006-01-02", datePart)
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			os.Remove(filepath.Join(w.dir, name))
		}
	}
}

// InitLogger 初始化结构化日志（控制台 + 按天滚动文件）
func InitLogger() {
	logDir := "logs"
	os.MkdirAll(logDir, 0755)

	// 按天滚动的文件 writer（保留 7 天）
	fileWriter := newDailyWriter(logDir, "app", 7)

	// 开发模式：控制台 + 文件；生产模式：JSON 只写文件
	var handler slog.Handler
	if config.App.Server.Mode == "release" {
		handler = slog.NewJSONHandler(fileWriter, &slog.HandlerOptions{
			Level:     slog.LevelInfo,
			AddSource: true,
		})
	} else {
		multi := io.MultiWriter(os.Stdout, fileWriter)
		handler = slog.NewTextHandler(multi, &slog.HandlerOptions{
			Level:     slog.LevelDebug,
			AddSource: true,
		})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	// 标准库 log 也重定向到文件
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
