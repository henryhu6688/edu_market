package model

import "time"

// FAQ 常见问题模型
type FAQ struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Question  string    `gorm:"type:varchar(500);not null" json:"question"`
	Answer    string    `gorm:"type:text;not null" json:"answer"`
	Category  string    `gorm:"type:varchar(50);default:general" json:"category"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (FAQ) TableName() string {
	return "faqs"
}
