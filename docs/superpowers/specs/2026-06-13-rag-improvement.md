# RAG 改进：语义向量检索 + 混合搜索

> 日期: 2026-06-13
> 分支: v15_rag_improve

## 目标

从 MySQL LIKE 关键词匹配升级为 DeepSeek Embedding + Redis Stack 混合向量搜索。

## 方案

### Embedding

- 用 DeepSeek Embedding API（已有账号，零部署）
- 调用 `https://api.deepseek.com/v1/embeddings`
- 输入文本 → 输出 `[]float32`（默认 1024 维）
- 写入 `document_chunks.embedding` 字段（新增 BLOB 列）

### 向量存储

**推荐：Redis Stack (RediSearch)**

- 现有 Redis 加装 module（同一台机器）
- 支持 HNSW 向量索引 + TAG/ TEXT 过滤
- 支持混合搜索：`FT.SEARCH` 同时按向量相似度和关键词匹配
- 零额外服务

### 检索流程

```
用户问题
  │
  ▼
调 DeepSeek Embedding API → 问题向量
  │
  ▼
Redis Stack FT.SEARCH：
  - KNN 向量搜索 (Top-K 最近邻)
  - 混合 TEXT 关键词搜索
  - 过滤条件：material_id、is_free_preview
  │
  ▼
返回 Top-K chunks（内客 + 分数）
  │
  ▼
去重 + 按分数降序
  │
  ▼
拼入 LLM 上下文
```

### 索引策略

```sql
FT.CREATE idx:chunks ON JSON PREFIX 1 doc: SCHEMA
  $.content AS content TEXT
  $.material_id AS material_id NUMERIC SORTABLE
  $.embedding AS embedding VECTOR HNSW 6 TYPE FLOAT32 DIM 1024 DISTANCE_METRIC COSINE
```

### 改动清单

| 文件 | 改动 |
|------|------|
| `model/document_chunk.go` | 新增 `Embedding []float32` 字段 |
| `service/agent_rag.go` | 新增 `RedisStackVectorStore` 实现 `VectorStore` 接口 |
| `service/agent_rag.go` | 新增 `embedText()` 调 DeepSeek Embedding API |
| `service/agent_rag.go` | `Index()` 方法里加 embedding 生成逻辑 |
| `service/agent_rag.go` | `InitRAG()` 默认切换到 `RedisStackVectorStore` |
| `config/app.example.yml` | 已有 `embedding_model` 和 `embedding_api_url` 配置 |

### 不变

- `VectorStore` 接口不变
- `RAGService` 不变
- `search_documents` tool 不变
- `SimpleSearchVectorStore` 保留作为 Redis Stack 不可用时的 fallback
