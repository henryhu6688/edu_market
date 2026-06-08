package controller

import (
	"edu_market/dto/request"
	"edu_market/service"
	"edu_market/utils"

	"github.com/gin-gonic/gin"
)

// AuthController 认证控制器
type AuthController struct {
	svc service.AuthService
}

// SendCode 发送手机验证码
func (ctr *AuthController) SendCode(c *gin.Context) {
	var req request.SendCodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	if err := ctr.svc.SendCode(req.Phone); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"phone": req.Phone, "message": "验证码已发送"})
}

// PhoneRegister 手机号验证码注册
func (ctr *AuthController) PhoneRegister(c *gin.Context) {
	var req request.PhoneRegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	// 先校验验证码
	if !utils.CaptchaStore.Verify(req.Phone, req.Code) {
		utils.BadRequest(c, "验证码错误或已过期")
		return
	}

	user, err := ctr.svc.PhoneRegister(req.Phone)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Created(c, gin.H{"id": user.ID, "username": user.Username, "phone": user.Phone})
}

// PhoneLogin 手机号验证码登录
func (ctr *AuthController) PhoneLogin(c *gin.Context) {
	var req request.PhoneLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	// 先校验验证码
	if !utils.CaptchaStore.Verify(req.Phone, req.Code) {
		utils.BadRequest(c, "验证码错误或已过期")
		return
	}

	token, user, err := ctr.svc.PhoneLogin(req.Phone)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, gin.H{
		"token": token,
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
			"phone":    user.Phone,
			"role":     user.Role,
			"avatar":   user.Avatar,
		},
	})
}

// Register 用户注册
func (ctr *AuthController) Register(c *gin.Context) {
	var req request.RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	user, err := ctr.svc.Register(req.Username, req.Email, req.Password)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Created(c, gin.H{"id": user.ID, "username": user.Username})
}

// Login 用户登录
func (ctr *AuthController) Login(c *gin.Context) {
	var req request.LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	token, user, err := ctr.svc.Login(req.Username, req.Password)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, gin.H{
		"token": token,
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
			"role":     user.Role,
			"avatar":   user.Avatar,
		},
	})
}
