# 项目经历

## EduMarket — 在线学习资料平台 + 自研 LLM Agent

**2025.03 — 2025.06 | 独立开发 | Go + Vue3 + DeepSeek + Redis Stack**

### 项目概述

在线学习资料交易平台，核心差异化在于**自研 LLM Agent 引擎**——不依赖 LangChain 等框架，纯 Go 实现 Tool Calling 循环，驱动智能客服、个性化推荐、RAG 深度答疑三大场景。

### 技术亮点

**1. LLM Agent 引擎（核心能力）**

- Go 自研，约 500 行核心代码实现完整 Tool Calling 循环。LLM 自主规划 9 个 Tool 的调用顺序（资料搜索、订单查询、FAQ 检索、RAG 文档搜索、购买卡片触发），最大 10 轮迭代
- 设计 Workflow + Agent 双层架构：Workflow 层做安全兜底（轮数限制、Tool 白名单、防死循环），Agent 层做自主决策（选 Tool、排序、错误重试）
- SSE 流式对话：Native 转发 DeepSeek SSE 流（非模拟逐字），首字延迟 < 200ms。踩过 `reader.cancel()` 丢弃缓冲数据、`stream:true` 与 `tools` 参数互斥等流式坑
- 上下文管理：取最近 20 轮对话拼入 System Prompt，兼容 deepseek-v4-pro 的 `reasoning_content` 必须在每次请求中回传的限制（模型字段 + DB 存储 + loadContext 恢复三层适配）

**2. RAG 语义检索系统**

- Embedding：接入硅基流动 BAAI/bge-large-zh-v1.5 模型（1024 维），批量 `embedTexts()` 支持 3 次指数退避重试。
- 向量存储：Redis Stack (RediSearch HNSW) 做 KNN 向量搜索，MySQL BLOB 做全量备份。设计 `VectorStore` 接口抽象（`Search/Index/Delete`），Swap 一行代码从 SimpleSearch(MySQL LIKE) 升级到向量检索或 Pinecone/Qdrant
- 降级方案：Redis 宕机自动 Fallback 到 Go 内存余弦相似度计算（`cosineSimilarity`），功能不挂
- 文档处理流水线：PDF(`pdftotext`)/DOCX/PPTX/MD → 纯文本提取 → `chunkText()` 切片(500字/50重叠) → Embedding → 双写 MySQL + Redis

**3. 在线文档编辑器**

- ByteMD WYSIWYG Markdown 编辑 + 左侧文档树（`parent_id` 层级），2s 防抖自动保存，保存后异 goroutine 触发 RAG 重新切片
- 中文 PDF 解析方案：Go 库 `ledongthuc/pdf`/`rsc.io/pdf` 对中文 CMap 字体编码支持差 → 改用系统 `pdftotext -layout -enc UTF-8`，完美支持 CJK

**4. 工程实践**

- JWT 双 Token 鉴权（Access 30min HS256 + Refresh 24h 随机 Hex），Axios 拦截器静默刷新
- 全链路 `request_id` 追踪（middleware → controller → service → engine），`slog` 结构化日志 + `AddSource` 显示行号
- Pre-commit Hook 拦截 master 直接提交，敏感配置通过环境变量 + `viper.New()` 独立实例读取，`app.example.yml` 做模板

### 技术栈

Go · Gin · GORM · MySQL · Redis Stack · DeepSeek API · SiliconFlow Embedding · Vue3 · Vite · ByteMD · slog · Viper
