package request

// CreateCourseReq 创建课程请求
type CreateCourseReq struct {
	Title       string  `json:"title" binding:"required,min=1,max=200"`
	Description string  `json:"description"`
	Price       float64 `json:"price" binding:"min=0"`
	CategoryID  uint    `json:"category_id" binding:"required"`
}

// UpdateCourseReq 更新课程请求
type UpdateCourseReq struct {
	Title       string  `json:"title" binding:"omitempty,min=1,max=200"`
	Description string  `json:"description"`
	Price       float64 `json:"price" binding:"omitempty,min=0"`
	CategoryID  uint    `json:"category_id"`
	Status      string  `json:"status" binding:"omitempty,oneof=draft published off"`
}

// CourseListReq 课程列表查询请求
type CourseListReq struct {
	Page       int    `form:"page" binding:"omitempty,min=1"`
	PageSize   int    `form:"page_size" binding:"omitempty,min=1,max=100"`
	CategoryID uint   `form:"category_id"`
	Keyword    string `form:"keyword"`
	Status     string `form:"status" binding:"omitempty,oneof=draft published off"`
}
