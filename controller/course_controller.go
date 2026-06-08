package controller

import (
	"strconv"

	"edu_market/dto/request"
	"edu_market/model"
	"edu_market/service"
	"edu_market/utils"

	"github.com/gin-gonic/gin"
)

// CourseController 课程控制器
type CourseController struct {
	svc service.CourseService
}

// List 课程列表，支持分页、分类筛选和关键词搜索
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

// GetByID 根据ID获取课程详情
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

// Create 管理员创建课程
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

// Update 管理员更新课程信息
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

// Delete 管理员删除课程
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
