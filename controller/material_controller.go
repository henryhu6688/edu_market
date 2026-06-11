package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"edu_market/dto/request"
	"edu_market/model"
	"edu_market/service"
	"edu_market/utils"

	"github.com/gin-gonic/gin"
)

// MaterialController 学习资料控制器
type MaterialController struct {
	svc *service.MaterialService
}

// NewMaterialController 创建控制器
func NewMaterialController() *MaterialController {
	return &MaterialController{svc: &service.MaterialService{}}
}

// List 资料列表（公开）
func (ctr *MaterialController) List(c *gin.Context) {
	var req request.MaterialListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	list, total, err := ctr.svc.List(req.Page, req.PageSize, req.CategoryID, req.Keyword, req.Status)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	req.Page, req.PageSize = service.GetPagination(req.Page, req.PageSize)
	utils.PageSuccess(c, list, total, req.Page, req.PageSize)
}

// GetByID 资料详情（公开）
func (ctr *MaterialController) GetByID(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}
	m, err := ctr.svc.GetByID(uint(id))
	if err != nil {
		utils.NotFound(c, err.Error())
		return
	}
	utils.Success(c, m)
}

// Create 发布资料（需登录，支持 multipart 上传封面图）
func (ctr *MaterialController) Create(c *gin.Context) {
	title := c.PostForm("title")
	desc := c.PostForm("description")
	catID, _ := strconv.ParseUint(c.PostForm("category_id"), 10, 64)
	price, _ := strconv.ParseFloat(c.PostForm("price"), 64)
	userID := c.GetUint("user_id")

	if title == "" {
		utils.BadRequest(c, "资料名称不能为空")
		return
	}

	coverImage := ""
	file, err := c.FormFile("cover_image")
	if err == nil {
		// 保存到 uploads/
		filename := fmt.Sprintf("covers/%d_%s", time.Now().UnixNano(), file.Filename)
		dst := filepath.Join("uploads", filename)
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err == nil {
			if err := c.SaveUploadedFile(file, dst); err == nil {
				coverImage = "/uploads/" + filename
			}
		}
	}

	m := &model.Material{
		Title: title, Description: desc, Price: price,
		CoverImage: coverImage, CategoryID: uint(catID),
		UserID: userID, Status: model.MaterialDraft,
	}
	if err := ctr.svc.Create(m); err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Created(c, m)
}

// Update 更新资料（仅发布者/admin）
func (ctr *MaterialController) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}
	var req request.UpdateMaterialReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	updates := make(map[string]interface{})
	if req.Title != "" {
		updates["title"] = req.Title
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.Price > 0 {
		updates["price"] = req.Price
	}
	if req.CoverImage != "" {
		updates["cover_image"] = req.CoverImage
	}
	if req.CategoryID > 0 {
		updates["category_id"] = req.CategoryID
	}
	if req.Status != "" {
		updates["status"] = req.Status
	}
	if err := ctr.svc.Update(uint(id), updates); err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, nil)
}

// Delete 删除资料（仅发布者/admin）
func (ctr *MaterialController) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}
	if err := ctr.svc.Delete(uint(id)); err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, nil)
}

// MyMaterials 我的资料
func (ctr *MaterialController) MyMaterials(c *gin.Context) {
	var req request.MaterialListReq
	_ = c.ShouldBindQuery(&req)
	userID := c.GetUint("user_id")
	list, total, err := ctr.svc.GetMyMaterials(userID, req.Page, req.PageSize)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	req.Page, req.PageSize = service.GetPagination(req.Page, req.PageSize)
	utils.PageSuccess(c, list, total, req.Page, req.PageSize)
}
