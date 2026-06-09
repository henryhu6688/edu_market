package service

import (
	"errors"
	"fmt"
	"math/rand"
	"time"

	"edu_market/database"
	"edu_market/model"
	"edu_market/utils"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// AuthService 认证服务
type AuthService struct{}

// SendCode 发送短信验证码
func (s *AuthService) SendCode(phone string) error {
	_, err := utils.CaptchaStore.Generate(phone)
	return err
}

// LoginByCode 统一入口：手机号+验证码登录/注册
func (s *AuthService) LoginByCode(phone string) (accessToken, refreshToken string, user *model.User, err error) {
	var u model.User
	result := database.DB.Where("phone = ?", phone).First(&u)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		// 新用户 → 自动注册
		u = model.User{
			Username:     fmt.Sprintf("user_%s", phone[7:]),
			Phone:        phone,
			PasswordHash: randomBcryptHash(),
			Role:         "student",
		}
		if err := database.DB.Create(&u).Error; err != nil {
			return "", "", nil, errors.New("自动注册失败")
		}
	} else if result.Error != nil {
		return "", "", nil, result.Error
	}

	// 生成双 Token
	accessToken, err = utils.GenerateAccessToken(u.ID, u.Username, u.Role)
	if err != nil {
		return "", "", nil, errors.New("Token生成失败")
	}

	refreshToken, err = utils.GenerateRefreshToken()
	if err != nil {
		return "", "", nil, errors.New("RefreshToken生成失败")
	}

	// 存 refresh token 到用户表
	expiresAt := time.Now().Add(utils.RefreshTTL())
	database.DB.Model(&u).Updates(map[string]interface{}{
		"refresh_token":      refreshToken,
		"refresh_expires_at": &expiresAt,
	})

	return accessToken, refreshToken, &u, nil
}

// Refresh 刷新双 Token
func (s *AuthService) Refresh(oldRefreshToken string) (accessToken, newRefreshToken string, err error) {
	var u model.User
	if err := database.DB.Where("refresh_token = ?", oldRefreshToken).First(&u).Error; err != nil {
		return "", "", errors.New("无效的refresh_token")
	}
	if u.RefreshExpiresAt == nil || time.Now().After(*u.RefreshExpiresAt) {
		return "", "", errors.New("refresh_token已过期，请重新登录")
	}

	accessToken, err = utils.GenerateAccessToken(u.ID, u.Username, u.Role)
	if err != nil {
		return "", "", errors.New("Token生成失败")
	}

	newRefreshToken, err = utils.GenerateRefreshToken()
	if err != nil {
		return "", "", errors.New("RefreshToken生成失败")
	}

	expiresAt := time.Now().Add(utils.RefreshTTL())
	database.DB.Model(&u).Updates(map[string]interface{}{
		"refresh_token":      newRefreshToken,
		"refresh_expires_at": &expiresAt,
	})

	return accessToken, newRefreshToken, nil
}

// randomBcryptHash 随机密码的 bcrypt hash（自动注册用）
func randomBcryptHash() string {
	raw := fmt.Sprintf("%08d", rand.Intn(100000000))
	hash, _ := bcrypt.GenerateFromPassword([]byte(raw), bcrypt.DefaultCost)
	return string(hash)
}
