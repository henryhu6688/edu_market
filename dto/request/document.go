package request

// CreateDocumentReq 创建文档请求
type CreateDocumentReq struct {
	ParentID      *uint  `json:"parent_id"`
	Title         string `json:"title" binding:"required,min=1,max=200"`
	IsFreePreview bool   `json:"is_free_preview"`
}

// UpdateDocumentReq 更新文档请求
type UpdateDocumentReq struct {
	Title         *string `json:"title"`
	Content       *string `json:"content"`
	ParentID      *uint   `json:"parent_id"`
	SortOrder     *int    `json:"sort_order"`
	IsFreePreview *bool   `json:"is_free_preview"`
	Status        *string `json:"status" binding:"omitempty,oneof=draft published"`
}
