# 项目经历

## EduMarket — 在线学习平台 + 自研 LLM Agent

**2025.03 — 2025.06 | 独立开发 | Go + Vue3 + DeepSeek + Redis Stack**

在线学习资料交易平台，核心差异：**不依赖 LangChain，纯 Go 自研 Agent 引擎**，驱动智能客服、资料推荐、RAG 答疑。

**重点成果：**

- **LLM Agent 引擎**：Go 自研 ~500 行，实现完整 Tool Calling 循环（9 个 Tool，最大 10 轮）。设计 Workflow（安全兜底）+ Agent（自主决策）双层架构。SSE 流式对话，首字延迟 < 200ms
- **RAG 语义检索**：SiliconFlow Embedding (BAAI/bge, 1024D) → Redis Stack HNSW 向量搜索 → MySQL 备份。设计 `VectorStore` 接口抽象，切换向量存储只需一行代码。Redis 宕机自动降级内存余弦相似度
- **文件 → 在线文档**：PDF/DOCX/PPTX/MD 上传自动转 Markdown，ByteMD 所见即所得编辑器 + 文档树 + 2s 自动保存，保存后异 goroutine 触发 RAG 重新切片

**技术栈：** Go · Gin · GORM · MySQL · Redis Stack · DeepSeek API · SiliconFlow Embedding · Vue3 · Vite · ByteMD
