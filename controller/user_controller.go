package controller

import (
	"edu-market/dto/request"
	"edu-market/service"
	"edu-market/utils"

	"github.com/gin-gonic/gin"
)

// UserController 用户控制器
type UserController struct {
	svc service.UserService
}

// GetProfile GET /api/user/profile
func (ctr *UserController) GetProfile(c *gin.Context) {
	userID := c.GetUint("user_id")
	user, err := ctr.svc.GetProfile(userID)
	if err != nil {
		utils.NotFound(c, err.Error())
		return
	}

	utils.Success(c, user)
}

// UpdateProfile PUT /api/user/profile
func (ctr *UserController) UpdateProfile(c *gin.Context) {
	var req request.UpdateProfileReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	userID := c.GetUint("user_id")
	updates := make(map[string]interface{})
	if req.Username != "" {
		updates["username"] = req.Username
	}
	if req.Email != "" {
		updates["email"] = req.Email
	}
	if req.Avatar != "" {
		updates["avatar"] = req.Avatar
	}

	user, err := ctr.svc.UpdateProfile(userID, updates)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, user)
}
