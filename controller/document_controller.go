package controller

import (
	"strconv"

	"edu_market/dto/request"
	"edu_market/model"
	"edu_market/service"
	"edu_market/utils"

	"github.com/gin-gonic/gin"
)

// DocumentController 文档控制器
type DocumentController struct {
	svc    *service.DocumentService
	parser *service.DocumentParser
}

// NewDocumentController 创建控制器
func NewDocumentController() *DocumentController {
	return &DocumentController{
		svc:    &service.DocumentService{},
		parser: service.NewDocumentParser(),
	}
}

// GetTree 文档目录树（公开）
func (ctr *DocumentController) GetTree(c *gin.Context) {
	mid, err := strconv.ParseUint(c.Param("mid"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}
	docs, err := ctr.svc.GetTree(uint(mid))
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, docs)
}

// GetByID 文档详情（需购买权限）
func (ctr *DocumentController) GetByID(c *gin.Context) {
	did, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}
	userID := c.GetUint("user_id")
	doc, err := ctr.svc.GetByID(uint(did), userID)
	if err != nil {
		utils.Forbidden(c, err.Error())
		return
	}
	utils.Success(c, doc)
}

// Create 新建文档（仅发布者）
func (ctr *DocumentController) Create(c *gin.Context) {
	mid, err := strconv.ParseUint(c.Param("mid"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}
	var req request.CreateDocumentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	userID := c.GetUint("user_id")
	doc := &model.Document{
		MaterialID: uint(mid), ParentID: req.ParentID,
		Title: req.Title, IsFreePreview: req.IsFreePreview,
		Status: "draft",
	}
	if err := ctr.svc.Create(doc, userID); err != nil {
		utils.Forbidden(c, err.Error())
		return
	}
	utils.Created(c, doc)
}

// Update 更新文档（仅发布者，自动保存走这个）
func (ctr *DocumentController) Update(c *gin.Context) {
	did, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}
	var req request.UpdateDocumentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	userID := c.GetUint("user_id")
	updates := make(map[string]interface{})
	if req.Title != nil {
		updates["title"] = *req.Title
	}
	if req.Content != nil {
		updates["content"] = *req.Content
	}
	if req.ParentID != nil {
		updates["parent_id"] = *req.ParentID
	}
	if req.SortOrder != nil {
		updates["sort_order"] = *req.SortOrder
	}
	if req.IsFreePreview != nil {
		updates["is_free_preview"] = *req.IsFreePreview
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if err := ctr.svc.Update(uint(did), userID, updates); err != nil {
		utils.Forbidden(c, err.Error())
		return
	}
	utils.Success(c, nil)
}

// Delete 删除文档（仅发布者）
func (ctr *DocumentController) Delete(c *gin.Context) {
	did, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}
	userID := c.GetUint("user_id")
	if err := ctr.svc.Delete(uint(did), userID); err != nil {
		utils.Forbidden(c, err.Error())
		return
	}
	utils.Success(c, nil)
}

// Upload 上传文件转文档
func (ctr *DocumentController) Upload(c *gin.Context) {
	mid, err := strconv.ParseUint(c.Param("mid"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}
	userID := c.GetUint("user_id")

	file, err := c.FormFile("file")
	if err != nil {
		utils.BadRequest(c, "请选择文件")
		return
	}
	f, err := file.Open()
	if err != nil {
		utils.InternalError(c, "读取文件失败")
		return
	}
	defer f.Close()

	content, err := ctr.parser.Parse(file.Filename, f)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	title := file.Filename
	if len([]rune(title)) > 200 {
		title = string([]rune(title)[:200])
	}
	doc := &model.Document{
		MaterialID: uint(mid), Title: title,
		Content: content, Status: "draft",
	}
	if err := ctr.svc.Create(doc, userID); err != nil {
		utils.Forbidden(c, err.Error())
		return
	}
	utils.Created(c, doc)
}
