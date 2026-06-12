# v4 Agent 意图分类简化

> 日期: 2026-06-12
> 状态: 设计完成
> 分支: v4_agentSimplify

## 问题

当前意图分类分两层：关键词匹配 → 不命中时调 `classifyByLLM` 做 LLM 快判 → 得到意图词 → Agent 引擎再调 LLM 回答。同一个 LLM 被调了两次：一次猜意图，一次回答问题。这是冗余的——Agent 本身就是一个 LLM，看到用户消息自然知道该做什么。

## 方案

砍掉二阶段 LLM 快判。关键词命中时按意图给提示，不命中时让 Agent 自己处理。

```
改前：
  用户消息 → 关键词匹配？
    → 命中 → GetAgentPrompt(intent) → Agent
    → 不命中 → classifyByLLM() → 拿到意图 → GetAgentPrompt(intent) → Agent

改后：
  用户消息 → 关键词匹配？
    → 命中 → GetAgentPrompt(intent) → Agent
    → 不命中 → 基础 Prompt（不附加意图提示）→ Agent
```

## 代码改动

### 删除

`service/agent_workflow.go`：`classifyByLLM` 函数

### 修改

`service/agent_workflow.go`：`ClassifyIntent` 关键词不命中直接返回 `""`

`service/agent_service.go`：intent 为空时不调 `GetAgentPrompt(intent)`，直接用 `SystemPromptV3`

### 不动

- 关键词路由表、四个 Intent 常量
- `GetAgentPrompt(intent)`
- `CheckPurchaseStatus`
- Agent 引擎
- Tool 定义
- 前端
