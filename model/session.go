package model

import "time"

// AgentType 定义
const (
	AgentCustomerService  = "customer_service"
	AgentCourseRecommend = "course_recommend"
	AgentQA              = "qa"
)

// SessionStatus 定义
const (
	SessionActive = "active"
	SessionClosed = "closed"
)

// Session AI 对话会话模型
type Session struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uint      `gorm:"not null;index" json:"user_id"`
	AgentType string    `gorm:"type:varchar(30);not null;default:customer_service" json:"agent_type"`
	Title     string    `gorm:"type:varchar(100);default:''" json:"title"`
	Status    string    `gorm:"type:varchar(20);not null;default:active" json:"status"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	User     User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Messages []Message `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE" json:"messages,omitempty"`
}

// TableName 指定表名
func (Session) TableName() string {
	return "sessions"
}
