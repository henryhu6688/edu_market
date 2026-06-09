package utils

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"edu_market/config"

	"github.com/golang-jwt/jwt/v5"
)

// Claims JWT Access Token 声明
type Claims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateAccessToken 生成短期 Access Token
func GenerateAccessToken(userID uint, username, role string) (string, error) {
	ttl := config.App.JWT.AccessTTLMin
	if ttl <= 0 {
		ttl = 30
	}
	claims := Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(ttl) * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "edu_market",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.App.JWT.Secret))
}

// ParseToken 解析 JWT Token
func ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		return []byte(config.App.JWT.Secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("无效的Token")
	}
	return claims, nil
}

// GenerateRefreshToken 生成 Refresh Token（随机字符串）
func GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// RefreshTTL 返回 Refresh Token 有效期
func RefreshTTL() time.Duration {
	ttl := config.App.JWT.RefreshTTLHours
	if ttl <= 0 {
		ttl = 24
	}
	return time.Duration(ttl) * time.Hour
}
