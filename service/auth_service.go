package service

import (
	"errors"

	"edu-market/database"
	"edu-market/model"
	"edu-market/utils"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// AuthService 认证服务
type AuthService struct{}

// Register 用户注册
func (s *AuthService) Register(username, email, password string) (*model.User, error) {
	// 检查用户名是否存在
	var existUser model.User
	if err := database.DB.Where("username = ?", username).First(&existUser).Error; err == nil {
		return nil, errors.New("用户名已存在")
	}

	// 检查邮箱是否存在
	if err := database.DB.Where("email = ?", email).First(&existUser).Error; err == nil {
		return nil, errors.New("邮箱已注册")
	}

	// 密码加密
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("密码加密失败")
	}

	user := &model.User{
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
		Role:         "student",
	}

	if err := database.DB.Create(user).Error; err != nil {
		return nil, errors.New("注册失败")
	}

	return user, nil
}

// Login 用户登录
func (s *AuthService) Login(username, password string) (string, *model.User, error) {
	var user model.User
	if err := database.DB.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil, errors.New("用户名或密码错误")
		}
		return "", nil, err
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", nil, errors.New("用户名或密码错误")
	}

	// 生成 JWT
	token, err := utils.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		return "", nil, errors.New("Token生成失败")
	}

	return token, &user, nil
}
