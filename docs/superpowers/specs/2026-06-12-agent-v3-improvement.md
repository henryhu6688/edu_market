# v3 Agent 改进设计：Workflow 骨架 + Agent 内核

> 日期: 2026-06-12
> 状态: 设计完成
> 分支: v3_agentImprove
> 基于: v2 Agent 设计 (2026-06-10-agent-design.md)

## 改进目标

从 "固定流程 Workflow" 升级为 "Workflow 做骨架，Agent 做内核"：

| | v2（现在） | v3（目标） |
|---|-----------|-----------|
| 路由 | 关键词匹配，不命中默认客服 | 关键词优先，LLM 二级判断，无关话题拒绝 |
| Tool 数量 | 每个 Agent 只有 1 个 tool | 每个 Agent 3-4 个 tool |
| 工具使用 | 调 1 个就回答 | 多 tool 串联：思考→行动→观察→再行动→回答 |
| 失败处理 | 错误信息给 LLM 兜底 | LLM 自主换策略 |
| Agent 动作 | 只能返回文字 | 新增 `action` SSE 事件，可触发前端操作 |

## 架构

```
用户消息
  │
  ▼
┌─────────────────────────────────┐
│      Workflow 层（固定）          │
│  - 关键词 + LLM 路由 → 确定领域    │
│  - Tool 白名单 → 安全边界         │
│  - 步数上限(10轮) → 防死循环      │
│  - 无关话题拒绝 → 礼貌引导回正轨    │
└──────────┬──────────────────────┘
           ▼
┌─────────────────────────────────┐
│      Agent 层（自主）             │
│  - 领域内多 tool 串联             │
│  - 思考-行动-观察循环             │
│  - 失败后自主换策略               │
│  - LLM 决定何时回答               │
│  - action 事件触发前端操作         │
└──────────┬──────────────────────┘
           ▼
     DeepSeek API
```

## Router 改进

### 三层路由

```
用户消息
  │
  ▼
第一层：关键词匹配（规则）
  ├─ 命中 → 直接路由到对应 Agent
  └─ 未命中 → 第二层
        ▼
第二层：LLM 快判（1 句话 prompt）
  ├─ customer_service → 客服
  ├─ course_recommend → 推荐
  ├─ qa → 答疑
  └─ irrelevant → 第三层
        ▼
第三层：客服 Agent 兜底 + 礼貌拒绝
  "我是学习助手，可以帮你找资料、查订单、解答课程问题。你想了解什么？"
```

### 关键词表（更新为 v3 术语）

| 意图 | 关键词 |
|------|--------|
| customer_service | 退款、订单、支付失败、怎么买、申诉、客服、联系、投诉、价格、优惠券 |
| course_recommend | 推荐、有什么资料、适合我、哪个好、入门、进阶、有没有、零基础、学什么 |
| qa | 目录、第几章、讲什么、内容、讲义、课件、解释、推导、证明、怎么理解 |

## 三个 Agent 的能力（v3 升级）

### 客服 Agent

| Tool | 说明 | 数据来源 |
|------|------|---------|
| `query_orders` | 查用户订单列表 | orders 表 |
| `get_order_detail` | 单笔订单详情（状态、金额、时间） | orders 表 |
| `search_faq` | 搜索平台 FAQ | 新增 faqs 表 |

新增 `faqs` 表：
```sql
CREATE TABLE faqs (
    id       BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    question VARCHAR(500) NOT NULL,
    answer   TEXT NOT NULL,
    category VARCHAR(50) DEFAULT 'general',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

### 推荐 Agent

| Tool | 说明 | 数据来源 |
|------|------|---------|
| `query_materials` | 按关键词/分类/价格搜索资料 | materials 表 |
| `get_material_detail` | 资料详情 + 评价数 + 购买数 + 文档目录概览 | materials + documents |
| `get_reviews` | 某资料的用户评价 | reviews 表 |
| `get_categories` | 所有分类列表 | categories 表 |

### 答疑 Agent

| Tool | 说明 | 数据来源 |
|------|------|---------|
| `get_material_outline` | 文档目录结构（章节标题） | documents 表 |
| `search_documents` | RAG 检索文档内容，买前限制返回量 | document_chunks |
| `get_document_content` | 某篇文档完整内容（仅买后开放） | documents 表 |
| `trigger_purchase_offer` | 引导购买，发送 action SSE 事件 | materials 表 |

## 答疑内容边界

| 能力 | 买前 | 买后 |
|------|------|------|
| 目录结构 | ✅ | ✅ |
| 试读文档 | ✅ 全文 | ✅ |
| "有没有X"类概括回答 | ✅ | ✅ |
| 全文检索 | ❌ | ✅ |
| 逐章详细讲解 | ❌ | ✅ |
| 代码/案例细节 | ❌ | ✅ |

### 技术兜底

不只靠 System Prompt。代码层强制：
- `search_documents` 买前调用时 `topK=1`，截断每个片段到 200 字
- `get_document_content` 买前直接返回错误："购买后可查看完整内容"

## 新的 SSE 事件：action

v2 只有 `thinking / delta / done / error`。v3 新增 `action`——让 Agent 不只是说，还能做。

### 事件定义

```
event: action
data: {"type":"<action_type>","payload":{...}}
```

### action 类型

| type | 触发时机 | 前端渲染 |
|------|---------|---------|
| `purchase_offer` | 答疑买前 → 用户表现出兴趣 | 购买卡片（标题、价格、按钮） |
| `transfer_agent` | 需要切换到其他 Agent | 切换提示 |
| `order_link` | 客服查订单后 | 订单详情卡片 |

### purchase_offer JSON 结构

```json
{
  "type": "purchase_offer",
  "payload": {
    "material_id": 1,
    "title": "Python 数据分析实战",
    "price": 29.90,
    "cover_image": "/uploads/covers/xxx.jpg"
  }
}
```

### 实现

`trigger_purchase_offer` tool：
1. 校验用户未购买该资料
2. 查 material 信息
3. 通过 SSEHandler 发送 `action` 事件
4. 前端收到后渲染购买卡片

## 前端改动

### action 事件处理

```javascript
if (currentEvent === 'action') {
  const d = JSON.parse(payload)
  if (d.type === 'purchase_offer') {
    messages.value.push({
      role: 'assistant',
      actionCard: {
        type: 'purchase',
        materialId: d.payload.material_id,
        title: d.payload.title,
        price: d.payload.price
      }
    })
  }
}
```

### 购买卡片组件

聊天气泡中渲染：
```
┌─────────────────────────────┐
│ 🛒 Python 数据分析实战       │
│    ¥29.90                   │
│    [立即购买]                 │
└─────────────────────────────┘
```

## 配置

```yaml
agent:
  max_tool_rounds: 10           # 提升到 10 轮（多 tool 串联需要更多步数）
  context_max_messages: 20
  purchase_boundary_topk: 1     # 买前检索 topK
  purchase_boundary_chars: 200  # 买前检索片段截断长度
```

## 改进点汇总

| # | v2 问题 | v3 改进 |
|---|---------|---------|
| 1 | 未命中关键词默认客服 | LLM 二级判断 + 无关话题拒绝 |
| 2 | 每个 Agent 只有 1 个 tool | 3-4 个 tool，支持多工具串联 |
| 3 | 答疑边界纯靠 Prompt | 代码层 topK/截断兜底 |
| 4 | Agent 只能返回文字 | action SSE 事件，触发购买/订单等操作 |
| 5 | 缺少 FAQ | 新增 faqs 表 + search_faq tool |
| 6 | Tool 循环上限 7 轮 | 提升到 10 轮 |
| 7 | transfer 标记硬编码 | 保留兼容，优先用 action 切换 |

## 向后兼容

- 现有 `sessions`、`messages` 表不变
- `[TRANSFER:xxx]` 标记继续支持，与 `action` 并行
- 旧 API 路由不变
- old `conversations` 表已删除
