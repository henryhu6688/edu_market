package model

import "time"

// User 用户模型
type User struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Username     string    `gorm:"type:varchar(50);uniqueIndex;not null" json:"username" binding:"required"`
	Email        string    `gorm:"type:varchar(100);uniqueIndex;not null" json:"email" binding:"required,email"`
	PasswordHash string    `gorm:"type:varchar(255);not null" json:"-"`
	Role         string    `gorm:"type:varchar(20);default:student;not null" json:"role"` // student | admin
	Avatar       string    `gorm:"type:varchar(255)" json:"avatar"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (User) TableName() string {
	return "users"
}
