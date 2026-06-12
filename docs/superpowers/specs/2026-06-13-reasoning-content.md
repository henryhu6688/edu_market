# reasoning_content 字段支持

> 日期: 2026-06-13
> 分支: v9_logger_upgrade

## 问题

deepseek-v4-pro 返回的 assistant 消息带 `reasoning_content` 字段。存 DB 时丢弃，loadContext 加载时缺失，API 报 400。

## 方案

三点改动：

1. `model/message.go` — Message 新增 `ReasoningContent string` 字段（TEXT）
2. `service/agent_engine.go` Run() — 存 assistant 消息时保存 `choice.Message.ReasoningContent`
3. `service/agent_engine.go` loadContext() — 加载 assistant 消息时设置 `msg.ReasoningContent`

不涉及 API 变更、不影响旧数据（新字段为空时 omitempty 不序列化）。
