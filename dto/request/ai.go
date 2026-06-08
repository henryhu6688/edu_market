package request

// ChatReq AI 对话请求
type ChatReq struct {
	Question string `json:"question" binding:"required,min=1"`
}

// HistoryReq AI 对话历史查询
type HistoryReq struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
}
