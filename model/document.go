package model

import "time"

// Document 在线文档模型
type Document struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	MaterialID    uint      `gorm:"not null;index" json:"material_id"`
	ParentID      *uint     `gorm:"index;default:null" json:"parent_id"`
	Title         string    `gorm:"type:varchar(200);not null" json:"title"`
	Content       string    `gorm:"type:longtext" json:"content"`
	SortOrder     int       `gorm:"default:0" json:"sort_order"`
	IsFreePreview bool      `gorm:"default:false" json:"is_free_preview"`
	Status        string    `gorm:"type:varchar(20);default:draft" json:"status"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	Material Material   `gorm:"foreignKey:MaterialID;constraint:OnDelete:CASCADE" json:"-"`
	Children []Document `gorm:"foreignKey:ParentID;constraint:OnDelete:SET NULL" json:"children,omitempty"`
}

// TableName 指定表名
func (Document) TableName() string {
	return "documents"
}
