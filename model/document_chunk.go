package model

import "time"

// DocumentChunk RAG 文档块模型
type DocumentChunk struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CourseID   uint      `gorm:"not null;index" json:"course_id"`
	Content    string    `gorm:"type:text;not null" json:"content"`
	Embedding  []byte    `gorm:"type:blob" json:"-"`
	ChunkIndex int       `gorm:"not null;default:0" json:"chunk_index"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`

	// CourseID 实际引用 materials.id（兼容旧字段名）

}

// TableName 指定表名
func (DocumentChunk) TableName() string {
	return "document_chunks"
}
