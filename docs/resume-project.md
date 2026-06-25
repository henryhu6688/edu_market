# 项目经历

## EduMarket — 在线学习平台 · 个人开发

**2026.03 — 至今 | Go + Gin + GORM + MySQL + Redis Stack + Vue3 + DeepSeek**

LLM Agent 驱动的学习资料交易平台，买方通过 AI 对话了解资料再决策，卖方无需逐一回复咨询。

**技术栈**：Go · Gin · GORM · MySQL · Redis Stack · DeepSeek · SSE · Vue3 · ByteMD

### 主要职责

**自研 Agent 引擎（~500 行 Go，未用 LangChain）**

平台需要能查库+引导购买的智能客服，直接调 API 无法实现，引入 LangChain 又过重。自研双层架构：Workflow 层约束安全边界（10 轮上限、防死循环），Agent 层由 LLM 自主规划 9 个 Tool 的调用策略，支持购买 5 步、售后 4 步、咨询 4 步多轮串联。转发 DeepSeek 原生 SSE，首字延迟 < 200ms。解决了 `reasoning_content` 回传缺失致 API 400、SSE buffer 截断、Tool 空结果死循环三个生产问题。

**RAG 语义检索**

文档切片（500 字/50 重叠）→ Embedding（1024D）→ Redis Stack HNSW 搜索。`VectorStore` 接口三方法抽象，一行代码切换存储引擎。MySQL 全量备份向量，Redis 宕机自动降级内存余弦相似度计算（几百 chunk < 10ms）。买前限目录级检索，买后开放全文。

**文档处理流水线**

支持 PDF/DOCX/PPTX 上传解析。中文 PDF 试遍 Go 库均因 CMap 字体乱码，最终切系统 `pdftotext` 解决。ByteMD 所见即所得编辑 + 文档树，2s 防抖自动保存，异步触发 RAG 重索引。

**工程实践**

Redis ZSet 滑动窗口限流（30 次/分钟/用户），令牌桶控 LLM 调用频率（10 次/s）。JWT 双 Token 鉴权，拦截器静默刷新。全链路 `request_id` 日志追踪，慢查询 > 200ms 告警。
