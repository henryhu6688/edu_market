package middleware

import (
	"log/slog"
	"time"

	"edu_market/utils"

	"github.com/gin-gonic/gin"
)

// Logger 请求日志中间件（生成 request_id 贯穿全链路）
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := utils.NewRequestID()
		c.Set("request_id", requestID)

		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		slog.Info("请求开始", "request_id", requestID, "method", method, "path", path)

		c.Next()

		duration := time.Since(start)
		status := c.Writer.Status()

		slog.Info("请求结束",
			"request_id", requestID,
			"method", method,
			"path", path,
			"status", status,
			"duration_ms", duration.Milliseconds(),
		)
	}
}
