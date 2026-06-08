package utils

import (
	"testing"
	"time"

	"edu_market/config"
)

// setupJWTTest 初始化测试用 JWT 配置
func setupJWTTest() {
	if config.App == nil {
		config.App = &config.Config{}
	}
	config.App.JWT.Secret = "test-secret-key"
	config.App.JWT.ExpireHours = 24
}

// TestGenerateToken 测试Token生成
func TestGenerateToken(t *testing.T) {
	setupJWTTest()

	token, err := GenerateToken(1, "testuser", "student")
	if err != nil {
		t.Fatalf("生成Token失败: %v", err)
	}
	if token == "" {
		t.Error("Token不应为空")
	}
	t.Logf("生成Token: %s", token)
}

// TestParseToken 测试Token解析
func TestParseToken(t *testing.T) {
	setupJWTTest()

	token, _ := GenerateToken(1, "testuser", "student")
	claims, err := ParseToken(token)
	if err != nil {
		t.Fatalf("解析Token失败: %v", err)
	}
	if claims.UserID != 1 {
		t.Errorf("UserID 应为 1，实际: %d", claims.UserID)
	}
	if claims.Username != "testuser" {
		t.Errorf("Username 应为 testuser，实际: %s", claims.Username)
	}
	if claims.Role != "student" {
		t.Errorf("Role 应为 student，实际: %s", claims.Role)
	}
}

// TestParseTokenInvalid 测试解析无效Token
func TestParseTokenInvalid(t *testing.T) {
	setupJWTTest()

	_, err := ParseToken("invalid-token-string")
	if err == nil {
		t.Error("无效Token应该返回错误")
	}
}

// TestParseTokenEmpty 测试解析空Token
func TestParseTokenEmpty(t *testing.T) {
	setupJWTTest()

	_, err := ParseToken("")
	if err == nil {
		t.Error("空Token应该返回错误")
	}
}

// TestTokenClaims 测试Token包含完整Claims
func TestTokenClaims(t *testing.T) {
	setupJWTTest()

	token, _ := GenerateToken(42, "admin_user", "admin")
	claims, err := ParseToken(token)
	if err != nil {
		t.Fatalf("解析Token失败: %v", err)
	}
	if claims.Issuer != "edu_market" {
		t.Errorf("Issuer 应为 edu_market，实际: %s", claims.Issuer)
	}
	if claims.ExpiresAt == nil {
		t.Error("ExpiresAt 不应为空")
	}
	if claims.IssuedAt == nil {
		t.Error("IssuedAt 不应为空")
	}
	// 验证过期时间约为24小时
	expectedExpire := time.Now().Add(24 * time.Hour)
	diff := claims.ExpiresAt.Time.Sub(expectedExpire)
	if diff < -time.Minute || diff > time.Minute {
		t.Errorf("过期时间偏差过大: %v", diff)
	}
}

// TestDifferentSecrets 测试不同密钥生成的Token互不解析
func TestDifferentSecrets(t *testing.T) {
	// 用密钥A生成
	setupJWTTest()
	token, _ := GenerateToken(1, "user", "student")

	// 换密钥B
	config.App.JWT.Secret = "different-secret"
	_, err := ParseToken(token)
	if err == nil {
		t.Error("不同密钥解析Token应该失败")
	}
}
