# 项目经历

## 在线学习平台 · 独立开发

**2025.03 — 至今 | Go + Vue3 + DeepSeek + Redis**

在线学习资料交易平台。最大的技术投入是自研了一套 Agent 引擎，替代 LangChain 这类现成框架，驱动平台的智能客服、资料推荐和 RAG 答疑。

**Agent 引擎（自研）**

没有用 LangChain，纯 Go 写了 ~500 行。核心是一个 Tool Calling 循环：LLM 自己决定调哪个 Tool、什么顺序、失败后换什么策略。最多 10 轮迭代，设了死循环检测（同一 Tool 连续 3 次自动打住）。SSE 流式对话踩了不少坑——`stream:true` 和 `tools` 参数互斥，最后拆成 Tool Calling 轮非流式 + 最终回复轮流式。推理模型要求 `reasoning_content` 必须在每次请求中回传，改了三层：Message 表加字段、引擎存的时候带上、loadContext 加载时恢复。

**RAG 检索**

文档上传后自动切成 500 字小块，调硅基流动的 Embedding API 生成 1024 维向量，双写 MySQL 和 Redis Stack。搜索优先走 Redis 的 HNSW KNN，Redis 挂了自动切到 Go 内存算余弦相似度。VectorStore 只定义了 Search/Index/Delete 三个方法，后面想切 Qdrant 改一行初始化就行。

**文件处理**

PDF/DOCX/PPTX/TXT/MD 上传自动转 Markdown。中文 PDF 试了两个 Go 库都是乱码，最后直接调系统 `pdftotext` 解决。编辑器用的 ByteMD，左侧文档树 + 右侧所见即所得，2 秒自动保存，保存后起一个 goroutine 触发 RAG 重新切片。

**并发控制**

Redis 滑动窗口做了 API 限流，buffered channel 封了个 Semaphore 控制 LLM（5 并发）、Embedding（3 并发）、文件解析（2 并发）的全局并发量。全链路 `request_id` 追日志，slog 开了 `AddSource` 显示行号。敏感配置用 viper 独立实例读，pre-commit hook 拦着不让直接在 master 提交。

**技术栈**

Go · Gin · GORM · MySQL · Redis Stack · DeepSeek · SiliconFlow · Vue3 · Vite · ByteMD · SSE
