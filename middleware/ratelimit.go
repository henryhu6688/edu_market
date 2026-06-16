package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"edu_market/database"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RateLimit Redis 滑动窗口限流中间件
func RateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := context.Background()
		userID := c.GetUint("user_id")
		ip := c.ClientIP()

		if userID > 0 {
			key := fmt.Sprintf("ratelimit:user:%d", userID)
			if !checkLimit(ctx, key, 30) {
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"code": 429, "message": "请求过于频繁"})
				return
			}
		}

		key := fmt.Sprintf("ratelimit:ip:%s", ip)
		if !checkLimit(ctx, key, 100) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"code": 429, "message": "请求过于频繁"})
			return
		}

		c.Next()
	}
}

func checkLimit(ctx context.Context, key string, limit int) bool {
	if database.RDB == nil {
		return true
	}
	now := time.Now().Unix()
	windowStart := now - 60

	pipe := database.RDB.Pipeline()
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprint(windowStart))
	pipe.ZCard(ctx, key)
	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return true
	}

	count, _ := cmds[1].(*redis.IntCmd).Result()
	if count >= int64(limit) {
		return false
	}

	database.RDB.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: fmt.Sprintf("%d", now)})
	database.RDB.Expire(ctx, key, 60*time.Second)
	return true
}
