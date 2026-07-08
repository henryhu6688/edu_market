# RAG 检索质量评测系统设计

## 背景

项目已有 Agent 行为评测（`scripts/eval/`），但缺少 RAG 检索管线本身的评测。需要在半天内建立一套检索质量评测，跑出 Precision、Recall、TopK 命中率、检索延迟四项指标，用于简历数据支撑。

当前数据规模：24 个资料、978 篇文档、5035 条切片。

## 目标

1. 自动生成 100 条测试问题 + ground truth（LLM 辅助，零人工标注）
2. 分层 A/B 对比四组配置（裸向量 → +BM25 → +Rerank → +缓存）
3. 输出 Markdown 报告，含分层对比表和延迟分位表
4. 半天内完成（3-4 小时）

## 架构

```
scripts/eval/rag/
├── main.go           # CLI，四个子命令
├── types.go          # 所有数据结构
├── metrics.go        # 四指标计算函数
├── runner.go         # 执行引擎（配置切换 + Search调用 + 计时）
├── reporter.go       # Markdown 报告生成
├── gen_queries.go    # Step 1: LLM 从切片内容反向生成问题
└── expand_gt.go      # Step 2: LLM 暴力搜索 + 判断 → 扩展 ground truth

data/eval/
└── rag_queries.json  # 100 条标注数据，持久化可复跑
```

## 数据流（三步骤）

### Step 1：生成问题 + 初步 ground truth

```
随机抽 100 条切片（覆盖不同 material）
  → 对每条切片调 LLM：
    "根据这段内容生成一个用户可能会搜的问题（5-15字）。
     内容：{chunk.Content}"
  → 输出 {query, relevant_chunk_ids: [该切片ID], material_id, category}

API：DeepSeek Chat，temperature=0.7，批量 10 条/次
耗时：~2 分钟
```

### Step 2：扩展 ground truth

```
对每条查询：
  → ragSvc.Search(materialID, query, topK=50, hasAccess=true)
  → 关 Rerank，裸向量检索
  → 将 50 条结果 + 查询丢给 LLM：
    "查询：{query}，下面 50 条结果是相关的吗？
     对每条结果返回 yes/no，只返回 JSON 数组。"
  → LLM 返回相关切片 ID 列表 → 合并进 relevant_chunk_ids

API：DeepSeek Chat，temperature=0，逐条调用（需读 50 条内容）
耗时：100 条 × ~2s = ~3 分钟
```

### Step 3：分层评测

```
四组配置依次执行：
  ① 裸向量: Hybrid=false  Rerank=false  Cache=false
  ② +BM25:   Hybrid=true   Rerank=false  Cache=false
  ③ +Rerank:  Hybrid=true   Rerank=true   Cache=false
  ④ +缓存:    Hybrid=true   Rerank=true   Cache=true  (生产)

每组：
  → 修改 config.App.RAG 对应字段（不碰 service/rag 代码）
  → 预跑一条 warmup 排除冷启动
  → 逐条调 ragSvc.Search(materialID, query, topK=5, true)，记录耗时 + 返回切片
  → 跑完切回原配置

耗时：100 条 × 4 组 × ~200ms ~= 80 秒（不含缓存组）
```

## 指标公式

```go
// Precision@K = |返回的前K条 ∩ ground_truth| / K
precision = intersection(returned[:K], relevant) / K

// Recall@K = |返回的前K条 ∩ ground_truth| / |ground_truth|
recall = intersection(returned[:K], relevant) / len(relevant)

// TopK命中率 = 返回的前K条中是否至少有一条在 ground_truth 里
topKHit = intersection(returned[:K], relevant) > 0

// 检索延迟 = time.Since(start).Milliseconds()
// 汇总后取 avg、P50、P95、P99
```

## CLI 接口

```bash
go run ./scripts/eval/rag/                        # 全流程
go run ./scripts/eval/rag/ --step=gen             # 只生成问题
go run ./scripts/eval/rag/ --step=expand          # 只扩展 ground truth
go run ./scripts/eval/rag/ --step=eval            # 只跑评测（需已有 queries.json）
go run ./scripts/eval/rag/ --step=report          # 只生成报告
go run ./scripts/eval/rag/ --queries=100          # 自定义问题数量
go run ./scripts/eval/rag/ --topk=10              # 自定义 K 值
```

## 报告格式

```markdown
# RAG 检索质量评测报告
**时间:** 2026-07-08 20:30  |  **查询数:** 100  |  **TopK:** 5

## 📊 分层对比

| 配置 | Precision@5 | Recall@5 | Top5命中率 | 平均延迟 | P50 | P95 | P99 |
|------|:----------:|:--------:|:---------:|:------:|:---:|:---:|:---:|
| ① 裸向量检索 | - | - | - | - | - | - | - |
| ② +BM25混合检索 | - | - | - | - | - | - | - |
| ③ +Rerank精排 | - | - | - | - | - | - | - |
| ④ +两级缓存 | - | - | - | - | - | - | - |

## 🔍 各层贡献分析

| 优化项 | Precision变化 | Recall变化 | 延迟变化 |
|--------|:----------:|:--------:|:------:|
| BM25混合检索 | +Xpp | +Xpp | +Xms |
| Rerank精排 | +Xpp | -Xpp | +Xms |
| 两级缓存（命中时） | — | — | -X% |

## 📈 分类统计

（按 category 字段分组，展示不同知识领域的检索质量）

## ⚠️ 局限说明

- Recall 依赖 LLM 判断 ground truth，存在 5-10% 的标注误差
- 每个问题的最小 ground truth 为 1（生成切片本身），Recall 可能略偏高
- 缓存组数据依赖命中率，冷启动条件下结果不同
```

## 关键设计决策

### 配置切换：改全局变量，不改 Search()

`ragSvc.Search()` 内部读 `config.App.RAG.HybridSearch/Rerank/CacheEnabled` 控制行为。runner 在每组评测前修改这些全局字段，跑完恢复。**零侵入，不改 service/rag/。**

### 缓存绕过：改配置即可

`Search()` 第 156 行 `if config.App.RAG.CacheEnabled && ...` —— 设为 false 就直接跳过所有缓存逻辑，无需额外清理 Redis。生产组（④）设 true，让缓存自然生效。

### LLM 容错

- gen_queries 失败 → 打印警告，跳过该切片，重试最多 3 次
- expand_gt 失败 → 保留原始 1 个 ground truth，标注 incomplete
- 批量调用时每条之间 sleep 200ms，避开限流

## 局限

1. **ground truth 不完美**：LLM 判断相关性有误差，但对 Precision/TopK 命中率无影响，仅 Recall 受影响
2. **单资料查询**：只测单个 material 内的检索，不测跨资料场景（search_documents 的多资料聚合是 Agent 层逻辑）
3. **中文为主**：查询生成是中文，评测结果主要代表中文检索质量
4. **缓存冷启动**：④ 组的数据是"温缓存"场景，需单独标注
