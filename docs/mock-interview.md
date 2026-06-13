# 模拟面试：edu_market 项目

> 基于你的 Go + Vue3 在线学习平台项目

---

## 一、项目概述

**Q1: 用一段话介绍你的项目**

A: 一个在线学习资料售卖 + AI 智能答疑平台。Go + Gin + GORM + MySQL + Redis 后端，Vue3 + Vite 前端。用户可以在线发布/购买学习资料，每份资料包含 Markdown 在线文档编辑器（ByteMD），支持 PDF/PPTX/DOCX 上传转 Markdown。核心差异点是集成了 LLM Agent 系统——基于 DeepSeek API 的单一 Agent，拥有 9 个 Tool，能自主搜索资料、查订单、检索 FAQ、深度答疑，Workflow 层做意图路由和安全兜底。

---

## 二、架构设计

**Q2: 为什么选择单体架构而不是微服务？**

A: 单人开发，项目规模 < 5000 行业务代码，用户量级不大。单体架构部署简单、调试方便、事务管理天然。真要拆也容易——按模块分 service 子包（agent/、material/、order/），后续拆微服务就是改 import 路径。

**Q3: 你的分层架构每层的职责是什么？**

A: router → middleware(CORS/Logger/JWT) → controller(参数绑定+调 service) → service(业务逻辑，不碰 gin.Context) → model(GORM) → MySQL。Controller 只管 HTTP 层、Service 只管业务层、Model 只管数据层，层级间零耦合。

**Q4: 为什么 Service 层不引用 gin.Context？**

A: Service 如果绑定 HTTP 框架，换框架（比如从 Gin 换成 Echo）就要改所有 Service。Service 只返回 Go error，HTTP 状态码的选择权在 Controller，更好的可测试性——测 Service 不需要启动 HTTP 服务。

---

## 三、Agent 系统

**Q5: 你的 Agent 是怎么做的？为什么不用 LangChain 这类框架？**

A: 自研了一个轻量 Agent 引擎，核心就是一个 Tool Calling 循环（约 200 行 Go 代码）。不用 LangChain 因为它是 Python 生态，引入多语言微服务增加部署复杂度。而且我们这个场景 Tool 数量有限（9 个），循环逻辑简单，框架反而是过度抽象。

**Q6: Tool Calling 循环具体怎么实现的？**

A: 
```
1. 加载历史消息（最近 20 条）+ System Prompt
2. 调 LLM API（带 9 个 Tool 的 JSON Schema 定义）
3. LLM 返回 tool_calls → 执行对应 Tool → 结果追加到上下文
4. LLM 返回 content → 流式输出给前端 → 结束
5. 最多 10 轮，超限强制结束
```

**Q7: Workflow 和 Agent 是怎么配合的？**

A: Workflow 只做安全兜底——意图路由（关键词匹配）、购买校验（查 orders 表）、买前内容限制（topK=1/截断 200 字）。Agent 做自主决策——选哪些 tool、什么顺序、失败后换策略、什么时候回答。Workflow 是"骨架"，Agent 是"肌肉"。

**Q8: 9 个 Tool 分别是什么？**

A: query_materials（搜资料）、get_material_detail（资料详情）、get_reviews（评价）、get_categories（分类）、query_orders（订单）、get_order_detail（订单详情）、search_faq（FAQ）、search_documents（RAG 检索）、trigger_purchase_offer（发购买卡片）。

**Q9: trigger_purchase_offer 是怎么触发前端购买卡片的？**

A: Tool 返回 JSON 带 `__action: "purchase_offer"` 标记 → Engine 检测到后发 `event: action` SSE 事件 → 前端收到后在聊天区渲染购买卡片（标题+价格+按钮）。

**Q10: 上下文过长怎么处理？**

A: 目前取最近 20 条消息。超出时保留 system prompt + 最近 2 轮 + 中间做摘要压缩（规划中）。

---

## 四、SSE 流式

**Q11: 你的流式输出怎么实现的？踩过什么坑？**

A: 第一版是假流式——非流式调 LLM 拿完整回复→逐字拆开发 SSE（20ms/字）。首字延迟 3-10 秒，用户感受很差。第二版改用 LLM 原生流式 API（`stream: true`），LLM 每生成一个 token 就转发给前端。

**踩过的大坑**：`reader.cancel()` 在收到 done 事件后丢弃了 TCP 缓冲区未读的 delta→前端只显示前几个字。改成 `break` 自然退出循环解决。

**Q12: Tool Calling 和流式同时怎么处理？**

A: 不能同时——API 限制 `stream: true` 和 `tools` 互斥。所以有 tool call 的轮次用非流式，只有最终纯文本回复轮次才用流式。和 ChatGPT 的做法一样。

**Q13: SSE 事件协议是什么样的？**

A: 5 种事件——thinking（调 tool 中）、delta（流式输出逐字）、action（触发前端操作如购买卡片）、done（完成）、error（出错）。

---

## 五、数据库

**Q14: GORM 你有什么使用经验？**

A: 
- `Updates(map[string]interface{})` 做部分更新，不用 `Save()`（会覆盖零值字段）
- `errors.Is(err, gorm.ErrRecordNotFound)` 区分"记录不存在"和数据库异常
- 分页先用 `Count` 再 `Offset/Limit`
- 敏感字段 `json:"-"` 防止序列化到客户端

**Q15: Message 表的 tool_calls 字段为什么用 JSON 类型？**

A: 每条 tool 调用有不同的参数和结果，JSON 是 Schemaless 的——不需要为每种 tool 建一张表。ToolCall 结构体实现了 `sql.Scanner` 和 `driver.Valuer` 接口，GORM 自动处理序列化/反序列化。

**Q16: Document 表的 content 存什么？**

A: 纯 Markdown 文本。编辑器用 ByteMD，输出就是 Markdown，直接存 TEXT 字段。RAG 切片时从 Markdown 提取纯文本（去格式标记）再 Embedding。

---

## 六、前端

**Q17: 前端 SSE 怎么接收？**

A: 用 `fetch + ReadableStream`（不是 EventSource，因为要 POST 请求）。`resp.body.getReader()` 逐块读取 → 解析 `event: xxx\ndata: xxx` 格式 → 根据事件类型更新 Vue 响应式数据。

**Q18: BYtemd 编辑器自动保存怎么做的？**

A: 监听 `onChange` 事件，debounce 2 秒，调 `PUT /api/documents/:id` 保存 Markdown 内容。触发 RAG 重新切片（异步 goroutine）。

---

## 七、文件解析

**Q19: 上传 PDF/DOCX/PPTX 怎么转成 Markdown 的？**

A: 
- TXT/MD：直接读
- DOCX：`nguyenthenguyen/docx` → 从 `<w:t>` XML 标签提取文字
- PDF：系统 `pdftotext -layout -enc UTF-8` 命令（CJK 支持最好）
- PPTX：`archive/zip` 解压 → 从 slide XML `<a:t>` 提取文字
- 提取的纯文本按段落转为 Markdown

**Q20: 为什么不用 Go 的 PDF 库而用系统命令？**

A: `ledongthuc/pdf` 和 `rsc.io/pdf` 对中文 PDF 的 CMap 字体编码支持差——全是乱码。`pdftotext` 是 C++ 实现的成熟工具，CJK 完美支持。

---

## 八、测试

**Q21: 你怎么跑测试？**

A: 独立测试数据库 `edu_market_test`，TestMain 自动建库+AutoMigrate，测试完清空。测试配置通过 `viper.New()` 从本地 `app.yml` 读敏感字段（API key、密码），不硬编码。所有包 `go test ./...` 一键跑。

---

## 九、开放题

**Q22: 如果重新做，你会怎么改进？**

A: 
1. 流式一开始就用原生 API，不搞模拟逐字
2. LLM 调用的 request/response 全量日志，排查问题更快
3. Tool 抽象用 interface+registry 模式，加新 tool 只需注册一个 struct
4. 前端状态管理用 VueUse 的 SSE composable 统一处理

**Q23: 如果用户量突然涨 10 倍，你会先优化什么？**

A: 
1. LLM 调用做结果缓存——相似问题不重复调 API
2. MySQL 读写分离 + 索引优化（`sessions.user_id`、`messages.session_id` 已有索引）
3. RAG 向量搜索从 SimpleSearch（MySQL LIKE）升级到真正的向量索引
4. Gin mode 从 debug 切 release，日志从同步改异步
