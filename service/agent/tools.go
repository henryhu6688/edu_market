package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
	Success           bool   `json:"success"`
	Content           string `json:"content"`
	Source            string `json:"source,omitempty"`             // "primary" | "fallback_l1" | "fallback_l2" | "error" | "blocked"
	ErrorCode         string `json:"error_code,omitempty"`         // 结构化错误码，供 LLM 决策恢复策略
	Recoverable       bool   `json:"recoverable"`                  // 是否可通过调整参数/换工具恢复
	RecommendedAction string `json:"recommended_action,omitempty"` // 建议的恢复动作：fix_arguments_and_retry / narrow_query / ask_user_for_xxx / tell_user_xxx / try_alternative_tool
}

// Tool 可执行的工具接口
type Tool interface {
	Definition() ToolDef
	AllowedModes() []string                         // 允许在哪些模式下调用
	ValidateArgs(argsJSON string) error             // 参数校验，不信任 LLM
	Execute(userID uint, argsJSON string) ToolResult
	Describe(argsJSON string, result ToolResult) string // 返回描述文本，用于 State Block 的 completed 列表
	AllowRepeat() bool                              // 是否允许相同参数连续重复调用（用户驱动操作返回 true，搜索查询返回 false）
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
	ToolQueryOrders     = "query_orders"
	ToolQueryMaterials  = "query_materials"
	ToolSearchMaterials = "search_course_materials"
)

// ============ 客服 Agent Tools ============

type queryOrdersTool struct{}

func (t queryOrdersTool) Definition() ToolDef {
	return ToolDef{
		Name:        ToolQueryOrders,
		Description: "查询当前登录用户的订单列表，返回最近10笔订单的订单号、金额、支付状态、创建时间。" +
			"不能查其他人的订单；不能查单笔订单详情，需用 get_order_detail；不能查退款进度或发起退款。" +
			"当用户问「我的订单」「买了什么」且未提供具体订单号时优先使用本工具。",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}
}

func (t queryOrdersTool) Execute(userID uint, _ string) ToolResult {
	var orders []model.Order
	if err := database.DB.Where("user_id = ?", userID).Order("created_at DESC").Limit(10).Find(&orders).Error; err != nil {
		return ToolResult{Success: false, Content: "查询订单失败，请稍后重试", ErrorCode: "DATABASE_ERROR", Recoverable: false, RecommendedAction: "tell_user_system_busy"}
	}
	if len(orders) == 0 {
		return ToolResult{Success: true, Content: `{"count":0,"orders":[],"hint":"用户暂无订单记录，可引导浏览资料"}`}
	}
	bytes, _ := json.Marshal(orders)
	return ToolResult{Success: true, Content: string(bytes)}
}

func (t queryOrdersTool) AllowedModes() []string { return []string{"support"} }
func (t queryOrdersTool) AllowRepeat() bool           { return false }
func (t queryOrdersTool) ValidateArgs(_ string) error { return nil }
func (t queryOrdersTool) Describe(argsJSON string, result ToolResult) string {
	return fmt.Sprintf("查询用户订单 → 共 %d 笔", countJSONItems(result.Content))
}

// ============ 推荐 Agent Tools ============

type queryCoursesTool struct{}

func (t queryCoursesTool) Definition() ToolDef {
	return ToolDef{
		Name:        ToolQueryMaterials,
		Description: "按关键词、分类、价格范围搜索平台上已发布的学习资料，返回资料标题、描述、价格、分类。" +
			"不能查资料的具体章节内容，需用 search_documents；不能查评价，需用 get_reviews；不能查资料详情，需用 get_material_detail。" +
			"用户表达找资料需求时优先使用；搜不到结果时直接告知用户，不要反复换关键词重试。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"keyword":     map[string]interface{}{"type": "string", "description": "搜索关键词，匹配资料标题和描述"},
				"category_id": map[string]interface{}{"type": "number", "description": "分类ID，用于筛选特定分类下的资料"},
				"min_price":   map[string]interface{}{"type": "number", "description": "最低价格筛选"},
				"max_price":   map[string]interface{}{"type": "number", "description": "最高价格筛选"},
				"sort_by":     map[string]interface{}{"type": "string", "enum": []string{"newest", "price_asc", "price_desc", "popular"}, "description": "排序方式：newest=最新发布，price_asc=价格从低到高，price_desc=价格从高到低，popular=按购买量排序。默认 newest。"},
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
		SortBy     string   `json:"sort_by"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{Success: false, Content: "参数解析失败，请调整参数格式后重试", ErrorCode: "INVALID_ARGUMENT", Recoverable: true, RecommendedAction: "fix_arguments_and_retry"}	}

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

	switch args.SortBy {
	case "price_asc":
		db = db.Order("price ASC")
	case "price_desc":
		db = db.Order("price DESC")
	case "popular":
		db = db.Order("buy_count DESC")
	default:
		db = db.Order("id DESC") // newest
	}

	var materials []model.Material
	if err := db.Limit(10).Find(&materials).Error; err != nil {
		return ToolResult{Success: false, Content: "搜索资料失败，请稍后重试", ErrorCode: "DATABASE_ERROR", Recoverable: false, RecommendedAction: "tell_user_system_busy"}
	}
	if len(materials) == 0 {
		return ToolResult{Success: true, Content: `{"count":0,"materials":[],"hint":"平台暂无相关资料，建议换个方向或浏览其他分类"}`}
	}
	bytes, _ := json.Marshal(materials)
	return ToolResult{Success: true, Content: string(bytes)}
}

func (t queryCoursesTool) AllowedModes() []string { return []string{"shopping", "tutoring"} }
func (t queryCoursesTool) AllowRepeat() bool           { return false }
func (t queryCoursesTool) ValidateArgs(argsJSON string) error {
	var args struct {
		Keyword string `json:"keyword"`
		SortBy  string `json:"sort_by"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	validSorts := map[string]bool{"newest": true, "price_asc": true, "price_desc": true, "popular": true, "": true}
	if !validSorts[args.SortBy] {
		return errors.New("sort_by 只能为 newest / price_asc / price_desc / popular")
	}
	return nil
}
func (t queryCoursesTool) Describe(argsJSON string, result ToolResult) string {
	var args struct{ Keyword string }
	json.Unmarshal([]byte(argsJSON), &args)
	return fmt.Sprintf("搜索「%s」→ 找到 %d 门资料", args.Keyword, countJSONItems(result.Content))
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
		Description: "在指定资料的文档内容中进行语义检索，返回与查询相关的文本片段，用于回答资料内容的具体知识点问题。" +
			"不能搜资料的基本信息如价格、目录，需用 get_material_detail；不能搜全平台资料，需用 query_materials；不能搜FAQ，需用 search_faq。" +
			"用户已指向某份资料并询问具体章节或知识点（如「第三章讲了什么」）时优先使用；搜不到时诚实告知，不要反复调用。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"material_id": map[string]interface{}{"type": "number", "description": "要检索的资料ID，需先通过 get_material_detail 确认资料存在"},
				"query":       map[string]interface{}{"type": "string", "description": "要搜索的问题或关键词，不超过200字。建议用具体章节号或知识点名称，不用整段描述"},
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
		return ToolResult{Success: false, Content: "参数解析失败，请调整参数格式后重试", ErrorCode: "INVALID_ARGUMENT", Recoverable: true, RecommendedAction: "fix_arguments_and_retry"}	}
	if t.searchFunc == nil {
		return ToolResult{Success: false, Content: "资料检索服务暂不可用，请稍后重试", ErrorCode: "SERVICE_UNAVAILABLE", Recoverable: false, RecommendedAction: "tell_user_service_unavailable"}
	}
	content, err := t.searchFunc(args.MaterialID, args.Query, 5)
	if err != nil {
		return ToolResult{Success: false, Content: "文档检索失败，建议换个问法或关键词重试", ErrorCode: "SEARCH_ERROR", Recoverable: true, RecommendedAction: "retry_or_narrow_query"}
	}
	if content == "" {
		return ToolResult{Success: true, Content: `{"found":false,"chunks":[],"hint":"资料中未找到相关内容，建议换个问法或确认资料是否正确"}`}
	}
	return ToolResult{Success: true, Content: content}
}

func (t searchMaterialsTool) AllowedModes() []string { return []string{"shopping", "tutoring"} }
func (t searchMaterialsTool) AllowRepeat() bool           { return false }
func (t searchMaterialsTool) ValidateArgs(argsJSON string) error {
	var args struct {
		MaterialID uint   `json:"material_id"`
		Query      string `json:"query"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if args.MaterialID == 0 {
		return errors.New("material_id 不能为空")
	}
	if strings.TrimSpace(args.Query) == "" {
		return errors.New("搜索关键词不能为空")
	}
	if len(args.Query) > 200 {
		return errors.New("搜索关键词过长，请精简到 200 字以内")
	}
	return nil
}
func (t searchMaterialsTool) Describe(argsJSON string, result ToolResult) string {
	var args struct{ Query string }
	json.Unmarshal([]byte(argsJSON), &args)
	return fmt.Sprintf("搜索文档「%s」→ 找到 %d 条", args.Query, countJSONItems(result.Content))
}

// ============ 新增 v3 Tool ============

type getMaterialDetailTool struct{}

func (t getMaterialDetailTool) Definition() ToolDef {
	return ToolDef{
		Name:        "get_material_detail",
		Description: "获取单份资料的完整信息，包括标题、价格、描述、浏览量、购买数、所属分类、文档目录（含试读标记）。" +
			"不能搜索文档具体内容，需用 search_documents；不能获取用户评价，需用 get_reviews；不能搜索多份资料，需用 query_materials。" +
			"用户提到具体资料名或问「这个资料多少钱」「有哪些章节」时优先使用。",
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
		return ToolResult{Success: false, Content: "资料不存在，请确认资料ID是否正确", ErrorCode: "NOT_FOUND", Recoverable: true, RecommendedAction: "confirm_material_id_with_user"}
	}

	type OutlineItem struct {
		Title  string `json:"title"`
		IsFree bool   `json:"is_free_preview"`
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
		"suggested_next": []string{
			"如果用户想了解具体章节内容，使用 search_documents 搜索文档",
			"如果用户表达购买意向，使用 trigger_purchase_offer 弹出购买卡片",
			"如果用户想看其他用户的评价，使用 get_reviews",
		},
	}
	b, _ := json.Marshal(result)
	return ToolResult{Success: true, Content: string(b)}
}

func (t getMaterialDetailTool) AllowedModes() []string { return []string{"shopping", "tutoring"} }
func (t getMaterialDetailTool) AllowRepeat() bool           { return false }
func (t getMaterialDetailTool) ValidateArgs(argsJSON string) error {
	var args struct{ MaterialID uint `json:"material_id"` }
	json.Unmarshal([]byte(argsJSON), &args)
	if args.MaterialID == 0 {
		return errors.New("material_id 不能为空")
	}
	return nil
}
func (t getMaterialDetailTool) Describe(argsJSON string, result ToolResult) string {
	var d struct{ Title string }
	json.Unmarshal([]byte(result.Content), &d)
	return fmt.Sprintf("查看《%s》详情", d.Title)
}

type getReviewsTool struct{}

func (t getReviewsTool) Definition() ToolDef {
	return ToolDef{
		Name:        "get_reviews",
		Description: "获取某份资料的用户评价列表，包含评分（1-5分）和评价内容，最多返回10条。" +
			"不能获取资料详情，需用 get_material_detail；不能回复、删除或修改评价；不能查某个用户的评价历史。" +
			"用户问「评价怎么样」「口碑如何」时使用，配合 get_material_detail 效果更好。",
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

func (t getReviewsTool) AllowedModes() []string  { return []string{"shopping", "tutoring"} }
func (t getReviewsTool) AllowRepeat() bool           { return false }
func (t getReviewsTool) ValidateArgs(_ string) error { return nil }
func (t getReviewsTool) Describe(argsJSON string, result ToolResult) string {
	count := countJSONItems(result.Content)
	return fmt.Sprintf("查看评价 → %d 条", count)
}

type getCategoriesTool struct{}

func (t getCategoriesTool) Definition() ToolDef {
	return ToolDef{
		Name:        "get_categories",
		Description: "获取平台所有资料分类列表，返回分类ID和名称。" +
			"不能返回分类下的资料列表，需用 query_materials 带 category_id 参数；不能创建或修改分类。" +
			"用户问「有哪些分类」「什么类型」时使用；帮用户缩小搜索范围时配合 query_materials 使用。",
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

func (t getCategoriesTool) AllowedModes() []string  { return []string{"shopping", "tutoring"} }
func (t getCategoriesTool) AllowRepeat() bool           { return false }
func (t getCategoriesTool) ValidateArgs(_ string) error { return nil }
func (t getCategoriesTool) Describe(argsJSON string, result ToolResult) string {
	return fmt.Sprintf("获取分类列表 → %d 个", countJSONItems(result.Content))
}

type searchFAQTool struct{}

func (t searchFAQTool) Definition() ToolDef {
	return ToolDef{
		Name:        "search_faq",
		Description: "在平台FAQ知识库中搜索相关问题，返回匹配的问答对，适用于退款政策、支付方式、使用指南等平台规则类问题。" +
			"不能搜资料内容，需用 search_documents；不能查订单信息，需用 query_orders 或 get_order_detail；FAQ 没覆盖的问题直接引导人工客服，不要编造答案。" +
			"用户问平台规则、退款、支付、售后等政策性问题时优先使用；搜不到时直接说「需要联系客服确认」，不要反复调用。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{"type": "string", "description": "要搜索的FAQ关键词，建议用简洁短语如「退款」「支付方式」「怎么使用」，不用完整句子"},
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

func (t searchFAQTool) AllowedModes() []string  { return []string{"shopping", "tutoring", "support"} }
func (t searchFAQTool) AllowRepeat() bool           { return false }
func (t searchFAQTool) ValidateArgs(argsJSON string) error {
	var args struct{ Query string `json:"query"` }
	json.Unmarshal([]byte(argsJSON), &args)
	if strings.TrimSpace(args.Query) == "" {
		return errors.New("搜索关键词不能为空")
	}
	if len(args.Query) > 100 {
		return errors.New("搜索关键词过长，请精简到 100 字以内")
	}
	return nil
}
func (t searchFAQTool) Describe(argsJSON string, result ToolResult) string {
	var args struct{ Query string }
	json.Unmarshal([]byte(argsJSON), &args)
	return fmt.Sprintf("搜索FAQ「%s」→ 找到 %d 条", args.Query, countJSONItems(result.Content))
}

type getOrderDetailTool struct{}

func (t getOrderDetailTool) Definition() ToolDef {
	return ToolDef{
		Name:        "get_order_detail",
		Description: "根据订单号查询单笔订单的完整信息，包括订单状态、支付状态、金额、关联资料、创建时间。" +
			"不能用手机号或描述查订单，需先用 query_orders 获取订单号；不能修改订单、发起退款或取消订单；不能查其他用户的订单。" +
			"用户提供了具体订单号时使用；用户问「这笔订单怎么样了」时先确认是否有订单号，没有则先用 query_orders。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"order_no": map[string]interface{}{"type": "string", "description": "订单号，只能从 query_orders 返回的结果中获取，不能编造。用户未提供时先用 query_orders 查列表"},
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
		return ToolResult{Success: false, Content: "订单不存在，请用 query_orders 确认订单号是否正确", ErrorCode: "NOT_FOUND", Recoverable: true, RecommendedAction: "check_order_no_or_use_query_orders"}
	}
	b, _ := json.Marshal(order)
	return ToolResult{Success: true, Content: string(b)}
}

func (t getOrderDetailTool) AllowedModes() []string  { return []string{"support"} }
func (t getOrderDetailTool) AllowRepeat() bool           { return false }
func (t getOrderDetailTool) ValidateArgs(_ string) error { return nil }
func (t getOrderDetailTool) Describe(argsJSON string, result ToolResult) string {
	return "查看订单详情"
}

type triggerPurchaseOfferTool struct{}

func (t triggerPurchaseOfferTool) Definition() ToolDef {
	return ToolDef{
		Name:        "trigger_purchase_offer",
		Description: "向用户弹出购买引导卡片，显示资料标题、价格和封面。这是让用户看到购买入口的唯一方式——不调用本工具，用户就无法下单。" +
			"不能替代文字回复，调用后仍需简要引导用户点击卡片；不能用于非购买场景；不能替用户做购买决定。" +
			"用户表达购买意向（「买」「下单」「就这个」「来一份」「怎么买」）时必须调用；即使用户之前看过卡片，再次表达意向也必须重新调用。不要只说「已发送卡片」而不调用本工具。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"material_id": map[string]interface{}{"type": "number", "description": "要推荐购买的资料ID，必须来自 query_materials 或 get_material_detail 返回的真实资料"},
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
		return ToolResult{Success: false, Content: "资料不存在，无法发送购买卡片，请用 query_materials 确认资料ID", ErrorCode: "NOT_FOUND", Recoverable: true, RecommendedAction: "confirm_material_id_with_user"}
	}

	result := map[string]interface{}{
		"__action":    "purchase_offer",
		"material_id": material.ID,
		"title":       material.Title,
		"price":       material.Price,
		"cover_image": material.CoverImage,
		"requires_user_action": true,
		"action_description": "购买卡片已发送，用户需要点击卡片完成下单，当前尚未产生订单",
		"suggested_next": []string{
			"告知用户点击卡片即可下单购买",
			"用户未完成下单前，不要声称「已下单」或「已购买」",
			"如果用户询问发货、退款等售后问题，使用 search_faq 查询",
			"如果用户犹豫是否购买，可使用 get_reviews 展示其他用户的评价",
		},
	}
	b, _ := json.Marshal(result)
	return ToolResult{Success: true, Content: string(b)}
}

func (t triggerPurchaseOfferTool) AllowedModes() []string { return []string{"shopping"} }
func (t triggerPurchaseOfferTool) AllowRepeat() bool           { return true }
func (t triggerPurchaseOfferTool) ValidateArgs(argsJSON string) error {
	var args struct{ MaterialID uint `json:"material_id"` }
	json.Unmarshal([]byte(argsJSON), &args)
	if args.MaterialID == 0 {
		return errors.New("material_id 不能为空")
	}
	var count int64
	database.DB.Model(&model.Material{}).Where("id = ? AND status = ?", args.MaterialID, "published").Count(&count)
	if count == 0 {
		return fmt.Errorf("资料 #%d 不存在或已下架", args.MaterialID)
	}
	return nil
}
func (t triggerPurchaseOfferTool) Describe(argsJSON string, result ToolResult) string {
	return "发送购买卡片"
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

// countJSONItems 统计 JSON 数组中的元素数量
func countJSONItems(content string) int {
	content = strings.TrimSpace(content)
	if content == "" || content == "[]" {
		return 0
	}
	// 简单统计 JSON 对象数：匹配 "{" 开头的顶层元素
	var depth, count int
	inString := false
	for _, ch := range content {
		if ch == '"' {
			inString = !inString
		}
		if inString {
			continue
		}
		switch ch {
		case '{':
			if depth == 0 {
				count++
			}
			depth++
		case '}':
			depth--
		case '[':
			depth++
		case ']':
			depth--
		}
	}
	return count
}

// toolDefsToOpenAI 将 Tool map 转为 OpenAI 格式的 tool 数组
func toolDefsToOpenAI(tools map[string]Tool) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(tools))
	for _, t := range tools {
		result = append(result, t.Definition().ToOpenAITool())
	}
	return result
}
