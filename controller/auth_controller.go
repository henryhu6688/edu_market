package controller

import (
	"log"

	"edu_market/dto/request"
	"edu_market/service"
	"edu_market/utils"

	"github.com/gin-gonic/gin"
)

// AuthController 认证控制器
type AuthController struct {
	svc service.AuthService
}

// GenerateCaptcha 获取图形验证码
func (ctr *AuthController) GenerateCaptcha(c *gin.Context) {
	id, b64s, err := utils.GenerateImageCaptcha()
	if err != nil {
		log.Printf("[图形验证码] 控制器层获取失败: %v", err)
		utils.InternalError(c, "图形验证码生成失败")
		return
	}
	utils.Success(c, gin.H{
		"captcha_id":    id,
		"captcha_image": b64s,
	})
}

// SendCode 发送短信验证码（需先过图形验证码）
func (ctr *AuthController) SendCode(c *gin.Context) {
	var req request.SendCodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	if !utils.VerifyImageCaptcha(req.CaptchaID, req.CaptchaCode) {
		utils.BadRequest(c, "图形验证码错误或已过期")
		return
	}

	if err := ctr.svc.SendCode(req.Phone); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "验证码已发送"})
}

// LoginByCode 统一登录/注册入口（手机号+验证码）
func (ctr *AuthController) LoginByCode(c *gin.Context) {
	var req request.LoginByCodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	if !utils.CaptchaStore.Verify(req.Phone, req.Code) {
		utils.BadRequest(c, "验证码错误或已过期")
		return
	}

	accessToken, refreshToken, user, err := ctr.svc.LoginByCode(req.Phone)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"phone":    user.Phone,
			"role":     user.Role,
			"avatar":   user.Avatar,
		},
	})
}

// Refresh 刷新 Token
func (ctr *AuthController) Refresh(c *gin.Context) {
	var req request.RefreshReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	accessToken, refreshToken, err := ctr.svc.Refresh(req.RefreshToken)
	if err != nil {
		utils.Unauthorized(c, err.Error())
		return
	}

	utils.Success(c, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}
