# 并发控制

> 日期: 2026-06-16
> 状态: 设计完成

## 三层并发控制

### 1. API 限流（中间件层）

- Redis 滑动窗口计数器
- 每用户 30 次/min，每 IP 100 次/min
- 超限返回 429

### 2. Agent 并发控制（service 层）

- 同一用户最多 3 个并发 SSE 连接（buffered channel semaphore）
- LLM API 全局最多 5 并发，Embedding API 全局最多 3 并发
- 超限排队或返回 "系统繁忙"

### 3. 文件解析控制（service 层）

- 全局最多 2 个并发解析 goroutine
- 超出排队

## 实现

三个改动：

1. `middleware/ratelimit.go` — Redis 滑动窗口限流中间件
2. `service/concurrency.go` — Semaphore 工具（buffered channel 封装）
3. 各处加 semaphore 控制（agent_engine、document_parser、embedding）

## 配置

```yaml
concurrency:
  user_rate_limit: 30       # 每用户每分钟 max 请求
  ip_rate_limit: 100         # 每 IP 每分钟 max 请求
  max_agent_conn: 3          # 单用户最大 SSE 并发
  max_llm_calls: 5           # LLM API 全局并发
  max_embed_calls: 3         # Embedding API 全局并发
  max_parser: 2              # 文件解析全局并发
```
