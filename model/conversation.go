package model

import "time"

// Conversation AI 对话记录模型
type Conversation struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID     uint      `gorm:"not null;index" json:"user_id"`
	Question   string    `gorm:"type:text;not null" json:"question" binding:"required"`
	Answer     string    `gorm:"type:text;not null" json:"answer"`
	Model      string    `gorm:"type:varchar(50)" json:"model"` // 使用的模型
	TokensUsed int       `gorm:"default:0" json:"tokens_used"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`

	// 关联
	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
}

// TableName 指定表名
func (Conversation) TableName() string {
	return "conversations"
}
