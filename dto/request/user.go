package request

// UpdateProfileReq 更新个人资料请求
type UpdateProfileReq struct {
	Username string `json:"username" binding:"min=3,max=50"`
	Email    string `json:"email" binding:"omitempty,email"`
	Avatar   string `json:"avatar"`
}
