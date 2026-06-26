# Agent 系统设计

## 架构

单一 Agent + 安全兜底。LLM 自主规划 + Tool 调用。`service/agent/` 包。

```
用户消息 → AgentService.Chat()
  ├── Workflow 层: ClassifyIntent(关键词) → CheckPurchaseStatus(代码)
  └── Agent 引擎: AgentEngine.Run()
        ├── loadContext(最近20条消息 + System Prompt)
        ├── Tool Calling 循环(最多10轮)
        │    ├── L1 精确重复熔断 → L2 语义回路 → L3 硬上限
        │    ├── 模式白名单 → 参数校验 → 调用预算
        │    ├── LLM 返回 tool_calls → 执行 tool → 继续
        │    └── LLM 返回 content → 最终回答
        └── SSE: thinking/delta/action/done/error
```

## 安全组件（service/agent/safety.go）

| 组件 | 说明 |
|------|------|
| CircuitBreaker | L1 精确重复熔断：Tool+Args 完全相同 → block。AllowRepeat()=true 跳过 |
| SemanticLoopDetector | L2 语义回路：最近 3 轮结果 bigram Jaccard > 0.8 → block |
| ToolBudget | 每个 Tool 独立调用配额，超出拦截 |
| checkToolMode | 模式白名单：shopping/tutoring/support 限制可用 Tool |
| ResolveMode | 根据本轮执行 Tool 自动判定模式，第一轮 mode="" 全开放 |

## ToolResult 结构化错误

```go
type ToolResult struct {
    Success           bool   `json:"success"`
    Content           string `json:"content"`
    Source            string `json:"source,omitempty"`             // "primary" | "fallback" | "error" | "blocked"
    ErrorCode         string `json:"error_code,omitempty"`         // NOT_FOUND | TOOL_BLOCKED | ... 9 种
    Recoverable       bool   `json:"recoverable"`                  // LLM 据此决策是否重试
    RecommendedAction string `json:"recommended_action,omitempty"` // fix_arguments_and_retry | tell_user_boundary | ...
}
```

## 9 个 Tool

| Tool | AllowedModes | AllowRepeat | 用途 | 数据源 |
|------|:---:|:---:|------|--------|
| `query_materials` | shopping, tutoring | — | 按关键词/分类/价格搜索资料 | materials |
| `get_material_detail` | shopping, tutoring | — | 资料详情 | materials+documents |
| `get_reviews` | shopping, tutoring | — | 用户评价列表 | reviews |
| `get_categories` | shopping, tutoring | — | 分类列表 | categories |
| `trigger_purchase_offer` | shopping | ✅ | 发送购买卡片（action SSE） | materials |
| `search_documents` | shopping, tutoring | — | RAG 检索文档内容 | `service/rag` |
| `query_orders` | support | — | 用户订单列表 | orders |
| `get_order_detail` | support | — | 单笔订单详情 | orders |
| `search_faq` | 全模式 | — | FAQ 搜索 | faqs |

## SSE 协议（不变）

| 事件 | 说明 |
|------|------|
| `thinking` | 正在执行 tool |
| `delta` | 逐字流式输出 |
| `action` | 触发前端操作（购买卡片） |
| `done` | 对话完成 |
| `error` | 出错 |

## 日志链路（全流程）

grep 一个 `request_id` 追全链路：

```
Agent 开始              → session_id, mode, question(截80字)
Agent 上下文就绪        → history_msgs, tools(完整名称列表)
Round N 开始            → mode, tools_usable(当前模式可用), tools_total(注册总数)
LLM 响应               → finish, tool_calls, content_len, tokens, llm_ms
Tool ✓                 → ok, len, ms, preview(截200字), 失败时 error_code+recoverable
Round N 结束            → tools_executed, new_mode
Agent 回复              → len, preview(截200字), has_citation, has_refusal
```

## Prompt 模块化

6 模块顺序拼装：Base Persona → Mode Block → State Block → User Context → Rules → Style

## 上下文管理

- `context_max_messages: 20`，从 messages 表加载，过滤 action 卡片
- reasoning_content 存 DB 并回传（DeepSeek v4 要求）
- System Prompt 在最前面

## 配置

```yaml
agent:
  max_tool_rounds: 10
  context_max_messages: 20
  embedding_model: "BAAI/bge-m3"
  embedding_api_url: "https://api.siliconflow.cn/v1/embeddings"
  embedding_api_key: ""
  chunk_size: 500
  chunk_overlap: 50

rag:
  qdrant_url: "http://localhost:6333"
  hybrid_search: true
  rerank: true
  rerank_topk: 3
  cache_enabled: true
  cache_ttl: 3600
  # ... 完整见 config/app.example.yml
```

## 相关文件

- `service/agent/engine.go` — 引擎主循环 + 集成安全组件
- `service/agent/tools.go` — Tool 接口 + 9 个 Tool 实现
- `service/agent/safety.go` — 熔断器 + 工具边界 + 状态机
- `service/agent/prompts.go` — 6 模块 Prompt 拼装 + SessionState
- `service/agent/memory.go` — L3 长期记忆读写
- `service/agent/quality.go` — HardFieldCorrector 硬字段修正
- `service/agent/service.go` — 会话管理 + 编排
- `service/agent/rate.go` — LLM API 令牌桶限流
- `service/rag/` — RAG 检索服务（独立包）
- `controller/agent_controller.go` — SSE handler
- `web/src/views/AgentChat.vue` — 前端聊天界面
