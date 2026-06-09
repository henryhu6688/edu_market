package model

import "time"

// Course 课程/资料模型
type Course struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Title       string    `gorm:"type:varchar(200);not null;index" json:"title" binding:"required"`
	Description string    `gorm:"type:text" json:"description"`
	Price       float64   `gorm:"type:decimal(10,2);not null;default:0" json:"price"`
	CoverImage  string    `gorm:"type:varchar(255)" json:"cover_image"`
	FileURL     string    `gorm:"type:varchar(255)" json:"file_url"`
	CategoryID  uint      `gorm:"not null;index" json:"category_id" binding:"required"`
	UserID      uint      `gorm:"not null;index" json:"user_id"` // 发布者
	Status      string    `gorm:"type:varchar(20);default:draft;not null" json:"status"` // draft | published | off
	ViewCount   int       `gorm:"default:0" json:"view_count"`
	BuyCount    int       `gorm:"default:0" json:"buy_count"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// 关联
	Category Category `gorm:"foreignKey:CategoryID;constraint:OnDelete:CASCADE" json:"category,omitempty"`
	User     User     `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
}

// TableName 指定表名
func (Course) TableName() string {
	return "courses"
}
