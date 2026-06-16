# 项目经历

## 在线学习平台 · 独立开发

**2025.03 — 至今 | Go + Vue3 + DeepSeek + Redis Stack**

在线学习资料交易平台，支持资料发布购买、Markdown 在线编辑、PDF/DOCX/PPTX 解析。核心亮点是自研 LLM Agent 引擎与 RAG 语义检索系统。

**Agent 引擎（Go 自研，~500 行）**

双层架构：Workflow 层定义安全边界（轮数上限、防死循环），Agent 层由 LLM 自主规划 9 个 Tool 的调用顺序与策略。支持 Tool Calling 多轮迭代与 SSE 流式对话。解决了 `stream:true` 与 `tools` 参数互斥、buffer 数据丢失导致的响应截断、推理模型 `reasoning_content` 上下文回传等关键问题。

**RAG 语义检索**

文档切片(500字/50重叠) → 硅基流动 Embedding(1024D) → Redis Stack HNSW 向量搜索 + MySQL 备份。VectorStore 接口三方法抽象，存储引擎一行代码切换。Redis 宕机自动降级内存余弦相似度计算

**文件处理流水线**

PDF/DOCX/PPTX/TXT/MD → 纯文本提取 → Markdown 编辑器。中文 PDF 对比多个 Go 库后采用系统 `pdftotext` 命令方案解决乱码问题。ByteMD WYSIWYG + 文档树 + 2s 防抖自动保存，保存后异步触发 RAG 重索引

**并发控制与工程实践**

Redis 滑动窗口实现 API 限流，防止单用户高频调用。令牌桶控制 LLM 和 Embedding API 的调用频率，避免触发第三方 429，不阻塞其他用户。全链路 `request_id` 追踪，结构化日志聚合

**技术栈：** Go · Gin · GORM · MySQL · Redis Stack · DeepSeek · SiliconFlow · Vue3 · Vite · ByteMD · SSE
