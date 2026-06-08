package service

import (
	"errors"
	"math"

	"edu-market/database"
	"edu-market/model"

	"gorm.io/gorm"
)

// CourseService 课程服务
type CourseService struct{}

// Create 创建课程
func (s *CourseService) Create(course *model.Course) error {
	return database.DB.Create(course).Error
}

// GetByID 根据ID获取课程详情
func (s *CourseService) GetByID(id uint) (*model.Course, error) {
	var course model.Course
	if err := database.DB.Preload("Category").Preload("User").First(&course, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("课程不存在")
		}
		return nil, err
	}
	// 增加浏览次数
	database.DB.Model(&course).UpdateColumn("view_count", gorm.Expr("view_count + ?", 1))
	return &course, nil
}

// List 课程列表（分页 + 筛选）
func (s *CourseService) List(page, pageSize int, categoryID uint, keyword, status string) ([]model.Course, int64, error) {
	var courses []model.Course
	var total int64

	query := database.DB.Model(&model.Course{}).Where("status = ?", "published")
	if status != "" {
		query = database.DB.Model(&model.Course{}).Where("status = ?", status)
	}
	if categoryID > 0 {
		query = query.Where("category_id = ?", categoryID)
	}
	if keyword != "" {
		query = query.Where("title LIKE ?", "%"+keyword+"%")
	}

	query.Count(&total)

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	offset := (page - 1) * pageSize
	if err := query.Preload("Category").Preload("User").Offset(offset).Limit(pageSize).
		Order("created_at DESC").Find(&courses).Error; err != nil {
		return nil, 0, err
	}

	return courses, total, nil
}

// Update 更新课程
func (s *CourseService) Update(id uint, updates map[string]interface{}) error {
	result := database.DB.Model(&model.Course{}).Where("id = ?", id).Updates(updates)
	if result.RowsAffected == 0 {
		return errors.New("课程不存在")
	}
	return result.Error
}

// Delete 删除课程
func (s *CourseService) Delete(id uint) error {
	result := database.DB.Delete(&model.Course{}, id)
	if result.RowsAffected == 0 {
		return errors.New("课程不存在")
	}
	return result.Error
}

// GetPagination 分页参数处理
func GetPagination(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	pageSize = int(math.Min(float64(pageSize), 100))
	return page, pageSize
}
