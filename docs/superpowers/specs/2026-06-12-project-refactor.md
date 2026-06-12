# 项目架构重组 设计

> 日期: 2026-06-12
> 状态: 设计完成
> 分支: v5_refactor

## 目标

- service/ 和 controller/ 按模块拆分独立子包
- CLAUDE.md 拆分为入口 + 子规则文件
- 纯文件移动 + import 路径更新，不改功能

## service/ 拆分

```
改前（扁平 17 source files）:
service/
  ├── agent_engine.go
  ├── agent_prompts.go
  ├── agent_router.go
  ├── agent_service.go
  ├── agent_tools.go
  ├── agent_workflow.go
  ├── agent_rag.go
  ├── auth_service.go
  ├── category_service.go
  ├── course_service.go
  ├── document_parser.go
  ├── document_service.go
  ├── material_service.go
  ├── order_service.go
  ├── review_service.go
  ├── user_service.go
  ├── setup_test.go (TestMain)
  └── *_test.go

改后:
service/
  ├── setup_test.go         ← TestMain 保留在 service 根
  ├── agent/               ← agent_engine, _prompts, _router, _service, _tools, _workflow, _rag
  │   ├── engine.go
  │   ├── prompts.go
  │   ├── router.go
  │   ├── service.go
  │   ├── tools.go
  │   ├── workflow.go
  │   └── rag.go
  ├── material/            ← material_, document_, parser
  │   ├── material.go
  │   ├── document.go
  │   └── parser.go
  ├── course/
  │   └── course.go
  ├── order/
  │   └── order.go
  ├── user/
  │   └── user.go
  ├── auth/
  │   └── auth.go
  ├── category/
  │   └── category.go
  └── review/
      └── review.go
```

测试文件同包子包放：`service/agent/engine_test.go`、`service/material/material_test.go` 等。

## controller/ 拆分

```
改前:
controller/
  ├── agent_controller.go
  ├── auth_controller.go
  ├── category_controller.go
  ├── course_controller.go
  ├── document_controller.go
  ├── material_controller.go
  ├── order_controller.go
  ├── review_controller.go
  └── user_controller.go

改后:
controller/
  ├── agent/
  │   └── agent.go
  ├── material/
  │   ├── material.go
  │   └── document.go
  ├── course/
  │   └── course.go
  ├── order/
  │   └── order.go
  ├── user/
  │   └── user.go
  ├── auth/
  │   └── auth.go
  ├── category/
  │   └── category.go
  └── review/
      └── review.go
```

## import 路径变更

所有文件 import 路径从 `edu_market/service` 改为对应子包：

| 原引用 | 新引用 |
|--------|--------|
| `service.AgentService` | `service/agent.Service` |
| `service.MaterialService` | `service/material.Service` |
| `service.DocumentParser` | `service/material.Parser` |
| `service.AgentEngine` | `service/agent.Engine` |
| ... | ... |

同时类型名去前缀：`AgentService` → `Service`，`MaterialService` → `Service`，因为包名已经提供了上下文。

## CLAUDE.md 拆分

```
CLAUDE.md                     → 入口（项目概述 + 启动命令 + 链接）
.claude/rules/
  ├── superpowers-workflow.md  → 已有
  ├── go/gorm.md               → 已有
  ├── go/testing.md            → 已有
  ├── project/conventions.md   → 已有
  ├── project/architecture.md  → 新建：目录结构、分层、模块说明
  ├── project/agent.md         → 新建：Agent 架构、Tool 定义、SSE
  └── project/materials.md     → 新建：资料/文档模型、权限
```

主 CLAUDE.md 内容：
- 启动命令（go run / npm run）
- 架构概览图
- 指向子规则文件的链接
- 不再放详细设计文档

## 改动清单

| 操作 | 文件数 | 说明 |
|------|--------|------|
| 移动 | ~20 source | service/*.go → service/<module>/*.go |
| 移动 | ~10 test | service/*_test.go → service/<module>/*_test.go |
| 移动 | ~9 source | controller/*.go → controller/<module>/*.go |
| 修改 | ~15 import | router.go, main.go, 各 controller/service 间引用 |
| 修改 | 所有子文件包名 | `package service` → `package agent` 等 |
| 新建 | 3 rule 文件 | architecture, agent, materials |
| 删 | 无需删除 | 旧文件移动后原地不留 |

## 不变

- API 路由不变
- 数据库表不变
- 前端不变
- go.mod 不变（子包在同一个 module 内）
- TestMain (setup_test.go) 保留在 service/ 根，不做子包拆分
