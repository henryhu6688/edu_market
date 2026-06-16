# 项目经历

## 在线学习平台 — 自研 Agent + RAG + 并发控制

**2025.03 — 至今 | 独立开发 | Go + Vue3 + DeepSeek + Redis Stack**

### 项目简介
在线学习资料交易平台，集成 **Go 自研 LLM Agent** 实现智能客服、个性化推荐、深度答疑。支持 Markdown 在线编辑器、多格式文件解析（PDF/DOCX/PPTX）。

### 重点成果

**LLM Agent 引擎**
- Go 自研 ~500 行实现完整 Tool Calling 循环，9 个 Tool，最大 10 轮迭代，不依赖 LangChain
- 设计 Workflow（安全兜底）+ Agent（自主决策）双层架构。死循环检测：同一 Tool 连续 3 次自动终止
- SSE 流式对话，`stream:true` 与 `tools` 互斥 → Tool Calling 轮非流式，最终回复轮流式
- 上下文管理：近 20 轮拼入上下文。深度适配推理模型的 `reasoning_content` 保存回传

**RAG 语义检索**
- 文档存储 → 切片(500字/50重叠) → 硅基流动 BAAI/bge 1024D Embedding → Redis Stack HNSW KNN 向量搜索
- `VectorStore` 接口抽象（Search/Index/Delete），切换存储引擎仅改一行代码
- Redis 宕机自动降级 Go 内存 `cosineSimilarity` 计算，功能不挂

**文件处理流水线**
- PDF(`pdftotext` CJK 完美)/DOCX(`<w:t>` XML 提取)/PPTX(archive/zip `<a:t>` 提取)/TXT/MD 上传自动转 Markdown
- ByteMD WYSIWYG 编辑器 + 文档树 + 2s 自动保存，保存后异步 goroutine 触发 RAG 重新切片

**并发控制与安全**
- Redis 滑动窗口 API 限流：30 次/用户/min，100 次/IP，buffered channel 实现 LLM/Embedding/Parser Semaphore 并发控制
- Pre-commit Hook 拦截 master 直接提交，敏感字段 `viper.New()` 独立实例读取
- 全链路 `request_id` 追踪，`slog` 结构化日志 + `AddSource` 行号显示

### 技术栈
Go · Gin · GORM · MySQL · Redis Stack · DeepSeek API · SiliconFlow Embedding · Vue3 · Vite · ByteMD · SSE · Semaphore · slog
