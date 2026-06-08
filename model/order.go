package model

import (
	"time"

	"gorm.io/gorm"
)

// Order 订单模型
type Order struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	OrderNo   string         `gorm:"type:varchar(32);uniqueIndex;not null" json:"order_no"` // 订单号
	UserID    uint           `gorm:"not null;index" json:"user_id"`
	CourseID  uint           `gorm:"not null;index" json:"course_id"`
	Amount    float64        `gorm:"type:decimal(10,2);not null" json:"amount"`
	Status    string         `gorm:"type:varchar(20);default:pending;not null" json:"status"` // pending | paid | cancelled
	PaidAt    *time.Time     `gorm:"default:null" json:"paid_at"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// 关联
	User   User   `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Course Course `gorm:"foreignKey:CourseID" json:"course,omitempty"`
}

// TableName 指定表名
func (Order) TableName() string {
	return "orders"
}
