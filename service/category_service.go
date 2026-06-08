package service

import (
	"errors"

	"edu-market/database"
	"edu-market/model"

	"gorm.io/gorm"
)

// CategoryService 分类服务
type CategoryService struct{}

// List 获取所有分类
func (s *CategoryService) List() ([]model.Category, error) {
	var categories []model.Category
	if err := database.DB.Order("id ASC").Find(&categories).Error; err != nil {
		return nil, err
	}
	return categories, nil
}

// Create 创建分类（管理员）
func (s *CategoryService) Create(name, description string, parentID *uint) (*model.Category, error) {
	category := &model.Category{
		Name:        name,
		Description: description,
		ParentID:    parentID,
	}
	if err := database.DB.Create(category).Error; err != nil {
		return nil, errors.New("创建分类失败")
	}
	return category, nil
}

// Update 更新分类（管理员）
func (s *CategoryService) Update(id uint, name, description string) error {
	result := database.DB.Model(&model.Category{}).Where("id = ?", id).
		Updates(map[string]interface{}{"name": name, "description": description})
	if result.RowsAffected == 0 {
		return errors.New("分类不存在")
	}
	return result.Error
}

// Delete 删除分类（管理员）
func (s *CategoryService) Delete(id uint) error {
	// 检查是否有子分类
	var count int64
	database.DB.Model(&model.Category{}).Where("parent_id = ?", id).Count(&count)
	if count > 0 {
		return errors.New("请先删除子分类")
	}

	// 检查是否有课程使用
	database.DB.Model(&model.Course{}).Where("category_id = ?", id).Count(&count)
	if count > 0 {
		return errors.New("该分类下有课程，无法删除")
	}

	result := database.DB.Delete(&model.Category{}, id)
	if result.RowsAffected == 0 {
		return errors.New("分类不存在")
	}
	return result.Error
}

// GetByID 根据 ID 获取分类
func (s *CategoryService) GetByID(id uint) (*model.Category, error) {
	var category model.Category
	if err := database.DB.First(&category, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("分类不存在")
		}
		return nil, err
	}
	return &category, nil
}
