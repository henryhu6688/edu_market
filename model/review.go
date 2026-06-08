package model

import "time"

// Review 评论/评价模型
type Review struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uint      `gorm:"not null;index" json:"user_id"`
	CourseID  uint      `gorm:"not null;index" json:"course_id"`
	Rating    int       `gorm:"not null;default:5" json:"rating" binding:"required,min=1,max=5"` // 1-5星
	Content   string    `gorm:"type:text" json:"content"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`

	// 关联
	User   User   `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Course Course `gorm:"foreignKey:CourseID" json:"course,omitempty"`
}

// TableName 指定表名
func (Review) TableName() string {
	return "reviews"
}
