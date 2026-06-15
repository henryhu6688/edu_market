# 项目经历

## EduMarket — 在线学习资料售卖与 AI 智能答疑平台

**2025.03 — 2025.06 | 独立开发**

在线学习资料交易平台，集成 LLM Agent 实现智能客服、资料推荐、深度答疑。支持 Markdown 在线编辑、多格式文件解析（PDF/DOCX/PPTX）。

**技术栈：** Go + Gin + GORM + MySQL + Redis + DeepSeek API + Vue3 + Vite + ByteMD

**核心贡献：**

- **自研 LLM Agent 引擎**：实现 Workflow 骨架 + Agent 自主决策的双层架构。LLM 在 Workflow 定义的安全边界内自主规划 9 个 Tool 的调用顺序，支持多轮 Tool Calling 串联（购买流程 5 步、售后流程 4 步、咨询流程 4 步）。引擎支持 SSE 流式对话（DeepSeek 原生 SSE 转发，首字延迟 < 200ms）、action 事件触发前端渲染购买卡片、上下文窗口截断、Tool 重试防死循环。

- **RAG 语义检索系统**：从零实现 Embedding + 向量搜索流水线。调用 DeepSeek Embedding API 对文档切片（500 字/50 重叠）生成 1024 维向量，Redis Stack (RediSearch HNSW) 做高性能 KNN 向量搜索，MySQL 做向量备份。设计 VectorStore 接口抽象，预留 Pinecone/Qdrant 切换能力。Redis 宕机自动降级到 Go 内存余弦相似度计算。

- **在线文档编辑器**：基于 ByteMD 实现所见即所得 Markdown 编辑，左侧文档树（parent_id 层级），2s 防抖自动保存。支持 PDF/DOCX/PPTX/TXT/MD 上传自动转 Markdown。中文 PDF 采用系统 pdftotext 命令解决 Go 库 CMap 字体编码乱码问题。

- **JWT 双 Token 鉴权**：access_token (30min HS256) + refresh_token (24h 随机 hex)，前端 Axios 拦截器自动静默刷新。Redis 存储图形验证码 + 短信验证码，支持限频和一次性消费。

**项目亮点：**

- Agent + RAG 组合实现了"买前概括答疑 → 买后深度讲解"的完整用户旅程
- VectorStore 接口抽象使向量存储从 SimpleSearch(MySQL LIKE) 升级到 Redis Stack 仅改一行代码
- 全链路 request_id 日志追踪（middleware → controller → service → engine），slog 结构化日志
- pre-commit hook 拦截 master 直接提交，敏感配置通过环境变量 + viper 读取
