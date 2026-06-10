package model

import "time"

// DocumentChunk RAG 文档块模型
type DocumentChunk struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CourseID   uint      `gorm:"not null;index" json:"course_id"`
	Content    string    `gorm:"type:text;not null" json:"content"`
	ChunkIndex int       `gorm:"not null;default:0" json:"chunk_index"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`

	Course Course `gorm:"foreignKey:CourseID;constraint:OnDelete:CASCADE" json:"-"`
}

// TableName 指定表名
func (DocumentChunk) TableName() string {
	return "document_chunks"
}
