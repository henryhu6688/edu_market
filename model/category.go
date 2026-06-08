package model

import "time"

// Category 分类模型
type Category struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string    `gorm:"type:varchar(50);not null" json:"name" binding:"required"`
	Description string    `gorm:"type:varchar(255)" json:"description"`
	ParentID    *uint     `gorm:"default:null;index" json:"parent_id"` // 父分类ID，nil表示顶级分类
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (Category) TableName() string {
	return "categories"
}
