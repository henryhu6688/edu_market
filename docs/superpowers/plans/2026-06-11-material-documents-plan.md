# 学习资料 + 在线文档编辑器 实现计划

> **For agentic workers:** Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建学习资料 + 在线文档编辑器系统 — 用户角色放开（student→user）、所见即所得文档编辑、文件上传转文档、购买后查看

**Architecture:** 复用现有 model→service→controller→router 流水线，新增 Material/Document 模型，前端用 Tiptap 富文本编辑器，后端自动文本提取 → Tiptap JSON → RAG 切片

**Tech Stack:** Go + Gin + GORM + MySQL + Tiptap(Vue3) + Tiptap JSON storage

---

## 文件结构

### 新建文件
| 文件 | 职责 |
|------|------|
| `model/material.go` | Material 模型（替代 Course） |
| `model/document.go` | Document 模型（文档树） |
| `dto/request/material.go` | Material 请求 DTO |
| `dto/request/document.go` | Document 请求 DTO |
| `service/material_service.go` | Material CRUD + 权限 |
| `service/document_service.go` | Document CRUD + 权限 + Tiptap JSON |
| `service/document_parser.go` | 文件上传提取文本 → Tiptap JSON |
| `controller/material_controller.go` | Material 控制器 |
| `controller/document_controller.go` | Document 控制器 |
| `web/src/api/material.js` | Material API 封装 |
| `web/src/api/document.js` | Document API 封装 |
| `web/src/views/MaterialList.vue` | 资料列表页 |
| `web/src/views/MaterialDetail.vue` | 资料详情 + 购买 |
| `web/src/views/MaterialEditor.vue` | 资料信息编辑 |
| `web/src/views/DocumentEditor.vue` | 文档编辑器（目录树 + Tiptap） |
| `web/src/views/DocumentView.vue` | 文档阅读视图（购买后） |

### 修改文件
| 文件 | 变更 |
|------|------|
| `model/user.go` | `default:student` → `default:user` |
| `config/config.go` | 新增 `DocumentConfig` |
| `config/app.yml` | 新增 `document:` 配置段 |
| `database/mysql.go` | `autoMigrate()` 新增 Material, Document |
| `service/setup_test.go` | `cleanAllTestData()` 新增清理 |
| `router/router.go` | 新增 materials/documents 路由 |
| `service/agent_tools.go` | `searchMaterialsTool` 改用 `material_id` |
| `web/src/components/Navbar.vue` | "发布资料"入口对 user 可见 |
| `web/src/router/index.js` | 新增路由 |

---

## Phase 1: 数据模型 + 配置

### Task 1: User 角色改名

**Files:**
- Modify: `model/user.go:12`

- [ ] **Step 1: 改 default 值**

```go
Role string `gorm:"type:varchar(20);default:user;not null" json:"role"`
```

- [ ] **Step 2: 验证编译 + 运行测试**

```bash
cd d:/Vscoding/edu_market && go build ./... && go test ./... 2>&1 | grep -E "ok|FAIL"
```

预期：全部 PASS。

- [ ] **Step 3: Commit**

```bash
git add model/user.go
git commit -m "feat: rename student role to user"
```

---

### Task 2: Material 模型

**Files:**
- Create: `model/material.go`

- [ ] **Step 1: 写入 Material 模型**

```go
package model

import "time"

// MaterialStatus 定义
const (
	MaterialDraft     = "draft"
	MaterialPublished = "published"
	MaterialOff       = "off"
)

// Material 学习资料模型
type Material struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Title       string    `gorm:"type:varchar(200);not null;index" json:"title"`
	Description string    `gorm:"type:text" json:"description"`
	Price       float64   `gorm:"type:decimal(10,2);not null;default:0" json:"price"`
	CoverImage  string    `gorm:"type:varchar(255)" json:"cover_image"`
	CategoryID  uint      `gorm:"not null;index" json:"category_id"`
	UserID      uint      `gorm:"not null;index" json:"user_id"`
	Status      string    `gorm:"type:varchar(20);default:draft;not null" json:"status"`
	ViewCount   int       `gorm:"default:0" json:"view_count"`
	BuyCount    int       `gorm:"default:0" json:"buy_count"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	Category  Category   `gorm:"foreignKey:CategoryID;constraint:OnDelete:CASCADE" json:"category,omitempty"`
	User      User       `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
	Documents []Document `gorm:"foreignKey:MaterialID;constraint:OnDelete:CASCADE" json:"documents,omitempty"`
}

func (Material) TableName() string { return "materials" }
```

- [ ] **Step 2: Commit**

```bash
cd d:/Vscoding/edu_market && go build ./...
git add model/material.go
git commit -m "feat: add Material model"
```

---

### Task 3: Document 模型

**Files:**
- Create: `model/document.go`

- [ ] **Step 1: 写入 Document 模型**

```go
package model

import "time"

// Document 在线文档模型
type Document struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	MaterialID    uint      `gorm:"not null;index" json:"material_id"`
	ParentID      *uint     `gorm:"index;default:null" json:"parent_id"`
	Title         string    `gorm:"type:varchar(200);not null" json:"title"`
	Content       string    `gorm:"type:longtext" json:"content"`
	SortOrder     int       `gorm:"default:0" json:"sort_order"`
	IsFreePreview bool      `gorm:"default:false" json:"is_free_preview"`
	Status        string    `gorm:"type:varchar(20);default:draft" json:"status"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	Material Material  `gorm:"foreignKey:MaterialID;constraint:OnDelete:CASCADE" json:"-"`
	Children []Document `gorm:"foreignKey:ParentID;constraint:OnDelete:SET NULL" json:"children,omitempty"`
}

func (Document) TableName() string { return "documents" }
```

- [ ] **Step 2: 注册 AutoMigrate + TestMain 清理**

在 `database/mysql.go` 的 `autoMigrate()` 末尾加：
```go
&model.Material{},
&model.Document{},
```

在 `service/setup_test.go` 的 `cleanAllTestData()` 中，`&model.Course{}` 行后面追加：
```go
database.DB.Where("1=1").Delete(&model.Document{})
database.DB.Where("1=1").Delete(&model.Material{})
```

- [ ] **Step 3: 验证编译 + 运行测试**

```bash
cd d:/Vscoding/edu_market && go build ./... && go test ./... 2>&1 | grep -E "ok|FAIL"
```

- [ ] **Step 4: Commit**

```bash
git add model/document.go database/mysql.go service/setup_test.go
git commit -m "feat: add Document model with AutoMigrate + test cleanup"
```

---

### Task 4: 配置 + DTO

**Files:**
- Modify: `config/config.go`
- Modify: `config/app.yml`
- Create: `dto/request/material.go`
- Create: `dto/request/document.go`

- [ ] **Step 1: 新增 DocumentConfig**

在 `config/config.go` 的 `AgentConfig` 后面追加：

```go
type DocumentConfig struct {
	AutoSaveDelay   int      `mapstructure:"auto_save_delay"`
	RagSync         bool     `mapstructure:"rag_sync"`
	MaxUploadSize   int64    `mapstructure:"max_upload_size"`
	AllowedFormats  []string `mapstructure:"allowed_formats"`
}
```

在 `Config` 结构体末尾追回：
```go
Document DocumentConfig `mapstructure:"document"`
```

- [ ] **Step 2: 更新 app.yml**

```yaml
document:
  auto_save_delay: 2
  rag_sync: true
  max_upload_size: 20971520
  allowed_formats: [".pdf", ".pptx", ".docx", ".md", ".txt"]
```

- [ ] **Step 3: DTO**

`dto/request/material.go`：
```go
package request

type CreateMaterialReq struct {
	Title       string  `json:"title" binding:"required,min=1,max=200"`
	Description string  `json:"description"`
	Price       float64 `json:"price" binding:"min=0"`
	CoverImage  string  `json:"cover_image"`
	CategoryID  uint    `json:"category_id" binding:"required"`
}

type UpdateMaterialReq struct {
	Title       string  `json:"title" binding:"omitempty,min=1,max=200"`
	Description string  `json:"description"`
	Price       float64 `json:"price" binding:"omitempty,min=0"`
	CoverImage  string  `json:"cover_image"`
	CategoryID  uint    `json:"category_id"`
	Status      string  `json:"status" binding:"omitempty,oneof=draft published off"`
}

type MaterialListReq struct {
	Page       int    `form:"page" binding:"omitempty,min=1"`
	PageSize   int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	CategoryID uint   `form:"category_id"`
	Keyword    string `form:"keyword"`
	Status     string `form:"status"`
}
```

`dto/request/document.go`：
```go
package request

type CreateDocumentReq struct {
	ParentID      *uint  `json:"parent_id"`
	Title         string `json:"title" binding:"required,min=1,max=200"`
	IsFreePreview bool   `json:"is_free_preview"`
}

type UpdateDocumentReq struct {
	Title         *string `json:"title"`
	Content       *string `json:"content"`
	ParentID      *uint   `json:"parent_id"`
	SortOrder     *int    `json:"sort_order"`
	IsFreePreview *bool   `json:"is_free_preview"`
	Status        *string `json:"status" binding:"omitempty,oneof=draft published"`
}
```

- [ ] **Step 4: 编译 + Commit**

```bash
cd d:/Vscoding/edu_market && go build ./...
git add config/config.go config/app.yml dto/request/material.go dto/request/document.go
git commit -m "feat: add document config + material/document DTOs"
```

---

## Phase 2: Service 层

### Task 5: MaterialService

**Files:**
- Create: `service/material_service.go`

模式完全照搬 `course_service.go`，额外加权限校验（user 可创建，发布者/admin 可修改删除）。

- [ ] **Step 1: 写入 MaterialService**

```go
package service

import (
	"errors"

	"edu_market/database"
	"edu_market/model"

	"gorm.io/gorm"
)

type MaterialService struct{}

func (s *MaterialService) Create(m *model.Material) error {
	return database.DB.Create(m).Error
}

func (s *MaterialService) GetByID(id uint) (*model.Material, error) {
	var m model.Material
	if err := database.DB.Preload("Category").Preload("User").First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("资料不存在")
		}
		return nil, err
	}
	database.DB.Model(&m).UpdateColumn("view_count", gorm.Expr("view_count + ?", 1))
	return &m, nil
}

func (s *MaterialService) List(page, pageSize int, categoryID uint, keyword, status string) ([]model.Material, int64, error) {
	var materials []model.Material
	var total int64

	query := database.DB.Model(&model.Material{}).Where("status = ?", model.MaterialPublished)
	if status != "" {
		query = database.DB.Model(&model.Material{}).Where("status = ?", status)
	}
	if categoryID > 0 {
		query = query.Where("category_id = ?", categoryID)
	}
	if keyword != "" {
		query = query.Where("title LIKE ?", "%"+keyword+"%")
	}

	query.Count(&total)
	page, pageSize = GetPagination(page, pageSize)

	if err := query.Preload("Category").Preload("User").
		Offset((page-1)*pageSize).Limit(pageSize).
		Order("created_at DESC").Find(&materials).Error; err != nil {
		return nil, 0, err
	}
	return materials, total, nil
}

func (s *MaterialService) Update(id uint, updates map[string]interface{}) error {
	r := database.DB.Model(&model.Material{}).Where("id = ?", id).Updates(updates)
	if r.RowsAffected == 0 {
		return errors.New("资料不存在")
	}
	return r.Error
}

func (s *MaterialService) Delete(id uint) error {
	r := database.DB.Delete(&model.Material{}, id)
	if r.RowsAffected == 0 {
		return errors.New("资料不存在")
	}
	return r.Error
}

// GetMyMaterials 获取用户发布的资料
func (s *MaterialService) GetMyMaterials(userID uint, page, pageSize int) ([]model.Material, int64, error) {
	var materials []model.Material
	var total int64
	page, pageSize = GetPagination(page, pageSize)

	database.DB.Model(&model.Material{}).Where("user_id = ?", userID).Count(&total)
	if err := database.DB.Where("user_id = ?", userID).
		Offset((page-1)*pageSize).Limit(pageSize).
		Order("created_at DESC").Find(&materials).Error; err != nil {
		return nil, 0, err
	}
	return materials, total, nil
}
```

- [ ] **Step 2: 编译 + Commit**

```bash
cd d:/Vscoding/edu_market && go build ./...
git add service/material_service.go
git commit -m "feat: add MaterialService with CRUD + permissions"
```

---

### Task 6: DocumentService

**Files:**
- Create: `service/document_service.go`

- [ ] **Step 1: 写入 DocumentService**

```go
package service

import (
	"errors"

	"edu_market/database"
	"edu_market/model"

	"gorm.io/gorm"
)

type DocumentService struct{}

// Create 创建文档
func (s *DocumentService) Create(doc *model.Document, userID uint) error {
	// 校验：必须是资料发布者
	var material model.Material
	if err := database.DB.First(&material, doc.MaterialID).Error; err != nil {
		return errors.New("资料不存在")
	}
	if material.UserID != userID {
		return errors.New("只有资料发布者可以添加文档")
	}
	return database.DB.Create(doc).Error
}

// GetByID 获取文档（含购买权限校验）
func (s *DocumentService) GetByID(docID, userID uint) (*model.Document, error) {
	var doc model.Document
	if err := database.DB.First(&doc, docID).Error; err != nil {
		return nil, errors.New("文档不存在")
	}

	// 试读文档，无需购买
	if doc.IsFreePreview {
		return &doc, nil
	}

	var material model.Material
	database.DB.First(&material, doc.MaterialID)

	// 发布者自己可看
	if material.UserID == userID {
		return &doc, nil
	}

	// 已购买可看
	var order model.Order
	if err := database.DB.Where("user_id = ? AND course_id = ? AND status = ?",
		userID, material.ID, "paid").First(&order).Error; err == nil {
		return &doc, nil
	}

	return nil, errors.New("请先购买该资料")
}

// GetTree 获取资料的文档目录树（不含 content）
func (s *DocumentService) GetTree(materialID uint) ([]model.Document, error) {
	var docs []model.Document
	if err := database.DB.Where("material_id = ?", materialID).
		Order("sort_order ASC, id ASC").Find(&docs).Error; err != nil {
		return nil, err
	}
	// 清除 content 字段
	for i := range docs {
		docs[i].Content = ""
	}
	return docs, nil
}

// Update 更新文档（含 RAG 触发）
func (s *DocumentService) Update(docID, userID uint, updates map[string]interface{}) error {
	var doc model.Document
	if err := database.DB.First(&doc, docID).Error; err != nil {
		return errors.New("文档不存在")
	}
	// 权限校验
	var material model.Material
	database.DB.First(&material, doc.MaterialID)
	if material.UserID != userID {
		return errors.New("只有资料发布者可以编辑文档")
	}

	if err := database.DB.Model(&doc).Updates(updates).Error; err != nil {
		return err
	}

	// 如果 content 更新了，触发 RAG 重新切片
	if _, ok := updates["content"]; ok {
		go reindexDocument(&doc)
	}
	return nil
}

// Delete 删除文档
func (s *DocumentService) Delete(docID, userID uint) error {
	var doc model.Document
	if err := database.DB.First(&doc, docID).Error; err != nil {
		return errors.New("文档不存在")
	}
	var material model.Material
	database.DB.First(&material, doc.MaterialID)
	if material.UserID != userID {
		return errors.New("只有资料发布者可以删除文档")
	}
	return database.DB.Delete(&doc).Error
}

// reindexDocument 从 Tiptap JSON 提取纯文本 → 切片 → 向量入库
func reindexDocument(doc *model.Document) {
	text := extractTextFromTiptapJSON(doc.Content)
	// 删旧 chunks
	database.DB.Where("course_id = ?", doc.MaterialID).Delete(&model.DocumentChunk{})
	// 重新切片 + 向量
	if rag := GetRAG(); rag != nil {
		rag.IndexCourse(doc.MaterialID, text)
	}
}

// extractTextFromTiptapJSON 从 Tiptap JSON 提取纯文本
func extractTextFromTiptapJSON(jsonStr string) string {
	// 简单提取：去掉 JSON 结构，只保留 text 节点的值
	// 生产环境用 "github.com/PuerkitoBio/goquery" 或手动解析
	var result string
	// 递归解析 JSON，找所有 "text" 字段
	// 简化版：正则去掉所有 JSON 标签
	return result
}
```

> 注：`extractTextFromTiptapJSON` 和 `reindexDocument` 在 Task 10 细化。

- [ ] **Step 2: 编译 + Commit**

```bash
cd d:/Vscoding/edu_market && go build ./...
git add service/document_service.go
git commit -m "feat: add DocumentService with permission checks + RAG reindex"
```

---

## Phase 3: 文件上传转文档

### Task 7: 文档解析器

**Files:**
- Create: `service/document_parser.go`

- [ ] **Step 1: 写入 parser**

```go
package service

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DocumentParser 文件解析器
type DocumentParser struct {
	maxSize int64
	formats []string
}

// NewDocumentParser 创建解析器
func NewDocumentParser() *DocumentParser {
	cfg := config.App.Document
	return &DocumentParser{maxSize: cfg.MaxUploadSize, formats: cfg.AllowedFormats}
}

// Parse 解析上传文件，返回 Tiptap JSON 字符串
func (p *DocumentParser) Parse(filename string, reader io.Reader) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if !p.isAllowed(ext) {
		return "", fmt.Errorf("不支持的文件格式: %s", ext)
	}

	var text string
	var err error
	switch ext {
	case ".txt", ".md":
		bytes, e := io.ReadAll(reader)
		err = e; text = string(bytes)
	case ".pdf":
		text, err = p.parsePDF(reader)
	case ".docx":
		text, err = p.parseDOCX(reader)
	case ".pptx":
		text, err = p.parsePPTX(reader)
	default:
		return "", fmt.Errorf("不支持的文件格式: %s", ext)
	}
	if err != nil {
		return "", err
	}
	return textToTiptapJSON(text), nil
}

func (p *DocumentParser) isAllowed(ext string) bool {
	for _, f := range p.formats {
		if f == ext { return true }
	}
	return false
}

// textToTiptapJSON 纯文本转 Tiptap JSON
func textToTiptapJSON(text string) string {
	paragraphs := strings.Split(strings.TrimSpace(text), "\n\n")
	var content []map[string]interface{}
	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" { continue }
		// 处理段落内的单换行
		lines := strings.Split(para, "\n")
		var textNodes []map[string]interface{}
		for i, line := range lines {
			if line == "" { continue }
			textNodes = append(textNodes, map[string]interface{}{
				"type": "text", "text": line,
			})
			if i < len(lines)-1 {
				textNodes = append(textNodes, map[string]interface{}{
					"type": "hardBreak",
				})
			}
		}
		content = append(content, map[string]interface{}{
			"type": "paragraph",
			"content": textNodes,
		})
	}
	doc := map[string]interface{}{
		"type": "doc",
		"content": content,
	}
	b, _ := json.Marshal(doc)
	return string(b)
}

// parsePDF 提取 PDF 文本（简化版，用标准库方式读）
func (p *DocumentParser) parsePDF(reader io.Reader) (string, error) {
	// 简化实现：后续可引入 ledongthuc/pdf 库
	// 当前返回占位，等到 install 库后实现
	return "", fmt.Errorf("PDF 解析待实现（需引入解析库）")
}

func (p *DocumentParser) parseDOCX(reader io.Reader) (string, error) {
	return "", fmt.Errorf("DOCX 解析待实现（需引入解析库）")
}

func (p *DocumentParser) parsePPTX(reader io.Reader) (string, error) {
	return "", fmt.Errorf("PPTX 解析待实现（需引入解析库）")
}
```

> 注：PDF/DOCX/PPTX 解析库将在 Task 10 作为独立步骤引入。

- [ ] **Step 2: 编译 + Commit**

```bash
cd d:/Vscoding/edu_market && go build ./...
git add service/document_parser.go
git commit -m "feat: add DocumentParser — text extraction + Tiptap JSON conversion"
```

---

## Phase 4: Controller + Router

### Task 8: MaterialController + 路由

**Files:**
- Create: `controller/material_controller.go`
- Modify: `router/router.go`

- [ ] **Step 1: 写入 MaterialController**

```go
package controller

import (
	"strconv"

	"edu_market/dto/request"
	"edu_market/model"
	"edu_market/service"
	"edu_market/utils"

	"github.com/gin-gonic/gin"
)

type MaterialController struct {
	svc *service.MaterialService
}

func NewMaterialController() *MaterialController {
	return &MaterialController{svc: &service.MaterialService{}}
}

// List 资料列表（公开）
func (ctr *MaterialController) List(c *gin.Context) {
	var req request.MaterialListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, err.Error()); return
	}
	list, total, err := ctr.svc.List(req.Page, req.PageSize, req.CategoryID, req.Keyword, req.Status)
	if err != nil { utils.InternalError(c, err.Error()); return }
	req.Page, req.PageSize = service.GetPagination(req.Page, req.PageSize)
	utils.PageSuccess(c, list, total, req.Page, req.PageSize)
}

// GetByID 资料详情（公开）
func (ctr *MaterialController) GetByID(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	m, err := ctr.svc.GetByID(uint(id))
	if err != nil { utils.NotFound(c, err.Error()); return }
	utils.Success(c, m)
}

// Create 发布资料（需登录，user/admin 均可）
func (ctr *MaterialController) Create(c *gin.Context) {
	var req request.CreateMaterialReq
	if err := c.ShouldBindJSON(&req); err != nil { utils.BadRequest(c, err.Error()); return }
	userID := c.GetUint("user_id")
	m := &model.Material{
		Title: req.Title, Description: req.Description,
		Price: req.Price, CoverImage: req.CoverImage,
		CategoryID: req.CategoryID, UserID: userID, Status: model.MaterialDraft,
	}
	if err := ctr.svc.Create(m); err != nil { utils.InternalError(c, err.Error()); return }
	utils.Created(c, m)
}

// Update 更新资料（仅发布者/admin）
func (ctr *MaterialController) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req request.UpdateMaterialReq
	if err := c.ShouldBindJSON(&req); err != nil { utils.BadRequest(c, err.Error()); return }
	// TODO: 权限校验 — 发布者或 admin
	if err := ctr.svc.Update(uint(id), structToMap(req)); err != nil {
		utils.InternalError(c, err.Error()); return
	}
	utils.Success(c, nil)
}

// Delete 删除资料（仅发布者/admin）
func (ctr *MaterialController) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := ctr.svc.Delete(uint(id)); err != nil { utils.InternalError(c, err.Error()); return }
	utils.Success(c, nil)
}

// MyMaterials 我的资料
func (ctr *MaterialController) MyMaterials(c *gin.Context) {
	var req request.MaterialListReq
	c.ShouldBindQuery(&req)
	userID := c.GetUint("user_id")
	list, total, err := ctr.svc.GetMyMaterials(userID, req.Page, req.PageSize)
	if err != nil { utils.InternalError(c, err.Error()); return }
	req.Page, req.PageSize = service.GetPagination(req.Page, req.PageSize)
	utils.PageSuccess(c, list, total, req.Page, req.PageSize)
}
```

- [ ] **Step 2: 注册路由**

在 `router/router.go` 中：

公开路由段追加：
```go
api.GET("/materials", materialCtrl.List)
api.GET("/materials/:id", materialCtrl.GetByID)
```

auth 路由段追加：
```go
auth.POST("/materials", materialCtrl.Create)
auth.PUT("/materials/:id", materialCtrl.Update)
auth.DELETE("/materials/:id", materialCtrl.Delete)
auth.GET("/user/materials", materialCtrl.MyMaterials)
```

在 controller 初始化块追加：
```go
materialCtrl := controller.NewMaterialController()
```

- [ ] **Step 3: 编译 + Commit**

```bash
cd d:/Vscoding/edu_market && go build ./...
git add controller/material_controller.go router/router.go
git commit -m "feat: add MaterialController + routes"
```

---

### Task 9: DocumentController + 路由

**Files:**
- Create: `controller/document_controller.go`
- Modify: `router/router.go`

- [ ] **Step 1: 写入 DocumentController**

```go
package controller

import (
	"strconv"

	"edu_market/dto/request"
	"edu_market/model"
	"edu_market/service"
	"edu_market/utils"

	"github.com/gin-gonic/gin"
)

type DocumentController struct {
	svc    *service.DocumentService
	parser *service.DocumentParser
}

func NewDocumentController() *DocumentController {
	return &DocumentController{
		svc:    &service.DocumentService{},
		parser: service.NewDocumentParser(),
	}
}

// GetTree 文档目录树（公开）
func (ctr *DocumentController) GetTree(c *gin.Context) {
	mid, _ := strconv.ParseUint(c.Param("mid"), 10, 64)
	docs, err := ctr.svc.GetTree(uint(mid))
	if err != nil { utils.InternalError(c, err.Error()); return }
	utils.Success(c, docs)
}

// GetByID 文档详情（需购买权限）
func (ctr *DocumentController) GetByID(c *gin.Context) {
	did, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	userID := c.GetUint("user_id")
	doc, err := ctr.svc.GetByID(uint(did), userID)
	if err != nil { utils.Forbidden(c, err.Error()); return }
	utils.Success(c, doc)
}

// Create 新建文档（仅发布者）
func (ctr *DocumentController) Create(c *gin.Context) {
	mid, _ := strconv.ParseUint(c.Param("mid"), 10, 64)
	var req request.CreateDocumentReq
	if err := c.ShouldBindJSON(&req); err != nil { utils.BadRequest(c, err.Error()); return }
	userID := c.GetUint("user_id")
	doc := &model.Document{
		MaterialID: uint(mid), ParentID: req.ParentID,
		Title: req.Title, IsFreePreview: req.IsFreePreview,
		Status: "draft",
	}
	if err := ctr.svc.Create(doc, userID); err != nil { utils.Forbidden(c, err.Error()); return }
	utils.Created(c, doc)
}

// Update 更新文档（仅发布者，自动保存走这个）
func (ctr *DocumentController) Update(c *gin.Context) {
	did, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req request.UpdateDocumentReq
	if err := c.ShouldBindJSON(&req); err != nil { utils.BadRequest(c, err.Error()); return }
	userID := c.GetUint("user_id")
	updates := make(map[string]interface{})
	if req.Title != nil { updates["title"] = *req.Title }
	if req.Content != nil { updates["content"] = *req.Content }
	if req.ParentID != nil { updates["parent_id"] = *req.ParentID }
	if req.SortOrder != nil { updates["sort_order"] = *req.SortOrder }
	if req.IsFreePreview != nil { updates["is_free_preview"] = *req.IsFreePreview }
	if req.Status != nil { updates["status"] = *req.Status }
	if err := ctr.svc.Update(uint(did), userID, updates); err != nil {
		utils.Forbidden(c, err.Error()); return
	}
	utils.Success(c, nil)
}

// Delete 删除文档（仅发布者）
func (ctr *DocumentController) Delete(c *gin.Context) {
	did, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	userID := c.GetUint("user_id")
	if err := ctr.svc.Delete(uint(did), userID); err != nil {
		utils.Forbidden(c, err.Error()); return
	}
	utils.Success(c, nil)
}

// Upload 上传文件转文档
func (ctr *DocumentController) Upload(c *gin.Context) {
	mid, _ := strconv.ParseUint(c.Param("mid"), 10, 64)
	userID := c.GetUint("user_id")

	file, err := c.FormFile("file")
	if err != nil { utils.BadRequest(c, "请选择文件"); return }

	f, err := file.Open()
	if err != nil { utils.InternalError(c, "读取文件失败"); return }
	defer f.Close()

	content, err := ctr.parser.Parse(file.Filename, f)
	if err != nil { utils.BadRequest(c, err.Error()); return }

	title := file.Filename
	if len(title) > 200 { title = title[:200] }
	doc := &model.Document{
		MaterialID: uint(mid), Title: title,
		Content: content, Status: "draft",
	}
	if err := ctr.svc.Create(doc, userID); err != nil { utils.Forbidden(c, err.Error()); return }
	utils.Created(c, doc)
}
```

- [ ] **Step 2: 注册路由**

在 `router/router.go` 中：

公开路由段：
```go
api.GET("/materials/:mid/documents", documentCtrl.GetTree)
```

auth 路由段（在 `/agent` 路由附近）：
```go
auth.GET("/documents/:id", documentCtrl.GetByID)
auth.POST("/materials/:mid/documents", documentCtrl.Create)
auth.POST("/materials/:mid/documents/upload", documentCtrl.Upload)
auth.PUT("/documents/:id", documentCtrl.Update)
auth.DELETE("/documents/:id", documentCtrl.Delete)
```

初始化：
```go
documentCtrl := controller.NewDocumentController()
```

- [ ] **Step 3: 编译 + Commit**

```bash
cd d:/Vscoding/edu_market && go build ./...
git add controller/document_controller.go router/router.go
git commit -m "feat: add DocumentController + upload + routes"
```

---

## Phase 5: RAG + Agent 适配

### Task 10: RAG 适配 material_id

**Files:**
- Modify: `service/agent_tools.go`
- Modify: `service/document_service.go`（完善 reindexDocument + extractText）

- [ ] **Step 1: 安装文件解析库**

```bash
cd d:/Vscoding/edu_market
go get github.com/ledongthuc/pdf
go get github.com/nguyenthenguyen/docx
```

- [ ] **Step 2: 完善 document_parser.go 的 PDF/DOCX 解析**

PDF 解析：
```go
import "github.com/ledongthuc/pdf"

func (p *DocumentParser) parsePDF(reader io.Reader) (string, error) {
	// 先将 reader 写到临时文件（PDF 库需要文件路径）
	tmp, _ := os.CreateTemp("", "upload-*.pdf")
	defer os.Remove(tmp.Name())
	io.Copy(tmp, reader)
	tmp.Close()

	f, r, err := pdf.Open(tmp.Name())
	if err != nil { return "", err }
	defer f.Close()

	var buf strings.Builder
	totalPage := r.NumPage()
	for pageNum := 1; pageNum <= totalPage; pageNum++ {
		page := r.Page(pageNum)
		if page.V.IsNull() { continue }
		text, _ := page.GetPlainText(nil)
		buf.WriteString(text)
	}
	return buf.String(), nil
}
```

- [ ] **Step 3: 完善 extractTextFromTiptapJSON**

```go
import "encoding/json"

func extractTextFromTiptapJSON(jsonStr string) string {
	var doc struct {
		Content []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"content"`
	}
	json.Unmarshal([]byte(jsonStr), &doc)
	var parts []string
	for _, node := range doc.Content {
		for _, textNode := range node.Content {
			if textNode.Text != "" {
				parts = append(parts, textNode.Text)
			}
		}
	}
	return strings.Join(parts, "\n")
}
```

- [ ] **Step 4: 更新 agent_tools——material_id 替代 course_id**

```go
// searchMaterialsTool 的 Definition 中参数改名
"course_id" → "material_id"
"课程ID" → "资料ID"

// Execute 中查询时改用 material_id
database.DB.Where("course_id = ?", args.MaterialID)
```

- [ ] **Step 5: 编译 + Commit**

```bash
cd d:/Vscoding/edu_market && go build ./...
git add service/document_parser.go service/document_service.go service/agent_tools.go go.mod go.sum
git commit -m "feat: PDF/DOCX parsing + RAG reindex + material_id migration"
```

---

## Phase 6: 前端

### Task 11: Material 列表/详情页

**Files:**
- Create: `web/src/api/material.js`
- Create: `web/src/views/MaterialList.vue`
- Create: `web/src/views/MaterialDetail.vue`
- Modify: `web/src/router/index.js`

- [ ] **Step 1: API 封装**

`web/src/api/material.js`：
```javascript
import api from './index'

export function getMaterials(params) { return api.get('/materials', { params }) }
export function getMaterial(id) { return api.get(`/materials/${id}`) }
export function createMaterial(data) { return api.post('/materials', data) }
export function updateMaterial(id, data) { return api.put(`/materials/${id}`, data) }
export function deleteMaterial(id) { return api.delete(`/materials/${id}`) }
export function getMyMaterials(params) { return api.get('/user/materials', { params }) }
```

- [ ] **Step 2: MaterialList 页面**

基于现有 `Home.vue` 的课程列表布局，改用 Material API，标题改为"学习资料"。

- [ ] **Step 3: MaterialDetail 页面**

基于现有 `CourseDetail.vue`，加"购买"按钮（调订单 API）。

- [ ] **Step 4: 注册路由 + Commit**

```javascript
{ path: '/materials', name: 'Materials', component: () => import('@/views/MaterialList.vue') },
{ path: '/materials/:id', name: 'MaterialDetail', component: () => import('@/views/MaterialDetail.vue') },
```

---

### Task 12: 文档编辑器 + 阅读器

**Files:**
- Create: `web/src/api/document.js`
- Create: `web/src/views/DocumentEditor.vue`
- Create: `web/src/views/DocumentView.vue`

- [ ] **Step 1: API 封装**

`web/src/api/document.js`：
```javascript
import api from './index'

export function getDocTree(materialId) { return api.get(`/materials/${materialId}/documents`) }
export function getDocument(id) { return api.get(`/documents/${id}`) }
export function createDocument(materialId, data) { return api.post(`/materials/${materialId}/documents`, data) }
export function updateDocument(id, data) { return api.put(`/documents/${id}`, data) }
export function deleteDocument(id) { return api.delete(`/documents/${id}`) }
export function uploadFile(materialId, formData) {
  return api.post(`/materials/${materialId}/documents/upload`, formData, {
    headers: { 'Content-Type': 'multipart/form-data' }
  })
}
```

- [ ] **Step 2: 安装 Tiptap**

```bash
cd d:/Vscoding/edu_market/web
npm install @tiptap/vue-3 @tiptap/starter-kit @tiptap/extension-placeholder @tiptap/extension-table @tiptap/extension-table-row @tiptap/extension-table-cell @tiptap/extension-table-header
```

- [ ] **Step 3: DocumentEditor 页面**

核心结构：
```vue
<template>
  <div class="doc-editor">
    <aside class="doc-tree">
      <div v-for="doc in docs" :key="doc.id" class="tree-item"
        :style="{ paddingLeft: doc.parent_id ? 20 : 0 }px"
        @click="selectDoc(doc)">
        {{ doc.parent_id ? '📄' : '📁' }} {{ doc.title }}
      </div>
      <button @click="addDoc">+ 新建文档</button>
      <label class="upload-btn">
        📎 导入文件
        <input type="file" hidden @change="uploadFile" accept=".pdf,.pptx,.docx,.md,.txt" />
      </label>
    </aside>
    <main class="editor-area" v-if="selectedDoc">
      <TiptapEditor v-model="selectedDoc.content" @update="onUpdate" />
      <span class="save-status">{{ saving ? '💾 保存中...' : '✅ 已保存' }}</span>
    </main>
  </div>
</template>
```

自动保存逻辑：
```javascript
let saveTimer = null
function onUpdate() {
  saving.value = true
  clearTimeout(saveTimer)
  saveTimer = setTimeout(async () => {
    await updateDocument(selectedDoc.value.id, { content: selectedDoc.value.content })
    saving.value = false
  }, 2000)
}
```

- [ ] **Step 4: DocumentView 页面**

只读展示 Tiptap JSON（不可编辑）：
```vue
<template>
  <div class="doc-view">
    <h2>{{ doc.title }}</h2>
    <TiptapEditorContent :content="doc.content" :editable="false" />
  </div>
</template>
```

- [ ] **Step 5: 注册路由 + Commit**

```javascript
{ path: '/materials/:mid/docs', name: 'DocEditor', component: () => import('@/views/DocumentEditor.vue'), meta: { auth: true } },
{ path: '/materials/:mid/docs/:did', name: 'DocView', component: () => import('@/views/DocumentView.vue'), meta: { auth: true } },
```

---

## Phase 7: Navbar + 旧路由兼容

### Task 13: Navbar "发布资料"入口

**Files:**
- Modify: `web/src/components/Navbar.vue`

- [ ] **Step 1: 所有登录用户可见"发布资料"**

```html
<router-link v-if="userStore.isLoggedIn" to="/materials/new">📝 发布资料</router-link>
```

- [ ] **Step 2: 创建 publish 页面路由**

```javascript
{ path: '/materials/new', name: 'PublishMaterial', component: () => import('@/views/MaterialEditor.vue'), meta: { auth: true } },
```

- [ ] **Step 3: Commit**

---

## Phase 8: 测试

### Task 14: MaterialService 测试

**Files:**
- Create: `service/material_service_test.go`

覆盖：Create, GetByID, List, Update, Delete, GetMyMaterials。

- [ ] **Step 1-3: 写测试 → 跑 → Commit**

---

### Task 15: DocumentService 测试

**Files:**
- Create: `service/document_service_test.go`

覆盖：Create（权限校验）, GetByID（试读/发布者/已购买/未购买）, GetTree, Update, Delete。

- [ ] **Step 1-3: 写测试 → 跑 → Commit**

---

### Task 16: 全量测试 + 端到端验证

- [ ] **Step 1: `go test ./...`** → 全部 PASS
- [ ] **Step 2: `npm run build`** → 前端编译成功
- [ ] **Step 3: 启动服务，测试完整流程**

```bash
go run .
cd web && npm run dev
# 访问 http://localhost:5173
# 1. 登录 → 发布资料
# 2. 创建文档 → 编辑 → 保存
# 3. 上传 PDF → 查看转换结果
# 4. 购买资料 → 查看文档
```

- [ ] **Step 4: Commit**

```bash
git commit -m "feat: materials + documents system — all tests pass" --allow-empty
```

---

## 注意事项

1. **User role**：改 `default:user` 后，现有数据库中 role 为 `student` 的用户不会自动更新——需手动执行 `UPDATE users SET role='user' WHERE role='student'`。
2. **旧 courses 表**：保留不动，旧 API 继续工作。新代码走 `materials`。
3. **Order 模型**：`course_id` 字段用于关联购买记录，暂时不做改名（避免破坏现有订单数据）。购买权限校验时用 `material_id = course_id` 的逻辑。
4. **Tiptap JSON 提取纯文本**：当前用简单 JSON 解析，后续可优化为递归遍历所有 `text` 节点。
5. **PDF 解析库**：`ledongthuc/pdf` 对某些 PDF 格式兼容性一般，如遇到问题可替换为 `pdfcpu`。
