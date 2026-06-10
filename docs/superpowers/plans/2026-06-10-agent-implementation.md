# v2 智能 Agent 系统实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建 Router + 3 Agent（客服/推荐/答疑）+ RAG 的智能 Agent 系统

**Architecture:** Go 自研 Agent 引擎（Tool Calling 循环 + SSE 流式输出），Redis Stack 向量检索，SSE 协议前后端通信，三 Agent 共享引擎，意图路由分发

**Tech Stack:** Go 1.x + Gin + GORM + MySQL + Redis Stack + DeepSeek API (OpenAI 兼容)

---

## 文件结构总览

### 新建文件（后端）
| 文件 | 职责 |
|------|------|
| `model/session.go` | Session 模型 |
| `model/message.go` | Message 模型 |
| `model/document_chunk.go` | DocumentChunk 模型 |
| `dto/request/agent.go` | Agent 请求 DTO |
| `service/agent_engine.go` | Agent 引擎核心（Tool 循环 + 上下文 + SSE） |
| `service/agent_tools.go` | 所有 Tool 定义和实现 |
| `service/agent_router.go` | 意图路由（规则 + LLM fallback） |
| `service/agent_prompts.go` | 三个 Agent 的 System Prompt |
| `service/agent_service.go` | AgentService 总调度（会话管理 + 编排） |
| `service/agent_rag.go` | RAG 服务（切片 + 向量化 + 检索 + VectorStore 接口） |
| `controller/agent_controller.go` | Agent 控制器（SSE handler + session CRUD） |
| `service/agent_engine_test.go` | 引擎测试 |
| `service/agent_router_test.go` | 路由测试 |
| `service/agent_service_test.go` | AgentService 集成测试 |
| `service/agent_rag_test.go` | RAG 测试 |

### 修改文件（后端）
| 文件 | 变更 |
|------|------|
| `config/config.go` | 新增 `AgentConfig` 结构体 |
| `config/app.yml` | 新增 `agent:` 配置段 |
| `database/mysql.go` | `autoMigrate()` 新增三个模型 |
| `service/setup_test.go` | `cleanAllTestData()` 新增三张表清理 |
| `router/router.go` | 新增 `/api/agent/*` 路由组 |

### 新建文件（前端）
| 文件 | 职责 |
|------|------|
| `web/src/api/agent.js` | Agent API 封装（SSE fetch） |
| `web/src/views/AgentChat.vue` | Agent 聊天主页面 |

### 修改文件（前端）
| 文件 | 变更 |
|------|------|
| `web/src/router/index.js` | 新增 `/agent` 路由 |

---

## Phase 1: 数据模型 + 配置

### Task 1: Session 模型

**Files:**
- Create: `model/session.go`

- [ ] **Step 1: 写入 Session 模型**

```go
package model

import "time"

// AgentType 定义
const (
	AgentCustomerService  = "customer_service"
	AgentCourseRecommend = "course_recommend"
	AgentQA              = "qa"
)

// SessionStatus 定义
const (
	SessionActive = "active"
	SessionClosed = "closed"
)

// Session AI 对话会话模型
type Session struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uint      `gorm:"not null;index" json:"user_id"`
	AgentType string    `gorm:"type:varchar(30);not null;default:customer_service" json:"agent_type"`
	Title     string    `gorm:"type:varchar(100);default:''" json:"title"`
	Status    string    `gorm:"type:varchar(20);not null;default:active" json:"status"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	User     User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Messages []Message `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE" json:"messages,omitempty"`
}

// TableName 指定表名
func (Session) TableName() string {
	return "sessions"
}
```

- [ ] **Step 2: 验证编译通过**

```bash
cd d:/Vscoding/edu_market && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add model/session.go
git commit -m "feat: add Session model"
```

---

### Task 2: Message 模型

**Files:**
- Create: `model/message.go`

- [ ] **Step 1: 写入 Message 模型**

```go
package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// MessageRole 定义
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// ToolCall JSON 结构（存 messages.tool_calls 字段）
type ToolCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Result    string `json:"result,omitempty"`
}

// ToolCalls 实现 sql.Scanner / driver.Valuer
type ToolCalls []ToolCall

func (tc *ToolCalls) Scan(value interface{}) error {
	if value == nil {
		*tc = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, tc)
}

func (tc ToolCalls) Value() (driver.Value, error) {
	if tc == nil {
		return nil, nil
	}
	return json.Marshal(tc)
}

// Message AI 对话消息模型
type Message struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID  uint      `gorm:"not null;index" json:"session_id"`
	Role       string    `gorm:"type:varchar(20);not null" json:"role"`
	Content    string    `gorm:"type:text;not null" json:"content"`
	ToolCalls  ToolCalls `gorm:"type:json;default:null" json:"tool_calls,omitempty"`
	TokensUsed int       `gorm:"default:0" json:"tokens_used"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`

	Session Session `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE" json:"-"`
}

// TableName 指定表名
func (Message) TableName() string {
	return "messages"
}
```

- [ ] **Step 2: 验证编译通过**

```bash
cd d:/Vscoding/edu_market && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add model/message.go
git commit -m "feat: add Message model with JSON ToolCalls type"
```

---

### Task 3: DocumentChunk 模型

**Files:**
- Create: `model/document_chunk.go`

- [ ] **Step 1: 写入 DocumentChunk 模型**

```go
package model

import "time"

// DocumentChunk RAG 文档块模型
type DocumentChunk struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CourseID   uint      `gorm:"not null;index" json:"course_id"`
	Content    string    `gorm:"type:text;not null" json:"content"`
	ChunkIndex int       `gorm:"not null;default:0" json:"chunk_index"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`

	Course Course `gorm:"foreignKey:CourseID;constraint:OnDelete:CASCADE" json:"-"`
}

// TableName 指定表名
func (DocumentChunk) TableName() string {
	return "document_chunks"
}
```

- [ ] **Step 2: 验证编译通过**

```bash
cd d:/Vscoding/edu_market && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add model/document_chunk.go
git commit -m "feat: add DocumentChunk model for RAG"
```

---

### Task 4: 注册模型到 AutoMigrate + TestMain 清理

**Files:**
- Modify: `database/mysql.go:48-58`
- Modify: `service/setup_test.go:42-51`

- [ ] **Step 1: mysql.go 注册新模型**

在 `database/mysql.go` 的 `autoMigrate()` 函数中，在 `&model.Conversation{}` 后面追加新模型：

```go
// autoMigrate 自动迁移所有模型
func autoMigrate() error {
	return DB.AutoMigrate(
		&model.User{},
		&model.Category{},
		&model.Course{},
		&model.Order{},
		&model.Review{},
		&model.Conversation{},
		&model.Session{},
		&model.Message{},
		&model.DocumentChunk{},
	)
}
```

- [ ] **Step 2: setup_test.go 注册清理**

在 `service/setup_test.go` 的 `cleanAllTestData()` 函数中，在 `Conversation` 行后面追加新模型清理（按外键顺序 Message → Session，DocumentChunk 不依赖其他新表）：

```go
func cleanAllTestData() {
	database.DB.Exec("SET FOREIGN_KEY_CHECKS = 0")
	database.DB.Where("1=1").Delete(&model.Message{})
	database.DB.Where("1=1").Delete(&model.Session{})
	database.DB.Where("1=1").Delete(&model.DocumentChunk{})
	database.DB.Where("1=1").Delete(&model.Review{})
	database.DB.Where("1=1").Delete(&model.Order{})
	database.DB.Where("1=1").Delete(&model.Conversation{})
	database.DB.Where("1=1").Delete(&model.Course{})
	database.DB.Where("1=1").Delete(&model.Category{})
	database.DB.Where("1=1").Delete(&model.User{})
	database.DB.Exec("SET FOREIGN_KEY_CHECKS = 1")
}
```

- [ ] **Step 3: 验证数据库迁移（启动一次看 AutoMigrate 日志无报错）**

```bash
cd d:/Vscoding/edu_market && go run . &
sleep 5 && curl http://localhost:8080/api/categories
```

预期：正常返回分类列表，日志中出现 sessions/messages/document_chunks 表创建信息。

- [ ] **Step 4: 跑测试确认 TestMain 通过**

```bash
cd d:/Vscoding/edu_market && go test ./service/ -v -run TestMain 2>&1 | head -20
```

- [ ] **Step 5: Commit**

```bash
git add database/mysql.go service/setup_test.go
git commit -m "feat: register Session/Message/DocumentChunk models in AutoMigrate and test cleanup"
```

---

### Task 5: Agent 配置 + DTO

**Files:**
- Modify: `config/config.go:18`（新增 AgentConfig 字段）
- Create: `config/config.go:70-85`（新增 AgentConfig 结构体）
- Modify: `config/app.yml`（新增 agent 配置段）
- Create: `dto/request/agent.go`

- [ ] **Step 1: 新增 AgentConfig 结构体**

在 `config/config.go` 中，在 `UploadConfig` 之后追加：

```go
// AgentConfig Agent 配置
type AgentConfig struct {
	MaxToolRounds     int    `mapstructure:"max_tool_rounds"`
	ContextMaxMsg     int    `mapstructure:"context_max_messages"`
	EmbeddingModel    string `mapstructure:"embedding_model"`
	EmbeddingAPIURL   string `mapstructure:"embedding_api_url"`
	ChunkSize         int    `mapstructure:"chunk_size"`
	ChunkOverlap      int    `mapstructure:"chunk_overlap"`
}
```

在 `Config` 结构体的 `Upload UploadConfig` 行后面追加：

```go
Agent AgentConfig `mapstructure:"agent"`
```

- [ ] **Step 2: 更新 app.yml**

在 `config/app.yml` 的 `upload:` 段后追加：

```yaml
agent:
  max_tool_rounds: 7
  context_max_messages: 20
  embedding_model: ""
  embedding_api_url: ""
  chunk_size: 500
  chunk_overlap: 50
```

- [ ] **Step 3: 新增请求 DTO**

创建 `dto/request/agent.go`：

```go
package request

// AgentChatReq Agent 对话请求
type AgentChatReq struct {
	SessionID *uint  `json:"session_id"`                    // 可选，不传则新建会话
	Question  string `json:"question" binding:"required,min=1"`
}

// AgentSessionsReq 会话列表请求
type AgentSessionsReq struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=50"`
}

// AgentMessagesReq 消息历史请求
type AgentMessagesReq struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
}
```

- [ ] **Step 4: 验证编译**

```bash
cd d:/Vscoding/edu_market && go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add config/config.go config/app.yml dto/request/agent.go
git commit -m "feat: add Agent config and request DTOs"
```

---

## Phase 2: Agent 引擎核心

### Task 6: Tool 定义接口 + 所有 Tool 实现

**Files:**
- Create: `service/agent_tools.go`

Tool 是 Agent 引擎的核心插件。每个 Tool 有名称、描述（给 LLM 看）、参数 Schema 和执行函数。

- [ ] **Step 1: 写入 service/agent_tools.go**

```go
package service

import (
	"encoding/json"
	"fmt"
	"strconv"

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

// ============ 客服 Agent Tools ============

const toolQueryOrders = "query_orders"

type queryOrdersTool struct{}

func (t queryOrdersTool) Definition() ToolDef {
	return ToolDef{
		Name:        toolQueryOrders,
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

const toolQueryCourses = "query_courses"

type queryCoursesTool struct{}

func (t queryCoursesTool) Definition() ToolDef {
	return ToolDef{
		Name:        toolQueryCourses,
		Description: "按关键词、分类ID、价格范围搜索课程列表",
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
		Keyword    string  `json:"keyword"`
		CategoryID *uint   `json:"category_id"`
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

const toolSearchMaterials = "search_course_materials"

type searchMaterialsTool struct {
	rag *RAGService
}

func (t searchMaterialsTool) Definition() ToolDef {
	return ToolDef{
		Name:        toolSearchMaterials,
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
	if t.rag == nil {
		return ToolResult{Success: false, Content: "资料检索服务暂不可用"}
	}
	results, err := t.rag.Search(args.CourseID, args.Query, 5)
	if err != nil {
		return ToolResult{Success: false, Content: "检索失败: " + err.Error()}
	}
	if len(results) == 0 {
		return ToolResult{Success: true, Content: "未找到相关资料"}
	}
	bytes, _ := json.Marshal(results)
	return ToolResult{Success: true, Content: string(bytes)}
}

// ============ 三个 Agent 的 Tool 集合 ============

// buildToolSet 构建 Agent 的 Tool 集合
func buildToolSet(agentType string, rag *RAGService) map[string]Tool {
	tools := make(map[string]Tool)

	switch agentType {
	case model.AgentCustomerService:
		tools[toolQueryOrders] = queryOrdersTool{}
	case model.AgentCourseRecommend:
		tools[toolQueryCourses] = queryCoursesTool{}
	case model.AgentQA:
		tools[toolSearchMaterials] = searchMaterialsTool{rag: rag}
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

- [ ] **Step 2: 验证编译**

```bash
cd d:/Vscoding/edu_market && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add service/agent_tools.go
git commit -m "feat: add Tool interface and all agent tool implementations"
```

---

### Task 7: Agent 引擎核心

**Files:**
- Create: `service/agent_engine.go`

Agent 引擎是核心模块：上下文管理器 + Tool Calling 循环 + SSE Writer。

- [ ] **Step 1: 写入 service/agent_engine.go**

```go
package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"edu_market/config"
	"edu_market/database"
	"edu_market/model"
)

// SSECallback SSE 事件回调接口（由控制器实现，注入 gin.Context 的 writer）
type SSECallback func(event string, data string) error

// chatMsg LLM 消息格式
type engineChatMsg struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []engineToolCall `json:"tool_calls,omitempty"`
}

type engineToolCall struct {
	ID       string                  `json:"id"`
	Type     string                  `json:"type"`
	Function engineToolCallFunction  `json:"function"`
}

type engineToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// engineReq LLM 请求体
type engineReq struct {
	Model    string         `json:"model"`
	Messages []engineChatMsg `json:"messages"`
	Stream   bool           `json:"stream"`
	Tools    []map[string]interface{} `json:"tools,omitempty"`
}

// engineChoice LLM 响应 choice（非流式）
type engineChoice struct {
	Message      engineChatMsg `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

// engineUsage 用量
type engineUsage struct {
	TotalTokens int `json:"total_tokens"`
}

// engineResp 非流式 LLM 响应
type engineResp struct {
	Choices []engineChoice `json:"choices"`
	Usage   engineUsage    `json:"usage"`
}

// AgentEngine Agent 引擎
type AgentEngine struct {
	maxRounds    int
	contextLimit int
}

// NewAgentEngine 创建引擎实例
func NewAgentEngine() *AgentEngine {
	cfg := config.App.Agent
	return &AgentEngine{
		maxRounds:    cfg.MaxToolRounds,
		contextLimit: cfg.ContextMaxMsg,
	}
}

// Run 执行 Agent 循环，通过 sseCallback 输出事件
func (e *AgentEngine) Run(
	session *model.Session,
	userMsg string,
	tools map[string]Tool,
	systemPrompt string,
	sseCallback SSECallback,
) error {
	// 1. 存用户消息
	userMessage := &model.Message{
		SessionID: session.ID,
		Role:      model.RoleUser,
		Content:   userMsg,
	}
	if err := database.DB.Create(userMessage).Error; err != nil {
		return fmt.Errorf("保存用户消息失败: %w", err)
	}

	// 2. 加载上下文（含 system prompt）
	messages := e.loadContext(session.ID, systemPrompt)

	// 3. Tool Calling 循环
	openAITools := toolDefsToOpenAI(tools)
	if len(openAITools) == 0 {
		openAITools = nil
	}

	for round := 0; round < e.maxRounds; round++ {
		// 发请求给 LLM
		resp, err := e.callLLM(messages, openAITools)
		if err != nil {
			sseCallback("error", `{"message":"AI 服务暂时不可用，请稍后再试"}`)
			return err
		}

		choice := resp.Choices[0]

		// LLM 返回了 Tool Calls → 执行工具
		if len(choice.Message.ToolCalls) > 0 {
			for _, tc := range choice.Message.ToolCalls {
				// 通知前端
				sseCallback("thinking", fmt.Sprintf(`{"tool":"%s","status":"executing"}`, tc.Function.Name))

				// 查找并执行工具
				tool, ok := tools[tc.Function.Name]
				var result ToolResult
				if !ok {
					result = ToolResult{Success: false, Content: fmt.Sprintf("未知工具: %s", tc.Function.Name)}
				} else {
					result = tool.Execute(session.UserID, tc.Function.Arguments)
				}

				// 存 tool message
				toolMsg := model.Message{
					SessionID: session.ID,
					Role:      model.RoleTool,
					Content:   result.Content,
					ToolCalls: model.ToolCalls{{Name: tc.Function.Name, Arguments: tc.Function.Arguments, Result: result.Content}},
				}
				if err := database.DB.Create(&toolMsg).Error; err != nil {
					return fmt.Errorf("保存工具消息失败: %w", err)
				}

				// tool result 加入上下文
				messages = append(messages,
					engineChatMsg{Role: "assistant", ToolCalls: []engineToolCall{{
						ID: tc.ID, Type: "function",
						Function: engineToolCallFunction{Name: tc.Function.Name, Arguments: tc.Function.Arguments},
					}}},
					engineChatMsg{Role: "tool", Content: result.Content, ToolCallID: tc.ID},
				)
			}
			continue
		}

		// 没有 Tool Call → 流式输出回答
		answer := choice.Message.Content
		if answer != "" {
			// 非流式 fallback: 全文输出
			if err := e.streamAnswer(answer, sseCallback); err != nil {
				return err
			}

			// 存 assistant message
			assistantMsg := &model.Message{
				SessionID:  session.ID,
				Role:       model.RoleAssistant,
				Content:    answer,
				TokensUsed: resp.Usage.TotalTokens,
			}
			if err := database.DB.Create(assistantMsg).Error; err != nil {
				return fmt.Errorf("保存回答失败: %w", err)
			}
		}

		// 完成
		sseCallback("done", fmt.Sprintf(`{"session_id":%d,"agent_type":"%s"}`, session.ID, session.AgentType))
		return nil
	}

	// 超过最大轮数
	sseCallback("error", `{"message":"抱歉，这个问题比较复杂，请联系人工客服"}`)
	return nil
}

// loadContext 从 messages 表加载上下文，system prompt 插入最前
func (e *AgentEngine) loadContext(sessionID uint, systemPrompt string) []engineChatMsg {
	messages := []engineChatMsg{
		{Role: "system", Content: systemPrompt},
	}

	var dbMsgs []model.Message
	database.DB.Where("session_id = ?", sessionID).
		Order("id ASC").Limit(e.contextLimit).Find(&dbMsgs)

	for _, m := range dbMsgs {
		msg := engineChatMsg{Role: m.Role, Content: m.Content}
		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				msg.ToolCalls = append(msg.ToolCalls, engineToolCall{
					ID:       fmt.Sprintf("call_%d", m.ID),
					Type:     "function",
					Function: engineToolCallFunction{Name: tc.Name, Arguments: tc.Arguments},
				})
			}
		}
		messages = append(messages, msg)
	}

	return messages
}

// callLLM 调用 LLM API（非流式）
func (e *AgentEngine) callLLM(messages []engineChatMsg, tools []map[string]interface{}) (*engineResp, error) {
	reqBody := engineReq{
		Model:    config.App.AI.Model,
		Messages: messages,
		Stream:   false,
		Tools:    tools,
	}

	jsonBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", config.App.AI.APIURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.App.AI.APIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AI API 返回状态 %d: %s", resp.StatusCode, string(body))
	}

	var result engineResp
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析 AI 响应失败: %w", err)
	}
	return &result, nil
}

// streamAnswer 将完整回答分段模拟流式输出
func (e *AgentEngine) streamAnswer(answer string, sseCallback SSECallback) error {
	// 按字符逐个输出，模拟流式效果
	runes := []rune(answer)
	for i := 0; i < len(runes); i++ {
		chunk := string(runes[i])
		data, _ := json.Marshal(map[string]string{"content": chunk})
		if err := sseCallback("delta", string(data)); err != nil {
			return err
		}
	}
	return nil
}

// callLLMStream 流式调用 LLM API（预留，当前用 callLLM 非流式 + 模拟流式输出）
func (e *AgentEngine) callLLMStream(messages []engineChatMsg, tools []map[string]interface{}, sseCallback SSECallback) (string, int, error) {
	reqBody := engineReq{
		Model:    config.App.AI.Model,
		Messages: messages,
		Stream:   true,
		Tools:    tools,
	}

	jsonBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", config.App.AI.APIURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.App.AI.APIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	var fullContent strings.Builder
	totalTokens := 0

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *engineUsage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		for _, c := range chunk.Choices {
			if c.Delta.Content != "" {
				fullContent.WriteString(c.Delta.Content)
				contentJSON, _ := json.Marshal(map[string]string{"content": c.Delta.Content})
				sseCallback("delta", string(contentJSON))
			}
		}
		if chunk.Usage != nil {
			totalTokens = chunk.Usage.TotalTokens
		}
	}

	return fullContent.String(), totalTokens, scanner.Err()
}
```

- [ ] **Step 2: 验证编译**

```bash
cd d:/Vscoding/edu_market && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add service/agent_engine.go
git commit -m "feat: add Agent engine core (Tool loop + context manager + SSE)"
```

---

## Phase 3: Router + 三个 Agent + AgentService

### Task 8: Agent Prompts

**Files:**
- Create: `service/agent_prompts.go`

- [ ] **Step 1: 写入三个 Agent 的 System Prompt**

```go
package service

// SystemPromptCustomerService 客服 Agent
const SystemPromptCustomerService = `你是 edu_market 在线学习平台的智能客服。你的职责是帮助用户解决订单、支付、退款、平台使用等问题。

行为准则：
- 回答要简洁精确，1-2 轮内解决问题
- 如果用户问到课程推荐相关的问题，先完成当前回答，然后在末尾标记 [TRANSFER:course_recommend]
- 可以调用 query_orders 查看用户订单
- 对平台不存在的功能（如优惠券、会员）诚实说明"该功能暂未开放"
- 始终保持礼貌和耐心`

// SystemPromptCourseRecommend 课程推荐 Agent
const SystemPromptCourseRecommend = `你是 edu_market 平台的专业学习顾问。你的职责是了解用户的学习目标和背景，推荐最合适的课程。

行为准则：
- 先了解用户的学习目标、现有基础，再给出推荐。不要一上来就扔课程列表
- 用 query_courses 搜索课程，把结果以友好的方式呈现
- 每次推荐 2-3 门课程，不要太多
- 推荐时简要说明理由（为什么适合用户）
- 如果用户对某门课程有深入疑问（如课程内容、难度、前置知识），标记 [TRANSFER:qa]
- 如果用户问退款、订单等非推荐问题，标记 [TRANSFER:customer_service]`

// SystemPromptQA 答疑 Agent
const SystemPromptQA = `你是 edu_market 平台的专业课程助教。你的职责是基于课程资料深度解答用户问题。

行为准则：
- 优先使用 search_course_materials 检索课程相关资料，基于资料原文回答
- 回答要详细严谨，引用资料原文时注明出处
- 鼓励用户追问和深入讨论
- 如果未检索到相关资料，用你自己的知识回答但标注"注意：以下回答基于通用知识，非课程资料"
- 如果用户讨论到课程选择或推荐话题，标记 [TRANSFER:course_recommend]
- 保持耐心，用通俗语言解释复杂概念`
```

- [ ] **Step 2: 验证编译**

```bash
cd d:/Vscoding/edu_market && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add service/agent_prompts.go
git commit -m "feat: add three agent system prompts"
```

---

### Task 9: Router 意图路由

**Files:**
- Create: `service/agent_router.go`

- [ ] **Step 1: 写入 Router**

```go
package service

import (
	"strings"

	"edu_market/model"
)

// routeKeywords 关键词路由表
var routeKeywords = map[string][]string{
	model.AgentCustomerService: {
		"退款", "订单", "支付失败", "怎么买", "申诉", "客服", "联系", "投诉",
		"价格", "优惠券", "发货", "物流", "退货",
	},
	model.AgentCourseRecommend: {
		"推荐", "有什么课", "适合我", "哪个好", "入门", "进阶", "有没有",
		"零基础", "学什么", "想学", "选课", "哪个课程", "对比",
	},
	model.AgentQA: {
		"这个公式", "第三章", "解释一下", "为什么", "怎么做", "详细讲讲",
		"讲义", "课件", "这里为什么", "推导", "证明", "怎么理解",
	},
}

// RouteIntent 根据用户消息路由到对应的 Agent 类型
// 返回 agentType 和是否需要 LLM 二次判断
func RouteIntent(message string) (agentType string, needLLM bool) {
	msgLower := strings.ToLower(message)

	// 1. 统计关键词命中数
	scores := map[string]int{
		model.AgentCustomerService:  0,
		model.AgentCourseRecommend:  0,
		model.AgentQA:               0,
	}

	for agentType, keywords := range routeKeywords {
		for _, kw := range keywords {
			if strings.Contains(msgLower, kw) {
				scores[agentType]++
			}
		}
	}

	// 2. 确定最高分
	maxScore := 0
	maxAgent := ""
	for agentType, score := range scores {
		if score > maxScore {
			maxScore = score
			maxAgent = agentType
		}
	}

	// 3. 如果有明确最高分 → 直接路由
	if maxScore > 0 {
		// 检查是否有并列
		tied := false
		for agentType, score := range scores {
			if score == maxScore && agentType != maxAgent {
				tied = true
				break
			}
		}
		if !tied {
			return maxAgent, false
		}
	}

	// 4. 无命中或并列 → 需要 LLM 判断
	return "", true
}

// DetectTransfer 检测 LLM 回答中的 Agent 切换标记
func DetectTransfer(answer string) (shouldTransfer bool, targetAgent string) {
	if strings.Contains(answer, "[TRANSFER:qa]") {
		return true, model.AgentQA
	}
	if strings.Contains(answer, "[TRANSFER:course_recommend]") {
		return true, model.AgentCourseRecommend
	}
	if strings.Contains(answer, "[TRANSFER:customer_service]") {
		return true, model.AgentCustomerService
	}
	return false, ""
}

// CleanTransferMarkers 清除回答中的切换标记，返回干净的文本
func CleanTransferMarkers(answer string) string {
	answer = strings.ReplaceAll(answer, "[TRANSFER:qa]", "")
	answer = strings.ReplaceAll(answer, "[TRANSFER:course_recommend]", "")
	answer = strings.ReplaceAll(answer, "[TRANSFER:customer_service]", "")
	return strings.TrimSpace(answer)
}

// GetAgentPrompt 根据 Agent 类型返回对应的 System Prompt
func GetAgentPrompt(agentType string) string {
	switch agentType {
	case model.AgentCustomerService:
		return SystemPromptCustomerService
	case model.AgentCourseRecommend:
		return SystemPromptCourseRecommend
	case model.AgentQA:
		return SystemPromptQA
	default:
		return SystemPromptCustomerService
	}
}
```

- [ ] **Step 2: 验证编译**

```bash
cd d:/Vscoding/edu_market && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add service/agent_router.go
git commit -m "feat: add intent router with keyword matching and transfer detection"
```

---

### Task 10: AgentService 总调度

**Files:**
- Create: `service/agent_service.go`

`AgentService` 是入口：会话管理 + 路由 + 调引擎 + 标题生成 + 切换检测。

- [ ] **Step 1: 写入 AgentService**

```go
package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"edu_market/config"
	"edu_market/database"
	"edu_market/model"

	"gorm.io/gorm"
)

// AgentService Agent 总调度服务
type AgentService struct {
	engine *AgentEngine
}

// NewAgentService 创建 AgentService
func NewAgentService(engine *AgentEngine) *AgentService {
	return &AgentService{engine: engine}
}

// Chat 发起/继续 Agent 对话
// rag 可以为 nil（客服/推荐 Agent 不用 RAG）
func (s *AgentService) Chat(userID uint, sessionID *uint, question string, rag *RAGService, sseCallback SSECallback) (*model.Session, error) {
	// 1. 获取或创建 Session
	var session *model.Session
	if sessionID != nil {
		session = &model.Session{}
		if err := database.DB.Where("id = ? AND user_id = ?", *sessionID, userID).First(session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("会话不存在")
			}
			return nil, err
		}
	} else {
		// 新会话：先路由，再创建
		agentType, needLLM := RouteIntent(question)
		if needLLM || agentType == "" {
			// LLM 路由未实现时默认客服
			agentType = model.AgentCustomerService
		}
		session = &model.Session{
			UserID:    userID,
			AgentType: agentType,
			Status:    model.SessionActive,
			Title:     truncateString(question, 30),
		}
		if err := database.DB.Create(session).Error; err != nil {
			return nil, fmt.Errorf("创建会话失败: %w", err)
		}
	}

	// 2. 构建 Prompt + Tools
	systemPrompt := GetAgentPrompt(session.AgentType)
	tools := buildToolSet(session.AgentType, rag)

	// 3. 运行引擎
	if err := s.engine.Run(session, question, tools, systemPrompt, sseCallback); err != nil {
		return session, err
	}

	// 4. 更新 title（首次对话后，从最后一条 assistant 消息生成）
	if session.Title == "" || session.Title == truncateString(question, 30) {
		s.updateTitle(session)
	}

	// 5. 检测 Agent 切换标记
	s.checkTransfer(session)

	return session, nil
}

// GetSessions 获取用户会话列表
func (s *AgentService) GetSessions(userID uint, page, pageSize int) ([]model.Session, int64, error) {
	page, pageSize = GetPagination(page, pageSize)
	var sessions []model.Session
	var total int64

	database.DB.Model(&model.Session{}).Where("user_id = ?", userID).Count(&total)
	if err := database.DB.Where("user_id = ?", userID).
		Order("updated_at DESC").Offset((page-1)*pageSize).Limit(pageSize).
		Find(&sessions).Error; err != nil {
		return nil, 0, err
	}
	return sessions, total, nil
}

// DeleteSession 关闭会话（软状态变更）
func (s *AgentService) DeleteSession(userID, sessionID uint) error {
	result := database.DB.Model(&model.Session{}).
		Where("id = ? AND user_id = ?", sessionID, userID).
		Update("status", model.SessionClosed)
	if result.RowsAffected == 0 {
		return errors.New("会话不存在")
	}
	return result.Error
}

// GetMessages 获取会话消息历史
func (s *AgentService) GetMessages(sessionID, userID uint, page, pageSize int) ([]model.Message, int64, error) {
	page, pageSize = GetPagination(page, pageSize)

	// 验权：session 属于该用户
	var session model.Session
	if err := database.DB.Where("id = ? AND user_id = ?", sessionID, userID).First(&session).Error; err != nil {
		return nil, 0, errors.New("会话不存在")
	}

	var messages []model.Message
	var total int64

	database.DB.Model(&model.Message{}).Where("session_id = ?", sessionID).Count(&total)
	if err := database.DB.Where("session_id = ?", sessionID).
		Order("id ASC").Offset((page-1)*pageSize).Limit(pageSize).
		Find(&messages).Error; err != nil {
		return nil, 0, err
	}
	return messages, total, nil
}

// updateTitle 用首条 assistant 回答截取前 15 字更新 session 标题
func (s *AgentService) updateTitle(session *model.Session) {
	var msg model.Message
	if err := database.DB.Where("session_id = ? AND role = ?", session.ID, model.RoleAssistant).
		Order("id ASC").First(&msg).Error; err != nil {
		return
	}
	title := truncateString(msg.Content, 15)
	database.DB.Model(session).Update("title", title)
}

// checkTransfer 检测最后一条 assistant 回答是否有切换标记
func (s *AgentService) checkTransfer(session *model.Session) {
	var msg model.Message
	if err := database.DB.Where("session_id = ? AND role = ?", session.ID, model.RoleAssistant).
		Order("id DESC").First(&msg).Error; err != nil {
		return
	}
	if should, targetAgent := DetectTransfer(msg.Content); should {
		// 清理回答中的切换标记
		cleaned := CleanTransferMarkers(msg.Content)
		database.DB.Model(&msg).Update("content", cleaned)
		// 更新 session 的 agent_type
		database.DB.Model(session).Update("agent_type", targetAgent)
	}
}

// truncateString 截取字符串前 n 个字符（Unicode 安全）
func truncateString(s string, maxLen int) string {
	// 去掉切换标记再截取
	s = CleanTransferMarkers(s)
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// callLLMRouter LLM 路由判断（预留，当前用关键词路由兜底）
func callLLMRouter(question string) (string, error) {
	messages := []engineChatMsg{
		{Role: "system", Content: "你是一个意图分类器。根据用户的问题，判断意图是客服(customer_service)、课程推荐(course_recommend)还是答疑(qa)。只回答一个词。"},
		{Role: "user", Content: question},
	}

	jsonBytes, _ := json.Marshal(engineReq{
		Model:    config.App.AI.Model,
		Messages: messages,
	})

	req, err := http.NewRequest("POST", config.App.AI.APIURL, strings.NewReader(string(jsonBytes)))
	// ... 此处为简化版，实际实现复刻 engine.callLLM 逻辑
	_ = req
	_ = err
	return model.AgentCustomerService, nil
}
```

> 注：`callLLMRouter` 是预留函数，当前版本不调用。实现时清理 `unused import`。

- [ ] **Step 2: 修复可能的编译问题**

检查 `callLLMRouter` 中引用的 `http`、`strings` 是否已 import，未使用的 `json`、`config` 是否清理。

- [ ] **Step 3: 验证编译**

```bash
cd d:/Vscoding/edu_market && go build ./...
```

修复所有编译错误后：

- [ ] **Step 4: Commit**

```bash
git add service/agent_service.go
git commit -m "feat: add AgentService (session mgmt + orchestration)"
```

---

## Phase 4: RAG

### Task 11: RAG 服务

**Files:**
- Create: `service/agent_rag.go`

- [ ] **Step 1: 写入 RAG 服务（含 VectorStore 接口 + 文本切片 + Redis Stack 实现）**

```go
package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"edu_market/config"
	"edu_market/database"
	"edu_market/model"
)

// SearchResult 检索结果
type SearchResult struct {
	ChunkID  uint    `json:"chunk_id"`
	Content  string  `json:"content"`
	Score    float32 `json:"score"`
}

// VectorStore 向量存储接口（预留切换 Pinecone/Qdrant）
type VectorStore interface {
	Search(courseID uint, query string, topK int) ([]SearchResult, error)
	Index(chunkID uint, courseID uint, content string) error
	Delete(courseID uint) error
}

// RAGService RAG 服务
type RAGService struct {
	vectorStore VectorStore
	chunkSize   int
	chunkOverlap int
}

// NewRAGService 创建 RAG 服务
func NewRAGService(vs VectorStore) *RAGService {
	cfg := config.App.Agent
	return &RAGService{
		vectorStore:  vs,
		chunkSize:    cfg.ChunkSize,
		chunkOverlap: cfg.ChunkOverlap,
	}
}

// IndexCourse 入库课程资料：切片 + 存 DB + 向量索引
func (r *RAGService) IndexCourse(courseID uint, fullText string) error {
	// 清空旧数据
	database.DB.Where("course_id = ?", courseID).Delete(&model.DocumentChunk{})
	r.vectorStore.Delete(courseID)

	// 切片
	chunks := r.chunkText(fullText)

	// 逐个入库
	for i, chunk := range chunks {
		dc := &model.DocumentChunk{
			CourseID:   courseID,
			Content:    chunk,
			ChunkIndex: i,
		}
		if err := database.DB.Create(dc).Error; err != nil {
			return fmt.Errorf("保存文档块失败: %w", err)
		}
		// 向量索引
		if err := r.vectorStore.Index(dc.ID, courseID, chunk); err != nil {
			// 向量索引失败不阻塞入库，打印警告
			fmt.Printf("警告: 向量索引失败 (chunk %d): %v\n", dc.ID, err)
		}
	}
	return nil
}

// Search 检索课程资料
func (r *RAGService) Search(courseID uint, query string, topK int) ([]SearchResult, error) {
	if r.vectorStore == nil {
		return nil, errors.New("向量存储未初始化")
	}
	return r.vectorStore.Search(courseID, query, topK)
}

// chunkText 文本切片：每 chunkSize 字一块，重叠 chunkOverlap 字
func (r *RAGService) chunkText(text string) []string {
	var chunks []string
	runes := []rune(text)
	total := len(runes)

	if total <= r.chunkSize {
		return []string{text}
	}

	step := r.chunkSize - r.chunkOverlap
	if step <= 0 {
		step = r.chunkSize
	}

	for i := 0; i < total; i += step {
		end := i + r.chunkSize
		if end > total {
			end = total
		}
		chunk := string(runes[i:end])
		chunks = append(chunks, strings.TrimSpace(chunk))
		if end == total {
			break
		}
	}
	return chunks
}

// ============ Redis Stack 向量存储实现 ============

// RedisStackVectorStore 基于 Redis Stack (RediSearch) 的向量存储
// 使用 FT.CREATE / FT.SEARCH 做向量检索
type RedisStackVectorStore struct{}

func NewRedisStackVectorStore() *RedisStackVectorStore {
	return &RedisStackVectorStore{}
}

func (vs *RedisStackVectorStore) Search(courseID uint, query string, topK int) ([]SearchResult, error) {
	// 简化实现：用 MySQL LIKE 搜索做关键词检索（Redis Stack 需要单独安装 module）
	// 后续可替换为真正的向量搜索
	var chunks []model.DocumentChunk
	if err := database.DB.Where("course_id = ? AND content LIKE ?", courseID, "%"+query+"%").
		Order("chunk_index ASC").Limit(topK).Find(&chunks).Error; err != nil {
		return nil, err
	}

	var results []SearchResult
	for _, c := range chunks {
		results = append(results, SearchResult{
			ChunkID:  c.ID,
			Content:  c.Content,
			Score:    0.5, // 非向量模式无实际分数
		})
	}
	return results, nil
}

func (vs *RedisStackVectorStore) Index(chunkID uint, courseID uint, content string) error {
	// 简化实现：暂不建向量索引（Redis Stack module 未就绪时跳过）
	// 后续安装 RediSearch module 后实现：
	// FT.CREATE idx:chunks PREFIX 1 doc: ...
	// HSET doc:<chunkID> content "<content>" embedding "<vector_bytes>"
	return nil
}

func (vs *RedisStackVectorStore) Delete(courseID uint) error {
	// 简化实现：清理时 document_chunks 表已通过 FK CASCADE 自动清理
	return nil
}

// SimpleSearchVectorStore 纯 MySQL 模糊搜索实现（未装 Redis Stack 时的 fallback）
type SimpleSearchVectorStore struct{}

func NewSimpleSearchVectorStore() *SimpleSearchVectorStore {
	return &SimpleSearchVectorStore{}
}

func (vs *SimpleSearchVectorStore) Search(courseID uint, query string, topK int) ([]SearchResult, error) {
	var chunks []model.DocumentChunk
	keywords := strings.Fields(query)
	db := database.DB.Where("course_id = ?", courseID)
	for i, kw := range keywords {
		if i == 0 {
			db = db.Where("content LIKE ?", "%"+kw+"%")
		} else {
			db = db.Or("content LIKE ?", "%"+kw+"%")
		}
	}
	if err := db.Order("chunk_index ASC").Limit(topK).Find(&chunks).Error; err != nil {
		return nil, err
	}

	var results []SearchResult
	for _, c := range chunks {
		results = append(results, SearchResult{
			ChunkID: c.ID,
			Content: c.Content,
			Score:   0.5,
		})
	}
	return results, nil
}

func (vs *SimpleSearchVectorStore) Index(chunkID uint, courseID uint, content string) error {
	return nil
}

func (vs *SimpleSearchVectorStore) Delete(courseID uint) error {
	return nil
}

// truncateBytes 按字节截断字符串
func truncateBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for utf8.ValidString(s[:maxBytes]) == false {
		maxBytes--
	}
	return s[:maxBytes]
}

// 避免 unused import 警告
var _ = json.Marshal
```

- [ ] **Step 2: 验证编译**

```bash
cd d:/Vscoding/edu_market && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add service/agent_rag.go
git commit -m "feat: add RAG service with VectorStore interface and simple fallback"
```

---

## Phase 5: Controller + Router

### Task 12: AgentController

**Files:**
- Create: `controller/agent_controller.go`

- [ ] **Step 1: 写入 AgentController**

```go
package controller

import (
	"encoding/json"
	"fmt"
	"net/http"

	"edu_market/dto/request"
	"edu_market/service"
	"edu_market/utils"

	"github.com/gin-gonic/gin"
)

// AgentController Agent 控制器
type AgentController struct {
	svc *service.AgentService
}

// NewAgentController 创建 AgentController
func NewAgentController(svc *service.AgentService) *AgentController {
	return &AgentController{svc: svc}
}

// Chat 发起/继续 Agent 对话（SSE 流式响应）
func (ctr *AgentController) Chat(c *gin.Context) {
	var req request.AgentChatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	userID := c.GetUint("user_id")

	// 设置 SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// 创建 SSE 回调（写入 gin.Context 的 ResponseWriter）
	sseCallback := func(event string, data string) error {
		_, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, data)
		if err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	}

	// 获取 RAG 服务（可选）
	rag := service.GetRAG() // 全局 RAG 实例

	session, err := ctr.svc.Chat(userID, req.SessionID, req.Question, rag, sseCallback)
	if err != nil {
		// 错误已在引擎中通过 SSE 发送，这里不做额外处理
		return
	}

	// 如果前端没传 session_id，通过 done 事件告知
	if req.SessionID == nil && session != nil {
		doneData, _ := json.Marshal(map[string]interface{}{
			"session_id": session.ID,
			"agent_type": session.AgentType,
		})
		fmt.Fprintf(c.Writer, "event: done\ndata: %s\n\n", string(doneData))
		c.Writer.Flush()
	}
}

// GetSessions 获取会话列表
func (ctr *AgentController) GetSessions(c *gin.Context) {
	var req request.AgentSessionsReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	userID := c.GetUint("user_id")
	sessions, total, err := ctr.svc.GetSessions(userID, req.Page, req.PageSize)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	req.Page, req.PageSize = service.GetPagination(req.Page, req.PageSize)
	utils.PageSuccess(c, sessions, total, req.Page, req.PageSize)
}

// DeleteSession 删除/关闭会话
func (ctr *AgentController) DeleteSession(c *gin.Context) {
	sessionID, err := parseUintParam(c, "id")
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	userID := c.GetUint("user_id")
	if err := ctr.svc.DeleteSession(userID, sessionID); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, nil)
}

// GetMessages 获取会话消息历史
func (ctr *AgentController) GetMessages(c *gin.Context) {
	var req request.AgentMessagesReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	sessionID, err := parseUintParam(c, "id")
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	userID := c.GetUint("user_id")
	messages, total, err := ctr.svc.GetMessages(sessionID, userID, req.Page, req.PageSize)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	req.Page, req.PageSize = service.GetPagination(req.Page, req.PageSize)
	utils.PageSuccess(c, messages, total, req.Page, req.PageSize)
}

// parseUintParam 解析路由中的 uint 参数
func parseUintParam(c *gin.Context, name string) (uint, error) {
	id := c.Param(name)
	if id == "" {
		return 0, fmt.Errorf("缺少参数 %s", name)
	}
	var result uint
	if _, err := fmt.Sscanf(id, "%d", &result); err != nil {
		return 0, fmt.Errorf("参数 %s 格式错误", name)
	}
	return result, nil
}
```

- [ ] **Step 2: 验证编译**

```bash
cd d:/Vscoding/edu_market && go build ./...
```

如有编译错误，修复后重新验证。

- [ ] **Step 3: Commit**

```bash
git add controller/agent_controller.go
git commit -m "feat: add AgentController with SSE chat and session CRUD"
```

---

### Task 13: 全局 RAG 实例 + 注册路由

**Files:**
- Modify: `router/router.go`
- Modify: `service/agent_rag.go`（加 `GetRAG`）

- [ ] **Step 1: 在 agent_rag.go 末尾添加全局 RAG 管理**

在 `service/agent_rag.go` 文件末尾追加：

```go
// 全局 RAG 服务实例
var globalRAG *RAGService

// InitRAG 初始化全局 RAG 服务
func InitRAG() {
	// 默认使用 SimpleSearch（不依赖 Redis Stack）
	vs := NewSimpleSearchVectorStore()
	globalRAG = NewRAGService(vs)
}

// GetRAG 获取全局 RAG 服务实例
func GetRAG() *RAGService {
	return globalRAG
}
```

- [ ] **Step 2: 注册 Agent 路由**

在 `router/router.go` 中，在 `aiCtrl := &controller.AIController{}` 后面追加：

```go
// 初始化 Agent 系统
agentEngine := service.NewAgentEngine()
agentSvc := service.NewAgentService(agentEngine)
agentCtrl := controller.NewAgentController(agentSvc)
```

在 `auth := api.Group("")` 的 `auth.Use(middleware.JWTAuth())` 块内，在 `// AI` 注释行下面追加：

```go
// Agent（新）
auth.POST("/agent/chat", agentCtrl.Chat)
auth.GET("/agent/sessions", agentCtrl.GetSessions)
auth.DELETE("/agent/sessions/:id", agentCtrl.DeleteSession)
auth.GET("/agent/sessions/:id/messages", agentCtrl.GetMessages)
```

- [ ] **Step 3: 在 main.go 初始化 RAG**

在 `main.go` 的 `database.Init()` 之前添加：

```go
// 初始化 RAG 服务
service.InitRAG()
```

- [ ] **Step 4: 验证编译**

```bash
cd d:/Vscoding/edu_market && go build ./...
```

- [ ] **Step 5: 启动服务验证路由可访问**

```bash
taskkill //F //IM edu_market.exe 2>/dev/null; taskkill //F //IM main.exe 2>/dev/null
cd d:/Vscoding/edu_market && go run . &
sleep 5
# 验证 Agent 路由已注册
curl -s http://localhost:8080/api/agent/sessions -H "Authorization: Bearer test" | head -5
```

预期：返回 401（token 无效），但路由存在。

- [ ] **Step 6: Commit**

```bash
git add router/router.go main.go service/agent_rag.go
git commit -m "feat: register agent routes and init RAG service"
```

---

## Phase 6: 测试

### Task 14: Agent Router 测试

**Files:**
- Create: `service/agent_router_test.go`

- [ ] **Step 1: 写入路由测试**

```go
package service

import (
	"testing"

	"edu_market/model"
)

func TestRouteIntent_CustomerService(t *testing.T) {
	tests := []string{
		"我要退款",
		"我的订单在哪里",
		"支付失败了怎么办",
		"怎么买课程",
	}

	for _, msg := range tests {
		agentType, needLLM := RouteIntent(msg)
		if needLLM {
			t.Errorf("消息 %q 应该直接匹配，却需要 LLM", msg)
		}
		if agentType != model.AgentCustomerService {
			t.Errorf("消息 %q 应路由到客服，实际 %s", msg, agentType)
		}
	}
}

func TestRouteIntent_CourseRecommend(t *testing.T) {
	tests := []string{
		"有什么推荐的课程",
		"零基础学什么好",
		"想学编程有没有入门课程",
	}

	for _, msg := range tests {
		agentType, needLLM := RouteIntent(msg)
		if needLLM {
			t.Errorf("消息 %q 应该直接匹配，却需要 LLM", msg)
		}
		if agentType != model.AgentCourseRecommend {
			t.Errorf("消息 %q 应路由到推荐，实际 %s", msg, agentType)
		}
	}
}

func TestRouteIntent_QA(t *testing.T) {
	tests := []string{
		"第三章的公式怎么推导",
		"这个讲义里的定理怎么证明",
		"课件上这里为什么这样写",
	}

	for _, msg := range tests {
		agentType, needLLM := RouteIntent(msg)
		if needLLM {
			t.Errorf("消息 %q 应该直接匹配，却需要 LLM", msg)
		}
		if agentType != model.AgentQA {
			t.Errorf("消息 %q 应路由到答疑，实际 %s", msg, agentType)
		}
	}
}

func TestRouteIntent_Unclear(t *testing.T) {
	tests := []string{
		"你好",
		"在吗",
		"帮我看看",
	}

	for _, msg := range tests {
		_, needLLM := RouteIntent(msg)
		if !needLLM {
			t.Errorf("消息 %q 应该需要 LLM 判断", msg)
		}
	}
}

func TestDetectTransfer(t *testing.T) {
	tests := []struct {
		answer       string
		should       bool
		target       string
	}{
		{"这是你的订单信息 [TRANSFER:qa]", true, model.AgentQA},
		{"推荐这门课程 [TRANSFER:course_recommend]", true, model.AgentCourseRecommend},
		{"帮你查一下 [TRANSFER:customer_service]", true, model.AgentCustomerService},
		{"这是正常的回答内容", false, ""},
	}

	for _, tc := range tests {
		should, target := DetectTransfer(tc.answer)
		if should != tc.should || target != tc.target {
			t.Errorf("DetectTransfer(%q) = (%v, %s), want (%v, %s)", tc.answer, should, target, tc.should, tc.target)
		}
	}
}

func TestCleanTransferMarkers(t *testing.T) {
	answer := "推荐这门课 [TRANSFER:qa] 详细内容如下"
	cleaned := CleanTransferMarkers(answer)
	if cleaned != "推荐这门课 详细内容如下" {
		t.Errorf("CleanTransferMarkers 结果: %q", cleaned)
	}
}
```

- [ ] **Step 2: 运行测试**

```bash
cd d:/Vscoding/edu_market && go test ./service/ -v -run "TestRouteIntent|TestDetectTransfer|TestCleanTransferMarkers"
```

预期：全部 PASS。

- [ ] **Step 3: Commit**

```bash
git add service/agent_router_test.go
git commit -m "test: add agent router tests"
```

---

### Task 15: Agent Engine 测试

**Files:**
- Create: `service/agent_engine_test.go`

- [ ] **Step 1: 写入引擎基础测试**

```go
package service

import (
	"testing"

	"edu_market/config"
	"edu_market/database"
	"edu_market/model"
)

// setupAgentTest 在已有 TestMain 基础上创建测试用 session
func setupAgentTest(t *testing.T) (*model.User, *model.Session) {
	t.Helper()
	// 创建测试用户
	user := &model.User{
		Username: "agent_test_user",
		Role:     "student",
	}
	if err := database.DB.Create(user).Error; err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
	session := &model.Session{
		UserID:    user.ID,
		AgentType: model.AgentCustomerService,
		Status:    model.SessionActive,
		Title:     "测试会话",
	}
	if err := database.DB.Create(session).Error; err != nil {
		t.Fatalf("创建测试会话失败: %v", err)
	}
	return user, session
}

func TestEngineContextLimit(t *testing.T) {
	cfg := config.App.Agent
	if cfg.ContextMaxMsg <= 0 {
		t.Skip("Agent 配置未加载")
	}

	engine := NewAgentEngine()
	if engine.contextLimit != cfg.ContextMaxMsg {
		t.Errorf("context limit = %d, want %d", engine.contextLimit, cfg.ContextMaxMsg)
	}
}

func TestEngineMaxRounds(t *testing.T) {
	engine := NewAgentEngine()
	if engine.maxRounds != config.App.Agent.MaxToolRounds {
		t.Errorf("max rounds = %d, want %d", engine.maxRounds, config.App.Agent.MaxToolRounds)
	}
}

func TestEngineLoadContext(t *testing.T) {
	user, session := setupAgentTest(t)

	// 创建测试消息
	msgs := []*model.Message{
		{SessionID: session.ID, Role: model.RoleUser, Content: "你好"},
		{SessionID: session.ID, Role: model.RoleAssistant, Content: "你好！有什么可以帮你的？"},
	}
	for _, m := range msgs {
		if err := database.DB.Create(m).Error; err != nil {
			t.Fatalf("创建测试消息失败: %v", err)
		}
	}

	engine := NewAgentEngine()
	messages := engine.loadContext(session.ID, "你是一个测试助手")

	if len(messages) < 2 {
		t.Errorf("loadContext 返回消息数 = %d, 期望 >= 2", len(messages))
	}
	if messages[0].Role != "system" {
		t.Errorf("第一条消息 role = %s, 期望 system", messages[0].Role)
	}
	if messages[0].Content != "你是一个测试助手" {
		t.Errorf("system prompt = %s, 期望 '你是一个测试助手'", messages[0].Content)
	}

	// 清理
	database.DB.Where("session_id = ?", session.ID).Delete(&model.Message{})
	database.DB.Delete(session)
	database.DB.Delete(user)
}
```

- [ ] **Step 2: 运行测试**

```bash
cd d:/Vscoding/edu_market && go test ./service/ -v -run "TestEngine"
```

预期：全部 PASS。

- [ ] **Step 3: Commit**

```bash
git add service/agent_engine_test.go
git commit -m "test: add agent engine tests"
```

---

### Task 16: AgentService 集成测试

**Files:**
- Create: `service/agent_service_test.go`

- [ ] **Step 1: 写入集成测试**

```go
package service

import (
	"testing"

	"edu_market/config"
	"edu_market/database"
	"edu_market/model"
)

func TestAgentService_CreateSession(t *testing.T) {
	// 清理残留
	database.DB.Where("1=1").Delete(&model.Message{})
	database.DB.Where("1=1").Delete(&model.Session{})

	engine := NewAgentEngine()
	svc := NewAgentService(engine)

	user := &model.User{Username: "svc_test_user1", Role: "student"}
	database.DB.Create(user)

	// 测试创建会话（无 session_id）
	var sseEvents []string
	callback := func(event, data string) error {
		sseEvents = append(sseEvents, event)
		if event == "delta" || event == "error" {
			// 收到 delta 或 error 后可以停止
		}
		return nil
	}

	// 注：这个测试会真实调 LLM API，需要配置有效 API Key
	// 如果 API 不可用，会收到 error 事件但不会 panic
	session, err := svc.Chat(user.ID, nil, "你好", nil, callback)
	if err != nil {
		t.Logf("Chat 返回 error（可能是 API 不可用）: %v", err)
	}
	if session == nil {
		t.Fatal("session 不应为 nil")
	}
	if session.ID == 0 {
		t.Fatal("session ID 不应为 0")
	}
	if session.AgentType == "" {
		t.Fatal("agent_type 不应为空")
	}

	// 验证 session 已入库
	var dbSession model.Session
	if err := database.DB.First(&dbSession, session.ID).Error; err != nil {
		t.Fatalf("查询 session 失败: %v", err)
	}
	if dbSession.UserID != user.ID {
		t.Errorf("session.UserID = %d, want %d", dbSession.UserID, user.ID)
	}

	// 验证消息已入库
	var count int64
	database.DB.Model(&model.Message{}).Where("session_id = ?", session.ID).Count(&count)
	if count == 0 {
		t.Error("应至少有 1 条消息（user message）入库")
	}

	// 清理
	database.DB.Where("session_id = ?", session.ID).Delete(&model.Message{})
	database.DB.Delete(session)
	database.DB.Delete(user)
}

func TestAgentService_GetSessions(t *testing.T) {
	database.DB.Where("1=1").Delete(&model.Message{})
	database.DB.Where("1=1").Delete(&model.Session{})

	engine := NewAgentEngine()
	svc := NewAgentService(engine)

	user := &model.User{Username: "svc_test_user2", Role: "student"}
	database.DB.Create(user)

	// 创建两个 session
	for i := 0; i < 2; i++ {
		s := &model.Session{UserID: user.ID, AgentType: model.AgentCustomerService, Status: model.SessionActive, Title: "测试"}
		database.DB.Create(s)
	}

	sessions, total, err := svc.GetSessions(user.ID, 1, 10)
	if err != nil {
		t.Fatalf("GetSessions 失败: %v", err)
	}
	if total < 2 {
		t.Errorf("total = %d, want >= 2", total)
	}
	if len(sessions) < 2 {
		t.Errorf("sessions count = %d, want >= 2", len(sessions))
	}

	// 清理
	database.DB.Where("user_id = ?", user.ID).Delete(&model.Session{})
	database.DB.Delete(user)
}

func TestAgentService_DeleteSession(t *testing.T) {
	database.DB.Where("1=1").Delete(&model.Message{})
	database.DB.Where("1=1").Delete(&model.Session{})

	engine := NewAgentEngine()
	svc := NewAgentService(engine)

	user := &model.User{Username: "svc_test_user3", Role: "student"}
	database.DB.Create(user)

	s := &model.Session{UserID: user.ID, AgentType: model.AgentCustomerService, Status: model.SessionActive, Title: "删除测试"}
	database.DB.Create(s)

	if err := svc.DeleteSession(user.ID, s.ID); err != nil {
		t.Fatalf("DeleteSession 失败: %v", err)
	}

	// 验证状态已改为 closed
	var updated model.Session
	database.DB.First(&updated, s.ID)
	if updated.Status != model.SessionClosed {
		t.Errorf("status = %s, want %s", updated.Status, model.SessionClosed)
	}

	// 清理
	database.DB.Delete(s)
	database.DB.Delete(user)
}
```

- [ ] **Step 2: 确保 config.App.Agent 有默认值（测试环境）**

在 `service/setup_test.go` 的 `TestMain` 中，`config.App` 初始化时追加 Agent 配置：

在 `config.App = &config.Config{ ... }` 块中，`Captcha:` 行后面追加：

```go
Agent: config.AgentConfig{MaxToolRounds: 7, ContextMaxMsg: 20, ChunkSize: 500, ChunkOverlap: 50},
```

- [ ] **Step 3: 运行集成测试**

```bash
cd d:/Vscoding/edu_market && go test ./service/ -v -run "TestAgentService"
```

预期：`TestAgentService_GetSessions` 和 `TestAgentService_DeleteSession` PASS。`TestAgentService_CreateSession` 在 API Key 有效时 PASS，无效时跳过（查看日志确认）。

- [ ] **Step 4: Commit**

```bash
git add service/agent_service_test.go service/setup_test.go
git commit -m "test: add AgentService integration tests"
```

---

## Phase 7: 前端

### Task 17: Agent API 封装 + Store

**Files:**
- Create: `web/src/api/agent.js`

- [ ] **Step 1: 写入 API 封装**

```javascript
import api from './index'

// SSE 对话（返回 fetch response，由调用方读取 ReadableStream）
export function agentChat({ session_id, question }, token) {
  return fetch('/api/agent/chat', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`
    },
    body: JSON.stringify({ session_id, question })
  })
}

// 会话列表
export function getSessions(params) {
  return api.get('/agent/sessions', { params })
}

// 删除会话
export function deleteSession(id) {
  return api.delete(`/agent/sessions/${id}`)
}

// 获取会话历史消息
export function getMessages(id, params) {
  return api.get(`/agent/sessions/${id}/messages`, { params })
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/api/agent.js
git commit -m "feat: add agent API client with SSE fetch support"
```

---

### Task 18: AgentChat 页面

**Files:**
- Create: `web/src/views/AgentChat.vue`
- Modify: `web/src/router/index.js`

- [ ] **Step 1: 写入 AgentChat.vue**

```vue
<template>
  <div class="agent-chat">
    <!-- 左侧会话列表 -->
    <aside class="sidebar">
      <div class="sidebar-header">
        <h3>AI 助手</h3>
        <button class="btn-new" @click="newChat">📝 新对话</button>
      </div>
      <div class="session-list">
        <div
          v-for="s in sessions"
          :key="s.id"
          class="session-item"
          :class="{ active: s.id === currentSessionId }"
          @click="switchSession(s.id)"
        >
          <span class="session-title">{{ s.title || '新对话' }}</span>
          <span class="session-tag">{{ agentTypeLabel(s.agent_type) }}</span>
          <button class="btn-del" @click.stop="removeSession(s.id)">×</button>
        </div>
      </div>
    </aside>

    <!-- 右侧聊天区域 -->
    <main class="chat-area">
      <div v-if="currentSessionId" class="chat-header">
        <span class="agent-badge">{{ agentTypeLabel(currentAgentType) }}</span>
        <span>{{ currentTitle }}</span>
      </div>

      <div class="messages" ref="msgContainer">
        <div v-for="(msg, i) in messages" :key="i" class="msg" :class="msg.role">
          <div v-if="msg.role === 'thinking'" class="thinking-bubble">
            🔍 正在查询...
          </div>
          <div v-else class="bubble" :class="msg.role">
            {{ msg.content }}
          </div>
        </div>
      </div>

      <div class="input-area">
        <input
          v-model="input"
          @keyup.enter="send"
          placeholder="输入你的问题..."
          :disabled="loading"
        />
        <button @click="send" :disabled="loading || !input.trim()">发送</button>
      </div>
    </main>
  </div>
</template>

<script setup>
import { ref, nextTick, onMounted } from 'vue'
import { useUserStore } from '@/stores/user'
import { agentChat, getSessions, deleteSession, getMessages } from '@/api/agent'

const userStore = useUserStore()

const sessions = ref([])
const currentSessionId = ref(null)
const currentAgentType = ref('')
const currentTitle = ref('')
const messages = ref([])
const input = ref('')
const loading = ref(false)
const msgContainer = ref(null)

const agentTypeLabel = (t) => ({
  customer_service: '客服',
  course_recommend: '推荐',
  qa: '答疑'
}[t] || t)

async function loadSessions() {
  try {
    const res = await getSessions({ page: 1, page_size: 50 })
    sessions.value = res.data.list || []
  } catch (e) {
    console.error('加载会话列表失败', e)
  }
}

async function loadMessages(sessionId) {
  try {
    const res = await getMessages(sessionId, { page: 1, page_size: 100 })
    messages.value = (res.data.list || []).map(m => ({
      role: m.role,
      content: m.content
    }))
    scrollToBottom()
  } catch (e) {
    console.error('加载消息失败', e)
  }
}

function newChat() {
  currentSessionId.value = null
  currentAgentType.value = ''
  currentTitle.value = ''
  messages.value = []
  input.value = ''
}

function switchSession(id) {
  currentSessionId.value = id
  const s = sessions.value.find(s => s.id === id)
  if (s) {
    currentAgentType.value = s.agent_type
    currentTitle.value = s.title
  }
  loadMessages(id)
}

async function removeSession(id) {
  try {
    await deleteSession(id)
    sessions.value = sessions.value.filter(s => s.id !== id)
    if (currentSessionId.value === id) newChat()
  } catch (e) {
    console.error('删除会话失败', e)
  }
}

async function send() {
  if (!input.value.trim() || loading.value) return
  const question = input.value.trim()
  input.value = ''
  loading.value = true

  // 用户消息立即显示
  messages.value.push({ role: 'user', content: question })

  try {
    const resp = await agentChat({
      session_id: currentSessionId.value || undefined,
      question
    }, userStore.accessToken)

    if (!resp.ok) {
      messages.value.push({ role: 'assistant', content: `请求失败: ${resp.status}` })
      loading.value = false
      return
    }

    const reader = resp.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    let currentAssistantIdx = -1

    while (true) {
      const { done, value } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        if (!line.startsWith('event: ') && !line.startsWith('data: ')) continue

        if (line.startsWith('event: ')) {
          const eventType = line.slice(7).trim()
          // 取下一行 data
          const dataIdx = lines.indexOf(line) + 1
          if (dataIdx >= lines.length) continue
          const dataLine = lines[dataIdx]
          if (!dataLine || !dataLine.startsWith('data: ')) continue
          const payload = dataLine.slice(6)

          if (eventType === 'thinking') {
            try {
              const d = JSON.parse(payload)
              messages.value.push({ role: 'thinking', content: `正在执行: ${d.tool}` })
            } catch {}
          } else if (eventType === 'delta') {
            try {
              const d = JSON.parse(payload)
              if (currentAssistantIdx === -1) {
                messages.value.push({ role: 'assistant', content: '' })
                currentAssistantIdx = messages.value.length - 1
              }
              messages.value[currentAssistantIdx].content += d.content
            } catch {}
          } else if (eventType === 'done') {
            try {
              const d = JSON.parse(payload)
              if (!currentSessionId.value && d.session_id) {
                currentSessionId.value = d.session_id
                currentAgentType.value = d.agent_type
                await loadSessions()
              }
            } catch {}
          } else if (eventType === 'error') {
            try {
              const d = JSON.parse(payload)
              messages.value.push({ role: 'assistant', content: d.message || '发生错误' })
            } catch {}
          }
        }
        scrollToBottom()
      }
    }
  } catch (e) {
    messages.value.push({ role: 'assistant', content: `连接失败: ${e.message}` })
  } finally {
    // 清理 thinking 消息
    messages.value = messages.value.filter(m => m.role !== 'thinking')
    loading.value = false
  }
}

function scrollToBottom() {
  nextTick(() => {
    if (msgContainer.value) {
      msgContainer.value.scrollTop = msgContainer.value.scrollHeight
    }
  })
}

onMounted(() => {
  loadSessions()
})
</script>

<style scoped>
.agent-chat {
  display: flex;
  height: calc(100vh - 64px);
  max-width: 1200px;
  margin: 0 auto;
}

.sidebar {
  width: 260px;
  border-right: 1px solid #e5e7eb;
  display: flex;
  flex-direction: column;
  background: #f9fafb;
}

.sidebar-header {
  padding: 16px;
  border-bottom: 1px solid #e5e7eb;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.sidebar-header h3 { margin: 0; font-size: 16px; }

.btn-new {
  padding: 4px 10px;
  border: 1px solid #3b82f6;
  background: #fff;
  color: #3b82f6;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
}

.session-list { flex: 1; overflow-y: auto; }

.session-item {
  padding: 10px 16px;
  border-bottom: 1px solid #f3f4f6;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 8px;
}

.session-item:hover { background: #f3f4f6; }
.session-item.active { background: #eff6ff; }

.session-title { flex: 1; font-size: 14px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.session-tag {
  font-size: 11px;
  padding: 1px 6px;
  border-radius: 4px;
  background: #e5e7eb;
  color: #6b7280;
}

.btn-del {
  background: none;
  border: none;
  font-size: 18px;
  color: #9ca3af;
  cursor: pointer;
  padding: 0 4px;
}
.btn-del:hover { color: #ef4444; }

.chat-area {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.chat-header {
  padding: 12px 20px;
  border-bottom: 1px solid #e5e7eb;
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 15px;
}

.agent-badge {
  padding: 2px 10px;
  border-radius: 12px;
  background: #dbeafe;
  color: #1d4ed8;
  font-size: 12px;
}

.messages {
  flex: 1;
  overflow-y: auto;
  padding: 20px;
}

.msg { margin-bottom: 16px; display: flex; }
.msg.user { justify-content: flex-end; }
.msg.assistant, .msg.thinking { justify-content: flex-start; }

.bubble {
  max-width: 70%;
  padding: 10px 16px;
  border-radius: 12px;
  font-size: 14px;
  line-height: 1.6;
  white-space: pre-wrap;
}

.msg.user .bubble { background: #3b82f6; color: #fff; border-bottom-right-radius: 4px; }
.msg.assistant .bubble { background: #f3f4f6; color: #1f2937; border-bottom-left-radius: 4px; }

.thinking-bubble {
  background: #fef3c7;
  padding: 8px 14px;
  border-radius: 8px;
  font-size: 13px;
  color: #92400e;
}

.input-area {
  padding: 16px 20px;
  border-top: 1px solid #e5e7eb;
  display: flex;
  gap: 10px;
}

.input-area input {
  flex: 1;
  padding: 10px 16px;
  border: 1px solid #d1d5db;
  border-radius: 8px;
  font-size: 14px;
  outline: none;
}

.input-area input:focus { border-color: #3b82f6; }

.input-area button {
  padding: 10px 24px;
  background: #3b82f6;
  color: #fff;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font-size: 14px;
}

.input-area button:disabled { background: #9ca3af; cursor: not-allowed; }
</style>
```

- [ ] **Step 2: 注册路由**

在 `web/src/router/index.js` 中，在 `ai-chat` 路由后面追加：

```javascript
{
  path: '/agent',
  name: 'AgentChat',
  component: () => import('@/views/AgentChat.vue'),
  meta: { title: 'AI 助手 - EduMarket', auth: true }
},
```

- [ ] **Step 3: 前端编译验证**

```bash
cd d:/Vscoding/edu_market/web && npm run build 2>&1 | tail -20
```

如有错误，修复后重新验证。

- [ ] **Step 4: Commit**

```bash
git add web/src/views/AgentChat.vue web/src/router/index.js web/src/api/agent.js
git commit -m "feat: add AgentChat page with SSE streaming UI"
```

---

## Phase 8: 端到端验证

### Task 19: 全链路验证

- [ ] **Step 1: 启动后端**

```bash
cd d:/Vscoding/edu_market && go run . &
sleep 5
```

- [ ] **Step 2: 获取 access_token**

```bash
# 先获取图形验证码
curl -s http://localhost:8080/api/captcha/image | jq '.data'

# 然后获取短信验证码（需要 Redis）
# ... 或用现有用户 token 直接测试
```

- [ ] **Step 3: 测试 Agent SSE 接口**

```bash
curl -N -X POST http://localhost:8080/api/agent/chat \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <your_token>" \
  -d '{"question":"推荐一个Python课程"}'
```

预期：看到 SSE 事件流输出（thinking / delta / done）。

- [ ] **Step 4: 测试会话列表**

```bash
curl -s http://localhost:8080/api/agent/sessions \
  -H "Authorization: Bearer <your_token>" | jq '.'
```

预期：返回会话列表。

- [ ] **Step 5: 前端开发服务器**

```bash
cd d:/Vscoding/edu_market/web && npm run dev
```

打开浏览器 `http://localhost:5173/agent`，发一条消息验证 SSE 打字机效果。

- [ ] **Step 6: 跑全量测试**

```bash
cd d:/Vscoding/edu_market && go test ./... -v 2>&1 | tail -30
```

预期：所有测试 PASS。

- [ ] **Step 7: Commit**

```bash
git commit -m "feat: v2 agent system end-to-end verified" --allow-empty
```

---

## 注意事项

1. **DeepSeek API 兼容性**：`tools` 格式与 OpenAI 一致。如果 DeepSeek 不支持 tool calling，需在 `callLLM` 中 fallback 为不带 tools 的普通请求，在 System Prompt 中描述工具。
2. **Redis Stack**：当前 RAG 用 MySQL LIKE 做 fallback 关键词检索。安装 Redis Stack 后替换 `SimpleSearchVectorStore` 为 `RedisStackVectorStore` 实现真正的向量搜索。
3. **流式 + Tool Calling 互斥**：DeepSeek（和其他 OpenAI 兼容 API）的 tool calling 模式通常不支持 `stream: true`。引擎先用非流式拿 Tool Call → 工具执行 → 最后一轮才流式输出答案。
4. **测试 `TestAgentService_CreateSession` 会真实调 LLM API**，需要 config 中有有效 API Key，否则该测试会失败。
