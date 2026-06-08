package service

import (
	"errors"

	"edu_market/database"
	"edu_market/model"

	"gorm.io/gorm"
)

// UserService 用户服务
type UserService struct{}

// GetProfile 获取用户信息
func (s *UserService) GetProfile(userID uint) (*model.User, error) {
	var user model.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("用户不存在")
		}
		return nil, err
	}
	return &user, nil
}

// UpdateProfile 更新用户信息
func (s *UserService) UpdateProfile(userID uint, updates map[string]interface{}) (*model.User, error) {
	var user model.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("用户不存在")
		}
		return nil, err
	}

	if err := database.DB.Model(&user).Updates(updates).Error; err != nil {
		return nil, errors.New("更新失败")
	}

	return &user, nil
}
