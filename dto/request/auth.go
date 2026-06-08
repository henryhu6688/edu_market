package request

// RegisterReq 注册请求
type RegisterReq struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6,max=100"`
}

// LoginReq 登录请求
type LoginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// SendCodeReq 发送验证码请求
type SendCodeReq struct {
	Phone string `json:"phone" binding:"required,len=11"`
}

// PhoneRegisterReq 手机号注册请求
type PhoneRegisterReq struct {
	Phone string `json:"phone" binding:"required,len=11"`
	Code  string `json:"code" binding:"required,len=6"`
}

// PhoneLoginReq 手机号登录请求
type PhoneLoginReq struct {
	Phone string `json:"phone" binding:"required,len=11"`
	Code  string `json:"code" binding:"required,len=6"`
}
