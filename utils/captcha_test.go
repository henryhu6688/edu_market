package utils

import (
	"testing"
	"time"

	"edu_market/config"
	"edu_market/database"
)

// setupCaptchaTest 初始化测试环境（Redis 连接 + 验证码存储器）
func setupCaptchaTest(t *testing.T) {
	if database.RDB == nil {
		config.App = &config.Config{}
		config.App.Redis = config.RedisConfig{
			Addr:     "127.0.0.1:6379",
			Password: "",
			DB:       1,
		}
		config.App.Captcha = config.CaptchaConfig{
			Length:        6,
			ExpireSeconds: 300,
			ResendSeconds: 1,
		}
		if err := database.InitRedis(); err != nil {
			t.Fatalf("Redis 连接失败: %v", err)
		}
		InitCaptcha()
	}
	// 清理测试数据
	database.RDB.FlushDB(t.Context()).Err()
}

// newTestStore 创建测试用验证码存储器（间隔1秒，有效期3秒）
func newTestStore() *CodeStore {
	return NewCodeStore(6, 3, 1)
}

// TestRedisGenerateCode 测试验证码生成并存入 Redis
func TestRedisGenerateCode(t *testing.T) {
	setupCaptchaTest(t)

	store := newTestStore()
	code, err := store.Generate("13800001111")
	if err != nil {
		t.Fatalf("生成验证码失败: %v", err)
	}
	if len(code) != 6 {
		t.Errorf("验证码长度应为6，实际: %d", len(code))
	}
	t.Logf("生成验证码: %s", code)

	// 验证 Redis 中确实存储了
	ctx := t.Context()
	stored, err := database.RDB.Get(ctx, store.codeKey("13800001111")).Result()
	if err != nil {
		t.Fatalf("Redis 中未找到验证码: %v", err)
	}
	if stored != code {
		t.Errorf("Redis 中验证码不匹配，期望: %s，实际: %s", code, stored)
	}
}

// TestRedisVerifyCorrectCode 测试正确验证码校验
func TestRedisVerifyCorrectCode(t *testing.T) {
	setupCaptchaTest(t)

	store := newTestStore()
	code, _ := store.Generate("13800002222")
	if !store.Verify("13800002222", code) {
		t.Error("正确验证码应该校验通过")
	}
}

// TestRedisVerifyWrongCode 测试错误验证码校验
func TestRedisVerifyWrongCode(t *testing.T) {
	setupCaptchaTest(t)

	store := newTestStore()
	store.Generate("13800003333")
	if store.Verify("13800003333", "000000") {
		t.Error("错误验证码不应该校验通过")
	}
}

// TestRedisVerifyOneTime 测试验证码一次性（校验后从 Redis 删除）
func TestRedisVerifyOneTime(t *testing.T) {
	setupCaptchaTest(t)

	store := newTestStore()
	code, _ := store.Generate("13800004444")

	// 第一次校验成功
	if !store.Verify("13800004444", code) {
		t.Fatal("第一次校验应该成功")
	}

	// 第二次用同样的码校验应失败（Redis 中已删除）
	if store.Verify("13800004444", code) {
		t.Error("验证码已被删除，第二次校验应该失败")
	}

	// 确认 Redis 中 key 已被删除
	ctx := t.Context()
	exists, _ := database.RDB.Exists(ctx, store.codeKey("13800004444")).Result()
	if exists > 0 {
		t.Error("验证码 key 应从 Redis 中删除")
	}
}

// TestRedisVerifyExpired 测试验证码过期（Redis TTL）
func TestRedisVerifyExpired(t *testing.T) {
	setupCaptchaTest(t)

	store := NewCodeStore(6, 1, 1) // 1秒过期

	code, _ := store.Generate("13800005555")
	t.Logf("验证码: %s, 等待过期...", code)

	time.Sleep(2 * time.Second) // 等 Redis TTL 过期

	if store.Verify("13800005555", code) {
		t.Error("过期验证码不应该校验通过")
	}
}

// TestRedisRateLimit 测试发送频率限制
func TestRedisRateLimit(t *testing.T) {
	setupCaptchaTest(t)

	store := NewCodeStore(6, 10, 2) // 2秒间隔

	// 第一次发送成功
	_, err := store.Generate("13800006666")
	if err != nil {
		t.Fatalf("第一次发送失败: %v", err)
	}

	// 立即第二次发送应被限频
	_, err = store.Generate("13800006666")
	if err == nil {
		t.Error("短时间重复发送应该返回错误")
	}
	t.Logf("限频错误信息: %v", err)

	// 等间隔过后可以再发
	time.Sleep(3 * time.Second)
	_, err = store.Generate("13800006666")
	if err != nil {
		t.Errorf("间隔过后应该可以发送，但失败: %v", err)
	}
}

// TestRedisGenerateRandomness 测试验证码随机性
func TestRedisGenerateRandomness(t *testing.T) {
	setupCaptchaTest(t)

	store := newTestStore()
	codes := make(map[string]bool)
	phones := []string{
		"13800000001", "13800000002", "13800000003", "13800000004", "13800000005",
		"13800000006", "13800000007", "13800000008", "13800000009", "13800000010",
		"13800000011", "13800000012", "13800000013", "13800000014", "13800000015",
		"13800000016", "13800000017", "13800000018", "13800000019", "13800000020",
	}
	for _, phone := range phones {
		code, _ := store.Generate(phone)
		codes[code] = true
	}
	if len(codes) < 10 {
		t.Errorf("验证码随机性不足，20次仅生成 %d 个不同验证码", len(codes))
	}
	t.Logf("20次生成产生了 %d 个不同验证码", len(codes))
}

// TestRedisVerifyWrongPhone 测试手机号不匹配
func TestRedisVerifyWrongPhone(t *testing.T) {
	setupCaptchaTest(t)

	store := newTestStore()
	code, _ := store.Generate("13800007777")
	if store.Verify("13800008888", code) {
		t.Error("不同手机号的验证码不应该校验通过")
	}
}

// TestRedisTTLAccuracy 测试 Redis TTL 设置正确
func TestRedisTTLAccuracy(t *testing.T) {
	setupCaptchaTest(t)

	store := NewCodeStore(6, 120, 30) // 2分钟过期
	store.Generate("13800009999")

	ctx := t.Context()
	ttl, err := database.RDB.TTL(ctx, store.codeKey("13800009999")).Result()
	if err != nil {
		t.Fatalf("获取 TTL 失败: %v", err)
	}
	if ttl < 119*time.Second || ttl > 121*time.Second {
		t.Errorf("TTL 应在 120s 左右，实际: %v", ttl)
	}
	t.Logf("Redis key TTL: %v", ttl)
}
