package service

import (
	"fmt"
	"testing"

	"edu_market/database"
	"edu_market/model"
)

// createTestUser 创建测试用户并返回ID
func createTestUser(t *testing.T, username string) uint {
	user := &model.User{
		Username:     username,
		Email:        fmt.Sprintf("%s@test.com", username),
		PasswordHash: "$2a$10$abc", // 假的hash
		Role:         "student",
	}
	if err := database.DB.Create(user).Error; err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
	return user.ID
}

// TestGetProfile 测试获取用户信息
func TestGetProfile(t *testing.T) {
	username := fmt.Sprintf("test_profile_%d", 99999)
	defer database.DB.Where("username = ?", username).Delete(&model.User{})
	userID := createTestUser(t, username)

	svc := &UserService{}
	user, err := svc.GetProfile(userID)
	if err != nil {
		t.Fatalf("获取个人信息失败: %v", err)
	}
	if user.Username != username {
		t.Errorf("用户名应为 %s，实际: %s", username, user.Username)
	}
}

// TestGetProfileNotFound 测试获取不存在用户
func TestGetProfileNotFound(t *testing.T) {
	svc := &UserService{}
	_, err := svc.GetProfile(99999)
	if err == nil {
		t.Error("不存在的用户应该返回错误")
	}
}

// TestUpdateProfile 测试更新用户信息
func TestUpdateProfile(t *testing.T) {
	username := fmt.Sprintf("test_update_%d", 88888)
	defer database.DB.Where("username = ?", username).Delete(&model.User{})
	userID := createTestUser(t, username)

	svc := &UserService{}
	updates := map[string]interface{}{
		"avatar": "https://example.com/avatar.png",
	}
	user, err := svc.UpdateProfile(userID, updates)
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if user.Avatar != "https://example.com/avatar.png" {
		t.Errorf("头像未更新，实际: %s", user.Avatar)
	}
}

// TestUpdateProfileNotFound 测试更新不存在用户
func TestUpdateProfileNotFound(t *testing.T) {
	svc := &UserService{}
	_, err := svc.UpdateProfile(99999, map[string]interface{}{"avatar": "x"})
	if err == nil {
		t.Error("更新不存在的用户应该返回错误")
	}
}
