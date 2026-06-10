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
type SearchFunc func(courseID uint, query string, topK int) (string, error)

// ============ 工具常量 ============

const (
	ToolQueryOrders   = "query_orders"
	ToolQueryCourses  = "query_courses"
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
		Name:        ToolQueryCourses,
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
		return ToolResult{Success: true, Content: "未找到匹配的课程"}
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
		Description: "搜索某门课程的学习资料内容，返回相关的文本片段。用于回答关于课程内容的具体问题。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"course_id": map[string]interface{}{"type": "number", "description": "课程ID"},
				"query":     map[string]interface{}{"type": "string", "description": "要搜索的问题或关键词"},
			},
			"required": []string{"course_id", "query"},
		},
	}
}

func (t searchMaterialsTool) Execute(_ uint, argsJSON string) ToolResult {
	var args struct {
		CourseID uint   `json:"course_id"`
		Query    string `json:"query"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{Success: false, Content: "参数解析失败"}
	}
	if t.searchFunc == nil {
		return ToolResult{Success: false, Content: "资料检索服务暂不可用"}
	}
	content, err := t.searchFunc(args.CourseID, args.Query, 5)
	if err != nil {
		return ToolResult{Success: false, Content: "检索失败: " + err.Error()}
	}
	if content == "" {
		return ToolResult{Success: true, Content: "未找到相关资料"}
	}
	return ToolResult{Success: true, Content: content}
}

// ============ Tool 集合构建 ============

// buildToolSet 构建 Agent 的 Tool 集合
func buildToolSet(agentType string, searchFunc SearchFunc) map[string]Tool {
	tools := make(map[string]Tool)

	switch agentType {
	case model.AgentCustomerService:
		tools[ToolQueryOrders] = queryOrdersTool{}
	case model.AgentCourseRecommend:
		tools[ToolQueryCourses] = queryCoursesTool{}
	case model.AgentQA:
		if searchFunc != nil {
			tools[ToolSearchMaterials] = newSearchMaterialsTool(searchFunc)
		}
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
