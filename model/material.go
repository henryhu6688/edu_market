package model

import "time"

// MaterialStatus 定义
const (
	MaterialDraft     = "draft"
	MaterialPublished = "published"
	MaterialOff       = "off"
)

// Material 学习资料模型
type Material struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Title       string    `gorm:"type:varchar(200);not null;index" json:"title"`
	Description string    `gorm:"type:text" json:"description"`
	Price       float64   `gorm:"type:decimal(10,2);not null;default:0" json:"price"`
	CoverImage  string    `gorm:"type:varchar(255)" json:"cover_image"`
	CategoryID  uint      `gorm:"not null;index" json:"category_id"`
	UserID      uint      `gorm:"not null;index" json:"user_id"`
	Status      string    `gorm:"type:varchar(20);default:draft;not null" json:"status"`
	ViewCount   int       `gorm:"default:0" json:"view_count"`
	BuyCount    int       `gorm:"default:0" json:"buy_count"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	Category  Category   `gorm:"foreignKey:CategoryID;constraint:OnDelete:CASCADE" json:"category,omitempty"`
	User      User       `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
	Documents []Document `gorm:"foreignKey:MaterialID;constraint:OnDelete:CASCADE" json:"documents,omitempty"`
}

// TableName 指定表名
func (Material) TableName() string {
	return "materials"
}
