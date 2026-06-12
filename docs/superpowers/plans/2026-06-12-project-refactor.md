# 项目架构重组 实现计划

> **For agentic workers:** Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** service/ 和 controller/ 按模块拆分子包，CLAUDE.md 拆分为入口 + 子规则文件

**Architecture:** 纯文件移动 + import 路径更新 + 类型名去前缀，不改功能，全部测试保持通过

**Tech Stack:** Go

---

## Phase 0: 准备工作

### Task 0: 创建目标目录结构

- [ ] **Step 1: 创建所有目标目录**

```bash
mkdir -p service/{agent,material,course,order,user,auth,category,review}
mkdir -p controller/{agent,material,course,order,user,auth,category,review}
```

- [ ] **Step 2: Commit**

```bash
git add -A
git commit -m "chore: create service/controller sub-package directories"
```

---

## Phase 1: 逐模块移动 + 更新引用

每个模块独立完成，确保每步编译通过。

### Task 1: agent 模块

**Files:**
- Move: `service/agent_*.go` → `service/agent/` (7 source + 4 test)
- Move: `controller/agent_controller.go` → `controller/agent/agent.go`
- Modify: `router/router.go` import
- Modify: `main.go` import

- [ ] **Step 1: 移动 agent service 文件**

```bash
# 移动 agent 源码（名称去 agent_ 前缀）
git mv service/agent_engine.go service/agent/engine.go
git mv service/agent_prompts.go service/agent/prompts.go
git mv service/agent_router.go service/agent/router.go
git mv service/agent_service.go service/agent/service.go
git mv service/agent_tools.go service/agent/tools.go
git mv service/agent_workflow.go service/agent/workflow.go
git mv service/agent_rag.go service/agent/rag.go
# 移动 agent 测试
git mv service/agent_engine_test.go service/agent/engine_test.go
git mv service/agent_router_test.go service/agent/router_test.go
git mv service/agent_service_test.go service/agent/service_test.go
git mv service/agent_workflow_test.go service/agent/workflow_test.go
# 移动 agent controller
git mv controller/agent_controller.go controller/agent/agent.go
```

- [ ] **Step 2: 更新 agent 子包内所有文件的 package 声明和 import 路径**

所有 `service/agent/*.go`：
- `package service` → `package agent`
- 内部引用去掉 `service.` 前缀（同包直接调）
- 外部引用：`edu_market/service` → 按需引入 `edu_market/service/agent`

- [ ] **Step 3: 更新 router.go**

```go
// 改前
import "edu_market/controller"
agentCtrl := controller.NewAgentController(agentSvc)

// 改后
import "edu_market/service/agent"  // 如果 router 直接引用 service
agentCtrl := controller.NewAgentController(agentSvc)  // controller 已移到 agent/
```

- [ ] **Step 4: 编译 + 测试**

```bash
go build ./... && go test ./service/agent/... -count=1
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor: move agent to service/agent + controller/agent"
```

---

### Task 2: material 模块

- [ ] **Step 1: 移动文件**

```bash
git mv service/material_service.go service/material/material.go
git mv service/document_service.go service/material/document.go
git mv service/document_parser.go service/material/parser.go
git mv service/material_service_test.go service/material/material_test.go
git mv controller/material_controller.go controller/material/material.go
git mv controller/document_controller.go controller/material/document.go
```

- [ ] **Step 2: 更新包名 + import**

`package service` → `package material`
内部引用 `service.MaterialService` → `material.Service`

- [ ] **Step 3: 编译 + 测试 + Commit**

```bash
go build ./... && go test ./service/material/... -count=1
```

---

### Task 3-8: 逐模块完成 course, order, user, auth, category, review

每模块相同步骤：`git mv` → 改包名 → 改 import → 编译测试 → commit。

---

## Phase 2: import 引用更新

### Task 9: 全局 import 更新

所有外部引用 `edu_market/service` 的子包需要更新 import 路径。

关键文件：
- `router/router.go`：import 所有 controller 子包
- `main.go`：import `service/agent` 等（如有直接引用）

- [ ] **Step 1: 逐文件检查和修复**

```bash
# 查找所有引用旧路径的文件
rg "service\.Agent" --type go -l
rg "service\.Material" --type go -l
```

- [ ] **Step 2: 更新 setup_test.go**

`setup_test.go` 保留在 `service/`，但其中引用的类型需要更新 import：
```go
import "edu_market/service/agent"
import "edu_market/service/material"
// etc
```

- [ ] **Step 3: 全量编译 + 测试**

```bash
go build ./... && go test ./... -count=1 2>&1 | grep -E "ok|FAIL"
```

---

## Phase 3: 类型名去前缀

### Task 10: 包名已提供上下文，类型去前缀

| 原类型名 | 新类型名 |
|---------|---------|
| `service.AgentService` | `agent.Service` |
| `service.AgentEngine` | `agent.Engine` |
| `service.MaterialService` | `material.Service` |
| `service.DocumentService` | `material.DocumentSvc` |
| `service.DocumentParser` | `material.Parser` |
| ...所有 `*Service` 类型 | 对应子包 `.Service` |

- [ ] **Step 1: 逐个子包重命名类型**

```bash
# agent 子包
sed -i 's/AgentService/Service/g' service/agent/*.go
sed -i 's/AgentEngine/Engine/g' service/agent/*.go
```

- [ ] **Step 2: 更新所有外部引用**

```go
// 改前: agentSvc := &service.AgentService{...}
// 改后: agentSvc := &agent.Service{...}
```

- [ ] **Step 3: 编译 + 测试 + Commit**

---

## Phase 4: CLAUDE.md + 规则文件

### Task 11: 编写 + 修改规则文件

- [ ] **Step 1: 新建 `project/architecture.md`** — 分层架构图 + 数据模型 + 路由表 + 前端概要

- [ ] **Step 2: 新建 `project/agent.md`** — Agent 设计 + Tool 定义 + SSE + System Prompt

- [ ] **Step 3: 新建 `project/materials.md`** — 资料/文档模型 + 编辑器 + 权限

- [ ] **Step 4: 修改 `project/conventions.md`** — student→user, courses→materials + Agent 概览

- [ ] **Step 5: 精简 CLAUDE.md** — 只保留启动命令 + 测试命令 + 索引

- [ ] **Step 6: Commit**

---

## Phase 5: 全量验证

### Task 12: 全量测试 + E2E

- [ ] `go test ./... -count=1` → 全部 PASS
- [ ] `npm run build` → 前端构建成功
- [ ] 启动服务验证 Agent SSE 对话正常
- [ ] Commit final

---

## 注意事项

1. **go.mod 不变**：子包在同一个 module `edu_market` 内，不需要修改
2. **循环依赖检查**：service/agent 不引用 service/material，保持独立
3. **setup_test.go**：TestMain 保留在 service/ 根，不拆到子包。但需要 import 所有子包
4. **git mv** 用 `git mv` 保留文件历史
5. **每步编译通过**：不积累错误，一个模块一个模块完成
