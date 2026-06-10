# v2 智能 Agent 系统设计

> 日期: 2026-06-10
> 状态: 设计完成
> 分支: v2_RAG

## 概述

将现有简单的 AI 问答升级为真正的智能 Agent 系统，覆盖三个核心场景：

- **智能客服** — 回答平台使用、订单、退款等问题
- **课程推荐** — 根据用户背景和兴趣个性化推荐课程
- **AI 答疑助教** — 基于课程资料的深度答疑（RAG）

## 整体架构

```
                         前端 (Vue3)
                            │
              POST /api/agent/chat
              { session_id?, question }
                            │
                            ▼
              ┌─────────────────────┐
              │   AgentController   │  ← 新增
              │   解析请求、SSE 管理  │
              └────────┬────────────┘
                       │
                       ▼
              ┌─────────────────────┐
              │    AgentService     │  ← 总调度
              │   1. 会话管理        │
              │   2. 意图路由        │
              │   3. 调用 Agent 引擎 │
              └────────┬────────────┘
                       │
          ┌────────────┼────────────┐
          ▼            ▼            ▼
      ┌──────┐   ┌──────┐   ┌──────┐
      │ 客服   │   │ 推荐   │   │ 答疑   │
      │Agent │   │Agent │   │Agent │
      └──┬───┘   └──┬───┘   └──┬───┘
         └──────────┼──────────┘
                    ▼
         ┌─────────────────────┐
         │    Agent 引擎        │
         │   - Tool 调用循环     │
         │   - 上下文管理器      │
         │   - SSE Writer       │
         └──────────┬──────────┘
                    ▼
         ┌─────────────────────┐
         │   LLM API (DeepSeek) │
         └─────────────────────┘
```

### 技术选型

- **Agent 引擎**: Go 自研，不引入 Agent 框架
  - 理由：Agent 核心逻辑约 100-200 行（Tool Calling 循环），框架反而带来耦合和排查困难
  - 与项目现有模式完全一致（直接调 HTTP API，数据库通过 `database.DB`）
- **向量存储**: Redis Stack (RediSearch)
  - 理由：复用现有 Redis，零额外运维
  - 接口层预留抽象，后续可切 Pinecone/Qdrant
- **LLM**: DeepSeek（现有 provider），OpenAI 兼容 API
- **Embedding**: 通过 LLM API 的 embedding endpoint（如 DeepSeek 或 OpenAI text-embedding-3-small）

## 数据模型

### 新增表

```sql
-- 会话表
CREATE TABLE sessions (
    id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id    BIGINT UNSIGNED NOT NULL,
    agent_type VARCHAR(30) NOT NULL DEFAULT 'customer_service',  -- customer_service | course_recommend | qa
    title      VARCHAR(100) DEFAULT '',                          -- 自动生成（首轮截取）
    status     VARCHAR(20) NOT NULL DEFAULT 'active',            -- active | closed
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_user_id (user_id),
    INDEX idx_user_status (user_id, status),
    CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- 消息表
CREATE TABLE messages (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    session_id  BIGINT UNSIGNED NOT NULL,
    role        VARCHAR(20) NOT NULL,          -- system | user | assistant | tool
    content     TEXT NOT NULL,                 -- 消息内容
    tool_calls  JSON DEFAULT NULL,             -- tool 调用信息（role=tool 时）
    tokens_used INT DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_session_id (session_id),
    INDEX idx_session_created (session_id, created_at),
    CONSTRAINT fk_messages_session FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

-- RAG 文档块表
CREATE TABLE document_chunks (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    course_id   BIGINT UNSIGNED NOT NULL,
    content     TEXT NOT NULL,                 -- 文本块内容
    chunk_index INT NOT NULL DEFAULT 0,        -- 块序号（保持原始顺序）
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_course_id (course_id),
    CONSTRAINT fk_chunks_course FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE
);
```

### 旧表兼容

- 旧 `conversations` 表保持不变，旧 `/api/ai/*` 路由仍然可用
- 新功能走 `sessions + messages`
- 两套共存，后续可迁移

## Agent 引擎

### Tool Calling 循环

```
用户提问
  → 存 user message
  → 从 messages 表取最近 N 轮上下文（最多 20 条）
  → 循环（最多 7 轮）：
      → 发 LLM 请求（历史 + tools 定义）
      → LLM 返回 tool_calls？→ 执行工具 → 存 tool message → 继续循环
      → LLM 返回 content？   → SSE 流式输出 → 存 assistant message → 结束
  → 超过 7 轮未结束 → 返回 fallback："抱歉，这个问题比较复杂，请联系人工客服"
```

### 上下文管理器

- 从 `messages` 表取 `session_id` 对应的最近消息
- 默认取最近 20 条（`agent.context_max_messages`）
- 超出上下文窗口时：保留 system prompt + 最近 2 轮 + 中间做摘要压缩
- 上下文组装为 `[]chatMsg` 发给 LLM

### SSE Writer

| 事件类型 | SSE data | 触发时机 | 前端效果 |
|---------|----------|---------|---------|
| `thinking` | `{"tool": "query_orders", "status": "executing"}` | Agent 执行工具时 | 显示"正在查询你的订单..." |
| `delta` | `{"content": "你"}` | LLM 流式输出文本 | 打字机逐字追加 |
| `done` | `{"session_id": 1, "agent_type": "customer_service"}` | 对话完成 | 停止接收，显示完整回答 |
| `error` | `{"message": "AI服务暂不可用"}` | 出错 | 显示错误提示 |

### Token 管理

- 每次 LLM 响应记录 `tokens_used` 到 `messages` 表
- Session 级别可汇总 token 消耗

## 三 Agent + Router

### Router（意图路由）

两阶段路由，优先规则，不确定时 LLM 兜底：

**第一阶段：关键词规则匹配**

| 意图 | 关键词/模式 |
|------|-----------|
| customer_service | 退款、订单、支付失败、怎么买、申诉、客服、联系、投诉、价格、优惠券 |
| course_recommend | 推荐、有什么课、适合我、哪个好、入门、进阶、有没有、零基础、学什么 |
| qa | 这个公式、第三章、解释一下、为什么、怎么做、详细讲讲、讲义、课件 |

**第二阶段：LLM 判断（规则匹配不明确时）**

用轻量模型一句话判断："用户意图是客服/推荐/答疑？只回答一个词。"（复用 Agent 的 LLM provider，不额外引入模型）

### 客服 Agent

| 项目 | 内容 |
|------|------|
| System Prompt | "你是 edu_market 平台的智能客服。你的职责是帮助用户解决订单、支付、退款、平台使用等问题。回答要简洁精确。如果用户问到课程推荐，回答后标记 [TRANSFER:course_recommend]。" |
| 对话风格 | 简洁高效，1-2 轮解决问题 |
| Tools | `query_orders`（查用户订单）、`get_order_detail`（查订单详情）、`faq_search`（FAQ 全文搜索） |

### 课程推荐 Agent

| 项目 | 内容 |
|------|------|
| System Prompt | "你是一个专业的学习顾问。你的职责是了解用户的学习目标和背景，推荐最合适的课程。要先了解用户情况再推荐，不要一上来就扔课程列表。如果用户对某门课程有深入疑问，标记 [TRANSFER:qa]。" |
| 对话风格 | 引导式探索，主动追问用户目标/基础 |
| Tools | `query_courses`（按分类/关键词/价格范围查课程）、`get_user_profile`（查用户学习情况）、`get_course_reviews`（查课程评价）、`get_categories`（查分类列表） |

### 答疑 Agent

| 项目 | 内容 |
|------|------|
| System Prompt | "你是一位专业课程助教。你需要基于课程资料深度解答用户问题。回答要详细严谨，引用资料原文。鼓励用户追问。如果发现用户未购买相关课程，建议先了解课程。" |
| 对话风格 | 详细严谨，引用原文，鼓励追问 |
| Tools | `search_course_materials`（RAG 检索课程资料）、`get_user_orders`（确认用户已购课程） |

### Agent 间切换

- Agent 的 System Prompt 中定义切换规则
- 切换标记：`[TRANSFER:qa]` / `[TRANSFER:course_recommend]` / `[TRANSFER:customer_service]`
- AgentService 检测到标记 → 更新 session 的 `agent_type` → 下一轮请求自动路由到新 Agent

## RAG 设计

### 资料入库流程

```
管理员上传资料（PDF/PPT/Markdown）
  → 文本提取（PDF → pdfparse, PPT → pptx, MD → 直接读）
  → 文本清洗（去页眉页脚、规范化空白）
  → 切片：500 字/块，50 字重叠
  → 调 Embedding API 生成向量
  → 存 document_chunks 表 + Redis 向量索引
```

### 查询检索流程

```
用户问题（如"第三章的公式怎么推导"）
  → 向量化（调 Embedding API）
  → Redis Stack 向量搜索（KNN, Top K=3-5）
  → 取回 chunks.content
  → 拼入 LLM 上下文：
    "基于以下课程资料回答用户问题：
     
     【资料片段 1】(来自第 X 章)
     {chunk.content}
     
     【资料片段 2】
     ...
     
     用户问题：{question}"
  → LLM 生成回答
```

### 向量存储接口抽象

```go
// VectorStore 向量存储接口（预留切换 Pinecone/Qdrant 的能力）
type VectorStore interface {
    // Search 向量相似度搜索，返回 topK 个最相似的块 ID + 相似度分数
    Search(courseID uint, embedding []float32, topK int) ([]SearchResult, error)
    // Index 写入向量索引
    Index(chunkID uint, embedding []float32, metadata map[string]string) error
    // Delete 删除某课程的所有向量
    Delete(courseID uint) error
}

// 默认实现：Redis Stack (RediSearch)
// 后续可替换为 QdrantVectorStore / PineconeVectorStore
```

## 前端改造

### SSE 客户端

```typescript
// 使用 fetch + ReadableStream（比 EventSource 好，支持 POST）
const response = await fetch('/api/agent/chat', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
  body: JSON.stringify({ session_id, question })
})

const reader = response.body.getReader()
const decoder = new TextDecoder()

while (true) {
  const { done, value } = await reader.read()
  if (done) break
  // 解析 SSE 行：data: {"type":"delta","content":"你"}
  parseSSELine(decoder.decode(value))
}
```

### 页面布局

```
┌─────────────┬──────────────────────────┐
│  会话列表     │     聊天区域               │
│              │                          │
│ 📝 新对话     │   标签：[客服] 订单退款问题  │
│              │                          │
│ 💬 Python入门 │   🔍 正在查询你的订单...   │
│ 💬 机器学习推荐│                          │
│ 💬 订单退款   │   您的订单 #20260101       │
│              │   状态：已支付             │
│              │   如有问题可申请退款...     │
│              │                          │
│              ├──────────────────────────┤
│              │  💬 输入消息...      [发送] │
└──────────────┴──────────────────────────┘
```

### 交互要点

- 左侧会话列表：可新建/切换/删除会话
- 新对话标题自动生成：首轮 LLM 回答截取前 15 字
- Agent 思考时：显示工具名 + loading 动画
- 打字机效果：delta 事件逐字追加到气泡
- Agent 标签：显示当前是客服/推荐/答疑
- 连接中断：显示重连按钮

## 配置

```yaml
# config/app.yml 新增
agent:
  max_tool_rounds: 7            # 最大 Tool Calling 轮数
  context_max_messages: 20      # 上下文最多取 recent N 条
  embedding_model: ""               # RAG 向量化模型（按 provider 配置，如 openai 用 text-embedding-3-small，deepseek 待确认）
  embedding_api_url: ""         # Embedding API 地址（留空则用 LLM provider 的）
  chunk_size: 500               # 文档切片大小（字符）
  chunk_overlap: 50             # 切片重叠长度
```

## API 设计

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| POST | `/api/agent/chat` | JWT | 发起/继续对话（SSE 流式响应） |
| GET | `/api/agent/sessions` | JWT | 获取会话列表 |
| DELETE | `/api/agent/sessions/:id` | JWT | 删除会话（软删或关闭） |
| GET | `/api/agent/sessions/:id/messages` | JWT | 获取某会话的消息历史 |

旧 `/api/ai/chat` 和 `/api/ai/history` 保持不变，向后兼容。

## 异常处理

| 异常场景 | 处理 |
|---------|------|
| LLM API 超时/不可用 | SSE error 事件："AI 服务暂时不可用，请稍后再试" |
| Tool 执行失败 | tool result 标记错误："工具执行失败，请换种方式处理" |
| 超过 7 轮 Tool Calling | 终止循环："抱歉，这个问题比较复杂，请联系人工客服" |
| SSE 连接中断 | 前端显示"连接中断" + 重连按钮 |
| 用户未购课却问答疑 | Agent 可切换或提示："这门课程可能需要购买后才能深入学习哦，要我为你介绍一下吗？" |
| Embedding 服务不可用 | RAG 工具返回空，LLM 用自身知识回答并标注"注意：基于通用知识，非课程资料" |

## 测试策略

遵循项目现有约定（`edu_market_test` 独立库、`setup_test.go` 放 TestMain）：

| 测试文件 | 测试内容 |
|---------|---------|
| `service/agent_engine_test.go` | Tool 循环逻辑、上下文截断、超时保护、7 轮上限 |
| `service/agent_router_test.go` | 规则路由准确率、LLM fallback 路由、切换标记检测 |
| `service/agent_customer_test.go` | 客服 Agent Prompt + Tool 调用正确性 |
| `service/agent_recommend_test.go` | 推荐 Agent Prompt + Tool 调用正确性 |
| `service/agent_qa_test.go` | 答疑 Agent Prompt + RAG Tool 调用正确性 |
| `service/agent_rag_test.go` | 文档切片、向量检索精度、接口抽象 |

测试数据在 `TestMain` 中初始化，跑完 `cleanAllTestData()` + `FlushDB()` 清空。

## 开发流水线

按项目约定，新功能开发顺序：

```
1. model/          → Session、Message、DocumentChunk 模型
2. dto/request/    → AgentChatReq、AgentHistoryReq
3. dto/response/   → SSE 事件结构体（如需要）
4. service/        → AgentEngine → AgentService → Router → 各 Agent
5. controller/     → AgentController（SSE handler）
6. router/         → 注册路由
```

旧代码不动，所有新文件放在对应目录即可。

## 已知限制与后续迭代

| 限制 | 后续方向 |
|------|---------|
| 向量搜索用 Redis Stack，功能有限 | 数据量上 10 万级时切 Pinecone/Qdrant |
| 上下文窗口靠截断，无智能摘要 | 引入 LLM 摘要中间层 |
| Agent 间切换靠标记，一轮延迟 | 升级 Router 为前置路由（每轮判断） |
| 无多模态（图片/视频）资料支持 | 后续按需加 |
