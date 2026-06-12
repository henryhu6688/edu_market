# v3 Agent 重新设计：单一 Agent + LLM 自主规划

> 日期: 2026-06-12
> 状态: 设计完成
> 分支: v3_agentImprove
> 取代: v2 Agent 设计 (2026-06-10-agent-design.md)

## 设计理念

**从 "3 个按规则运作的 Workflow" 变成一个 "LLM 自主规划的 Agent"。**

核心理念：LLM 拥有规划权。Workflow 层只做安全兜底。

## 架构

```
用户消息
  │
  ▼
┌─────────────────────────────────────────┐
│           Workflow 层（极薄）              │
│  - Tool 白名单（只读 DB，不下线）            │
│  - 最大步数 10（防死循环）                 │
│  - 步间延迟、Token 计数                   │
│  - SSE 连接管理                          │
│  - 兜底：100% 有回复                      │
└──────────────────┬──────────────────────┘
                   ▼
┌─────────────────────────────────────────┐
│            一个 Agent                    │
│                                         │
│  全部 Tool 可用（10 个）                   │
│  LLM 自己规划：先做什么、后做什么            │
│  LLM 自己决策：Tool 结果不好 → 换策略       │
│  LLM 自己判断：什么时候回答、什么时候发action │
│  LLM 自己处理：闲聊就闲聊、无关就拒绝        │
└──────────────────┬──────────────────────┘
                   ▼
             DeepSeek API
```

## 和 v2 的本质区别

| | v2（3 个 Agent + Router） | v3（1 个 Agent） |
|---|---|---|
| 谁会做什么 | Router 关键词匹配决定 | LLM 自己分析用户意图 |
| Tool 给谁 | 硬编码分配给 3 个 Agent | 全部 tool 都可用，LLM 自己选 |
| 多步骤 | 每次 1 个 tool | LLM 规划多步串联 |
| 跨领域 | Transfer 标记切换 | LLM 自己决定转变话题 |
| 兜底 | 不匹配默认客服 | LLM 判断是否无关，礼貌拒绝 |
| 自主性 | 无。走固定流程 | 有。LLM 可以临时调整计划 |

## 全部 Tool（一个 Agent 用所有）

| Tool | 说明 | 数据来源 |
|------|------|---------|
| `query_materials` | 按关键词/分类/价格搜索资料 | materials 表 |
| `get_material_detail` | 资料详情：价格、评价数、购买数、文档目录 | materials + documents |
| `get_material_outline` | 文档目录结构（章节标题） | documents 表 |
| `get_reviews` | 某资料的用户评价 | reviews 表 |
| `get_categories` | 所有分类 | categories 表 |
| `query_orders` | 当前用户订单列表 | orders 表 |
| `get_order_detail` | 单笔订单详情 | orders 表 |
| `search_faq` | 搜索平台 FAQ | 新增 faqs 表 |
| `search_documents` | RAG 检索文档内容（买前限制） | document_chunks |
| `trigger_purchase_offer` | 引导购买（发 action SSE） | materials 表 |

## System Prompt（核心）

```
你是 edu_market 学习平台的智能助手。你拥有以下能力：

搜资料、查订单、看评价、检索资料内容、平台FAQ、引导购买。

你的工作方式：
1. 收到用户请求后，自己分析需要什么信息
2. 自己决定调哪些工具、按什么顺序
3. 工具结果不理想时，自己换策略
4. 信息够了就给回答，不要多余操作

答疑内容边界：
- 未购买资料的用户问内容 → 只回答目录级别 + "有没有X"的概括，不暴露具体内容
- 已购买用户可以深度答疑，检索全文
- 主动判断用户是否购买过资料

关于引导购买：
- 用户表现出对某资料的兴趣且未购买 → 主动发购买卡片
- 用户问的内容超出买前边界 → 提示购买后可查看

关于边界：
- 用户说无关话题（写代码、写作文）→ 礼貌引导回学习资料相关
- 不确定时宁可多问一句，不要猜
- 始终友好、专业、简洁
```

## 答疑边界技术实现

不只靠 Prompt，代码层强制：

| Tool | 买前限制 | 买后 |
|------|---------|------|
| `search_documents` | topK=1，每个片段截断到 200 字 | 无限制 |
| `get_document_content` | 返回错误："购买后可查看" | 返回全文 |

## SSE 事件

| 事件 | 说明 | v3 新增 |
|------|------|---------|
| `thinking` | 正在调 tool | - |
| `delta` | 逐字输出 | - |
| `done` | 完成 | - |
| `error` | 出错 | - |
| `action` | 触发前端操作 | ✅ 新增 |

### action 类型

| type | 触发时机 | payload |
|------|---------|---------|
| `purchase_offer` | 用户表现出购买兴趣 | material_id, title, price, cover |
| `transfer_agent` | 保留，暂不使用（v3 只有一个 Agent） | target |

## 数据模型

### 新增 faqs 表

```sql
CREATE TABLE faqs (
    id        BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    question  VARCHAR(500) NOT NULL,
    answer    TEXT NOT NULL,
    category  VARCHAR(50) DEFAULT 'general',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

其他表不变：`sessions`、`messages`、`document_chunks` 继续使用。

## Agent 引擎改动

v3 引擎和 v2 核心逻辑相同（Tool Calling 循环），改动点：

| 改动 | 说明 |
|------|------|
| 去掉 `agentType` 参数 | 只有一个 Agent，不需要区分类型 |
| Router 逻辑简化 | 去掉关键词路由，LLM 自己判断 |
| Tool 注册改为全量 | `buildToolSet` 不再按 agentType 筛选 |
| System Prompt 统一 | 一个 prompt，不再分三种 |
| 最大轮数提升 | 7 → 10 |
| action 事件 | 引擎支持发送 action SSE 事件 |

## 前端改动

### action 事件渲染

```javascript
if (currentEvent === 'action') {
  const d = JSON.parse(payload)
  if (d.type === 'purchase_offer') {
    messages.value.push({
      role: 'action',
      action: { type: 'purchase', ...d.payload }
    })
  }
}
```

### 购买卡片

聊天气泡中渲染操作卡片（不是纯文字）。

### Agent 标签

去掉客服/推荐/答疑标签。改为统一的"AI 助手"。

## 配置

```yaml
agent:
  max_tool_rounds: 10
  context_max_messages: 20
  purchase_boundary_topk: 1
  purchase_boundary_chars: 200
```

## API 不变

```
POST /api/agent/chat          SSE 对话
GET  /api/agent/sessions      会话列表
DELETE /api/agent/sessions/:id  删除会话
GET  /api/agent/sessions/:id/messages  消息历史
```

## 与 v2 的向后兼容

- `sessions.agent_type` 字段保留但废弃，默认 `agent`
- `[TRANSFER:xxx]` 检测保留但不依赖
- 旧 `conversations` 表已删除

## 测试策略

| 测试 | 内容 |
|------|------|
| `agent_engine_test.go` | Tool 循环、上下文、10 轮上限、action 事件 |
| `agent_router_test.go` | LLM 判断路由、无关话题拒绝 |
| `agent_tools_test.go` | 各 tool 正确性、内容边界限制 |
| `agent_service_test.go` | 会话管理、权限、SSE 输出 |
