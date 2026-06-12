# v3 Agent 重新设计：Workflow 骨架 + Agent 内核

> 日期: 2026-06-12
> 状态: 设计完成
> 分支: v3_agentImprove
> 取代: v2 Agent 设计 (2026-06-10-agent-design.md)

## 设计理念

**Workflow 固定步骤骨架 + Agent 在节点内自主决策。**

- Workflow：定义"必须走哪几步"，确保流程完整，防止 LLM 跑偏
- Agent：定义"这一步怎么做"，LLM 自己选 tool、自己筛结果、自己决定表达方式

## 架构

```
用户消息进入
      │
      ▼
  ┌─────────────┐
  │ 意图分类      │ ← Agent 判断：买/查/问/闲聊？
  └──────┬──────┘
         │
    ┌────┼────┬────┐
    ▼    ▼    ▼    ▼
  购买   售后  咨询  闲聊
 流程   流程  流程  自由
```

---

## 四大 Flow

### Flow 1: 购买流程（固定 5 步）

```
2a. 确认目标 ─ Agent ─ 理解用户想学什么方向
        │
2b. 搜索匹配 ─ Agent ─ 自己选 tool、选参数、筛结果
        │
2c. 对比推荐 ─ Agent ─ 从结果中挑 2-3 个、说明推荐理由
        │
2d. 发购买卡 ─ 固定 ─ trigger_purchase_offer → action SSE
        │
2e. 跳支付   ─ 固定 ─ 前端处理，与 Agent 无关
```

### Flow 2: 售后流程（固定 4 步）

```
3a. 查订单   ─ 固定 ─ query_orders / get_order_detail 必须调
        │
3b. 定位问题 ─ Agent ─ 分析"支付失败还是想退款？"等
        │
3c. 给方案   ─ Agent ─ 结合 FAQ + 订单状态生成解决步骤
        │
3d. 执行     ─ 固定 ─ 退款链接/申诉入口/人工客服引导
```

### Flow 3: 资料咨询（固定 4 步）

```
4a. 判断买前买后 ─ 固定 ─ 代码查 orders 表，不靠 LLM 猜
        │
       ├── 买前 ──→
       │  4b. 买前答疑 ─ Agent ─ 概括回答 + 技术兜底（topK=1, 200字截断）
       │        │
       │  4d. 发购买引导 ─ Agent 决定 ─ 用户表现出兴趣才发 purchase_offer
       │
       └── 买后 ──→
          4c. 买后答疑 ─ Agent ─ 全文检索 + 深度讲解（无限制）
```

### Flow 4: 闲聊/无关 — 纯 Agent

无固定步骤。自由对话，礼貌引导回正轨。

---

## 节点分类：固定 vs Agent

| 固定步骤（代码控制） | Agent 节点（LLM 自主） |
|---------------------|----------------------|
| 意图分类（规则 + LLM 快判兜底） | 确认用户目标 |
| 查订单（必须调） | 搜索匹配资料 |
| 判断买前/买后（查 orders 表） | 对比推荐 |
| 发购买卡片（trigger_purchase_offer） | 定位售后问题 |
| 跳支付（前端处理） | 给解决方案 |
| 买前答疑边界（代码截断 topK/字数） | 买前/买后答疑内容 |
| 最大步数 10、SSE 连接 | 是否发购买引导 |
| Tool 白名单 | 闲聊回复 |

## 全部 Tool

| Tool | 说明 | 数据来源 | 新增/现有 |
|------|------|---------|----------|
| `query_materials` | 按关键词/分类/价格搜索资料 | materials | 现有（原 query_courses） |
| `get_material_detail` | 资料详情：价格、评价数、文档目录 | materials+documents | ✅ 新增 |
| `get_material_outline` | 文档目录结构（章节标题） | documents | ✅ 新增 |
| `get_reviews` | 某资料的用户评价 | reviews | ✅ 新增 |
| `get_categories` | 所有分类 | categories | ✅ 新增 |
| `query_orders` | 当前用户订单列表 | orders | 现有 |
| `get_order_detail` | 单笔订单详情 | orders | ✅ 新增 |
| `search_faq` | 搜索平台 FAQ | faqs（新表） | ✅ 新增 |
| `search_documents` | RAG 检索文档内容 | document_chunks | 现有 |
| `trigger_purchase_offer` | 引导购买，发送 action SSE | materials | ✅ 新增 |

## System Prompt（一个就够了）

```
你是 edu_market 学习平台的智能助手。你能搜索资料、查订单、看评价、检索资料内容。

工作方式：
- 收到请求后自己分析需要什么信息
- 自己决定调哪些工具、按什么顺序
- 工具结果不理想时换策略
- 信息够了就给回答，不要多余操作

答疑内容边界：
- 未购买用户问资料内容 → 只回答目录级别 + 概括，不暴露具体内容
- 已购买用户可以深度答疑，全文检索

引导购买：
- 用户表现出买前兴趣时，主动发购买卡片

无关话题：
- 用户说无关话题 → 礼貌引导回学习资料相关
```

## 答疑边界技术兜底

| Tool | 买前 | 买后 |
|------|------|------|
| `search_documents` | topK=1，片段截断 200 字 | 无限制 |
| `get_document_content` | 返回错误："购买后可查看" | 返回全文 |

## SSE 事件

| 事件 | 说明 |
|------|------|
| `thinking` | 正在调 tool |
| `delta` | 逐字输出 |
| `done` | 完成 |
| `error` | 出错 |
| `action` | ✅ 新增：触发前端操作 |

### action 类型

| type | 触发 | payload |
|------|------|---------|
| `purchase_offer` | 购买引导 | material_id, title, price, cover |

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

其他表不变。

## 配置

```yaml
agent:
  max_tool_rounds: 10
  context_max_messages: 20
  purchase_boundary_topk: 1
  purchase_boundary_chars: 200
```

## API 不变

## 与 v2 的向后兼容

- `sessions.agent_type` 保留，废弃为 `agent`
- `[TRANSFER:xxx]` 检测保留但不依赖
- 旧 `conversations` 表已删除
