package request

// SendCodeReq 发送短信验证码请求
type SendCodeReq struct {
	Phone      string `json:"phone" binding:"required,len=11"`
	CaptchaID  string `json:"captcha_id" binding:"required"`
	CaptchaCode string `json:"captcha_code" binding:"required"`
}

// LoginByCodeReq 手机号验证码登录/注册请求
type LoginByCodeReq struct {
	Phone string `json:"phone" binding:"required,len=11"`
	Code  string `json:"code" binding:"required,len=6"`
}

// RefreshReq 刷新 Token 请求
type RefreshReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}
