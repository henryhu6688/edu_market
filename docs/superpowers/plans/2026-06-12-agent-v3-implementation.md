# v3 Agent 改进实现计划

> **For agentic workers:** Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 实现 Workflow 骨架 + Agent 内核 —— 单一 Agent 拥有 10 个 Tool，LLM 在 Workflow 固定步骤内自主规划

**Architecture:** Workflow 层做意图路由+安全兜底，Agent 层做多 tool 串联+自主决策。新增 action SSE 事件支持 purchase_offer。从 3 Agent+Router 合并为 1 Agent。

**Tech Stack:** Go + Gin + GORM + MySQL + DeepSeek API + SSE

---

## 文件结构

### 新建
| 文件 | 职责 |
|------|------|
| `model/faq.go` | FAQ 数据模型 |
| `service/agent_workflow.go` | Workflow 层：意图分类 + 四大 Flow 骨架 |
| `service/agent_action.go` | action 事件定义与发送 |

### 修改
| 文件 | 改动 |
|------|------|
| `service/agent_tools.go` | 新增 5 个 tool、更新现有 tool |
| `service/agent_engine.go` | 支持 action 事件、支持单 Agent 全量 tool |
| `service/agent_router.go` | 简化：去掉三 Agent 路由，保留关键词+LLM 判断 |
| `service/agent_prompts.go` | 三 Prompt → 一个统一 Prompt |
| `service/agent_service.go` | 去掉 agentType 参数、集成 Workflow |
| `controller/agent_controller.go` | 支持 action SSE 事件处理 |
| `database/mysql.go` | AutoMigrate 注册 FAQ |
| `service/setup_test.go` | TestMain 清理 FAQ、AgentConfig 加新字段 |
| `config/config.go` | AgentConfig 新增 purchase_boundary 字段 |
| `config/app.yml` | 新增 purchase_boundary 配置 |
| `model/message.go` | 不变（ToolCalls JSON 已经支持 action 数据） |
| `web/src/views/AgentChat.vue` | 渲染 action 卡片、去掉 Agent 标签 |

### 删除
| 文件/代码 | 说明 |
|-----------|------|
| `service/agent_prompts.go` 中三 Prompt | 合并为 1 个 |
| `agent_router.go` 中 transfer 检测 | 保留但简化 |

---

## Phase 1: 数据准备

### Task 1: FAQ 模型 + 数据库迁移

- [ ] **Step 1: 创建 `model/faq.go`**

```go
package model

import "time"

type FAQ struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Question  string    `gorm:"type:varchar(500);not null" json:"question"`
	Answer    string    `gorm:"type:text;not null" json:"answer"`
	Category  string    `gorm:"type:varchar(50);default:general" json:"category"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (FAQ) TableName() string { return "faqs" }
```

- [ ] **Step 2: 注册到 AutoMigrate + TestMain 清理**

`database/mysql.go`:
```go
&model.FAQ{},
```

`service/setup_test.go`:
```go
database.DB.Where("1=1").Delete(&model.FAQ{})
```

- [ ] **Step 3: 种子数据 + 验证**

SQL 插入几条默认 FAQ：
```sql
INSERT INTO faqs (question, answer, category) VALUES
('怎么退款？', '进入"我的订单"找到对应订单，点击"申请退款"。退款将在 3-5 个工作日退回原支付账户。', 'refund'),
('支付失败怎么办？', '1. 检查支付方式是否有效 2. 确认账户余额充足 3. 如果多次失败，切换到其他支付方式再试。仍有问题可联系客服。', 'payment'),
('买了资料怎么查看？', '购买后进入"我的订单"找到对应订单，点击"查看资料"即可阅读全部文档内容。', 'usage');
```

- [ ] **Step 4: 编译 + 测试 + Commit**

```bash
go build ./... && go test ./...
git add model/faq.go database/mysql.go service/setup_test.go
git commit -m "feat: add FAQ model with seed data"
```

### Task 2: 配置新增 purchase_boundary 字段

- [ ] **Step 1: config.go AgentConfig 新增**

```go
type AgentConfig struct {
	MaxToolRounds          int    `mapstructure:"max_tool_rounds"`
	ContextMaxMsg          int    `mapstructure:"context_max_messages"`
	EmbeddingModel         string `mapstructure:"embedding_model"`
	EmbeddingAPIURL        string `mapstructure:"embedding_api_url"`
	ChunkSize              int    `mapstructure:"chunk_size"`
	ChunkOverlap           int    `mapstructure:"chunk_overlap"`
	PurchaseBoundaryTopK   int    `mapstructure:"purchase_boundary_topk"`
	PurchaseBoundaryChars  int    `mapstructure:"purchase_boundary_chars"`
}
```

- [ ] **Step 2: app.yml 新增 + setup_test.go 更新**

```yaml
agent:
  purchase_boundary_topk: 1
  purchase_boundary_chars: 200
```

```go
// setup_test.go
Agent: config.AgentConfig{...PurchaseBoundaryTopK: 1, PurchaseBoundaryChars: 200, ...},
```

- [ ] **Step 3: 编译 + Commit**

---

## Phase 2: 新增 5 个 Tool

### Task 3: 实现新 Tool

**Files:**
- Modify: `service/agent_tools.go`

在现有 3 个 tool 基础上新增 5 个：

- [ ] **Step 1: `get_material_detail`**

```go
type getMaterialDetailTool struct{}

func (t getMaterialDetailTool) Definition() ToolDef {
	return ToolDef{
		Name: "get_material_detail",
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
	var args struct {
		MaterialID uint `json:"material_id"`
	}
	json.Unmarshal([]byte(argsJSON), &args)

	var material model.Material
	if err := database.DB.Preload("Category").Preload("Documents").
		First(&material, args.MaterialID).Error; err != nil {
		return ToolResult{Success: false, Content: "资料不存在"}
	}

	// 只返回目录结构，不返回文档内容
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
```

- [ ] **Step 2: `get_material_outline`**

```go
type getMaterialOutlineTool struct{}

func (t getMaterialOutlineTool) Definition() ToolDef {
	return ToolDef{
		Name: "get_material_outline",
		Description: "获取资料的文档目录结构（章节标题列表），不含文档内容",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"material_id": map[string]interface{}{"type": "number"},
			},
			"required": []string{"material_id"},
		},
	}
}

func (t getMaterialOutlineTool) Execute(_ uint, argsJSON string) ToolResult {
	var args struct{ MaterialID uint `json:"material_id"` }
	json.Unmarshal([]byte(argsJSON), &args)

	var docs []model.Document
	database.DB.Where("material_id = ?", args.MaterialID).
		Order("sort_order ASC, id ASC").Find(&docs)

	type Item struct {
		Title    string `json:"title"`
		IsFree   bool   `json:"is_free_preview"`
		ParentID *uint  `json:"parent_id,omitempty"`
	}
	var items []Item
	for _, d := range docs {
		items = append(items, Item{Title: d.Title, IsFree: d.IsFreePreview, ParentID: d.ParentID})
	}
	b, _ := json.Marshal(items)
	return ToolResult{Success: true, Content: string(b)}
}
```

- [ ] **Step 3: `get_reviews`**

```go
type getReviewsTool struct{}

func (t getReviewsTool) Definition() ToolDef {
	return ToolDef{
		Name: "get_reviews",
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
```

- [ ] **Step 4: `get_categories`**

```go
type getCategoriesTool struct{}

func (t getCategoriesTool) Definition() ToolDef {
	return ToolDef{
		Name: "get_categories",
		Description: "获取平台所有学习资料分类列表",
		Parameters: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{},
		},
	}
}

func (t getCategoriesTool) Execute(_ uint, _ string) ToolResult {
	var cats []model.Category
	database.DB.Order("id ASC").Find(&cats)

	type Item struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
	}
	var items []Item
	for _, c := range cats {
		items = append(items, Item{ID: c.ID, Name: c.Name})
	}
	b, _ := json.Marshal(items)
	return ToolResult{Success: true, Content: string(b)}
}
```

- [ ] **Step 5: `search_faq` + `get_order_detail` + `trigger_purchase_offer`**

```go
type searchFAQTool struct{}

func (t searchFAQTool) Definition() ToolDef {
	return ToolDef{
		Name: "search_faq",
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

	type Item struct {
		Question string `json:"question"`
		Answer   string `json:"answer"`
	}
	var items []Item
	for _, f := range faqs {
		items = append(items, Item{Question: f.Question, Answer: f.Answer})
	}
	b, _ := json.Marshal(items)
	return ToolResult{Success: true, Content: string(b)}
}

type getOrderDetailTool struct{}

func (t getOrderDetailTool) Definition() ToolDef {
	return ToolDef{
		Name: "get_order_detail",
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
		Name: "trigger_purchase_offer",
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

	// 返回特殊标记，引擎检测到后发 action SSE 事件
	result := map[string]interface{}{
		"__action": "purchase_offer",
		"material_id": material.ID,
		"title": material.Title,
		"price": material.Price,
		"cover_image": material.CoverImage,
	}
	b, _ := json.Marshal(result)
	return ToolResult{Success: true, Content: string(b)}
}
```

- [ ] **Step 6: 更新 buildToolSet —— 全部 tool 注册**

```go
func buildToolSet() map[string]Tool {
	return map[string]Tool{
		"query_materials":       queryCoursesTool{},
		"get_material_detail":   getMaterialDetailTool{},
		"get_material_outline":  getMaterialOutlineTool{},
		"get_reviews":           getReviewsTool{},
		"get_categories":        getCategoriesTool{},
		"query_orders":          queryOrdersTool{},
		"get_order_detail":      getOrderDetailTool{},
		"search_faq":            searchFAQTool{},
		"search_documents":      newSearchMaterialsTool(nil), // 由 caller 注入 searchFunc
		"trigger_purchase_offer": triggerPurchaseOfferTool{},
	}
}
```

- [ ] **Step 7: 编译 + Commit**

---

## Phase 3: Engine + Workflow 重新设计

### Task 4: 统一 System Prompt

**Files:**
- Modify: `service/agent_prompts.go`

替换三个 Prompt 为一个：

```go
const SystemPromptV3 = `你是 edu_market 学习平台的智能助手。你能搜索资料、查订单、看评价、检索资料内容、搜索 FAQ。

你的工作方式：
1. 收到用户请求后，自己分析需要什么信息
2. 自己决定调哪些工具、按什么顺序、调几次
3. 工具结果不理想时，自己换策略
4. 信息够了就给回答，不要多余操作

答疑内容边界（重要）：
- 未购买资料的用户问资料内容 → 只回答目录级别 + "有没有X"的概括，不暴露具体操作细节
- 已购买用户 → 可以深度答疑，检索全文

引导购买：
- 用户表现出买前兴趣时，主动调用 trigger_purchase_offer 发购买卡片
- 用户直接表示要买 → 发购买卡片

无关话题：
- 用户说无关话题（写作文、写代码）→ 礼貌引导回学习资料相关
- 不确定时宁可多问一句
- 始终友好、专业、简洁

你拥有的工具：
- query_materials: 搜索资料（可按关键词、分类、价格范围）
- get_material_detail: 获取资料详细信息
- get_material_outline: 获取资料文档目录
- get_reviews: 获取用户评价
- get_categories: 获取分类列表
- query_orders: 查询当前用户订单
- get_order_detail: 获取单笔订单详情
- search_faq: 搜索 FAQ（退款、支付、使用问题）
- search_documents: 搜索资料文档内容
- trigger_purchase_offer: 向用户发送购买卡片`
```

- [ ] **Step 2: 删除三个旧 Prompt 常量**

---

### Task 5: Engine 支持 action 事件

**Files:**
- Modify: `service/agent_engine.go`

改动点：
1. `triggerPurchaseOfferTool` 返回 `__action` 标记时，引擎发 action SSE 事件
2. `search_documents` 根据买前/买后调整参数
3. 最大轮数 7 → 10

- [ ] **Step 1: 引擎 Tool Call 处理加 action 检测**

在 `Run()` 方法的 tool 执行后追加：

```go
// Tool 执行完成，检测是否有 action 标记
if strings.Contains(result.Content, `"__action"`) {
    var actionData map[string]interface{}
    if json.Unmarshal([]byte(result.Content), &actionData) == nil {
        if actionType, ok := actionData["__action"].(string); ok {
            actionJSON, _ := json.Marshal(map[string]interface{}{
                "type": actionType,
                "payload": actionData,
            })
            sseHandler("action", string(actionJSON))
        }
    }
    continue
}
```

- [ ] **Step 2: search_documents tool 加买前限制**

在 `searchMaterialsTool.Execute()` 中，通过配置判断是否需要限制：

```go
if config.App.Agent.PurchaseBoundaryTopK > 0 {
    topK = config.App.Agent.PurchaseBoundaryTopK
}
```

- [ ] **Step 3: maxRounds 默认 7 → 10**

```go
// 在 NewAgentEngine 中读取配置
maxRounds: cfg.MaxToolRounds  // app.yml 改为 10
```

- [ ] **Step 4: 编译 + Commit**

---

### Task 6: Workflow 层

**Files:**
- Create: `service/agent_workflow.go`

Workflow 层负责意图分类和四大 Flow 骨架：

```go
package service

import (
	"edu_market/database"
	"edu_market/model"
)

// Intent 意图类型
const (
	IntentPurchase = "purchase"
	IntentAfterSale = "aftersale"
	IntentConsult   = "consult"
	IntentChat      = "chat"
)

// ClassifyIntent 意图分类（Workflow 固定步骤）
func ClassifyIntent(question string) string {
	// 1. 关键词快速匹配
	msgLower := strings.ToLower(question)
	buyWords := []string{"买", "购买", "下单", "怎么收费", "多少钱"}
	afterSaleWords := []string{"退款", "订单", "支付失败", "投诉", "申诉", "退"}
	consultWords := []string{"有没有", "内容", "目录", "讲什么", "适合", "推荐", "学什么"}
	
	if matchAny(msgLower, buyWords) { return IntentPurchase }
	if matchAny(msgLower, afterSaleWords) { return IntentAfterSale }
	if matchAny(msgLower, consultWords) { return IntentConsult }
	
	// 2. 其他归为闲聊
	return IntentChat
}

// CheckPurchaseStatus 判断是否已购买（Workflow 固定步骤——代码查库，不靠 LLM）
func CheckPurchaseStatus(userID, materialID uint) bool {
	var count int64
	database.DB.Model(&model.Order{}).
		Where("user_id = ? AND course_id = ? AND status = ?", userID, materialID, "paid").
		Count(&count)
	return count > 0
}

func matchAny(msg string, words []string) bool {
	for _, w := range words {
		if strings.Contains(msg, w) { return true }
	}
	return false
}
```

- [ ] **Step 2: 编译 + Commit**

---

### Task 7: AgentService 重构

**Files:**
- Modify: `service/agent_service.go`

核心改动：去掉 `agentType` 参数，统一用一个 Agent。

```go
// Chat 发起 Agent 对话（v3：单一 Agent，无 agentType）
func (s *AgentService) Chat(userID uint, sessionID *uint, question string, searchFunc SearchFunc, sseHandler SSEHandler) (*model.Session, error) {
    // 1. 获取或创建 Session
    var session *model.Session
    if sessionID != nil {
        session = &model.Session{}
        if err := database.DB.Where("id = ? AND user_id = ?", *sessionID, userID).First(session).Error; err != nil {
            if errors.Is(err, gorm.ErrRecordNotFound) { return nil, errors.New("会话不存在") }
            return nil, err
        }
    } else {
        session = &model.Session{
            UserID: userID, AgentType: "agent", Status: model.SessionActive,
            Title: truncateRunes(question, 30),
        }
        if err := database.DB.Create(session).Error; err != nil { return nil, err }
    }

    // 2. Workflow 层：意图分类
    intent := ClassifyIntent(question)

    // 3. 构建上下文（含意图提示）
    prompt := buildPromptWithIntent(intent)

    // 4. 全量 Tool 注册（含 searchFunc 注入）
    tools := buildToolSet()
    if fn, ok := tools["search_documents"].(searchMaterialsTool); ok && searchFunc != nil {
        tools["search_documents"] = newSearchMaterialsTool(searchFunc)
    }

    // 5. 运行引擎
    if err := s.engine.Run(session, question, tools, prompt, sseHandler); err != nil {
        return session, err
    }

    // 6. 更新标题
    s.updateTitle(session)
    return session, nil
}

func buildPromptWithIntent(intent string) string {
    base := SystemPromptV3
    switch intent {
    case IntentPurchase:
        return base + "\n\n当前意图：用户想购买资料。按购买流程走：了解需求→搜索→对比推荐→发购买卡。"
    case IntentAfterSale:
        return base + "\n\n当前意图：售后问题。先调 query_orders 查订单，再定位问题给方案。"
    case IntentConsult:
        return base + "\n\n当前意图：资料咨询。先判断是否购买过，买前只答概括，买后可深度答疑。"
    default:
        return base
    }
}
```

- [ ] **Step 2: 去掉 checkTransfer 调用**

---

## Phase 4: 控制器 + 前端

### Task 8: Controller 适配 action + 前端 action 卡片

**Files:**
- Modify: `controller/agent_controller.go`（无需改，action 事件已通过引擎发送）
- Modify: `web/src/views/AgentChat.vue`（加 action 事件处理）

- [ ] **Step 1: 前端 action 事件解析**

在 SSE 事件解析中追加：
```javascript
} else if (currentEvent === 'action') {
    const d = JSON.parse(payload)
    if (d.type === 'purchase_offer') {
        messages.value.push({
            role: 'action',
            action: { type: 'purchase', ...d.payload }
        })
    }
}
```

- [ ] **Step 2: 购买卡片模板**

```html
<div v-if="msg.role === 'action' && msg.action.type === 'purchase'" class="action-card">
    <div class="card-title">🛒 {{ msg.action.title }}</div>
    <div class="card-price">¥{{ msg.action.price }}</div>
    <button @click="$router.push('/orders')">立即购买</button>
</div>
```

- [ ] **Step 3: Agent 标签统一为"AI 助手"**

- [ ] **Step 4: 构建 + Commit**

---

### Task 9: 配置更新

- [ ] **Step 1: app.yml 更新 max_tool_rounds**

```yaml
agent:
  max_tool_rounds: 10
```

- [ ] **Step 2: 全量测试 + Commit**

---

## Phase 5: 测试

### Task 10: Tool 测试

- [ ] 测试新增 5 个 tool 返回格式正确
- [ ] 测试 `trigger_purchase_offer` 返回 `__action` 标记
- [ ] 测试 `search_documents` 买前限制逻辑

### Task 11: Engine + Workflow 测试

- [ ] 测试意图分类准确率
- [ ] 测试 action 事件发送
- [ ] 测试最大轮数 10

### Task 12: 全量回归 + E2E

- [ ] `go test ./...` 全部 PASS
- [ ] `npm run build` 构建成功
- [ ] 启动服务走一遍完整购买/售后/咨询流程
