package service

import (
	"testing"

	"edu_market/database"
	"edu_market/model"
	"edu_market/utils"
)

// cleanTestUserByPhone 清理测试用户
func cleanTestUserByPhone(t *testing.T, phones ...string) {
	for _, phone := range phones {
		database.DB.Where("phone = ?", phone).Delete(&model.User{})
	}
}

// TestSendCodeSuccess 测试发送短信验证码（Redis 环境）
func TestSendCodeSuccess(t *testing.T) {
	if database.RDB == nil {
		t.Skip("Redis 未连接，跳过")
	}
	svc := &AuthService{}
	err := svc.SendCode("13988000001")
	if err != nil {
		t.Fatalf("发送验证码失败: %v", err)
	}
}

// TestLoginByCodeNewUser 测试新用户首次登录自动注册
func TestLoginByCodeNewUser(t *testing.T) {
	if database.RDB == nil {
		t.Skip("Redis 未连接，跳过")
	}
	phone := "13988000010"
	defer cleanTestUserByPhone(t, phone)

	svc := &AuthService{}

	// 发验证码
	utils.CaptchaStore.Generate(phone)

	// 登录（应该自动注册）
	accessToken, refreshToken, user, err := svc.LoginByCode(phone)
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	if accessToken == "" {
		t.Error("accessToken 不应为空")
	}
	if refreshToken == "" {
		t.Error("refreshToken 不应为空")
	}
	if user.Phone != phone {
		t.Errorf("手机号应为 %s，实际: %s", phone, user.Phone)
	}
	if user.Username == "" {
		t.Error("用户名不应为空")
	}
	if user.Role != "student" {
		t.Errorf("角色应为 student，实际: %s", user.Role)
	}
}

// TestLoginByCodeExistingUser 测试已有用户登录
func TestLoginByCodeExistingUser(t *testing.T) {
	if database.RDB == nil {
		t.Skip("Redis 未连接，跳过")
	}
	phone := "13988000011"
	defer cleanTestUserByPhone(t, phone)

	svc := &AuthService{}

	// 先注册一次
	utils.CaptchaStore.Generate(phone)
	svc.LoginByCode(phone)

	// 再登录一次
	utils.CaptchaStore.Generate(phone)
	accessToken, refreshToken, user2, err := svc.LoginByCode(phone)
	if err != nil {
		t.Fatalf("第二次登录失败: %v", err)
	}
	if accessToken == "" || refreshToken == "" {
		t.Error("双 Token 不应为空")
	}
	if user2.Phone != phone {
		t.Error("手机号不匹配")
	}
}

// TestRefreshToken 测试刷新 Token
func TestRefreshToken(t *testing.T) {
	phone := "13988000012"
	defer cleanTestUserByPhone(t, phone)

	svc := &AuthService{}

	// 先登录获取 refresh_token
	if database.RDB != nil {
		utils.CaptchaStore.Generate(phone)
	}
	_, oldRefresh, _, err := svc.LoginByCode(phone)
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}

	// 刷新
	access, newRefresh, err := svc.Refresh(oldRefresh)
	if err != nil {
		t.Fatalf("刷新失败: %v", err)
	}
	if access == "" {
		t.Error("新 accessToken 不应为空")
	}
	if newRefresh == "" {
		t.Error("新 refreshToken 不应为空")
	}
	if newRefresh == oldRefresh {
		t.Error("新 refreshToken 应与旧的不同")
	}
}

// TestRefreshTokenInvalid 测试无效刷新
func TestRefreshTokenInvalid(t *testing.T) {
	svc := &AuthService{}
	_, _, err := svc.Refresh("invalid-refresh-token")
	if err == nil {
		t.Error("无效 refreshToken 应该返回错误")
	}
}
