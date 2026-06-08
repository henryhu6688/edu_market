package request

// CreateReviewReq 创建评论请求
type CreateReviewReq struct {
	CourseID uint   `json:"course_id" binding:"required"`
	Rating   int    `json:"rating" binding:"required,min=1,max=5"`
	Content  string `json:"content"`
}

// ReviewListReq 评论列表查询
type ReviewListReq struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
}
