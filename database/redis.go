package database

import (
	"context"
	"log"
	"time"

	"edu_market/config"

	"github.com/redis/go-redis/v9"
)

// RDB 全局 Redis 客户端
var RDB *redis.Client

// InitRedis 初始化 Redis 连接（返回错误由调用方决定是否 fatal）
func InitRedis() error {
	cfg := config.App.Redis
	RDB = redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := RDB.Ping(ctx).Err(); err != nil {
		RDB = nil
		return err
	}

	log.Printf("Redis 连接成功 (%s, DB:%d)", cfg.Addr, cfg.DB)
	return nil
}
