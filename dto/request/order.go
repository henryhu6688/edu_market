package request

// CreateOrderReq 创建订单请求
type CreateOrderReq struct {
	CourseID uint `json:"course_id" binding:"required"`
}

// OrderListReq 订单列表查询
type OrderListReq struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=100"`
	Status   string `form:"status" binding:"omitempty,oneof=pending paid cancelled"`
}
