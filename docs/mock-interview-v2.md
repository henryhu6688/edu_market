# 模拟面试：edu_market 项目

> 基于你的实际项目，面试官视角提问 + 参考答案

---

## Q1: 简单介绍一下你的项目

**参考回答：**

我独立开发了一个在线学习资料交易平台，后端 Go + Gin，前端 Vue3。核心亮点是自研了一套 LLM Agent 引擎——没有用 LangChain 这类框架，纯 Go 写了约 500 行的 Tool Calling 循环，驱动智能客服、资料推荐和 RAG 答疑。还做了完整的文档处理流水线，PDF/DOCX/PPTX 上传自动转 Markdown 在线编辑器。生产环境上加了 Redis 滑动窗口限流、Semaphore 并发控制、全链路 request_id 日志追踪。

---

## Q2: 你的 Agent 引擎怎么设计的？和直接调 LLM API 有什么区别？

**参考回答：**

直接调 API 就是你问一句它答一句，没有工具调用能力。我的 Agent 引擎本质是一个 Tool Calling 循环——LLM 可以调用 9 个 Tool（搜资料、查订单、FAQ、RAG 检索、发购买卡片等），自己决定调哪个、什么顺序、失败后换什么策略。最大 10 轮迭代。

架构上是双层：Workflow 层做安全兜底（轮数上限、防死循环、Tool 白名单）和嵌入式提示——Engine 层让 LLM 自主规划。与 LangChain 等框架的主要区别在于它是在 Go 环境中原生执行的，避免了 Python 微服务的额外部署负担。

**追问：为什么不用 LangChain，要自己写？**

虽然 LangChain 生态系统完善，但引入它会增加技术栈复杂性和部署成本。我们的场景涉及 9 个固定 Tool 和最多 10 步的执行路径，逻辑相对确定，自研框架完全可控且调试便捷。

---

## Q3: Tool Calling 循环具体怎么实现？LLM 返回 tool_calls 你怎么处理？

**参考回答：**

核心在 `agent_engine.go` 的 Run 方法。循环里调 DeepSeek API（非流式），返回的 JSON 里有 `tool_calls` 数组。我解析出来看 `function.name` 和 `function.arguments`，查 tool map 找到对应的 Tool struct，调用 `Execute()` 执行。执行结果作为 `role: tool` 消息拼回上下文，继续下一轮 LLM 调用。如果 LLM 返回的是纯文本而不是 tool_calls，说明它认为信息够了，就走流式输出给前端。

其中一个关键细节是：`stream:true` 和 `tools` 参数在 DeepSeek API 上互斥。所以前几轮的 Tool Calling 用非流式，最后一轮（最终回复）才开到流式模式。

---

## Q4: Agent 执行过程中出问题怎么兜底？

**参考回答：**

多层兜底。最外层 Workflow 限制了最大 10 轮，超了就返回"问题较复杂，请联系人工客服"。

中间层做了死循环防范：同一 Tool 连续调用超过 2 次就自动跳过，不再执行。

内层是对第三方 API 的保护：LLM API 和 Embedding API 都用 Semaphore 控制了全局并发数，避免触发 429 限流。LLM 调用本身也有 60 秒超时，超时返回"服务暂不可用"。

此外 Redis 存储还设计了自动降级机制：向量搜索如果 Redis 集群故障，会自动切换为 MySQL 加载向量 + Go 内存计算余弦相似度的备选方案。

---

## Q5: 介绍一下你项目里的 RAG 怎么实现的？

**参考回答：**

完整流水线：文档上传 → 纯文本提取 → 切片（500字/50重叠）→ 调硅基流动的 Embedding API 生成 1024 维向量 → 双写 MySQL 和 Redis Stack。搜索时先调 Embedding API 把用户问题向量化，然后 Redis Stack 做 HNSW KNN 搜索取 Top-5，把结果拼入 LLM 的 System Prompt 作为上下文参考。

设计了一个 `VectorStore` 接口，只有 `Search/Index/Delete` 三个方法。从最简单的 MySQL LIKE 搜索升级到 Redis Stack 向量搜索，只需要改一行初始化代码。以后要切到 Pinecone 或 Qdrant，也是实现这个接口就行。

**追问：Agent 怎么决定什么时候用 RAG？**

在我的工作流程中，Agent 会先分析用户意图——如果是"寻资料"就调用 query_materials，如果是"问资料内容/章节"就先 get_material_detail 再 search_documents。买前只答目录级内容，买后才能深度检索全文。Agent 可以自主决定调用 RAG 的时机，不需要硬编码——System Prompt 里用自然语言描述了"什么时候该搜"。

---

## Q6: 你用了 SSE，和 WebSocket 有什么区别？为什么选 SSE？

**参考回答：**

SSE 是 HTTP 单向推送——服务端可以持续推数据给客户端，但客户端只能通过普通的 HTTP 请求发消息。WebSocket 是全双工，双向都可以随便发。

我们的场景是：用户发一个问题，Agent 流式返回回答。这是典型的单向推送——服务端推内容、客户端只需要接收。SSE 足够，而且天然走 HTTP，不用额外协议升级，部署简单，Nginx 代理也不需要特殊配置。如果用 WebSocket 需要额外引入 Gorilla WebSocket 和心跳保活，本项目可维持简单 SSE 方案。

**追问：SSE 在你的代码里具体怎么用的？**

引擎层用 Gin 的 `c.Writer` 直接写 `text/event-stream` 响应头，然后每条 delta 用 `fmt.Fprintf` 写 `event: delta/data: {"content":"文字"}/空行`。前端用 `fetch + ReadableStream` 逐块接收，实时追加到聊天气泡。一个踩过的坑是：`done` 事件后用了 `reader.cancel()`，把 TCP 缓冲区里还没读完的 delta 全丢了——前端只显示前几十字就截断。改 `break` 自然退出就解决了。

---

## Q7: token 怎么实现无感刷新？

**参考回答：**

双 Token 机制。Access Token 是 JWT，30 分钟有效。Refresh Token 是随机 hex 字符串，24 小时有效，存 Redis。

前端 Axios 拦截器拦截 401 响应，判断如果不是重试请求，就用 Refresh Token 调 `/api/refresh` 换新 Token。换成功就把新 Token 写入 Pinia store，然后重发原请求。整个过程用户看不到登录页跳转，静默完成。其他并发请求在刷新中排队等待，刷新完成后再一起重试。如果 Refresh Token 也过期了（24 小时没登录），才会跳转登录页。

---

## Q8: 文件上传怎么处理？中文 PDF 遇到过什么问题？

**参考回答：**

支持 PDF、DOCX、PPTX、TXT、MD 五种格式。DOCX 用 `nguyenthenguyen/docx` 库从 XML 标签 `<w:t>` 提取文字。PPTX 用 Go 标准库 `archive/zip` 解压后从 slide XML 的 `<a:t>` 提取。TXT 和 MD 直接读。

PDF 坑最多。试了两个 Go 库——`ledongthuc/pdf` 和 `rsc.io/pdf`——遇到中文 PDF 输出全是乱码。原因是中国 PDF 常用 CMap 字体编码，Go 库不支持。最后切成系统命令 `pdftotext -layout -enc UTF-8`，Git Bash 自带，中文完美。

提取出纯文本后，按双换行分段、单换行加软换行，转成 Markdown 存入 `documents` 表，同时在 ByteMD 编辑器里展示。

---

## Q9: 数据库层面你是怎么防高并发的？

**参考回答：**

做了两层。第一层是 API 限流，Redis 滑动窗口计数器。每用户每分钟最多 30 次请求，每 IP 每分钟最多 100 次。Redis 记录每次请求的时间戳，每次新请求进来先清 60 秒前的旧记录，再数当前窗口内还剩多少次。超限返回 429。

第二层是资源并发控制。LLM API 调用全局最多同时 5 个，Embedding API 最多 3 个，文件解析最多 2 个。用 Go 的 buffered channel 做了个轻量 Semaphore——`Acquire()` 往里塞一个空 struct，`Release()` 取出来。塞不进去就阻塞等，实现自然排队。

GORM 连接池已经设了 `MaxOpenConns: 100, MaxIdleConns: 10`，MySQL 侧不用额外处理。

**追问：buffered channel 塞满怎么办？**

就是阻塞等待，不会有请求失败。调用方在 goroutine 里等，排到队就继续执行。对于文件解析这种不紧急的操作，阻塞排队完全可接受。

---

## Q10: 你提到了 Redis 降级，Redis 挂了搜索还能用吗？

**参考回答：**

能用。`RedisStackVectorStore.Search()` 里先尝试 `searchRedis()`。如果 Redis 连不上或者返回空结果，就记一条 Warn 日志，然后调 `searchInMemory()`——从 MySQL 加载对应资料的所有向量到 Go 内存，逐个算余弦相似度，排序取 Top-K 返回。比 Redis 慢一点（几百个 chunk 内存算 10ms 左右），但功能完全正常。Redis 恢复后自动回到 KNN 模式。

MySQL 里存了每个 chunk 的 embedding 字段作为备份数据，Redis 的索引是热数据缓存层。

---

## Q11: 你怎么处理多个用户同时发消息的并发问题？

**参考回答：**

每个请求是独立的 goroutine，Gin 框架自动处理。数据层有 MySQL 的事务隔离——用户发消息、Agent 返回消息，都是独立的 Session 范围，不存在跨用户数据竞争。

唯一的潜在冲突是：同一个用户用两个标签页同时发消息，可能创建两条 Session。这个是前端做了防重（发消息时按钮置灰），后端不做额外限制——因为创建多条 Session 也不会出错，用户以后想清理可以手动删。

---

## Q12: 说说你的项目里最大的一次技术难题？

**参考回答：**

DeepSeek v4-pro 推理模型的 `reasoning_content` 问题。这个模型每次回复都在 JSON 里带了一个 `reasoning_content` 字段，是它的思考过程。我把回复存到 messages 表时只存了 `content` 文本，忘了存 `reasoning_content`。

下次用户再发消息，loadContext 从 DB 加载历史拼回上下文发给 API——API 发现之前的助理消息缺少 `reasoning_content`，直接报 400。排查了很久才发现是存 DB 时漏了字段。

最后改了三层：Message 模型加 `ReasoningContent` 字段、引擎 Run 方法存 Assistant 消息时保存 `choice.Message.ReasoningContent`、loadContext 加载历史时从 DB 读出设回 `agentChatMsg.ReasoningContent`。内存中 Tool Calling 生成的合成助理消息也会保留原始 `ReasoningContent`，否则下一轮 API 会再次 400。这套设计还兼容了后续历史数据的回放，确保多轮对话不会中断。

另外一次是 SSE 流式输出的数据丢失问题。流式处理本身需要 Tool Calling 轮和最终回复轮分别调用非流式和流式模式，但`reader.cancel()` 会在 `done` 事件到达时立刻关闭 stream，TCP 缓冲区的延迟 delta 数据被丢弃——表现为前端只显示前几个字就截断。排查了两天才定位到一行代码。

---

### 补充实战题（对应简历各模块）

**Q: 简历提到"Redis 宕机自动降级内存余弦相似度计算"，具体怎么实现的？**

```go
func (vs *RedisStackVectorStore) Search(courseID uint, query string, topK int) ([]SearchResult, error) {
    vecs, _ := embedTexts([]string{query})
    vec := vecs[0]

    // 优先 Redis KNN
    if database.RDB != nil {
        results, err := vs.searchRedis(courseID, vec, topK)
        if err == nil { return results, nil }
        slog.Warn("Redis 搜索失败，降级到内存计算", "err", err)
    }

    // 降级：MySQL 加载向量 → 内存算余弦相似度
    return vs.searchInMemory(courseID, vec, topK)
}
```

`searchInMemory` 从 MySQL 加载该资料所有 chunk 的 embedding，逐个和问题向量算余弦相似度，过滤低于 0.5 的，排序取 Top-K。几百个 chunk 内存算 10ms 内完成。MySQL 的 embedding 字段是备份，Redis 是热缓存——挂了重建不需要再调 Embedding API。

**追问：为什么不直接用 MySQL 做向量搜索？**

MySQL 不适合——向量是 1024 维 float32 数组，SQL 没有原生向量运算符，用 LIKE 只能做关键词不能做语义。Redis Stack 的 HNSW 索引是专门为 KNN 搜索优化的数据结构，对数复杂度。MySQL 只做备份。

**Q: 简历提到"解决了 buffer 数据丢失导致的响应截断"，展开说说？**

SSE 流式场景：后端发 delta → 前端逐字渲染 → 收到 done 事件后原来调了 `reader.cancel()` 立即销毁 ReadableStream。但 `done` 可能比最后几个 delta 早到前端（TCP 乱序或调度延迟），`cancel()` 把缓冲里未读的 delta 全丢了——用户看到的是截断后的半句话。

排查时对比后端日志（完整回复已生成）和前端实际显示内容，发现长度差了一大截。改成收到 done 后只设 `streamEnded = true` 然后 `break` 自然退出循环，让浏览器自行处理缓冲数据，不再丢字了。

**Q: 简历提到"推理模型 reasoning_content 上下文回传"，这个怎么做的？**

DeepSeek v4-pro 每次回复带 `reasoning_content` 字段（模型的思考过程），API 要求下一次请求中必须将同条消息的 `reasoning_content` 原样传回。

改了三处：
1. `model/message.go`：Message 表新增 `ReasoningContent string` 字段
2. `agent_engine.go` Run()：存 assistant 消息时把 `choice.Message.ReasoningContent` 写入 DB
3. `agent_engine.go` loadContext()：从 DB 加载历史消息时，助理消息设置 `msg.ReasoningContent = m.ReasoningContent`

内存中 Tool Calling 循环新建的合成 assistant 消息也得带 `ReasoningContent`——如果漏了，下一轮 LLM 一样报 400。

**Q: 简历提到"VectorStore 接口三方法抽象，存储引擎一行代码切换"，展开讲讲？**

```go
type VectorStore interface {
    Search(courseID uint, query string, topK int) ([]SearchResult, error)
    Index(chunkID uint, courseID uint, content string) error
    Delete(courseID uint) error
}
```

初始化时决定用哪个实现：

```go
// 现用版本
vs := NewRedisStackVectorStore()

// 未来切 Qdrant
vs := NewQdrantVectorStore()

// 简易版（测试环境）
vs := NewSimpleSearchVectorStore()
```

业务代码只依赖接口，不管底层是 Redis 还是 Qdrant 还是 MySQL。从最简单的 MySQL LIKE 升级到 Redis Stack 向量搜索时只改了初始化一行。

**Q: 简历提到"buffered channel 控制全局并发数"，怎么用 channel 做限流？**

```go
type Semaphore struct { ch chan struct{} }

func NewSemaphore(capacity int) *Semaphore {
    return &Semaphore{ch: make(chan struct{}, capacity)}
}

func (s *Semaphore) Acquire() { s.ch <- struct{}{} }
func (s *Semaphore) Release() { <-s.ch }
```

全局声明 `var LLMSem = NewSemaphore(5)`，每个 LLM 调用前 `LLMSem.Acquire()`，完成后 `defer LLMSem.Release()`。当 5 个 goroutine 都占着时，第 6 个请求会被 channel 阻塞排队，等到前面的 Release 后才能继续。没有用到任何第三方库。

分别设了三个量：LLM 5 并发、Embedding 3 并发、文件解析 2 并发。这三个是独立的 channel，互不影响。

**Q: 简历提到"滑动窗口 API 限流"，Redis 怎么实现滑动窗口？**

每次请求记录当前时间戳到 ZSet：`ZADD ratelimit:user:123 <timestamp> <timestamp>`。然后清掉 60 秒前的旧数据：`ZREMRANGEBYSCORE ... 0 <now-60s>`。在 `ZCard` 查询剩余计数，如果 >= 限额就 429。

选择 ZSet 而不是 INCR 的原因是滑动窗口比固定窗口更精确——第 59 秒 30 次请求，第 61 秒就清零重新计数，不会被绕过。

---

## 项目相关八股文

### Go 基础

**Q: goroutine 和 channel 在你项目里怎么用的？**

goroutine 有两处。一是文档保存后异步触发 RAG 重切片：`go reindexDocument(&doc)`，不阻塞 HTTP 响应。二是 Semaphore 的并发控制，buffered channel 限制了 LLM 调用的全局并发数。

**Q: defer 的执行顺序？你项目里 defer 用来做什么？**

defer 是 LIFO 栈——后进先出。项目里主要用于：`defer LLMSem.Release()` 保证信号量一定释放，`defer resp.Body.Close()` 保证 HTTP Body 一定关闭，`defer tmp.Close()` 清理临时文件。defer 在 panic 时也会执行，所以资源不会泄露。

**Q: Go 的 nil interface 和 nil pointer 什么区别？**

nil pointer 是 `*T` 类型的零值，nil interface 是 interface 变量的零值。关键坑：一个 interface 变量如果底层 type 不为 nil 但 value 为 nil，那么它本身 != nil。项目里主要在 GORM `First()` 返回 `ErrRecordNotFound` 时做了区分——不能用 nil check，必须用 `errors.Is(err, gorm.ErrRecordNotFound)`。

**Q: Go 的 GC 对 web 服务性能有什么影响？你优化过吗？**

Go 1.19+ 的 GC STW 已经很低（<1ms），对大多数 web 服务不是瓶颈。项目里没做特殊优化，GIN_MODE=release 时日志开销比 GC 大多了。

---

### Gin / 中间件

**Q: Gin 中间件怎么实现的？和洋葱模型有什么区别？**

Gin 中间件是通过 `c.Next()` 实现的前后置执行。请求进来按注册顺序依次执行，`c.Next()` 后面的代码在 handler 执行完后才运行。这跟 Koa 的洋葱模型不一样——Koa 是 `await next()` 前后都有代码执行，Gin 是线性链 + `c.Next()` 回调。项目里的 Logger 中间件就用这个模式记录请求耗时。

**Q: `c.ShouldBindJSON` 和 `c.BindJSON` 有什么区别？**

`ShouldBindJSON` 校验失败只返回 error，让开发者自己处理。`BindJSON` 校验失败自动回写 400 给客户端。项目统一用了 ShouldBindJSON，因为需要自定义错误文案走 utils.BadRequest。

**Q: Gin 的 Context 是并发安全的吗？**

不是。`gin.Context` 不能跨 goroutine 传递或并发读写。项目里 Agent Engine 的 `sseHandler` 回调就是直接在请求 goroutine 里执行的——如果 Engine 内部开了 goroutine，需要通过 SSEHandler 的函数闭包安全访问 Writer。

---

### GORM

**Q: GORM 的零值更新问题你怎么处理的？**

GORM 用 struct 更新时会跳过零值字段，这是常见坑。比如 `price: 0` 会被忽略，必须用 `Select` 显式指定或 `map[string]interface{}` 手动构造更新。

**Q: GORM 的 Preload 什么时候用？有什么性能问题？**

Preload 是解决 N+1 查询的——查 100 条资料时，每条资料还要查 Category 和 User，不用 Preload 会跑 201 次 SQL。用 Preload 只跑 3 次。但多层嵌套 Preload 会产生 JOIN 爆炸，项目里最深只到 `.Preload("Category").Preload("User")`，没有再嵌套。

**Q: 你怎么处理慢查询？**

项目里开了 GORM 的 Logger，SQL 超过 200ms 会标红。关键的查询加了索引：`sessions(user_id, status)`、`messages(session_id, created_at)`、`materials(category_id, status, price)` 都是常用的查询组合索引。

---

### MySQL

**Q: MySQL 索引最左匹配原则是什么？举个例子。**

联合索引 `(a, b, c)` 相当于建了三个索引：`(a)`、`(a,b)`、`(a,b,c)`。查 `WHERE a=1 AND c=3` 只能用 a 不能用到 c，因为跳过了 b。项目里 `messages` 表索引是 `(session_id, created_at)`——查所有消息时用 session_id 过滤，排序用 created_at，符合最左匹配。

**Q: MySQL 事务隔离级别？项目里用哪个？**

MySQL 默认 RR（可重复读），通过 MVCC 解决幻读。项目没手动开事务——所有操作都是 `database.DB.Create/Update` 的单条 SQL，GORM 默认 autocommit。创建订单时如果涉及多个表的写入，会用 `database.DB.Transaction(func(tx *gorm.DB) error {...})` 保证原子性。

**Q: EXPLAIN 看过吗？常见字段什么意思？**

`type` 是最重要的——const > eq_ref > ref > range > index > ALL。ALL 是全表扫描，必须避免。`rows` 是预估扫描行数。`Extra` 里的 `Using index` 是覆盖索引（好），`Using filesort` 是额外排序（差）。项目里 Agent 查历史消息是 `WHERE session_id = ? ORDER BY created_at ASC LIMIT 20`，type 应该是 ref，rows < 20。

---

### Redis

**Q: Redis 在你项目里怎么用的？**

三种场景：验证码存储（5 分钟过期，带限频）、JWT Refresh Token 存储（24 小时过期）、API 限流计数器（滑动窗口）。还有一个向量存储场景——Redis Stack 的 HNSW 索引做向量搜索，相比 MySQL LIKE 搜索在性能和准确性上都有显著提升。

**Q: Redis 缓存穿透、击穿、雪崩分别是什么？怎么解决？**

穿透是查不存在的数据，每次穿透缓存查 DB。用布隆过滤器或者缓存空值（设短过期时间）。击穿是热点 key 过期瞬间大量请求涌向 DB。用互斥锁或"永不过期 + 异步刷新"。雪崩是大量 key 同时过期。给过期时间加随机偏移，或集群部署避免单点。项目里验证码不是热点 key，不存在这问题；Refresh Token 用主动失效机制，也不会同时过期。

**Q: 你用 Redis 做限流，滑动窗口和固定窗口有什么区别？**

固定窗口：第 59 秒突发 30 次合法，下一秒立即又能 30 次——实际 2 秒内 60 次，绕过了限流。滑动窗口：每个请求的时间戳都记录，清 60 秒前的旧记录后计当前窗口内剩余次数，无法绕过。项目用 ZSet 实现了滑动窗口——score 存时间戳，ZRemRangeByScore 清旧数据，ZCard 计数。

**Q: Redis 的 RDB 和 AOF 持久化方案？你怎么选的？**

RDB 是定时快照，AOF 是追加每条写操作。RDB 恢复快但会丢最后一次快照后的数据，AOF 数据更完整但恢复慢。项目里验证码这种短过期数据不需要持久化，挂了就用户重新获取。对于 JWT Refresh Token，Redis 只是辅助存储，挂了用 MySQL 中的最新 token 回放即可。向量索引的数据备份在 MySQL，Redis 的重建成本低，不需要特殊持久化策略。

---

### JWT / 认证

**Q: JWT 和 Session 的区别？为什么你用双 Token？**

JWT 是无状态的——服务端不存，靠签名验证。Session 是有状态的——服务端存 Session ID。JWT 的好处是水平扩展不需要共享 Session 存储，坏处是无法主动失效。双 Token 靠 Refresh Token（存在 Redis，可主动失效）弥补了这个问题——Access Token 短期（30min），Refresh Token 可控（24h，随时删 Redis key 踢下线）。

**Q: JWT 可以存放敏感信息吗？**

绝对不行。JWT 的 payload 是 base64 编码的，不是加密——任何人拿到 JWT 直接 base64 decode 就能看到内容。签名只防篡改不防窥探。项目的 JWT 只存了 user_id、username、role 三个非敏感字段，密码、Refresh Token 这些绝不放进去。

---

### SSE / 网络

**Q: SSE 的底层实现是什么？**

SSE 本质是 HTTP 长连接 + `Content-Type: text/event-stream`。服务端写完数据后不关连接，客户端用 EventSource API 或 `fetch + ReadableStream` 逐块接收。服务端通过 `Transfer-Encoding: chunked` 持续推送，每段数据前面有 `data:` 前缀，段之间用空行分隔。

**Q: HTTP/2 对 SSE 有什么影响？**

HTTP/2 多路复用允许单连接承载多个 SSE 流，不会像 HTTP/1.1 那样每个流占一个 TCP 连接。但浏览器对 HTTP/2 并发流有限制（Chrome 默认 100），大量 SSE 场景还是要注意。

**Q: 你的 SSE 实现中怎么处理连接中断？**

前端加了 30 秒超时保护——`Promise.race` 如果 read 操作 30 秒没完成就强制跳出。一完成消息接收（done 事件）就用 break 退出循环确保连接正常关闭。服务端问题主要是 `reader.cancel()` 在 done 到达时丢失 delta 数据——改为 break 自然退出后就稳定了。

