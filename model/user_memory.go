package model

import "time"

// UserMemory 长期记忆模型（L3）
type UserMemory struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID     uint      `gorm:"not null;index" json:"user_id"`
	MemKey     string    `gorm:"type:varchar(100);not null" json:"mem_key"`
	MemValue   string    `gorm:"type:text" json:"mem_value"`
	Source     string    `gorm:"type:varchar(50);default:explicit" json:"source"`
	Confidence float64   `gorm:"default:0.5" json:"confidence"`
	Status     string    `gorm:"type:varchar(20);default:active" json:"status"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (UserMemory) TableName() string {
	return "user_memories"
}
