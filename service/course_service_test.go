package service

import (
	"fmt"
	"testing"

	"edu_market/database"
	"edu_market/model"
)

// createTestCategory 创建测试分类
func createTestCategory(t *testing.T, name string) *model.Category {
	cat := &model.Category{Name: name}
	if err := database.DB.Create(cat).Error; err != nil {
		t.Fatalf("创建测试分类失败: %v", err)
	}
	return cat
}

// createTestCourse 创建测试课程（自动创建依赖的 user 和 category）
func createTestCourse(t *testing.T, title string) *model.Course {
	// 创建测试用户
	user := &model.User{
		Username: fmt.Sprintf("test_course_user_%s", title),
		Email:    fmt.Sprintf("test_course_%s@test.com", title),
		PasswordHash: "$2a$10$abc123",
		Role:     "student",
	}
	if err := database.DB.Create(user).Error; err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
	// 创建测试分类
	cat := &model.Category{Name: fmt.Sprintf("测试分类-%s", title)}
	if err := database.DB.Create(cat).Error; err != nil {
		t.Fatalf("创建测试分类失败: %v", err)
	}

	course := &model.Course{
		Title:       title,
		Description: "测试课程描述",
		Price:       19.99,
		CategoryID:  cat.ID,
		UserID:      user.ID,
		Status:      "published",
	}
	if err := database.DB.Create(course).Error; err != nil {
		t.Fatalf("创建测试课程失败: %v", err)
	}
	return course
}

// cleanTestCourse 清理测试课程及其依赖
func cleanTestCourse(t *testing.T, course *model.Course) {
	database.DB.Delete(&model.Course{}, course.ID)
	database.DB.Where("id = ?", course.CategoryID).Delete(&model.Category{})
	database.DB.Where("id = ?", course.UserID).Delete(&model.User{})
}

// TestCourseCreate 测试创建课程
func TestCourseCreate(t *testing.T) {
	course := createTestCourse(t, "测试课程-单元测试")
	defer cleanTestCourse(t, course)

	if course.ID == 0 {
		t.Error("课程ID不应为0")
	}
}

// TestCourseGetByID 测试根据ID获取课程
func TestCourseGetByID(t *testing.T) {
	course := createTestCourse(t, "测试-获取详情")
	defer cleanTestCourse(t, course)

	svc := &CourseService{}
	found, err := svc.GetByID(course.ID)
	if err != nil {
		t.Fatalf("获取课程失败: %v", err)
	}
	if found.Title != course.Title {
		t.Errorf("课程标题不匹配: %s", found.Title)
	}
}

// TestCourseGetByIDNotFound 测试获取不存在课程
func TestCourseGetByIDNotFound(t *testing.T) {
	svc := &CourseService{}
	_, err := svc.GetByID(99999)
	if err == nil {
		t.Error("获取不存在的课程应该返回错误")
	}
}

// TestCourseList 测试课程列表分页
func TestCourseList(t *testing.T) {
	svc := &CourseService{}
	courses, total, err := svc.List(1, 10, 0, "", "")
	if err != nil {
		t.Fatalf("获取课程列表失败: %v", err)
	}
	t.Logf("课程总数: %d, 当前页: %d", total, len(courses))
}

// TestCourseUpdate 测试更新课程
func TestCourseUpdate(t *testing.T) {
	course := createTestCourse(t, "测试-更新前")
	defer cleanTestCourse(t, course)

	svc := &CourseService{}
	err := svc.Update(course.ID, map[string]interface{}{
		"title": "测试-更新后",
		"price": 29.99,
	})
	if err != nil {
		t.Fatalf("更新课程失败: %v", err)
	}

	updated, _ := svc.GetByID(course.ID)
	if updated.Title != "测试-更新后" {
		t.Errorf("标题应为'测试-更新后'，实际: %s", updated.Title)
	}
}

// TestCourseUpdateNotFound 测试更新不存在课程
func TestCourseUpdateNotFound(t *testing.T) {
	svc := &CourseService{}
	err := svc.Update(99999, map[string]interface{}{"title": "x"})
	if err == nil {
		t.Error("更新不存在的课程应该返回错误")
	}
}

// TestCourseDelete 测试删除课程
func TestCourseDelete(t *testing.T) {
	course := createTestCourse(t, "测试-待删除")
	defer cleanTestCourse(t, course)

	svc := &CourseService{}
	err := svc.Delete(course.ID)
	if err != nil {
		t.Fatalf("删除课程失败: %v", err)
	}

	_, err = svc.GetByID(course.ID)
	if err == nil {
		t.Error("已删除的课程不应该查到")
	}
}

// TestCourseDeleteNotFound 测试删除不存在课程
func TestCourseDeleteNotFound(t *testing.T) {
	svc := &CourseService{}
	err := svc.Delete(99999)
	if err == nil {
		t.Error("删除不存在的课程应该返回错误")
	}
}

// TestGetPagination 测试分页参数处理
func TestGetPagination(t *testing.T) {
	tests := []struct {
		page, pageSize     int
		expectedPage, expectedSize int
	}{
		{0, 0, 1, 10},
		{-1, 0, 1, 10},
		{5, 20, 5, 20},
		{1, 200, 1, 100}, // 超过上限截断
		{3, 15, 3, 15},
	}
	for _, tc := range tests {
		p, ps := GetPagination(tc.page, tc.pageSize)
		if p != tc.expectedPage || ps != tc.expectedSize {
			t.Errorf("GetPagination(%d,%d) = (%d,%d)，期望 (%d,%d)",
				tc.page, tc.pageSize, p, ps, tc.expectedPage, tc.expectedSize)
		}
	}
}

// TestCourseListKeyword 测试关键词搜索
func TestCourseListKeyword(t *testing.T) {
	keyword := fmt.Sprintf("独特搜索词_%d", 12345)
	course := createTestCourse(t, keyword)
	defer cleanTestCourse(t, course)

	svc := &CourseService{}
	courses, total, err := svc.List(1, 10, 0, keyword, "")
	if err != nil {
		t.Fatalf("搜索课程失败: %v", err)
	}
	if total < 1 {
		t.Error("关键词搜索应有结果")
	}
	found := false
	for _, c := range courses {
		if c.ID == course.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("搜索结果中未找到创建的课程")
	}
}
