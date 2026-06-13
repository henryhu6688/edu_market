# 模拟面试：edu_market 项目

> 基于 Go + Vue3 在线学习资料售卖 + AI 答疑平台

---

## 项目介绍

**Q1: 用一段话介绍你的项目**

A: 在线学习资料售卖 + AI 智能答疑平台。Go + Gin + GORM + MySQL + Redis 后端，Vue3 + Vite 前端。核心差异点是自研 LLM Agent 系统——基于 DeepSeek API 的单一 Agent，拥有 9 个 Tool，能自主搜索资料、查订单、FAQ、深度答疑。Workflow 做安全兜底，Agent 做自主决策。支持 Markdown 在线编辑器、PDF/DOCX/PPTX 上传转文档、SSE 流式对话。

**Q2: 项目亮点和难点分别是什么？**

**亮点**：
1. **自研 Agent 引擎**：<500 行实现完整 Tool Calling 循环，9 个 Tool，Workflow 骨架 + Agent 自主决策
2. **SSE 流式对话**：原生 DeepSeek 流式 API 转发，首字延迟 <200ms，踩过 reader.cancel() 丢数据等坑
3. **多格式文件解析**：PDF/DOCX/PPTX → Markdown，PDF 中文用系统 pdftotext 解决 Go 库 CJK 乱码
4. **Markdown 在线编辑器**：ByteMD WYSIWYG + 文档树 + 2s 自动保存 + RAG 自动切片

**难点**：
1. **LLM 返回截断**：deepseek-chat 随机截断，加了 max_tokens=4096 + Prompt + 截断检测
2. **reasoning_content 兼容**：v4-pro 要求思考过程保存回传，改了三层（模型字段 + DB + loadContext）
3. **Tool 死循环**：空搜索结果 LLM 反复重试，Prompt 引导 + 文案优化解决
4. **流式数据丢失**：reader.cancel() 丢弃缓冲数据，排查两天才定位

---

## 架构设计

**Q3: 为什么选择单体架构而不是微服务？**

A: 单人开发，<5000 行业务代码。单体部署简单、调试方便、事务管理天然。真要拆按模块分 service 子包，改 import 路径即可。

**Q4: 分层架构每层的职责是什么？**

A: router → middleware(CORS/Logger/JWT) → controller(参数绑定+调 service)→ service(业务逻辑，不碰 gin.Context)→ model(GORM)→ MySQL。Controller 只管 HTTP，Service 只管业务，Model 只管数据，层级间零耦合。

**Q5: 为什么 Service 层不引用 gin.Context？**

A: Service 绑定 HTTP 框架 → 换框架要改所有 Service。Service 只返回 Go error，Controller 决定 HTTP 状态码。测试 Service 不需要启动 HTTP 服务。

---

## Agent 系统

**Q6: Agent 怎么做的？为什么不用 LangChain？**

A: 自研轻量引擎（~200 行核心循环）。LangChain 是 Python 生态，引入微服务增加部署复杂度。9 个 Tool、循环逻辑简单，框架是过度抽象。

**Q7: Tool Calling 循环怎么实现？**

```
1. 加载历史消息（最近20条）+ System Prompt
2. 调 LLM API（带 9 个 Tool 的 JSON Schema）
3. LLM 返回 tool_calls → 执行 Tool → 结果追加到上下文
4. LLM 返回 content → 流式输出给前端 → 结束
5. 最多 10 轮，超限强制结束
```

**Q8: Workflow 和 Agent 怎么配合？**

A: Workflow 做安全兜底——意图路由（关键词）、购买校验（查 orders 表）、买前内容限制（topK=1/200字截断）。Agent 做自主决策——选 tool、排序、失败换策略、何时回答。

**Q9: 9 个 Tool 分别是什么？**

| Tool | 数据源 |
|------|--------|
| query_materials | materials 表 |
| get_material_detail | materials+documents |
| get_reviews | reviews 表 |
| get_categories | categories 表 |
| query_orders | orders 表 |
| get_order_detail | orders 表 |
| search_faq | faqs 表 |
| search_documents | document_chunks 表（RAG） |
| trigger_purchase_offer | materials 表（action SSE） |

**Q10: trigger_purchase_offer 怎么触发前端购买卡片？**

A: Tool 返回 JSON 带 `__action: "purchase_offer"` → Engine 检测后发 `event: action` SSE → 前端渲染购买卡片。

**Q11: 上下文过长怎么处理？**

A: 取最近 20 条消息。超出时保留 system prompt + 最近 2 轮 + 中间做摘要压缩（规划中）。

**Q12: Skill 和 MCP 的区别，在项目中怎么用？**

- **Skill**：Claude Code 的行为模板（brainstorming、TDD、verification），控制在开发流程中怎么做
- **MCP**：外部工具协议（GitHub、Playwright、Context7），扩展 Claude 能做什么

项目中用 Superpowers Skills 执行完整开发流程（brainstorm→plan→execute→verify→finish），用 MCP GitHub 管理代码、Context7 查文档。

---

## SSE 流式

**Q13: 流式输出怎么实现？踩过什么坑？**

**第一版（假流式）**：非流式调 LLM → streamAnswer() 逐字拆发 SSE（20ms/字）。首字延迟 3-10s，用户感受差。

**第二版（真流式）**：`callLLMStream(stream: true)` → LLM 每生成一个 token 就转发给前端。首字延迟 <200ms。

**踩坑**：
1. `reader.cancel()` 在 done 到达时丢弃 TCP 缓冲区的未读 delta → 前端只显示前几个字。改 `break` 自然退出。
2. Tool Calling 和流式互斥——有 tool call 轮次非流式，最终回复才流式。

**Q14: SSE 事件协议？**

5 种事件：thinking（调 tool）、delta（流式输出）、action（触发购买卡片）、done（完成）、error（出错）。

---

## 认证与 JWT

**Q15: JWT 怎么签发的？双 Token 机制是什么？**

A: `golang-jwt/jwt/v5`，HS256，secret 从 `config/app.yml` 读取。签发伪代码：

```go
claims := jwt.MapClaims{
    "user_id": userID, "username": username, "role": role,
    "exp": time.Now().Add(30 * time.Minute).Unix(),
}
token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
return token.SignedString([]byte(secret))
```

| Token | TTL | 用途 |
|-------|-----|------|
| access_token | 30 分钟 | 接口认证 |
| refresh_token | 24 小时 | 静默刷新 access_token |

**Q16: 注册/登录流程中 JWT 怎么流转？**

```
GET /api/captcha/image → POST /api/send-code(phone+captcha) → POST /api/login(phone+code)
  ├─ 新用户 → 自动注册
  └─ 返回 { access_token, refresh_token, user }
→ 前端存 token → Authorization: Bearer <access_token>
→ 过期 → POST /api/refresh { refresh_token } → 换新 token
```

**Q17: JWT 和传统 Session 的区别？**

| | JWT | Session |
|---|---|---|
| 存储位置 | 客户端 | 服务端（Redis） |
| 扩展性 | 无状态，任何服务器可验证 | 需共享存储 |
| 实时失效 | 需黑名单 | 直接删 key |
| 适合场景 | 微服务、API 网关 | 单体应用 |

项目选 JWT + 双 Token 机制平衡安全性和体验。

---

## 数据库

**Q18: Session 表怎么设计的？**

```
sessions                         messages
┌──────────────────────┐        ┌──────────────────────────┐
│ id (PK)              │──┐     │ id (PK)                   │
│ user_id (FK→users)   │  │     │ session_id (FK→sessions)  │
│ agent_type           │  │     │ role (user/assistant/tool) │
│ title                │  │     │ content (TEXT)            │
│ status (active/closed)│  │     │ tool_calls (JSON)          │
│ created_at/updated_at │  │     │ reasoning_content (TEXT)   │
└──────────────────────┘  │     │ tokens_used                │
                           └─────└──────────────────────────┘
```

- `agent_type`：v2 时三个 Agent，v3 统一为 "agent"
- `tool_calls`：JSON 类型，实现 `sql.Scanner`/`driver.Valuer` 接口
- `reasoning_content`：deepseek-v4-pro 需要保存思考过程
- 外键 `ON DELETE CASCADE`

**Q19: Message 的 tool_calls 为什么用 JSON？**

A: 每条 tool 调用的参数和结果结构不同，JSON 是 Schemaless 的——不用为每种 tool 建表。实现了 Scanner/Valuer 接口，GORM 自动序列化。

**Q20: GORM 使用经验？**

- `Updates(map[string]interface{})` 做部分更新，不用 Save()（会覆盖零值字段）
- `errors.Is(err, gorm.ErrRecordNotFound)` 区分"记录不存在"和数据库异常
- 分页先 Count 再 Offset/Limit
- 敏感字段 `json:"-"` 防止序列化

---

## RAG

**Q21: RAG 怎么实现的？**

A: 文档保存 → extractTextFromMarkdown() 提取纯文本 → 切片(500字,50重叠) → 存 document_chunks 表 → Agent 调 search_documents tool → MySQL LIKE 关键词检索 → 拼入 LLM 上下文："基于以下资料回答：{chunks}"

**Q22: 如果 RAG 检索不准确怎么优化？**

1. **向量检索**：调 embedding API + Redis Stack/Qdrant，替代 MySQL LIKE
2. **混合检索**：关键词 + 向量融合互补
3. **重排序**：Cross-Encoder 二次排序 Top-K 结果
4. **切片优化**：按文档结构切（章节/段落）非固定 500 字
5. **元数据过滤**：加 material_id、is_free_preview 条件
6. **查询改写**：LLM 先改写用户问题再检索

---

## 文件解析

**Q23: PDF/DOCX/PPTX 怎么转 Markdown？**

- TXT/MD：直接读
- DOCX：`nguyenthenguyen/docx` → 从 `<w:t>` XML 标签提取文字
- PDF：系统 `pdftotext -layout -enc UTF-8` 命令（CJK 完美支持）
- PPTX：`archive/zip` 解压 → 从 slide XML `<a:t>` 提取文字

**Q24: 为什么不用 Go 的 PDF 库用系统命令？**

A: `ledongthuc/pdf` 和 `rsc.io/pdf` 对中文 PDF 的 CMap 字体编码支持差 → 全是乱码。`pdftotext` 是成熟 C++ 工具，中文完美。

---

## 前端

**Q25: 前端 SSE 怎么接收？**

A: `fetch + ReadableStream`（不用 EventSource，需要 POST）。`resp.body.getReader()` 逐块读 → 解析 `event: x\ndata: y` → 更新 Vue 响应式数据。

**Q26: ByteMD 编辑器自动保存怎么做的？**

A: `onChange` → debounce 2s → `PUT /api/documents/:id` → 后端异步 goroutine 触发 RAG 重新切片。

---

## 测试

**Q27: 测试怎么跑？**

A: 独立数据库 `edu_market_test`，TestMain 自动建库+AutoMigrate+清空。敏感字段用 `viper.New()` 从本地 `app.yml` 读。`go test ./...` 一键全量。

---

## 开放题

**Q28: 如果重新做会怎么改进？**

1. 流式一开始用原生 API，不搞模拟逐字
2. LLM 调用的 request/response 全量日志，快速排查
3. Tool 抽象用 interface+registry 模式，加新 tool 只注册 struct
4. 前端状态管理用 VueUse 的 SSE composable

**Q29: 用户量涨 10 倍，先优化什么？**

1. LLM 结果缓存——相似问题不重复调 API
2. MySQL 读写分离 + 索引优化
3. RAG 从 MySQL LIKE 升级到向量检索
4. Gin mode 切 release，日志同步改异步
