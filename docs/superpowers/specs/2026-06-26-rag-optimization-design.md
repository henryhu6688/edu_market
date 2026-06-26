# RAG 系统优化设计

## 一、设计概述

### 目标
对 edu_market RAG 检索系统做全链路优化，覆盖离线入库、在线检索、生成约束三个环节。

### 架构变更
向量数据库从 Redis Stack 切换为 Qdrant，利用其内置混合检索（RRF）、Payload 元数据存储、自动索引管理等能力。

### 配置开关（方案 C）
每个模块独立开关，支持 AB 测试和线上降级：

```yaml
# config/app.yml
rag:
  qdrant_url: "http://localhost:6333"
  qdrant_collection: "chunks"
  hybrid_search: true          # 混合检索（向量+关键词 RRF）
  rerank: true                 # Rerank 精排
  rerank_topk: 3               # Rerank 后保留条数
  cleaner_enabled: true        # 文档清洗
  structural_chunk: true       # 结构切片（按标题）
  chunk_size: 500              # 目标 chunk 大小（字）
  chunk_min: 300               # 最小 chunk（字），低于合并
  chunk_max: 800              # 最大 chunk（字），超过内部再切
  cache_enabled: true           # 查询结果缓存
  cache_ttl: 3600               # 精确缓存 TTL（秒）
  cache_sim_threshold: 0.85     # 语义缓存相似度阈值
  cache_max_entries: 10         # 每个资料最多缓存条数
```

### Embedding 模型
从 `BAAI/bge-large-zh-v1.5` 切换为 `BAAI/bge-m3`（中英文多语言统一，1024维）。

---

## 二、在线检索链路

### 完整流程

```
用户问题 → Agent 决定调 search_documents(query, material_id)
  │
  ├─ ① 缓存检查 [开关: cache_enabled]
  │   L1 精确: redis.GET("rag:exact:<md5(query+materialID)>") → 命中直接返回
  │   L2 语义: Embedding(带缓存) → 和历史向量余弦 ≥0.85 → 复用历史结果
  │
  ├─ ② Qdrant 混合检索
  │   - 向量检索: bge-m3 embedding
  │   - 关键词检索: 稀疏向量（Qdrant 内置）
  │   - 元数据过滤: @course_id = material_id
  │   - 融合: RRF (Reciprocal Rank Fusion)
  │   - 召回: top-K = 10
  │
  ├─ ③ Rerank 精排 [开关: rerank]
  │   - 模型: Pro/BAAI/bge-reranker-v2-m3
  │   - 流程: 10 条 → 相关性打分 → 按分重排 → 保留 top-3
  │   - API: SiliconFlow Rerank API
  │
  ├─ ④ 元数据拼装
  │   - 收集所有 DocumentID → 批量 WHERE id IN 查标题
  │   - 格式化上下文:
  │     [来源：《%s》> %s]（可信度：%s）
  │     %s
  │   - 可信度: ≥0.7→高 | ≥0.4→中 | <0.4→低
  │
  ├─ ⑤ 写入缓存
  │   L1: SetEx("rag:exact:...", results, 1h)
  │   L2: appendRecentQuery(materialID, query, vec, results)
  │
  └─ ⑥ 返回 Agent → LLM 生成
```

### 模块 1：Qdrant 向量存储

新增 `service/rag/qdrant_store.go`，实现 `VectorStore` 接口。

```go
type QdrantVectorStore struct {
    client     *http.Client
    baseURL    string
    collection string
}

// Search 混合检索：向量 + 关键词 RRF 融合
func (q *QdrantVectorStore) Search(courseID uint, query string, topK int) ([]SearchResult, error) {
    // 1. 向量化
    vec := embedCached(query)

    // 2. Qdrant Search API
    req := map[string]interface{}{
        "vector":       vec,
        "limit":        topK,
        "with_payload": true,
        "filter": map[string]interface{}{
            "must": []map[string]interface{}{
                {"key": "course_id", "match": map[string]interface{}{"value": courseID}},
            },
        },
    }
    // 混合检索：Qdrant payload text 索引（multilingual tokenizer 支持中英文分词）
    if config.App.RAG.HybridSearch {
        req["filter"].(map[string]interface{})["must"] = append(
            req["filter"].(map[string]interface{})["must"].([]map[string]interface{}),
            map[string]interface{}{
                "key": "content", "match": map[string]interface{}{"text": query},
            },
        )
    }

    // 3. 解析返回：SearchResult{ChunkID, Content, Score, DocumentID, SectionPath}
}

// Index 写入向量 + payload
func (q *QdrantVectorStore) Index(chunkID, courseID uint, content string) error {
    // Qdrant Upsert: vector + payload{course_id, document_id, section_path, content}
}

// Delete 删除某资料的全部向量（按 course_id 过滤删除）
func (q *QdrantVectorStore) Delete(courseID uint) error {}
```

配置：

```yaml
rag:
  qdrant_url: "http://localhost:6333"
  qdrant_collection: "chunks"
```

部署：`docker run -p 6333:6333 qdrant/qdrant`（开源 Apache 2.0，免费）。

Collection 创建时建 payload text 索引用于混合检索：

```
PUT /collections/chunks/index
{ "field_name": "content", "field_type": "text", "tokenizer": "multilingual" }
```

`multilingual` tokenizer 自动处理中英文分词——中文走内部分词器，英文走空格分词，混合文本同时生效。

### 模块 2：混合检索（Rerank 精排）

新增 `service/rag/rerank.go`。

```go
const rerankModel = "Pro/BAAI/bge-reranker-v2-m3"

type Reranker struct {
    apiURL string
    apiKey string
}

// Rerank 对召回的 chunks 精排，保留 topK 条
func (r *Reranker) Rerank(query string, chunks []SearchResult, topK int) ([]SearchResult, error) {
    // 1. 提取 chunk 文本列表
    // 2. 调 SiliconFlow Rerank API
    // 3. 按 relevance_score 降序排列
    // 4. 返回前 topK 条
}
```

Pipeline 位置：

```
Search()
  ├─ qdrantStore.Search(courseID, query, 10)  → 10 条
  ├─ if config.App.RAG.Rerank:
  │     reranker.Rerank(query, chunks, config.App.RAG.RerankTopK) → 3 条
  └─ 返回结果
```

延迟：Embedding ~180ms + Qdrant ~40ms + Rerank ~100ms = 总计 ~320ms。

### 模块 3：查询结果缓存

两级缓存，用 Redis（已有）。

**L1 精确匹配：**

```
key:   "rag:exact:<md5(query + material_id)>"
value: JSON(SearchResult[])
TTL:   1h
```

命中条件：同一用户或不同用户，对同一份资料问完全一样的问题。

**L2 语义匹配：**

```
key:   "rag:sem:<material_id>"
value: JSON[CachedQuery×10]  — 最近 10 条 {query, vector, results}
```

命中条件：新 query 的 Embedding 向量和历史 query 向量的余弦相似度 ≥ 0.85。

**完整缓存路径：**

```
query + material_id

  ├─ L1 精确匹配
  │   redis.GET("rag:exact:<md5>") → 命中? 直接返回（0ms，省全程）
  │
  ├─ Embedding（带缓存）
  │   embedCached(query):
  │     key = "emb:<md5(text)>", TTL 24h
  │     命中 → 直接返回向量
  │     未命中 → 调 API → 写入缓存
  │
  ├─ L2 语义匹配
  │   取 redis.GET("rag:sem:<material_id>") → 提取 10 条历史向量
  │   余弦相似度(V_new, V_history) ≥ 0.85? → 复用历史结果（省 Qdrant + Rerank）
  │
  ├─ 都没中 → 完整管线（Qdrant + Rerank）
  │
  └─ 写入缓存
       L1: redis.SetEx("rag:exact:...", results, 1h)
       L2: appendRecentQuery(materialID, {query, V_new, results})
           → 保留最近 10 条 → 写回 Redis
```

**为什么语义匹配不需要额外的 API：**

bge-m3 产出的向量共处同一语义空间。`"闭包啥意思"` 和 `"闭包怎么理解"` 语义相近，向量天然离得近，余弦相似度直接算。不需要额外模型。

```
"闭包啥意思"    → bge-m3 → [0.12, 0.34, ...]
"闭包怎么理解"  → bge-m3 → [0.13, 0.33, ...]
   余弦 = 0.92 ≥ 0.85 ✅

"闭包啥意思"    → bge-m3 → [0.12, 0.34, ...]
"订单退款"      → bge-m3 → [-0.05, 0.78, ...]
   余弦 = 0.15 < 0.85 ❌
```

**缓存维护：**

- 日常追加：每次完整管线跑完 → push 到列表头部 → 保留 10 条 → 写回
- 清空触发：`IndexCourse(materialID)` 重建索引时 → `redis.Del("rag:sem:<material_id>")` — chunk 全变了，旧缓存作废
- L1 精确缓存靠 TTL 自然过期，不主动维护

**内存估算：**

每条缓存~5KB（向量 4KB + 结果 ~1KB），10条/资料 × 100 份资料 = 5MB。可忽略不计。

### 模块 4：元数据

`model/document_chunk.go` 新增 2 个字段：

```go
type DocumentChunk struct {
    // 现有字段不变...
    DocumentID  uint   `gorm:"index" json:"document_id"`
    SectionPath string `gorm:"type:varchar(500)" json:"section_path"`
}
```

`SearchResult` 同步新增：

```go
type SearchResult struct {
    ChunkID       uint    `json:"chunk_id"`
    Content       string  `json:"content"`
    Score         float32 `json:"score"`
    DocumentID    uint    `json:"document_id"`
    SectionPath   string  `json:"section_path"`
}
```

写入：`IndexCourse()` 切片时填充 DocumentID + SectionPath。
读取：检索后 `WHERE id IN (docIDs)` 批量查 titles 拼装来源引用。

**不冗余存 DocumentTitle** — 作者改名无残留。

---

## 三、离线入库链路

### 完整流程

```
用户上传文档
  │
  ├─ ① 格式路由
  │   .md/.txt → 直接读
  │   .docx    → docx 库解析
  │   .pdf     → pdftotext → 可读性检查
  │   .pptx    → 拒绝（不支持）
  │
  ├─ ② 解析层清洗（格式专用）
  │   cleanPDF():  页眉页脚 / 页码 / 硬换行合并 / 水印 / 目录残留
  │   cleanDOCX(): 表格线残留 / 页眉页脚
  │   cleanTXT():  控制字符
  │
  ├─ ③ OCR 降级（仅 PDF）
  │   ┌─ pdftotext 可读 → 直接进入清洗
  │   └─ 不可读 → tesseract OCR → 二次可读检查
  │        ├─ 可读 → 清洗
  │        └─ 不可读 → 拒绝入库："PDF 质量过低，建议上传文字版"
  │
  ├─ ④ 转 Markdown 存入 documents.content
  │
  ├─ ⑤ Markdown 层清洗 [开关: cleaner_enabled]
  │   去图片占位符 / 去链接URL留文本 / 去加粗标记 / 去删除线
  │   / 去控制字符 / 全角→半角 / 空行合并 / 格式符号
  │
  ├─ ⑥ 结构切片 [开关: structural_chunk]
  │   ┌─ MD:   按 # 标题层级 → SectionPath
  │   ├─ DOCX: 按 pStyle 标题样式 → SectionPath
  │   └─ PDF/TXT: 段落+大小阈值 → SectionPath 留空
  │
  ├─ ⑦ batch Embedding (bge-m3, 并发3 + 令牌桶8/s)
  │
  └─ ⑧ 双写
      Qdrant: vector + payload{course_id, document_id, section_path, content}
      MySQL:  document_chunks（备份 + 降级）
```

### 模块 5：文档加载与解析层清洗

改 `service/document_parser.go` + 新增 `service/rag/cleaner.go`。

**4.1 解析层清洗（document_parser.go，每个解析函数内部调用）**

**cleanPDF()：**
- 页眉页脚：同一行出现 ≥ 3 次 且长度 < 50 字 → 全文删除
- 页码：纯数字或短横包数字，长度 ≤ 5 字 → 删除
- 硬换行合并：上行以中文（非句号）或小写字母结尾 → 合并到上一行
- 水印：同一短文本（< 20 字）出现 ≥ 5 次 → 全文删除
- 目录残留：含连续 5 个以上 `.` → 整行删除

**cleanDOCX()：**
- 表格线残留：`│├─┼┤┬┴└┘┌┐╭╮╰╯` 等制表符 → 全部删除
- 页眉页脚：同 PDF 逻辑，复用 `removeRepeatingLines()`

**共用的辅助函数：**
```go
func removeRepeatingLines(text string) string  // 出现 ≥3 次同文行 → 删
func removePageNumbers(text string) string      // 纯数字短行 → 删
func deduplicateParagraphs(text string) string  // 重复段落去重
```

**4.2 OCR 降级（仅 PDF）：**

```go
func parsePDF(filePath string) (string, error) {
    text := pdftotext(filePath)
    if isReadable(text) {
        return cleanPDF(text), nil    // 合格 → 清洗
    }
    ocrText := tesseractOCR(filePath)  // 不合格 → OCR
    if !isReadable(ocrText) {
        return "", ErrUnreadable        // OCR 仍不合格 → 拒绝入库
    }
    return cleanPDF(ocrText), nil
}

// isReadable: 正常字符率 > 70% 且乱码率 < 10%
func isReadable(text string) bool {
    // 正常字符 = 中文 + 英文 + 数字 + ASCII 标点 + 空格 + 换行
    // 乱码 = U+FFFD / U+FFFF / 控制字符（非 \n \t \r）
}
```

**4.3 Markdown 层清洗（service/rag/cleaner.go）：**

```go
func cleanMarkdown(text string) string {
    // 1. 控制字符 (\x00 \xFFFD \x0C) → 删除
    // 2. 图片占位符 ![...](...) → 删除
    // 3. 链接 [text](url) → 保留 text
    // 4. 加粗 **text** → 保留 text
    // 5. 删除线 ~~text~~ → 保留 text
    // 6. 空行合并 \n{3,} → \n\n
    // 7. 全角英文/数字 → 半角
    // 8. 保留 # 标题标记（留给切片器，切片完成后切片器自己清理）
}
```

**4.4 移除 PPTX 支持：**
- 删除 `parsePPTX()` 和 `extractTextFromPPTXXML()`
- 上传格式限制从 5 种 → 4 种（.txt .md .docx .pdf）

### 模块 6：结构切片

新增 `service/rag/chunker.go`。

**统一接口：**

```go
type ParsedSection struct {
    Title    string
    Level    int      // 1=H1, 2=H2, 3=H3
    Content  string
    Children []*ParsedSection
}
```

**各格式的结构识别：**

| 格式 | 标题信号 | 方法 |
|------|---------|------|
| MD | `#` `##` `###` 行 | parseMDSections() |
| DOCX | `<pStyle w:val="Heading1"/>` 或 字号 > 12 | parseDOCXSections() |
| PDF/TXT | 无信号 | 回退到段落+大小切片 |

**切片策略：**

```
1. 找到所有标题边界 → 生成 ParsedSection 树
2. 叶节点（最深层标题）内容作为最小切片单元
3. 叶节点 < 300 字 → 合并到上一个同级
4. 任一片段 > 800 字 → 按段落边界内部再切（不切断在句子中间）
5. 切片器清理 # 标记（清洗不删 #，切片后移除）
```

**非 MD/非 DOCX 回退策略：**

```
1. 按空行分段落
2. 以段落为最小单元，累加直到接近 chunkSize (500字)
3. 不切断在句子中间（以句号、换行为边界）
4. SectionPath 留空
```

### 模块 7：Embedding batch 并发

改 `service/rag/embedding.go`。

```go
func embedTexts(texts []string) ([][]float32, error) {
    results := make([][]float32, len(texts))
    var mu sync.Mutex
    g := new(errgroup.Group)
    g.SetLimit(3) // 并发 3 个请求

    for i, t := range texts {
        i, t := i, t
        g.Go(func() error {
            embedRate.Wait(context.Background()) // 令牌桶 8/s
            vec, err := embedCached(t)
            mu.Lock()
            results[i] = vec
            mu.Unlock()
            return err
        })
    }
    return results, g.Wait()
}

func embedCached(text string) ([]float32, error) {
    // 1. 查 Embedding 缓存: redis.GET("emb:<md5(text)>") → 命中直接返回
    // 2. 未命中 → 调 SiliconFlow API（3次指数退避重试）
    // 3. 写入缓存: redis.SetEx("emb:<md5(t)>", vec, 24h)
}
```

模型：`BAAI/bge-m3`，SiliconFlow API，维度 1024。

---

## 四、生成约束

### 模块 8：Prompt 引用约束

改 `service/agent/prompts.go` 的 `rulesBlock`，新增：

```
【回答规则 - 引用约束】
1. 只基于上面【参考资料】回答，资料中没有的内容必须说"资料中未涉及"
2. 每条论断必须标注来源：[《文档名》> 章节]
3. 禁止使用你自身的参数知识补充资料中没有的内容
4. 参考资料中可信度为"低"的只能作为参考，不能作为主要依据
5. 无法回答时直接说"这个问题在资料中没有找到相关内容"，不要猜测
```

---

## 五、诊断与排查

### 模块 9：日志排查标注

不改引擎主循环，在关键节点加日志字段：

**RAG 检索日志（`controller/agent_controller.go` 的 searchFunc）：**

```go
slog.Info("RAG检索",
    "request_id", rid,
    "query", query,
    "material_id", materialID,

    // 缓存命中
    "cache_hit",        cLabel, // "L1精确" | "L2语义" | "未命中"
    "cache_score",      cSim,   // L2 命中时的相似度
    "cache_src",        cSrc,   // L2 命中时的历史 query

    // 快速判定标签
    "recall_quality",  label,   // "高(≥0.7)" | "中(≥0.4)" | "低(<0.4)" | "空"
    "rerank_quality",  label,   // "高(≥0.7)" | "中(≥0.4)" | "低(<0.4)" | "未开启"
    "chunk_quality",   label,   // "OK" | "短碎片(<50字)" | "过长(>800字)"

    // 排查详情
    "recall_top5_scores", topScores,
    "rerank_top3_scores", rerankTopScores,
    "returned_sources", formattedSources,
)
```

**Agent 回复日志（`engine.go`，已有基础上新增）：**

```go
slog.Info("Agent 回复",
    "has_citation",     label,  // "有引用" | "无引用"
    "has_refusal",      label,  // "有(资料中未找到)" | "无"
    "has_hallucination", false, // 预留，人工标注
)
```

### 排查决策表

| RAG 日志 | Tool preview | Agent 日志 | 诊断 |
|----------|-------------|------------|------|
| recall 全低 | — | — | 检索侧① Embedding 不匹配 或 ④文档未覆盖 |
| recall 高 rerank 低 | — | — | 检索侧③ Reranker 排错 |
| — | 乱码/碎片 | — | 检索侧② Chunk 质量（切片/清洗问题）|
| rerank 高 | 有正确答案 | has_citation="无引用" | 生成侧① Prompt 约束不够强 |
| rerank 高 | 有正确答案 | 答歪了 | 生成侧② 上下文噪声 |
| rerank 高 | 有正确答案 | has_refusal="有" | 生成侧③ 引用约束过强 |
| rerank 高 | 有正确答案 | 编了资料没有的 | 生成侧⑤ 幻觉 |
| 全正常 | 正确 | 正确 | ✅ 没问题 |

全部通过一条 `grep <request_id>` 查看完整链路。

---

## 六、降级路径

| 故障 | 降级方式 |
|------|---------|
| Qdrant 不可用 | `simple_store.go` MySQL LIKE 关键词搜索（含 DocumentID/SectionPath）|
| Embedding API 不可用 | SimpleSearchVectorStore（不需要向量）|
| Rerank API 不可用 | 开关关闭 → 直接用混合检索结果 |
| Redis 不可用 | 缓存失效 → 走完整管线（Qdrant + Embedding 不受影响）|
| OCR 失败 | 拒绝入库，提示用户上传文字版 PDF |

---

## 七、文件结构

```
service/rag/
├── rag.go             — RAGService + VectorStore 接口 + 切片入口 + Init/Get
├── qdrant_store.go    — QdrantVectorStore（新增，替代 redis_store.go）
├── simple_store.go    — MySQL LIKE 降级（保留）
├── rerank.go          — Reranker（新增）
├── cleaner.go         — cleanMarkdown（新增）
├── chunker.go         — 结构切片 + ParsedSection（新增）
├── embedding.go       — batch 并发 embed（改）
├── redis_store.go     — 删除（被 qdrant_store.go 替代）

service/
├── document_parser.go — 解析层清洗 + OCR 降级 + cleanPDF/cleanDOCX + 删 PPTX（改）

model/
├── document_chunk.go  — +DocumentID +SectionPath（改）

config/
├── config.go          — +RAGConfig 配置块（改）
├── app.example.yml     — +rag 配置（改）

service/agent/
├── prompts.go         — rulesBlock 加引用约束（改）
├── engine.go          — Agent 回复日志加排查字段（改）

controller/
├── agent_controller.go — RAG 检索日志加排查字段（改）
```

---

## 八、不做的

- ❌ Query 改写 — Agent Tool Calling 中 LLM 已天然改写 query
- ❌ 向量库选型纠结 — Qdrant 确定
- ❌ PPTX 格式 — 删除支持
- ❌ 时间过滤元数据 — 不需要，课程资料非时效性

---

## 九、测试要点

| 模块 | 测试内容 |
|------|---------|
| QdrantVectorStore | Search/Index/Delete 单元测试 |
| Reranker | 正常精排 / API 超时降级 |
| 清洗 | cleanPDF/DOCX/Markdown 各 13 项的输入输出 |
| 切片 | MD/DOCX 标题树解析 / 合并(300) / 内部再切(800) / 纯文本回退 |
| OCR | 可读/不可读/OCR后仍不可读 三路径 |
| 混合检索 | RRF 融合开关 / Rerank 开关 |
| 配置开关 | 每个模块 on/off 不影响主流程 |
| Embedding | batch 并发正确性 / 令牌桶限流 |
| 缓存 | L1精确+L2语义 命中/未命中/清空 |
