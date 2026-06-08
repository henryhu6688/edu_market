package controller

import (
	"strconv"

	"edu-market/service"
	"edu-market/utils"

	"github.com/gin-gonic/gin"
)

// CategoryController 分类控制器
type CategoryController struct {
	svc service.CategoryService
}

// List GET /api/categories
func (ctr *CategoryController) List(c *gin.Context) {
	categories, err := ctr.svc.List()
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, categories)
}

// Create POST /api/admin/categories
func (ctr *CategoryController) Create(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		ParentID    *uint  `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	category, err := ctr.svc.Create(req.Name, req.Description, req.ParentID)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Created(c, category)
}

// Update PUT /api/admin/categories/:id
func (ctr *CategoryController) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的分类ID")
		return
	}

	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	if err := ctr.svc.Update(uint(id), req.Name, req.Description); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, nil)
}

// Delete DELETE /api/admin/categories/:id
func (ctr *CategoryController) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的分类ID")
		return
	}

	if err := ctr.svc.Delete(uint(id)); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, nil)
}
