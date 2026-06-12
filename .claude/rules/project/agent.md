# Agent 系统设计

## 架构

单一 Agent + Workflow 安全兜底。LLM 自主规划 + Tool 调用。

```
用户消息 → AgentService.Chat()
  ├── Workflow 层: ClassifyIntent(关键词) → CheckPurchaseStatus(代码)
  └── Agent 层: AgentEngine.Run()
        ├── loadContext(最近20条消息 + System Prompt)
        ├── Tool Calling 循环(最多10轮)
        │    ├── LLM 返回 tool_calls → 执行 tool → 继续
        │    └── LLM 返回 content → streamAnswer(60ms逐字)
        └── SSE: thinking/delta/action/done/error
```

## 9 个 Tool

| Tool | 用途 | 数据源 |
|------|------|--------|
| `query_materials` | 按关键词/分类/价格搜索资料 | materials |
| `get_material_detail` | 资料详情（价格、目录、评价数） | materials+documents |
| `get_reviews` | 用户评价列表 | reviews |
| `get_categories` | 分类列表 | categories |
| `query_orders` | 用户订单列表 | orders |
| `get_order_detail` | 单笔订单详情 | orders |
| `search_faq` | FAQ 搜索 | faqs |
| `search_documents` | RAG 检索文档内容 | document_chunks |
| `trigger_purchase_offer` | 发送购买卡片（action SSE） | materials |

## SSE 协议

| 事件 | 说明 |
|------|------|
| `thinking` | 正在执行 tool |
| `delta` | 逐字流式输出 |
| `action` | 触发前端操作（购买卡片） |
| `done` | 对话完成 |
| `error` | 出错 |

## 上下文管理

- 从 `messages` 表加载最近 20 条
- System Prompt 在最前面
- Tool call 结果带上 `tool_call_id`

## 配置

```yaml
agent:
  max_tool_rounds: 10
  context_max_messages: 20
  purchase_boundary_topk: 1   # 买前检索限制
  purchase_boundary_chars: 200
```

## 相关文件

- `service/agent_engine.go` — 引擎主循环
- `service/agent_tools.go` — Tool 定义
- `service/agent_workflow.go` — 意图分类 + 购买校验
- `service/agent_prompts.go` — System Prompt
- `service/agent_rag.go` — RAG 向量检索
- `service/agent_service.go` — 会话管理 + 编排
- `controller/agent_controller.go` — SSE handler
- `web/src/views/AgentChat.vue` — 前端聊天界面
