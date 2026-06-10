package request

// AgentChatReq Agent 对话请求
type AgentChatReq struct {
	SessionID *uint  `json:"session_id"`                    // 可选，不传则新建会话
	Question  string `json:"question" binding:"required,min=1"`
}

// AgentSessionsReq 会话列表请求
type AgentSessionsReq struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=50"`
}

// AgentMessagesReq 消息历史请求
type AgentMessagesReq struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
}
