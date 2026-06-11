package service

import (
	"testing"

	"edu_market/database"
	"edu_market/model"
)

func setupMaterialTest(t *testing.T) (*model.User, *model.Category, *MaterialService) {
	t.Helper()
	database.DB.Where("1=1").Delete(&model.Document{})
	database.DB.Where("1=1").Delete(&model.Material{})
	database.DB.Where("username LIKE ?", "mtest_%").Delete(&model.User{})

	user := &model.User{Username: "mtest_" + t.Name(), Role: "user"}
	if err := database.DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	cat := &model.Category{Name: "测试分类_" + t.Name()}
	if err := database.DB.Create(cat).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
	return user, cat, &MaterialService{}
}

func TestMaterialService_Create(t *testing.T) {
	user, cat, svc := setupMaterialTest(t)
	m := &model.Material{Title: "测试资料", Description: "描述", Price: 9.9, CategoryID: cat.ID, UserID: user.ID}
	if err := svc.Create(m); err != nil {
		t.Fatalf("create: %v", err)
	}
	if m.ID == 0 {
		t.Error("material ID should not be 0")
	}
}

func TestMaterialService_GetByID(t *testing.T) {
	user, cat, svc := setupMaterialTest(t)
	m := &model.Material{Title: "详情测试", Price: 0, CategoryID: cat.ID, UserID: user.ID}
	database.DB.Create(m)

	result, err := svc.GetByID(m.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if result.Title != "详情测试" {
		t.Errorf("title = %q, want 详情测试", result.Title)
	}
}

func TestMaterialService_Delete(t *testing.T) {
	user, cat, svc := setupMaterialTest(t)
	m := &model.Material{Title: "删除测试", Price: 0, CategoryID: cat.ID, UserID: user.ID}
	database.DB.Create(m)

	if err := svc.Delete(m.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
}
