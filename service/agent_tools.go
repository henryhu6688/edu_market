package service

import (
	"encoding/json"

	"edu_market/database"
	"edu_market/model"
)

// ToolDef LLM Tool 定义（OpenAI Function Calling 格式）
type ToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ToolResult 工具执行结果
type ToolResult struct {
	Success bool   `json:"success"`
	Content string `json:"content"`
}

// Tool 可执行的工具接口
type Tool interface {
	Definition() ToolDef
	Execute(userID uint, argsJSON string) ToolResult
}

// ToOpenAITool 转为 OpenAI 兼容的 tool 定义
func (t ToolDef) ToOpenAITool() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        t.Name,
			"description": t.Description,
			"parameters":  t.Parameters,
		},
	}
}

// SearchFunc RAG 检索函数类型（由 RAGService 注入，避免循环依赖）
type SearchFunc func(materialID uint, query string, topK int) (string, error)

// ============ 工具常量 ============

const (
	ToolQueryOrders   = "query_orders"
	ToolQueryMaterials = "query_materials"
	ToolSearchMaterials = "search_course_materials"
)

// ============ 客服 Agent Tools ============

type queryOrdersTool struct{}

func (t queryOrdersTool) Definition() ToolDef {
	return ToolDef{
		Name:        ToolQueryOrders,
		Description: "查询当前用户的订单列表，返回订单号、金额、状态、创建时间",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}
}

func (t queryOrdersTool) Execute(userID uint, _ string) ToolResult {
	var orders []model.Order
	if err := database.DB.Where("user_id = ?", userID).Order("created_at DESC").Limit(10).Find(&orders).Error; err != nil {
		return ToolResult{Success: false, Content: "查询订单失败: " + err.Error()}
	}
	if len(orders) == 0 {
		return ToolResult{Success: true, Content: "您暂无订单记录"}
	}
	bytes, _ := json.Marshal(orders)
	return ToolResult{Success: true, Content: string(bytes)}
}

// ============ 推荐 Agent Tools ============

type queryCoursesTool struct{}

func (t queryCoursesTool) Definition() ToolDef {
	return ToolDef{
		Name:        ToolQueryMaterials,
		Description: "按关键词、分类ID、价格范围搜索课程列表，返回课程标题、描述、价格、分类等信息",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"keyword":     map[string]interface{}{"type": "string", "description": "搜索关键词"},
				"category_id": map[string]interface{}{"type": "number", "description": "分类ID（可选）"},
				"min_price":   map[string]interface{}{"type": "number", "description": "最低价格（可选）"},
				"max_price":   map[string]interface{}{"type": "number", "description": "最高价格（可选）"},
			},
		},
	}
}

func (t queryCoursesTool) Execute(_ uint, argsJSON string) ToolResult {
	var args struct {
		Keyword    string   `json:"keyword"`
		CategoryID *uint    `json:"category_id"`
		MinPrice   *float64 `json:"min_price"`
		MaxPrice   *float64 `json:"max_price"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{Success: false, Content: "参数解析失败"}
	}

	db := database.DB.Where("status = ?", "published").Preload("Category").Preload("User")
	if args.Keyword != "" {
		db = db.Where("title LIKE ? OR description LIKE ?", "%"+args.Keyword+"%", "%"+args.Keyword+"%")
	}
	if args.CategoryID != nil {
		db = db.Where("category_id = ?", *args.CategoryID)
	}
	if args.MinPrice != nil {
		db = db.Where("price >= ?", *args.MinPrice)
	}
	if args.MaxPrice != nil {
		db = db.Where("price <= ?", *args.MaxPrice)
	}

	var courses []model.Course
	if err := db.Order("id DESC").Limit(10).Find(&courses).Error; err != nil {
		return ToolResult{Success: false, Content: "搜索课程失败: " + err.Error()}
	}
	if len(courses) == 0 {
		return ToolResult{Success: true, Content: "平台暂无相关资料，建议换个方向"}
	}
	bytes, _ := json.Marshal(courses)
	return ToolResult{Success: true, Content: string(bytes)}
}

// ============ 答疑 Agent Tools ============

type searchMaterialsTool struct {
	searchFunc SearchFunc
}

func newSearchMaterialsTool(fn SearchFunc) searchMaterialsTool {
	return searchMaterialsTool{searchFunc: fn}
}

func (t searchMaterialsTool) Definition() ToolDef {
	return ToolDef{
		Name:        ToolSearchMaterials,
		Description: "搜索某份学习资料的文档内容，返回相关的文本片段。用于回答关于资料内容的具体问题。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"material_id": map[string]interface{}{"type": "number", "description": "资料ID"},
				"query":       map[string]interface{}{"type": "string", "description": "要搜索的问题或关键词"},
			},
			"required": []string{"material_id", "query"},
		},
	}
}

func (t searchMaterialsTool) Execute(_ uint, argsJSON string) ToolResult {
	var args struct {
		MaterialID uint   `json:"material_id"`
		Query      string `json:"query"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{Success: false, Content: "参数解析失败"}
	}
	if t.searchFunc == nil {
		return ToolResult{Success: false, Content: "资料检索服务暂不可用"}
	}
	content, err := t.searchFunc(args.MaterialID, args.Query, 5)
	if err != nil {
		return ToolResult{Success: false, Content: "检索失败: " + err.Error()}
	}
	if content == "" {
		return ToolResult{Success: true, Content: "资料中未找到相关内容，建议换个问法"}
	}
	return ToolResult{Success: true, Content: content}
}

// ============ 新增 v3 Tool ============

type getMaterialDetailTool struct{}

func (t getMaterialDetailTool) Definition() ToolDef {
	return ToolDef{
		Name:        "get_material_detail",
		Description: "获取某份学习资料的详细信息，包括价格、评价数、购买数、文档目录结构",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"material_id": map[string]interface{}{"type": "number", "description": "资料ID"},
			},
			"required": []string{"material_id"},
		},
	}
}

func (t getMaterialDetailTool) Execute(_ uint, argsJSON string) ToolResult {
	var args struct{ MaterialID uint `json:"material_id"` }
	json.Unmarshal([]byte(argsJSON), &args)

	var material model.Material
	if err := database.DB.Preload("Category").Preload("Documents").
		First(&material, args.MaterialID).Error; err != nil {
		return ToolResult{Success: false, Content: "资料不存在"}
	}

	type OutlineItem struct {
		Title    string `json:"title"`
		IsFree   bool   `json:"is_free_preview"`
	}
	var outline []OutlineItem
	for _, d := range material.Documents {
		outline = append(outline, OutlineItem{Title: d.Title, IsFree: d.IsFreePreview})
	}

	result := map[string]interface{}{
		"id": material.ID, "title": material.Title, "price": material.Price,
		"description": material.Description, "view_count": material.ViewCount,
		"buy_count": material.BuyCount, "category": material.Category.Name,
		"outline": outline,
	}
	b, _ := json.Marshal(result)
	return ToolResult{Success: true, Content: string(b)}
}

type getReviewsTool struct{}

func (t getReviewsTool) Definition() ToolDef {
	return ToolDef{
		Name:        "get_reviews",
		Description: "获取某份资料的用户评价列表，含评分和内容",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"material_id": map[string]interface{}{"type": "number"},
			},
			"required": []string{"material_id"},
		},
	}
}

func (t getReviewsTool) Execute(_ uint, argsJSON string) ToolResult {
	var args struct{ MaterialID uint `json:"material_id"` }
	json.Unmarshal([]byte(argsJSON), &args)

	var reviews []model.Review
	database.DB.Where("course_id = ?", args.MaterialID).
		Order("created_at DESC").Limit(10).Find(&reviews)

	type ReviewItem struct {
		Rating  int    `json:"rating"`
		Content string `json:"content"`
	}
	var items []ReviewItem
	for _, r := range reviews {
		items = append(items, ReviewItem{Rating: r.Rating, Content: r.Content})
	}
	b, _ := json.Marshal(map[string]interface{}{
		"count": len(reviews), "reviews": items,
	})
	return ToolResult{Success: true, Content: string(b)}
}

type getCategoriesTool struct{}

func (t getCategoriesTool) Definition() ToolDef {
	return ToolDef{
		Name:        "get_categories",
		Description: "获取平台所有学习资料分类列表",
		Parameters: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{},
		},
	}
}

func (t getCategoriesTool) Execute(_ uint, _ string) ToolResult {
	var cats []model.Category
	database.DB.Order("id ASC").Find(&cats)

	type CatItem struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
	}
	var items []CatItem
	for _, c := range cats {
		items = append(items, CatItem{ID: c.ID, Name: c.Name})
	}
	b, _ := json.Marshal(items)
	return ToolResult{Success: true, Content: string(b)}
}

type searchFAQTool struct{}

func (t searchFAQTool) Definition() ToolDef {
	return ToolDef{
		Name:        "search_faq",
		Description: "在平台FAQ中搜索相关问题，用于解答退款、支付、使用等问题",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{"type": "string", "description": "搜索关键词"},
			},
			"required": []string{"query"},
		},
	}
}

func (t searchFAQTool) Execute(_ uint, argsJSON string) ToolResult {
	var args struct{ Query string `json:"query"` }
	json.Unmarshal([]byte(argsJSON), &args)

	var faqs []model.FAQ
	database.DB.Where("question LIKE ? OR answer LIKE ?",
		"%"+args.Query+"%", "%"+args.Query+"%").Limit(5).Find(&faqs)

	type FAQItem struct {
		Question string `json:"question"`
		Answer   string `json:"answer"`
	}
	var items []FAQItem
	for _, f := range faqs {
		items = append(items, FAQItem{Question: f.Question, Answer: f.Answer})
	}
	b, _ := json.Marshal(items)
	return ToolResult{Success: true, Content: string(b)}
}

type getOrderDetailTool struct{}

func (t getOrderDetailTool) Definition() ToolDef {
	return ToolDef{
		Name:        "get_order_detail",
		Description: "获取单笔订单的详细信息：订单号、金额、状态、时间、关联资料",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"order_no": map[string]interface{}{"type": "string", "description": "订单号"},
			},
			"required": []string{"order_no"},
		},
	}
}

func (t getOrderDetailTool) Execute(userID uint, argsJSON string) ToolResult {
	var args struct{ OrderNo string `json:"order_no"` }
	json.Unmarshal([]byte(argsJSON), &args)

	var order model.Order
	if err := database.DB.Where("order_no = ? AND user_id = ?", args.OrderNo, userID).
		First(&order).Error; err != nil {
		return ToolResult{Success: false, Content: "订单不存在"}
	}
	b, _ := json.Marshal(order)
	return ToolResult{Success: true, Content: string(b)}
}

type triggerPurchaseOfferTool struct{}

func (t triggerPurchaseOfferTool) Definition() ToolDef {
	return ToolDef{
		Name:        "trigger_purchase_offer",
		Description: "向用户发送购买引导卡片。仅在用户表现出购买兴趣时调用。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"material_id": map[string]interface{}{"type": "number", "description": "要推荐的资料ID"},
			},
			"required": []string{"material_id"},
		},
	}
}

func (t triggerPurchaseOfferTool) Execute(_ uint, argsJSON string) ToolResult {
	var args struct{ MaterialID uint `json:"material_id"` }
	json.Unmarshal([]byte(argsJSON), &args)

	var material model.Material
	if err := database.DB.First(&material, args.MaterialID).Error; err != nil {
		return ToolResult{Success: false, Content: "资料不存在"}
	}

	result := map[string]interface{}{
		"__action":    "purchase_offer",
		"material_id": material.ID,
		"title":       material.Title,
		"price":       material.Price,
		"cover_image": material.CoverImage,
	}
	b, _ := json.Marshal(result)
	return ToolResult{Success: true, Content: string(b)}
}

// ============ Tool 集合构建（v3：全量注册，不按 agentType 筛选） ============

func buildToolSet(searchFunc SearchFunc) map[string]Tool {
	tools := map[string]Tool{
		"query_materials":        queryCoursesTool{},
		"get_material_detail":    getMaterialDetailTool{},
		"get_reviews":            getReviewsTool{},
		"get_categories":         getCategoriesTool{},
		"query_orders":           queryOrdersTool{},
		"get_order_detail":       getOrderDetailTool{},
		"search_faq":             searchFAQTool{},
		"trigger_purchase_offer": triggerPurchaseOfferTool{},
	}
	if searchFunc != nil {
		tools["search_documents"] = newSearchMaterialsTool(searchFunc)
	}
	return tools
}

// toolDefsToOpenAI 将 Tool map 转为 OpenAI 格式的 tool 数组
func toolDefsToOpenAI(tools map[string]Tool) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(tools))
	for _, t := range tools {
		result = append(result, t.Definition().ToOpenAITool())
	}
	return result
}
