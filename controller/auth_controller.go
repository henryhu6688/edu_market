package controller

import (
	"edu-market/dto/request"
	"edu-market/service"
	"edu-market/utils"

	"github.com/gin-gonic/gin"
)

// AuthController 认证控制器
type AuthController struct {
	svc service.AuthService
}

// Register POST /api/register
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

// Login POST /api/login
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
