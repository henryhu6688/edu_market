package utils

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"edu_market/config"
	"edu_market/database"

	"github.com/mojocn/base64Captcha"
)

// CodeStore 短信验证码存储器（Redis 版）
type CodeStore struct {
	codeLen  int
	ttl      time.Duration
	interval time.Duration
}

// NewCodeStore 创建短信验证码存储器
func NewCodeStore(codeLen int, ttlSeconds, intervalSeconds int) *CodeStore {
	return &CodeStore{
		codeLen:  codeLen,
		ttl:      time.Duration(ttlSeconds) * time.Second,
		interval: time.Duration(intervalSeconds) * time.Second,
	}
}

func (s *CodeStore) codeKey(phone string) string  { return fmt.Sprintf("captcha:sms:%s", phone) }
func (s *CodeStore) limitKey(phone string) string  { return fmt.Sprintf("captcha:smslimit:%s", phone) }

// Generate 生成短信验证码并存入 Redis
func (s *CodeStore) Generate(phone string) (string, error) {
	if database.RDB == nil {
		return "", fmt.Errorf("Redis 未连接")
	}
	ctx := context.Background()

	exists, err := database.RDB.Exists(ctx, s.limitKey(phone)).Result()
	if err != nil {
		return "", fmt.Errorf("Redis错误: %v", err)
	}
	if exists > 0 {
		return "", fmt.Errorf("验证码发送过于频繁，请%d秒后再试", int(s.interval.Seconds()))
	}

	code := s.randomCode()
	if err := database.RDB.Set(ctx, s.codeKey(phone), code, s.ttl).Err(); err != nil {
		return "", fmt.Errorf("验证码存储失败: %v", err)
	}
	if err := database.RDB.Set(ctx, s.limitKey(phone), "1", s.interval).Err(); err != nil {
		return "", fmt.Errorf("频率限制设置失败: %v", err)
	}

	log.Printf("[短信验证码] 手机号 %s 的验证码: %s (有效期 %v)", phone, code, s.ttl)
	return code, nil
}

// Verify 校验短信验证码，成功后删除
func (s *CodeStore) Verify(phone, code string) bool {
	if database.RDB == nil {
		return false
	}
	ctx := context.Background()

	stored, err := database.RDB.Get(ctx, s.codeKey(phone)).Result()
	if err != nil {
		return false
	}
	if stored != code {
		return false
	}
	database.RDB.Del(ctx, s.codeKey(phone))
	database.RDB.Del(ctx, s.limitKey(phone))
	return true
}

func (s *CodeStore) randomCode() string {
	format := fmt.Sprintf("%%0%dd", s.codeLen)
	return fmt.Sprintf(format, rand.Intn(pow10(s.codeLen)))
}

func pow10(n int) int {
	v := 1
	for i := 0; i < n; i++ {
		v *= 10
	}
	return v
}

// ==================== 图形验证码 ====================

const imgCaptchaPrefix = "captcha:img:"
const imgCaptchaTTL = 2 * time.Minute // 图形码 2 分钟过期

var imgCaptchaStore = base64Captcha.DefaultMemStore

// GenerateImageCaptcha 生成图形验证码，返回 base64 图片 + captcha_id
func GenerateImageCaptcha() (captchaID string, b64s string, err error) {
	imgCaptchaStore = base64Captcha.NewMemoryStore(100, imgCaptchaTTL)
	driver := base64Captcha.NewDriverString(36, 120, 0,
		base64Captcha.OptionShowSlimeLine|base64Captcha.OptionShowSineLine,
		4, "123456789abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ",
		nil, nil, nil)
	c := base64Captcha.NewCaptcha(driver, imgCaptchaStore)
	id, b64s, _, err := c.Generate()
	if err != nil {
		return "", "", err
	}
	return id, b64s, nil
}

// VerifyImageCaptcha 校验图形验证码，一次性
func VerifyImageCaptcha(id, code string) bool {
	if id == "" || code == "" {
		return false
	}
	return imgCaptchaStore.Verify(id, code, true)
}

// ==================== 全局实例 ====================

var CaptchaStore *CodeStore

// InitCaptcha 初始化验证码存储器
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
