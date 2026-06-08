package controller

import (
	"strconv"

	"edu_market/dto/request"
	"edu_market/service"
	"edu_market/utils"

	"github.com/gin-gonic/gin"
)

// ReviewController 评论控制器
type ReviewController struct {
	svc service.ReviewService
}

// Create POST /api/reviews
func (ctr *ReviewController) Create(c *gin.Context) {
	var req request.CreateReviewReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	userID := c.GetUint("user_id")
	review, err := ctr.svc.Create(userID, req.CourseID, req.Rating, req.Content)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Created(c, review)
}

// ListByCourse GET /api/courses/:id/reviews
func (ctr *ReviewController) ListByCourse(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的课程ID")
		return
	}

	var req request.ReviewListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	reviews, total, err := ctr.svc.ListByCourse(uint(id), req.Page, req.PageSize)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	req.Page, req.PageSize = service.GetPagination(req.Page, req.PageSize)
	utils.PageSuccess(c, reviews, total, req.Page, req.PageSize)
}
