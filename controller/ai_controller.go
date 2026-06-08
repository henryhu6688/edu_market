package controller

import (
	"edu-market/dto/request"
	"edu-market/service"
	"edu-market/utils"

	"github.com/gin-gonic/gin"
)

// AIController AI 控制器
type AIController struct {
	svc service.AIService
}

// Chat POST /api/ai/chat
func (ctr *AIController) Chat(c *gin.Context) {
	var req request.ChatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	userID := c.GetUint("user_id")
	conv, err := ctr.svc.Chat(userID, req.Question)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, conv)
}

// History GET /api/ai/history
func (ctr *AIController) History(c *gin.Context) {
	var req request.HistoryReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	userID := c.GetUint("user_id")
	history, total, err := ctr.svc.GetHistory(userID, req.Page, req.PageSize)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	req.Page, req.PageSize = service.GetPagination(req.Page, req.PageSize)
	utils.PageSuccess(c, history, total, req.Page, req.PageSize)
}
