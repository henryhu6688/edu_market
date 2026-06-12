# 流式输出改进：LLM 原生 SSE

> 日期: 2026-06-13
> 分支: v9_logger_upgrade

## 现状

`callLLM` 非流式 → 拿完整回复 → `streamAnswer` 一次性发 delta → 无打字机效果。

## 方案

复用已有的 `callLLMStream` 存根，Tool Calling 轮次保持非流式，最后一轮改用原生流式 SSE 转发。

```
用户提问
  │
  ▼
循环（最多 10 轮）：
  callLLM(非流式) → 有 tool_calls? → 执行 tool → 继续循环
                  → 无 tool_calls? → callLLMStream(流式) → 转发 delta → 结束
```

## 引擎改动

`agent_engine.go`:

1. 最终回答时调 `callLLMStream` 替代 `callLLM` + `streamAnswer`
2. `callLLMStream` 返回 `(content, tokens, error)`，流式过程中直接 `sseHandler("delta", ...)`
3. 删 `streamAnswer` 函数

## 前端不变

`delta` 事件格式不变，前端不用动。

## 注意

流式模式下如果 LLM 中途返回 tool_calls（极少见），回退到非流式处理。
