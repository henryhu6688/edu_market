package controller

import (
	"strconv"

	"edu-market/dto/request"
	"edu-market/model"
	"edu-market/service"
	"edu-market/utils"

	"github.com/gin-gonic/gin"
)

// CourseController 课程控制器
type CourseController struct {
	svc service.CourseService
}

// List GET /api/courses
func (ctr *CourseController) List(c *gin.Context) {
	var req request.CourseListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	courses, total, err := ctr.svc.List(req.Page, req.PageSize, req.CategoryID, req.Keyword, req.Status)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	req.Page, req.PageSize = service.GetPagination(req.Page, req.PageSize)
	utils.PageSuccess(c, courses, total, req.Page, req.PageSize)
}

// GetByID GET /api/courses/:id
func (ctr *CourseController) GetByID(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的课程ID")
		return
	}

	course, err := ctr.svc.GetByID(uint(id))
	if err != nil {
		utils.NotFound(c, err.Error())
		return
	}

	utils.Success(c, course)
}

// Create POST /api/admin/courses
func (ctr *CourseController) Create(c *gin.Context) {
	var req request.CreateCourseReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	userID := c.GetUint("user_id")
	course := &model.Course{
		Title:       req.Title,
		Description: req.Description,
		Price:       req.Price,
		CategoryID:  req.CategoryID,
		UserID:      userID,
		Status:      "draft",
	}

	if err := ctr.svc.Create(course); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Created(c, course)
}

// Update PUT /api/admin/courses/:id
func (ctr *CourseController) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的课程ID")
		return
	}

	var req request.UpdateCourseReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	updates := make(map[string]interface{})
	if req.Title != "" {
		updates["title"] = req.Title
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.Price >= 0 {
		updates["price"] = req.Price
	}
	if req.CategoryID > 0 {
		updates["category_id"] = req.CategoryID
	}
	if req.Status != "" {
		updates["status"] = req.Status
	}

	if err := ctr.svc.Update(uint(id), updates); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, nil)
}

// Delete DELETE /api/admin/courses/:id
func (ctr *CourseController) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的课程ID")
		return
	}

	if err := ctr.svc.Delete(uint(id)); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, nil)
}
