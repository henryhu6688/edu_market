package request

// CreateMaterialReq 创建资料请求
type CreateMaterialReq struct {
	Title       string  `json:"title" binding:"required,min=1,max=200"`
	Description string  `json:"description"`
	Price       float64 `json:"price" binding:"min=0"`
	CoverImage  string  `json:"cover_image"`
	CategoryID  uint    `json:"category_id" binding:"required"`
}

// UpdateMaterialReq 更新资料请求
type UpdateMaterialReq struct {
	Title       string  `json:"title" binding:"omitempty,min=1,max=200"`
	Description string  `json:"description"`
	Price       float64 `json:"price" binding:"omitempty,min=0"`
	CoverImage  string  `json:"cover_image"`
	CategoryID  uint    `json:"category_id"`
	Status      string  `json:"status" binding:"omitempty,oneof=draft published off"`
}

// MaterialListReq 资料列表请求
type MaterialListReq struct {
	Page       int    `form:"page" binding:"omitempty,min=1"`
	PageSize   int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	CategoryID uint   `form:"category_id"`
	Keyword    string `form:"keyword"`
	Status     string `form:"status"`
}
