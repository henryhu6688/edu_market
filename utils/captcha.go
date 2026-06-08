package utils

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"edu_market/config"
	"edu_market/database"
)

// CodeStore 验证码存储器（Redis 版）
type CodeStore struct {
	codeLen  int
	ttl      time.Duration
	interval time.Duration
}

// NewCodeStore 创建验证码存储器
func NewCodeStore(codeLen int, ttlSeconds, intervalSeconds int) *CodeStore {
	return &CodeStore{
		codeLen:  codeLen,
		ttl:      time.Duration(ttlSeconds) * time.Second,
		interval: time.Duration(intervalSeconds) * time.Second,
	}
}

// codeKey 验证码 Redis key
func (s *CodeStore) codeKey(phone string) string {
	return fmt.Sprintf("captcha:code:%s", phone)
}

// limitKey 发送频率限制 Redis key
func (s *CodeStore) limitKey(phone string) string {
	return fmt.Sprintf("captcha:limit:%s", phone)
}

// Generate 生成验证码并存入 Redis
func (s *CodeStore) Generate(phone string) (string, error) {
	if database.RDB == nil {
		return "", fmt.Errorf("Redis 未连接")
	}

	ctx := context.Background()

	// 检查发送间隔
	exists, err := database.RDB.Exists(ctx, s.limitKey(phone)).Result()
	if err != nil {
		return "", fmt.Errorf("Redis错误: %v", err)
	}
	if exists > 0 {
		return "", fmt.Errorf("验证码发送过于频繁，请%d秒后再试", int(s.interval.Seconds()))
	}

	// 生成随机验证码
	code := s.randomCode()

	// 存入 Redis（带 TTL）
	if err := database.RDB.Set(ctx, s.codeKey(phone), code, s.ttl).Err(); err != nil {
		return "", fmt.Errorf("验证码存储失败: %v", err)
	}

	// 设置频率限制 key
	if err := database.RDB.Set(ctx, s.limitKey(phone), "1", s.interval).Err(); err != nil {
		return "", fmt.Errorf("频率限制设置失败: %v", err)
	}

	log.Printf("[验证码] 手机号 %s 的验证码: %s (有效期 %v)", phone, code, s.ttl)
	return code, nil
}

// Verify 校验验证码，匹配后删除（一次性）
func (s *CodeStore) Verify(phone, code string) bool {
	if database.RDB == nil {
		return false
	}

	ctx := context.Background()

	// 从 Redis 获取验证码
	stored, err := database.RDB.Get(ctx, s.codeKey(phone)).Result()
	if err != nil {
		// key 不存在或已过期
		return false
	}

	if stored != code {
		return false
	}

	// 验证成功，删除验证码（一次性使用）
	database.RDB.Del(ctx, s.codeKey(phone))
	// 同时删除限频 key，允许立即再次发送（用于登录等场景）
	database.RDB.Del(ctx, s.limitKey(phone))
	return true
}

// randomCode 生成指定位数的随机数字验证码
func (s *CodeStore) randomCode() string {
	format := fmt.Sprintf("%%0%dd", s.codeLen)
	return fmt.Sprintf(format, rand.Intn(pow10(s.codeLen)))
}

// pow10 返回 10^n
func pow10(n int) int {
	v := 1
	for i := 0; i < n; i++ {
		v *= 10
	}
	return v
}

// CaptchaStore 全局验证码存储器实例
var CaptchaStore *CodeStore

// InitCaptcha 初始化验证码存储器（依赖 Redis 已初始化）
func InitCaptcha() {
	cfg := config.App.Captcha
	if cfg.Length == 0 {
		cfg.Length = 6
	}
	if cfg.ExpireSeconds == 0 {
		cfg.ExpireSeconds = 300
	}
	if cfg.ResendSeconds == 0 {
		cfg.ResendSeconds = 60
	}
	CaptchaStore = NewCodeStore(cfg.Length, cfg.ExpireSeconds, cfg.ResendSeconds)
	log.Printf("验证码存储器初始化完成 (长度:%d 有效期:%ds 间隔:%ds)", cfg.Length, cfg.ExpireSeconds, cfg.ResendSeconds)
}
