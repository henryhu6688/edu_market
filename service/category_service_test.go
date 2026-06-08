package service

import (
	"testing"

	"edu_market/database"
	"edu_market/model"
)

// cleanTestCategories 清理测试分类
func cleanTestCategories(t *testing.T, ids ...uint) {
	for _, id := range ids {
		database.DB.Delete(&model.Category{}, id)
	}
}

// TestCategoryCreate 测试创建分类
func TestCategoryCreate(t *testing.T) {
	svc := &CategoryService{}
	cat, err := svc.Create("测试分类", "这是一个测试分类", nil)
	if err != nil {
		t.Fatalf("创建分类失败: %v", err)
	}
	defer cleanTestCategories(t, cat.ID)

	if cat.Name != "测试分类" {
		t.Errorf("分类名应为'测试分类'，实际: %s", cat.Name)
	}
	if cat.ID == 0 {
		t.Error("分类ID不应为0")
	}
}

// TestCategoryList 测试获取分类列表
func TestCategoryList(t *testing.T) {
	svc := &CategoryService{}
	cat1, _ := svc.Create("测试分类A", "描述A", nil)
	cat2, _ := svc.Create("测试分类B", "描述B", nil)
	defer cleanTestCategories(t, cat1.ID, cat2.ID)

	categories, err := svc.List()
	if err != nil {
		t.Fatalf("获取分类列表失败: %v", err)
	}
	if len(categories) < 2 {
		t.Errorf("分类列表至少应有2条，实际: %d", len(categories))
	}
}

// TestCategoryUpdate 测试更新分类
func TestCategoryUpdate(t *testing.T) {
	svc := &CategoryService{}
	cat, _ := svc.Create("旧名称", "旧描述", nil)
	defer cleanTestCategories(t, cat.ID)

	err := svc.Update(cat.ID, "新名称", "新描述")
	if err != nil {
		t.Fatalf("更新分类失败: %v", err)
	}

	updated, _ := svc.GetByID(cat.ID)
	if updated.Name != "新名称" {
		t.Errorf("分类名应为'新名称'，实际: %s", updated.Name)
	}
}

// TestCategoryUpdateNotFound 测试更新不存在分类
func TestCategoryUpdateNotFound(t *testing.T) {
	svc := &CategoryService{}
	err := svc.Update(99999, "name", "desc")
	if err == nil {
		t.Error("更新不存在的分类应该返回错误")
	}
}

// TestCategoryDelete 测试删除分类
func TestCategoryDelete(t *testing.T) {
	svc := &CategoryService{}
	cat, _ := svc.Create("待删除分类", "描述", nil)
	defer cleanTestCategories(t, cat.ID)

	err := svc.Delete(cat.ID)
	if err != nil {
		t.Fatalf("删除分类失败: %v", err)
	}

	_, err = svc.GetByID(cat.ID)
	if err == nil {
		t.Error("已删除的分类不应该查到")
	}
}

// TestCategoryDeleteNotFound 测试删除不存在分类
func TestCategoryDeleteNotFound(t *testing.T) {
	svc := &CategoryService{}
	err := svc.Delete(99999)
	if err == nil {
		t.Error("删除不存在的分类应该返回错误")
	}
}

// TestCategoryDeleteWithChildren 测试删除有子分类的分类
func TestCategoryDeleteWithChildren(t *testing.T) {
	svc := &CategoryService{}
	parent, _ := svc.Create("父分类", "描述", nil)
	child, _ := svc.Create("子分类", "描述", &parent.ID)
	defer cleanTestCategories(t, child.ID, parent.ID)

	err := svc.Delete(parent.ID)
	if err == nil {
		t.Error("有子分类时应该拒绝删除")
	}
}

// TestCategoryGetByID 测试根据ID获取分类
func TestCategoryGetByID(t *testing.T) {
	svc := &CategoryService{}
	cat, _ := svc.Create("目标分类", "描述", nil)
	defer cleanTestCategories(t, cat.ID)

	found, err := svc.GetByID(cat.ID)
	if err != nil {
		t.Fatalf("获取分类失败: %v", err)
	}
	if found.Name != "目标分类" {
		t.Errorf("分类名应为'目标分类'，实际: %s", found.Name)
	}
}
