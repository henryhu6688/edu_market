# RAG 改进：DeepSeek Embedding + Redis Stack 向量检索

> 日期: 2026-06-13
> 分支: v15_rag_improve

## 目标

从 MySQL LIKE 关键词匹配升级为 DeepSeek Embedding 语义向量检索。

## 存量资产

| 组件 | 位置 | 说明 |
|------|------|------|
| 文档原文 | `documents.content` | Markdown 全文 |
| 切片 | `document_chunks` | `chunkText()` 500字/50重叠 |
| 搜索（旧） | `SimpleSearchVectorStore` | MySQL `LIKE '%关键词%'` |
| 接口 | `VectorStore` | `Search/Index/Delete` 已预留 |

## 方案

### 架构

MySQL 做主存储 + Redis Stack 做向量搜索引擎。

```
入库：
  文档保存
    → extractTextFromMarkdown()         纯文本
    → chunkText()                       切片 500字
    → 逐块 INSERT document_chunks (MySQL) ← 主数据，永久存储
    → embedText(块内容)                 调 DeepSeek Embedding
    → Redis HSET doc:N embedding content material_id ← 向量索引
    → MySQL UPDATE embedding              ← 备份向量

搜索：
  用户问题
    → embedText(问题)                   向量化
    → Redis FT.SEARCH KNN               向量搜索 (Top-5)
    → Redis 挂了？→ MySQL 加载向量 → Go 内存余弦相似度
    → 返回 chunks → 拼入 LLM 上下文
```

### 为什么 MySQL + Redis 双写

| | MySQL | Redis Stack |
|---|---|---|
| 存什么 | content 原文 + embedding 备份 | embedding 向量索引 |
| 用途 | 数据持久化、备份恢复 | 高性能向量搜索 |
| 规模 | 无限（磁盘） | 热数据（内存） |

### Redis 宕机恢复

1. Agent 调 `search_documents` → Redis 连接失败
2. 降级：从 MySQL 加载 embedding → Go 内存余弦相似度
3. Redis 恢复后：遍历 `document_chunks.embedding` → `HSET` 批量重建 → `FT.CREATE` 重建索引

## 数据模型

`document_chunks` 新增 `embedding` 字段：

```go
type DocumentChunk struct {
	ID         uint      `gorm:"primaryKey;autoIncrement"`
	CourseID   uint      `gorm:"not null;index"`
	Content    string    `gorm:"type:text;not null"`
	Embedding  []float32 `gorm:"type:blob"`           // 新增：1024 维向量
	ChunkIndex int       `gorm:"not null;default:0"`
	CreatedAt  time.Time `gorm:"autoCreateTime"`
}
```

## Embedding

- 服务：DeepSeek Embedding API（已有账号）
- 输入：文本字符串
- 输出：`[]float32`（1024 维）
- 维度：1024
- 存储：约 4KB/chunk

## Redis 索引

```sql
FT.CREATE idx:chunks ON HASH PREFIX 1 doc: SCHEMA
  content AS content TEXT
  material_id AS course_id NUMERIC SORTABLE
  embedding AS embedding VECTOR HNSW 6 TYPE FLOAT32 DIM 1024 DISTANCE_METRIC COSINE
```

## 改动清单

| 文件 | 改动 |
|------|------|
| `model/document_chunk.go` | 新增 `Embedding []float32` |
| `service/agent_rag.go` | 新增 `embedText()` + `cosineSimilarity()` |
| `service/agent_rag.go` | 实现 `RedisStackVectorStore`（Redis KNN + MySQL fallback） |
| `service/agent_rag.go` | `InitRAG()` 切换为新实现 |
| `config/app.example.yml` | 新增 `embedding_model` / `embedding_api_url` |
| `service/setup_test.go` | 同步配置 |

## 不变

- `VectorStore` 接口
- `RAGService`
- `search_documents` tool
- `reindexDocument()` 调用链
- 前端
