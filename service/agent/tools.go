package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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
	ValidateArgs(argsJSON string) error                                // 参数校验，不信任 LLM
	Execute(userID uint, argsJSON string) ToolResult
	Describe(argsJSON string, result ToolResult) string // 返回描述文本，用于 State Block 的 completed 列表
	AllowRepeat() bool                                 // 是否允许相同参数连续重复调用（用户驱动操作返回 true，搜索查询返回 false）
}

// StepRecord 单步执行记录，用于 State 中的 completed/failed 列表。
type StepRecord struct {
	Action  string `json:"action"`          // 人类可读描述，如 "搜索文档「函数」"
	Tool    string `json:"tool"`            // 工具名
	Args    string `json:"args"`            // 参数 JSON，用于去重检测
	Error   string `json:"error,omitempty"` // 失败原因
	Success bool   `json:"success"`
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
type SearchFunc func(materialID uint, query string, topK int, hasAccess bool) (string, error)

// ============ 工具常量 ============

const (
	ToolSearchMaterials   = "search_materials"
	ToolGetMaterialDetail = "get_material_detail"
	ToolMyMaterials       = "my_materials"
	ToolSearchDocuments   = "search_documents"
	ToolGetOrders         = "get_orders"
	ToolSearchFAQ         = "search_faq"
	ToolPurchase          = "purchase"
)

// ============ search_materials — 搜全平台资料 ============

type searchMaterialsTool struct{}

func (t searchMaterialsTool) Definition() ToolDef {
	return ToolDef{
		Name: ToolSearchMaterials,
		Description: "能做什么\n  在平台已发布资料中按关键词、分类、价格范围搜索，返回匹配列表。\n\n" +
			"不能做什么\n  不返回资料的具体章节内容或文档正文（用 search_documents）\n" +
			"  不返回用户的已购/已发布资料（用 my_materials）\n" +
			"  不返回评价详情（get_material_detail 已包含评价）\n" +
			"  不搜下架或草稿状态的资料\n\n" +
			"何时优先使用\n  - 用户问「有没有XX方向的资料」「帮我找XX相关的课程」\n" +
			"  - 用户表达学习方向但未指定具体资料名\n" +
			"  - 搜不到时直接告知，不要反复换关键词重试",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"keyword":     map[string]interface{}{"type": "string", "description": "1-50字，匹配标题和描述，不用完整句子"},
				"category_id": map[string]interface{}{"type": "number", "description": "只能来自 get_material_detail 返回的 category.id，不可编造"},
				"min_price":   map[string]interface{}{"type": "number", "description": ">=0"},
				"max_price":   map[string]interface{}{"type": "number", "description": "<=10000"},
				"sort_by":     map[string]interface{}{"type": "string", "enum": []string{"newest", "price_asc", "price_desc", "popular"}, "description": "排序方式，默认 newest"},
				"page":        map[string]interface{}{"type": "number", "description": "1-100，默认 1"},
				"page_size":   map[string]interface{}{"type": "number", "description": "1-10，默认 5"},
			},
		},
	}
}

func (t searchMaterialsTool) Execute(_ uint, argsJSON string) ToolResult {
	var args struct {
		Keyword    string   `json:"keyword"`
		CategoryID *uint    `json:"category_id"`
		MinPrice   *float64 `json:"min_price"`
		MaxPrice   *float64 `json:"max_price"`
		SortBy     string   `json:"sort_by"`
		Page       int      `json:"page"`
		PageSize   int      `json:"page_size"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{Success: false, Content: "参数解析失败", ErrorCode: "INVALID_ARGUMENT", Recoverable: true, RecommendedAction: "fix_arguments_and_retry"}
	}
	if args.Page < 1 { args.Page = 1 }
	if args.PageSize < 1 || args.PageSize > 10 { args.PageSize = 5 }

	db := database.DB.Where("status = ?", "published").Preload("Category")
	if args.Keyword != "" {
		k := "%" + strings.TrimSpace(args.Keyword) + "%"
		db = db.Where("title LIKE ? OR description LIKE ?", k, k)
	}
	if args.CategoryID != nil { db = db.Where("category_id = ?", *args.CategoryID) }
	if args.MinPrice != nil { db = db.Where("price >= ?", *args.MinPrice) }
	if args.MaxPrice != nil { db = db.Where("price <= ?", *args.MaxPrice) }

	switch args.SortBy {
	case "price_asc": db = db.Order("price ASC")
	case "price_desc": db = db.Order("price DESC")
	case "popular": db = db.Order("buy_count DESC")
	default: db = db.Order("id DESC")
	}

	var materials []model.Material
	offset := (args.Page - 1) * args.PageSize
	if err := db.Offset(offset).Limit(args.PageSize).Find(&materials).Error; err != nil {
		return ToolResult{Success: false, Content: "搜索失败，请稍后重试", ErrorCode: "DATABASE_ERROR", Recoverable: false, RecommendedAction: "tell_user_system_busy"}
	}

	type item struct {
		ID           uint    `json:"id"`
		Title        string  `json:"title"`
		Price        float64 `json:"price"`
		CategoryName string  `json:"category_name"`
		BuyCount     int     `json:"buy_count"`
		Description  string  `json:"description"`
	}
	items := make([]item, 0, len(materials))
	for _, m := range materials {
		catName := ""
		if m.Category.ID != 0 { catName = m.Category.Name }
		items = append(items, item{m.ID, m.Title, m.Price, catName, m.BuyCount, TruncateRunes(m.Description, 80)})
	}
	b, _ := json.Marshal(map[string]interface{}{"success": true, "materials": items, "total": len(items), "page": args.Page})
	return ToolResult{Success: true, Content: string(b)}
}

func (t searchMaterialsTool) AllowRepeat() bool { return false }
func (t searchMaterialsTool) ValidateArgs(argsJSON string) error {
	var args struct {
		SortBy   string `json:"sort_by"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	valid := map[string]bool{"newest": true, "price_asc": true, "price_desc": true, "popular": true, "": true}
	if !valid[args.SortBy] { return errors.New("sort_by 只能为 newest/price_asc/price_desc/popular") }
	if args.Page > 100 { return errors.New("page 不能超过 100") }
	if args.PageSize > 10 { return errors.New("page_size 不能超过 10") }
	return nil
}
func (t searchMaterialsTool) Describe(argsJSON string, result ToolResult) string {
	var args struct{ Keyword string }
	json.Unmarshal([]byte(argsJSON), &args)
	return fmt.Sprintf("搜索「%s」→ 找到 %d 门资料", args.Keyword, countJSONItems(result.Content))
}

// ============ get_material_detail — 资料详情 + 目录 + 评价 ============

type getMaterialDetailTool struct{}

func (t getMaterialDetailTool) Definition() ToolDef {
	return ToolDef{
		Name: ToolGetMaterialDetail,
		Description: "能做什么\n  查看单份资料的完整信息：基本信息、发布者、文档目录（含试读标记）、用户评价、当前用户对该资料的访问权限。\n\n" +
			"不能做什么\n  不返回文档正文内容（用 search_documents）\n  不搜全平台资料（用 search_materials）\n  不修改资料信息\n\n" +
			"何时优先使用\n  - 用户提到具体资料名或资料ID\n  - 用户问「这个资料多少钱」「有哪些章节」「评价怎么样」\n" +
			"  - 用户对 search_materials 返回的某项结果感兴趣\n  - 与 search_documents 配合：先确认资料存在，再搜内容",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"material_id": map[string]interface{}{"type": "number", "description": ">0，来自 search_materials 或 my_materials 返回的ID，不可编造"},
			},
			"required": []string{"material_id"},
		},
	}
}

func (t getMaterialDetailTool) Execute(userID uint, argsJSON string) ToolResult {
	var args struct{ MaterialID uint `json:"material_id"` }
	json.Unmarshal([]byte(argsJSON), &args)

	var material model.Material
	if err := database.DB.Preload("Category").Preload("Documents").Preload("User").First(&material, args.MaterialID).Error; err != nil {
		return ToolResult{Success: false, Content: fmt.Sprintf("资料 #%d 不存在或已下架，请确认ID或用 search_materials 重新搜索", args.MaterialID), ErrorCode: "NOT_FOUND", Recoverable: true, RecommendedAction: "confirm_material_id_with_user"}
	}

	type outlineItem struct {
		Title     string `json:"title"`
		IsFree    bool   `json:"is_free_preview"`
		DocID     uint   `json:"document_id"`
		SortOrder int    `json:"sort_order"`
	}
	outline := make([]outlineItem, 0, len(material.Documents))
	for _, d := range material.Documents {
		outline = append(outline, outlineItem{d.Title, d.IsFreePreview, d.ID, d.SortOrder})
	}

	var reviews []model.Review
	database.DB.Where("course_id = ?", material.ID).Preload("User").Order("created_at DESC").Limit(10).Find(&reviews)
	type reviewItem struct {
		Rating   int    `json:"rating"`
		Content  string `json:"content"`
		Username string `json:"username"`
	}
	revItems := make([]reviewItem, 0, len(reviews))
	for _, r := range reviews {
		uname := ""
		if r.User.ID != 0 { uname = r.User.Username }
		revItems = append(revItems, reviewItem{r.Rating, r.Content, uname})
	}
	if revItems == nil { revItems = []reviewItem{} }

	hasAccess := checkHasAccess(userID, material.ID)
	b, _ := json.Marshal(map[string]interface{}{
		"success": true,
		"material": map[string]interface{}{
			"id":          material.ID,
			"title":       material.Title,
			"price":       material.Price,
			"description": material.Description,
			"cover_image": material.CoverImage,
			"category":    map[string]interface{}{"id": material.Category.ID, "name": material.Category.Name},
			"publisher":   map[string]interface{}{"id": material.User.ID, "username": material.User.Username},
			"stats":       map[string]interface{}{"view_count": material.ViewCount, "buy_count": material.BuyCount, "review_count": len(reviews), "avg_rating": avgRating(reviews)},
			"access":      map[string]interface{}{"is_owner": material.UserID == userID, "has_purchased": hasAccess},
			"outline":     outline,
			"reviews":     revItems,
		},
	})
	return ToolResult{Success: true, Content: string(b)}
}

func (t getMaterialDetailTool) AllowRepeat() bool  { return false }
func (t getMaterialDetailTool) ValidateArgs(argsJSON string) error {
	var args struct{ MaterialID uint `json:"material_id"` }
	json.Unmarshal([]byte(argsJSON), &args)
	if args.MaterialID == 0 { return errors.New("material_id 不能为空") }
	return nil
}
func (t getMaterialDetailTool) Describe(argsJSON string, result ToolResult) string {
	var d struct {
		Material struct{ Title string } `json:"material"`
	}
	json.Unmarshal([]byte(result.Content), &d)
	return fmt.Sprintf("查看《%s》详情", d.Material.Title)
}

// ============ my_materials — 我的资料 ============

type myMaterialsTool struct{}

func (t myMaterialsTool) Definition() ToolDef {
	return ToolDef{
		Name: ToolMyMaterials,
		Description: "能做什么\n  列出当前登录用户可访问的全部资料：自己发布的 + 已购买并支付成功的。\n\n" +
			"不能做什么\n  不搜全平台资料（用 search_materials）\n  不返回文档内容（用 search_documents）\n  不查订单详情如支付时间、物流状态（用 get_orders）\n\n" +
			"何时优先使用\n  - 用户说「我的资料」「我买的资料」「我发布的资料」\n" +
			"  - 用户想在自己拥有的资料范围内搜索具体内容时，先调此工具拿到列表，再调 search_documents\n" +
			"  - 这是唯一能告诉 LLM「用户到底有哪些资料」的工具。遇到含「我的」的查询应优先考虑",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}
}

func (t myMaterialsTool) Execute(userID uint, _ string) ToolResult {
	type item struct {
		ID       uint    `json:"id"`
		Title    string  `json:"title"`
		Price    float64 `json:"price"`
		BuyCount int     `json:"buy_count"`
		Status   string  `json:"status,omitempty"`
		OrderNo  string  `json:"order_no,omitempty"`
		PaidAt   string  `json:"paid_at,omitempty"`
	}

	var published []item
	var mats []model.Material
	database.DB.Where("user_id = ?", userID).Find(&mats)
	for _, m := range mats {
		published = append(published, item{ID: m.ID, Title: m.Title, Price: m.Price, BuyCount: m.BuyCount, Status: m.Status})
	}
	if published == nil { published = []item{} }

	var purchased []item
	var orders []model.Order
	database.DB.Where("user_id = ? AND status = ?", userID, "paid").Find(&orders)
	for _, o := range orders {
		paidStr := ""
		if o.PaidAt != nil { paidStr = o.PaidAt.Format("2006-01-02") }
		// 查 materials 表获取资料标题
		var m model.Material
		title := ""
		if database.DB.Select("title").First(&m, o.CourseID).Error == nil {
			title = m.Title
		}
		purchased = append(purchased, item{ID: o.CourseID, Title: title, Price: o.Amount, OrderNo: o.OrderNo, PaidAt: paidStr})
	}
	if purchased == nil { purchased = []item{} }

	total := len(published) + len(purchased)
	b, _ := json.Marshal(map[string]interface{}{
		"success": true,
		"materials": map[string]interface{}{
			"published": published,
			"purchased": purchased,
		},
		"total": total,
		"hint":  fmt.Sprintf("共 %d 份资料可访问（发布 %d 份，购买 %d 份）", total, len(published), len(purchased)),
	})
	return ToolResult{Success: true, Content: string(b)}
}

func (t myMaterialsTool) AllowRepeat() bool  { return false }
func (t myMaterialsTool) ValidateArgs(_ string) error { return nil }
func (t myMaterialsTool) Describe(argsJSON string, result ToolResult) string {
	total := countJSONItems(result.Content)
	return fmt.Sprintf("我的资料 → 共 %d 份可访问", total)
}

// ============ search_documents — 搜文档内容 ============

type searchDocumentsTool struct {
	searchFunc SearchFunc
}

func (t searchDocumentsTool) Definition() ToolDef {
	return ToolDef{
		Name: ToolSearchDocuments,
		Description: "能做什么\n  在资料文档中进行语义检索，返回与用户问题相关的文本片段。\n" +
			"  不传 material_ids 时自动搜索用户全部可访问资料。\n" +
			"  传 material_ids 数组可一次搜多份（如 [6,7]），只计 1 次预算。\n\n" +
			"不能做什么\n  不搜资料基本信息如标题、价格、目录（用 get_material_detail）\n  不搜全平台资料（用 search_materials）\n  不搜 FAQ（用 search_faq）\n" +
			"  不保证搜到结果——语义检索可能存在召回盲区\n\n" +
			"何时优先使用\n  - 用户指定了一份或多份资料并问具体知识点 → 传 material_ids\n" +
			"  - 用户没指定资料 → 不传 ID，搜全部可访问资料\n" +
			"  - 搜不到时诚实告知「资料中未涉及该内容」，不要反复换 query 重试\n" +
			"  - 来源为 preview 的片段只能用于介绍和引导购买，不能作为完整答案的依据",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query":        map[string]interface{}{"type": "string", "description": "1-100字。从用户问题中提取核心知识点，用自然短语而非完整问句。\n✅「闭包的作用域」「函数的参数传递」— 概念明确长度适中\n❌「闭包」— 太短精度低 | 「用户问闭包相关问题帮我搜」— 问句噪音多\n方法：用户问\"XX怎么实现的\" → 传\"XX的实现\"；问\"XX和YY区别\" → 传\"XX YY 区别\""},
				"material_ids": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "number"}, "description": "资料ID列表，来自 my_materials 或 get_material_detail。可传 [6] 搜单份、[6,7] 搜多份。留空搜全部可访问资料。"},
			},
			"required": []string{"query"},
		},
	}
}

func (t searchDocumentsTool) Execute(userID uint, argsJSON string) ToolResult {
	var args struct {
		Query       string `json:"query"`
		MaterialIDs []uint `json:"material_ids"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return ToolResult{Success: false, Content: "参数解析失败，请调整参数格式后重试", ErrorCode: "INVALID_ARGUMENT", Recoverable: true, RecommendedAction: "fix_arguments_and_retry"}
		}
		slog.Info("search_documents 参数", "material_ids", args.MaterialIDs, "query", TruncateRunes(args.Query, 60))

		if strings.TrimSpace(args.Query) == "" {
			return ToolResult{Success: false, Content: "查询内容不能为空，请提供具体问题", ErrorCode: "MISSING_PARAMETER", Recoverable: true, RecommendedAction: "ask_user_for_query"}
		}
		if t.searchFunc == nil {
			return ToolResult{Success: false, Content: "资料检索服务暂不可用，请稍后重试", ErrorCode: "SERVICE_UNAVAILABLE", Recoverable: false, RecommendedAction: "tell_user_service_unavailable"}
		}

		// 不传 material_ids → 搜全部可访问资料
		if len(args.MaterialIDs) == 0 {
			args.MaterialIDs = getUserAccessibleMaterialIDs(userID)
		}
		return t.searchMultiple(userID, args.MaterialIDs, args.Query)
}

// searchMultiple 搜多份资料，汇总结果。
func (t searchDocumentsTool) searchMultiple(userID uint, materialIDs []uint, query string) ToolResult {
	if len(materialIDs) == 0 {
		return ToolResult{Success: true, Content: `{"success":true,"results":[],"total":0,"searched_materials":0,"hint":"未找到可访问的资料"}`}
	}
	var allResults []string
	searched := 0
	for _, mid := range materialIDs {
		hasAccess := checkHasAccess(userID, mid)
		content, err := t.searchFunc(mid, query, 5, hasAccess)
		if err != nil { continue }
		if content == "" || isSearchEmpty(content) { continue }
		allResults = append(allResults, content)
		searched++
	}
	if len(allResults) == 0 {
		return ToolResult{Success: true, Content: fmt.Sprintf(`{"success":true,"results":[],"total":0,"searched_materials":%d,"hint":"资料中未找到相关内容"}`, searched)}
	}
	combined := strings.Join(allResults, "\n")
	return ToolResult{Success: true, Content: combined}
}

func (t searchDocumentsTool) AllowRepeat() bool { return false }
func (t searchDocumentsTool) ValidateArgs(argsJSON string) error {
	var args struct {
		Query      string `json:"query"`
		MaterialID uint   `json:"material_id"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if strings.TrimSpace(args.Query) == "" { return errors.New("query 不能为空") }
	if len(args.Query) > 100 { return errors.New("query 不能超过 100 字") }
	return nil
}
func (t searchDocumentsTool) Describe(argsJSON string, result ToolResult) string {
	var args struct{ Query string }
	json.Unmarshal([]byte(argsJSON), &args)
	return fmt.Sprintf("搜索文档「%s」→ 找到 %d 条", args.Query, countJSONItems(result.Content))
}

// ============ get_orders — 订单查询 ============

type getOrdersTool struct{}

func (t getOrdersTool) Definition() ToolDef {
	return ToolDef{
		Name: ToolGetOrders,
		Description: "能做什么\n  查询当前用户的订单。不传 order_no 返回最近订单列表，传了返回该笔订单完整详情。\n\n" +
			"不能做什么\n  不查其他用户的订单\n  不能发起退款、取消订单或修改订单——这是只读工具\n  不查已购资料的内容（用 search_documents 或 get_material_detail）\n\n" +
			"何时优先使用\n  - 用户问「我的订单」「买了什么」「查看订单」且未提供订单号 → 不传 order_no，返回列表\n" +
			"  - 用户提供了具体订单号 → 传入 order_no 获取详情\n" +
			"  - 用户说「我的资料」不是问订单，用 my_materials",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"order_no":  map[string]interface{}{"type": "string", "description": "订单号字符串。只能来自本工具列表返回的 order_no 或用户直接提供。不可编造。"},
				"status":    map[string]interface{}{"type": "string", "enum": []string{"pending", "paid", "cancelled"}, "description": "筛选状态，仅列表模式有效"},
				"page":      map[string]interface{}{"type": "number", "description": "1-100，默认 1"},
				"page_size": map[string]interface{}{"type": "number", "description": "1-20，默认 10"},
			},
		},
	}
}

func (t getOrdersTool) Execute(userID uint, argsJSON string) ToolResult {
	var args struct {
		OrderNo  string `json:"order_no"`
		Status   string `json:"status"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
	json.Unmarshal([]byte(argsJSON), &args)

	db := database.DB.Where("user_id = ?", userID)

	// 详情模式
	if args.OrderNo != "" {
		var order model.Order
		if err := db.Where("order_no = ?", args.OrderNo).First(&order).Error; err != nil {
			return ToolResult{Success: false, Content: fmt.Sprintf("订单 %s 不存在，可用列表模式查看所有订单", args.OrderNo), ErrorCode: "NOT_FOUND", Recoverable: true, RecommendedAction: "use_list_mode_or_confirm_order_no"}
		}
		b, _ := json.Marshal(map[string]interface{}{
			"success": true,
			"order": map[string]interface{}{
				"order_no":   order.OrderNo,
				"course_id":  order.CourseID,
				"amount":     order.Amount,
				"status":     order.Status,
				"paid_at":    order.PaidAt,
				"created_at": order.CreatedAt.Format("2006-01-02 15:04"),
			},
		})
		return ToolResult{Success: true, Content: string(b)}
	}

	// 列表模式
	if args.Status != "" { db = db.Where("status = ?", args.Status) }
	if args.Page < 1 { args.Page = 1 }
	if args.PageSize < 1 || args.PageSize > 20 { args.PageSize = 10 }

	var total int64
	db.Model(&model.Order{}).Count(&total)
	if total == 0 {
		return ToolResult{Success: true, Content: `{"success":true,"orders":[],"total":0,"hint":"暂无订单记录"}`}
	}

	var orders []model.Order
	offset := (args.Page - 1) * args.PageSize
	db.Order("created_at DESC").Offset(offset).Limit(args.PageSize).Find(&orders)

	type item struct {
		OrderNo   string  `json:"order_no"`
		CourseID  uint    `json:"course_id"`
		Amount    float64 `json:"amount"`
		Status    string  `json:"status"`
		CreatedAt string  `json:"created_at"`
	}
	items := make([]item, 0, len(orders))
	for _, o := range orders {
		items = append(items, item{o.OrderNo, o.CourseID, o.Amount, o.Status, o.CreatedAt.Format("2006-01-02 15:04")})
	}
	b, _ := json.Marshal(map[string]interface{}{"success": true, "orders": items, "total": total})
	return ToolResult{Success: true, Content: string(b)}
}

func (t getOrdersTool) AllowRepeat() bool { return false }
func (t getOrdersTool) ValidateArgs(argsJSON string) error {
	var args struct {
		Status   string `json:"status"`
		PageSize int    `json:"page_size"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if args.Status != "" {
		valid := map[string]bool{"pending": true, "paid": true, "cancelled": true}
		if !valid[args.Status] { return errors.New("status 只能为 pending/paid/cancelled") }
	}
	if args.PageSize > 20 { return errors.New("page_size 不能超过 20") }
	return nil
}
func (t getOrdersTool) Describe(argsJSON string, result ToolResult) string {
	var args struct{ OrderNo string }
	json.Unmarshal([]byte(argsJSON), &args)
	if args.OrderNo != "" { return "查看订单详情" }
	return fmt.Sprintf("查询订单列表 → 共 %d 笔", countJSONItems(result.Content))
}

// ============ search_faq — 搜索 FAQ ============

type searchFAQTool struct{}

func (t searchFAQTool) Definition() ToolDef {
	return ToolDef{
		Name: ToolSearchFAQ,
		Description: "能做什么\n  在平台 FAQ 知识库中搜索匹配的问答对，覆盖退款政策、支付方式、使用指南等平台规则。\n\n" +
			"不能做什么\n  不能搜资料内容（用 search_documents）\n  不能查订单信息（用 get_orders）\n  不能编造 FAQ 中没有的答案——搜不到就说需要联系客服\n\n" +
			"何时优先使用\n  - 用户问平台规则：「怎么退款」「支持哪些支付方式」「多久发货」\n" +
			"  - 用户遇到售后问题需要政策依据\n  - 搜不到时直接告知「需要联系人工客服确认」，不要反复调用",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{"type": "string", "description": "1-100字。简洁关键词如「退款」「支付方式」，不用完整问句。"},
			},
			"required": []string{"query"},
		},
	}
}

func (t searchFAQTool) Execute(_ uint, argsJSON string) ToolResult {
	var args struct{ Query string `json:"query"` }
	json.Unmarshal([]byte(argsJSON), &args)

	var faqs []model.FAQ
	database.DB.Where("question LIKE ? OR answer LIKE ?", "%"+args.Query+"%", "%"+args.Query+"%").Limit(5).Find(&faqs)

	if len(faqs) == 0 {
		return ToolResult{Success: true, Content: `{"success":true,"faqs":[],"total":0,"hint":"FAQ 中未找到相关内容，建议联系人工客服"}`, ErrorCode: "NOT_FOUND", Recoverable: true, RecommendedAction: "tell_user_contact_support"}
	}

	type item struct {
		Question string `json:"question"`
		Answer   string `json:"answer"`
	}
	items := make([]item, 0, len(faqs))
	for _, f := range faqs { items = append(items, item{f.Question, f.Answer}) }
	b, _ := json.Marshal(map[string]interface{}{"success": true, "faqs": items, "total": len(items)})
	return ToolResult{Success: true, Content: string(b)}
}

func (t searchFAQTool) AllowRepeat() bool { return false }
func (t searchFAQTool) ValidateArgs(argsJSON string) error {
	var args struct{ Query string `json:"query"` }
	json.Unmarshal([]byte(argsJSON), &args)
	if strings.TrimSpace(args.Query) == "" { return errors.New("query 不能为空") }
	if len(args.Query) > 100 { return errors.New("query 不能超过 100 字") }
	return nil
}
func (t searchFAQTool) Describe(argsJSON string, result ToolResult) string {
	var args struct{ Query string }
	json.Unmarshal([]byte(argsJSON), &args)
	return fmt.Sprintf("搜索FAQ「%s」→ 找到 %d 条", args.Query, countJSONItems(result.Content))
}

// ============ purchase — 发起购买 ============

type purchaseTool struct{}

func (t purchaseTool) Definition() ToolDef {
	return ToolDef{
		Name: ToolPurchase,
		Description: "能做什么\n  向用户发送购买卡片。这是让用户看到购买入口的唯一方式——不调用此工具，用户无法下单。\n\n" +
			"不能做什么\n  不能替代文字回复——调用后仍需简要说明\n  不能替用户做购买决定\n  不能在用户未明确表达购买意向时调用\n\n" +
			"何时优先使用\n  - 用户明确表达购买意向：「买」「下单」「就这个」「来一份」「怎么买」\n" +
			"  - 用户已在多次对话中持续关注某份资料，最后确认想购买\n" +
			"  - 调用前应确认用户尚未拥有该资料（通过 get_material_detail.access.has_purchased）\n" +
			"  - 调用后等用户决策，不要再推其他资料",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"material_id": map[string]interface{}{"type": "number", "description": ">0。只能来自 search_materials 或 get_material_detail 返回的真实ID。"},
			},
			"required": []string{"material_id"},
		},
	}
}

func (t purchaseTool) Execute(userID uint, argsJSON string) ToolResult {
	var args struct{ MaterialID uint `json:"material_id"` }
	json.Unmarshal([]byte(argsJSON), &args)

	var material model.Material
	if err := database.DB.First(&material, args.MaterialID).Error; err != nil {
		return ToolResult{Success: false, Content: fmt.Sprintf("资料 #%d 不存在或已下架，请用 search_materials 重新查找", args.MaterialID), ErrorCode: "NOT_FOUND", Recoverable: true, RecommendedAction: "confirm_material_id_with_user"}
	}

	if checkHasAccess(userID, args.MaterialID) {
		return ToolResult{Success: false, Content: "您已拥有该资料，无需重复购买", ErrorCode: "ALREADY_OWNED", Recoverable: false, RecommendedAction: "tell_user_already_owned"}
	}

	b, _ := json.Marshal(map[string]interface{}{
		"success":              true,
		"__action":             "purchase_offer",
		"material_id":          material.ID,
		"title":                material.Title,
		"price":                material.Price,
		"cover_image":          material.CoverImage,
		"requires_user_action": true,
		"hint": "购买卡片已发送，用户需点击卡片完成下单。在用户完成下单前，不要声称已购买。",
	})
	return ToolResult{Success: true, Content: string(b)}
}

func (t purchaseTool) AllowRepeat() bool { return true }
func (t purchaseTool) ValidateArgs(argsJSON string) error {
	var args struct{ MaterialID uint `json:"material_id"` }
	json.Unmarshal([]byte(argsJSON), &args)
	if args.MaterialID == 0 { return errors.New("material_id 不能为空") }
	var count int64
	database.DB.Model(&model.Material{}).Where("id = ? AND status = ?", args.MaterialID, "published").Count(&count)
	if count == 0 { return fmt.Errorf("资料 #%d 不存在或已下架", args.MaterialID) }
	return nil
}
func (t purchaseTool) Describe(argsJSON string, result ToolResult) string { return "发送购买卡片" }

// ============ 辅助函数 ============

// avgRating 计算评价平均分。
func avgRating(reviews []model.Review) float64 {
	if len(reviews) == 0 { return 0 }
	sum := 0
	for _, r := range reviews { sum += r.Rating }
	return float64(sum) / float64(len(reviews))
}

// isSearchEmpty 检查 searchFunc 返回的内容是否为空结果（found:false 包装）。
func isSearchEmpty(content string) bool {
	return strings.Contains(content, `"found":false`) || strings.Contains(content, `"found\":false`)
}

// getUserAccessibleMaterialIDs 获取用户可访问的所有资料ID（发布的 + 购买的）。
func getUserAccessibleMaterialIDs(userID uint) []uint {
	var ids []uint
	var published []model.Material
	database.DB.Select("id").Where("user_id = ?", userID).Find(&published)
	for _, m := range published { ids = append(ids, m.ID) }
	var orders []model.Order
	database.DB.Select("course_id").Where("user_id = ? AND status = ?", userID, "paid").Find(&orders)
	for _, o := range orders { ids = appendUnique(ids, o.CourseID) }
	return ids
}

// ============ Tool 集合构建 ============

// buildToolSet 构建全量 Tool 集合（7 个）。
func buildToolSet(searchFunc SearchFunc) map[string]Tool {
	tools := map[string]Tool{
		ToolSearchMaterials:   searchMaterialsTool{},
		ToolGetMaterialDetail: getMaterialDetailTool{},
		ToolMyMaterials:       myMaterialsTool{},
		ToolGetOrders:         getOrdersTool{},
		ToolSearchFAQ:         searchFAQTool{},
		ToolPurchase:          purchaseTool{},
	}
	if searchFunc != nil {
		tools[ToolSearchDocuments] = searchDocumentsTool{searchFunc: searchFunc}
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
