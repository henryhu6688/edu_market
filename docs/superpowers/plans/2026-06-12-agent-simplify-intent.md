# v4 Agent 意图分类简化 实现计划

> **For agentic workers:** Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 砍掉 `classifyByLLM`，关键词不命中时不再调 LLM 判意图，直接让 Agent 自己处理

**Architecture:** 删 `classifyByLLM` 函数，`ClassifyIntent` 不命中返回 `""`，`AgentService.Chat` 对空 intent 用基础 Prompt

**Tech Stack:** Go

---

### Task 1: 删 classifyByLLM + 改流程

**Files:**
- Modify: `service/agent_workflow.go:49-100`
- Modify: `service/agent_service.go:42-45`

- [ ] **Step 1: 删 classifyByLLM**

删 `service/agent_workflow.go` 中 `classifyByLLM` 函数（第 49-100 行），同时删 `ClassifyIntent` 中末尾的 `return classifyByLLM(question)` 调用。

`ClassifyIntent` 末行改为：

```go
// 关键词不命中 → 让 Agent 自己判断，不再二阶段 LLM
return ""
```

- [ ] **Step 2: AgentService 处理空 intent**

`service/agent_service.go:42-45`，`intent` 为空时不调 `GetAgentPrompt`：

```go
intent := ClassifyIntent(question)
var systemPrompt string
if intent != "" {
    systemPrompt = GetAgentPrompt(intent)
} else {
    systemPrompt = SystemPromptV3
}
```

- [ ] **Step 3: 清理未使用的 import**

`agent_workflow.go` 中 `classifyByLLM` 删除后，检查 `bytes`、`encoding/json`、`fmt`、`io`、`net/http`、`time` 是否还被其他函数引用。未使用的删掉。

- [ ] **Step 4: 更新测试**

`service/agent_workflow_test.go` 中删掉 `TestClassifyIntent_*_LLM` 测试（四个 LLM 相关测试用例）。

- [ ] **Step 5: 编译 + 测试 + Commit**

```bash
cd d:/Vscoding/edu_market && go build ./... && go test ./... -count=1 2>&1 | grep -E "ok|FAIL"
```

预期：全部 PASS。

```bash
git add service/agent_workflow.go service/agent_service.go service/agent_workflow_test.go
git commit -m "refactor: remove classifyByLLM — let Agent handle unmatched intents natively"
```
