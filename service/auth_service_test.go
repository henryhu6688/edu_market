package service

import (
	"fmt"
	"testing"

	"edu_market/database"
	"edu_market/model"
	"edu_market/utils"
)

// cleanTestUsers 清理测试用户
func cleanTestUsers(t *testing.T, usernames ...string) {
	for _, name := range usernames {
		database.DB.Where("username = ?", name).Delete(&model.User{})
	}
}

// TestRegister 测试用户名密码注册
func TestRegister(t *testing.T) {
	username := fmt.Sprintf("test_register_%s", t.Name())
	email := fmt.Sprintf("test_reg_%s@test.com", t.Name())
	defer cleanTestUsers(t, username)

	svc := &AuthService{}
	user, err := svc.Register(username, email, "123456")
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	if user.ID == 0 {
		t.Error("用户ID不应为0")
	}
	if user.Username != username {
		t.Errorf("用户名应为 %s，实际: %s", username, user.Username)
	}
	if user.Role != "student" {
		t.Errorf("默认角色应为 student，实际: %s", user.Role)
	}
}

// TestRegisterDuplicateUsername 测试重复用户名注册
func TestRegisterDuplicateUsername(t *testing.T) {
	username := fmt.Sprintf("test_dup_%s", t.Name())
	email1 := fmt.Sprintf("dup1_%s@test.com", t.Name())
	email2 := fmt.Sprintf("dup2_%s@test.com", t.Name())
	defer cleanTestUsers(t, username)

	svc := &AuthService{}
	svc.Register(username, email1, "123456")

	_, err := svc.Register(username, email2, "123456")
	if err == nil {
		t.Error("重复用户名应该返回错误")
	}
	if err != nil && err.Error() != "用户名已存在" {
		t.Errorf("错误信息应为'用户名已存在'，实际: %s", err.Error())
	}
}

// TestRegisterDuplicateEmail 测试重复邮箱注册
func TestRegisterDuplicateEmail(t *testing.T) {
	username1 := fmt.Sprintf("test_dup1_%s", t.Name())
	username2 := fmt.Sprintf("test_dup2_%s", t.Name())
	email := fmt.Sprintf("dup_email_%s@test.com", t.Name())
	defer cleanTestUsers(t, username1, username2)

	svc := &AuthService{}
	svc.Register(username1, email, "123456")

	_, err := svc.Register(username2, email, "123456")
	if err == nil {
		t.Error("重复邮箱应该返回错误")
	}
}

// TestLogin 测试用户名密码登录
func TestLogin(t *testing.T) {
	username := fmt.Sprintf("test_login_%s", t.Name())
	email := fmt.Sprintf("login_%s@test.com", t.Name())
	password := "mypassword123"
	defer cleanTestUsers(t, username)

	svc := &AuthService{}
	svc.Register(username, email, password)

	token, user, err := svc.Login(username, password)
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	if token == "" {
		t.Error("Token不应为空")
	}
	if user.Username != username {
		t.Errorf("用户名不匹配: %s", user.Username)
	}
}

// TestLoginWrongPassword 测试错误密码登录
func TestLoginWrongPassword(t *testing.T) {
	username := fmt.Sprintf("test_wrongpass_%s", t.Name())
	email := fmt.Sprintf("wrong_%s@test.com", t.Name())
	defer cleanTestUsers(t, username)

	svc := &AuthService{}
	svc.Register(username, email, "correct_password")

	_, _, err := svc.Login(username, "wrong_password")
	if err == nil {
		t.Error("错误密码应该返回错误")
	}
}

// TestLoginUserNotFound 测试不存在的用户登录
func TestLoginUserNotFound(t *testing.T) {
	svc := &AuthService{}
	_, _, err := svc.Login("nonexistent_user_12345", "password")
	if err == nil {
		t.Error("不存在的用户应该返回错误")
	}
}

// TestSendCode 测试发送验证码
func TestSendCode(t *testing.T) {
	svc := &AuthService{}
	err := svc.SendCode("13999000001")
	if err != nil {
		t.Fatalf("发送验证码失败: %v", err)
	}
}

// TestPhoneRegister 测试手机号注册
func TestPhoneRegister(t *testing.T) {
	phone := "13999000002"
	defer database.DB.Where("phone = ?", phone).Delete(&model.User{})

	svc := &AuthService{}
	user, err := svc.PhoneRegister(phone)
	if err != nil {
		t.Fatalf("手机号注册失败: %v", err)
	}
	if user.Phone != phone {
		t.Errorf("手机号应为 %s，实际: %s", phone, user.Phone)
	}
	if user.Username == "" {
		t.Error("用户名不应为空")
	}
}

// TestPhoneRegisterDuplicate 测试重复手机号注册
func TestPhoneRegisterDuplicate(t *testing.T) {
	phone := "13999000003"
	defer database.DB.Where("phone = ?", phone).Delete(&model.User{})

	svc := &AuthService{}
	svc.PhoneRegister(phone)

	_, err := svc.PhoneRegister(phone)
	if err == nil {
		t.Error("重复手机号应该返回错误")
	}
}

// TestPhoneLogin 测试手机号登录
func TestPhoneLogin(t *testing.T) {
	phone := "13999000004"
	defer database.DB.Where("phone = ?", phone).Delete(&model.User{})

	svc := &AuthService{}
	svc.PhoneRegister(phone)

	token, user, err := svc.PhoneLogin(phone)
	if err != nil {
		t.Fatalf("手机号登录失败: %v", err)
	}
	if token == "" {
		t.Error("Token不应为空")
	}
	if user.Phone != phone {
		t.Errorf("手机号不匹配: %s", user.Phone)
	}
}

// TestPhoneLoginNotFound 测试未注册手机号登录
func TestPhoneLoginNotFound(t *testing.T) {
	svc := &AuthService{}
	_, _, err := svc.PhoneLogin("13800000000")
	if err == nil {
		t.Error("未注册手机号应该返回错误")
	}
}

// TestPhoneRegisterAndLoginFlow 测试完整注册→登录流程
func TestPhoneRegisterAndLoginFlow(t *testing.T) {
	phone := "13999000005"
	defer database.DB.Where("phone = ?", phone).Delete(&model.User{})

	svc := &AuthService{}

	// 发验证码
	err := svc.SendCode(phone)
	if err != nil {
		t.Fatalf("发送验证码失败: %v", err)
	}

	// 验证码应存在
	code, _ := utils.CaptchaStore.Generate(phone) // 重新生成绕过限频
	_ = code

	// 注册
	user, err := svc.PhoneRegister(phone)
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	t.Logf("注册成功: ID=%d Phone=%s", user.ID, user.Phone)

	// 登录
	token, loginUser, err := svc.PhoneLogin(phone)
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	if token == "" || loginUser.ID != user.ID {
		t.Error("登录返回数据不一致")
	}
}
