package service

import (
	"errors"

	"edu_market/database"
	"edu_market/model"

	"gorm.io/gorm"
)

// MaterialService 学习资料服务
type MaterialService struct{}

// Create 发布资料
func (s *MaterialService) Create(m *model.Material) error {
	return database.DB.Create(m).Error
}

// GetByID 资料详情
func (s *MaterialService) GetByID(id uint) (*model.Material, error) {
	var m model.Material
	if err := database.DB.Preload("Category").Preload("User").First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("资料不存在")
		}
		return nil, err
	}
	database.DB.Model(&m).UpdateColumn("view_count", gorm.Expr("view_count + ?", 1))
	return &m, nil
}

// List 资料列表
func (s *MaterialService) List(page, pageSize int, categoryID uint, keyword, status string) ([]model.Material, int64, error) {
	var materials []model.Material
	var total int64

	query := database.DB.Model(&model.Material{}).Where("status = ?", model.MaterialPublished)
	if status != "" {
		query = database.DB.Model(&model.Material{}).Where("status = ?", status)
	}
	if categoryID > 0 {
		query = query.Where("category_id = ?", categoryID)
	}
	if keyword != "" {
		query = query.Where("title LIKE ? OR description LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}

	query.Count(&total)
	page, pageSize = GetPagination(page, pageSize)

	if err := query.Preload("Category").Preload("User").
		Offset((page-1)*pageSize).Limit(pageSize).
		Order("created_at DESC").Find(&materials).Error; err != nil {
		return nil, 0, err
	}
	return materials, total, nil
}

// Update 更新资料
func (s *MaterialService) Update(id uint, updates map[string]interface{}) error {
	r := database.DB.Model(&model.Material{}).Where("id = ?", id).Updates(updates)
	if r.RowsAffected == 0 {
		return errors.New("资料不存在")
	}
	return r.Error
}

// Delete 删除资料
func (s *MaterialService) Delete(id uint) error {
	r := database.DB.Delete(&model.Material{}, id)
	if r.RowsAffected == 0 {
		return errors.New("资料不存在")
	}
	return r.Error
}

// GetMyMaterials 获取用户发布的资料
func (s *MaterialService) GetMyMaterials(userID uint, page, pageSize int) ([]model.Material, int64, error) {
	var materials []model.Material
	var total int64
	page, pageSize = GetPagination(page, pageSize)

	database.DB.Model(&model.Material{}).Where("user_id = ?", userID).Count(&total)
	if err := database.DB.Where("user_id = ?", userID).
		Offset((page-1)*pageSize).Limit(pageSize).
		Order("created_at DESC").Find(&materials).Error; err != nil {
		return nil, 0, err
	}
	return materials, total, nil
}
