package model

import "time"

// DocumentChunk RAG 文档块模型
type DocumentChunk struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CourseID   uint      `gorm:"not null;index" json:"course_id"`
	Content    string    `gorm:"type:text;not null" json:"content"`
	Embedding  []byte    `gorm:"type:blob" json:"-"`
	ChunkIndex int       `gorm:"not null;default:0" json:"chunk_index"`
	// DocumentID 来源文档 ID（documents.id）
	DocumentID uint `gorm:"index" json:"document_id"`
	// SectionPath 章节路径，如 "第三章 > 3.2 闭包"
	SectionPath string `gorm:"type:varchar(500)" json:"section_path"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`

	// CourseID 实际引用 materials.id（兼容旧字段名）
}

// TableName 指定表名
func (DocumentChunk) TableName() string {
	return "document_chunks"
}
