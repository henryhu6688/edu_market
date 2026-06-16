# 项目经历

## 在线学习平台 · 独立开发

**2025.03 — 至今 | Go + Vue3 + DeepSeek + Redis Stack**

在线学习资料交易平台，支持资料发布购买、Markdown 在线编辑、PDF/DOCX/PPTX 解析。核心亮点是自研 LLM Agent 引擎与 RAG 语义检索系统。

**Agent 引擎（Go 自研，~500 行）**

设计 Workflow（安全兜底）+ Agent（自主决策）双层架构。LLM 在 Workflow 约束内自主规划 9 个 Tool 的调用顺序。支持 Tool Calling 多轮迭代、死循环检测、SSE 流式对话。解决了 `stream:true` 与 `tools` 互斥、`reader.cancel()` 丢弃缓冲区数据、推理模型 `reasoning_content` 回传（Message 表字段 + 引擎存取 + loadContext 恢复）等问题。

**RAG 语义检索**

文档切片(500字/50重叠) → 硅基流动 Embedding(1024D) → Redis Stack HNSW 向量搜索 + MySQL 备份。VectorStore 接口三方法抽象，存储引擎一行代码切换。Redis 宕机自动降级内存余弦相似度计算

**文件处理流水线**

PDF/DOCX/PPTX/TXT/MD → 纯文本提取 → Markdown 编辑器。中文 PDF 采用 `pdftotext` 命令方案(Go 库 CMap 编码失败)。ByteMD WYSIWYG + 文档树 + 2s 防抖保存，异 goroutine 触发 RAG 重切片

**并发控制与工程实践**

API 限流（Redis 滑动窗口，30次/用户/min，100次/IP）、LLM/Embedding/Parser Semaphore 并发控制(buffered channel)。全链路 `request_id` 追踪，slog 结构化日志 + `AddSource`。Pre-commit Hook、敏感配置 `viper.New()` 独立实例读取

**技术栈：** Go · Gin · GORM · MySQL · Redis Stack · DeepSeek · SiliconFlow · Vue3 · Vite · ByteMD · SSE
