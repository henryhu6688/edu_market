package service

import (
	"errors"
	"fmt"
	"math/rand"

	"edu_market/database"
	"edu_market/model"
	"edu_market/utils"

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

// SendCode 发送手机验证码（开发阶段控制台打印）
func (s *AuthService) SendCode(phone string) error {
	_, err := utils.CaptchaStore.Generate(phone)
	if err != nil {
		return err
	}
	return nil
}

// PhoneRegister 手机号注册（验证码已由 controller 校验通过）
func (s *AuthService) PhoneRegister(phone string) (*model.User, error) {
	// 检查手机号是否已注册
	var existUser model.User
	if err := database.DB.Where("phone = ?", phone).First(&existUser).Error; err == nil {
		return nil, errors.New("手机号已注册")
	}

	// 自动生成用户名和随机密码
	username := fmt.Sprintf("user_%s", phone[7:]) // 用手机号后4位生成用户名
	password := fmt.Sprintf("%08d", rand.Intn(100000000)) // 8位随机密码

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("密码加密失败")
	}

	user := &model.User{
		Username:     username,
		Phone:        phone,
		PasswordHash: string(hash),
		Role:         "student",
	}

	if err := database.DB.Create(user).Error; err != nil {
		return nil, errors.New("注册失败")
	}

	return user, nil
}

// PhoneLogin 手机号验证码登录
func (s *AuthService) PhoneLogin(phone string) (string, *model.User, error) {
	var user model.User
	if err := database.DB.Where("phone = ?", phone).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil, errors.New("手机号未注册")
		}
		return "", nil, err
	}

	// 生成 JWT
	token, err := utils.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		return "", nil, errors.New("Token生成失败")
	}

	return token, &user, nil
}
