# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 启动与运行

```bash
# 后端 — 确保 MySQL + Redis 运行
taskkill //F //IM edu_market.exe 2>/dev/null; taskkill //F //IM main.exe 2>/dev/null
go run .

# 前端开发服务器 (web/)
cd web && npm run dev

# 测试
go test ./...                     # 全量
go test ./service/ -v             # 单包
go test ./service/ -run TestXxx   # 单个用例
```

后端 `:8080`，前端 `:5173`。

## 项目概要

Go + Gin + GORM + MySQL + Redis 后端，Vue3 + Vite 前端。在线学习资料售卖 + AI 答疑平台。

架构分层：`router → middleware(CORS/Logger/JWT) → controller → service → model(MySQL) / utils → Redis`

## 关键约定

- 新功能按 **model → dto/request → service → controller → router** 顺序开发
- 所有 HTTP 响应走 `utils/response.go`（Success/BadRequest/NotFound 等），禁止 `c.JSON()`
- Service 层不碰 `gin.Context`，只返回 Go `error`
- JWT 注入 `user_id`、`username`、`role`（`user`|`admin`）到 ctx
- `config/app.yml` 已 gitignore，用 `config/app.example.yml` 做模板
- 敏感字段环境变量覆盖：`AI_API_KEY`、`JWT_SECRET`、`DB_PASSWORD`、`REDIS_PASSWORD`
- 测试库独立（`edu_market_test`），TestMain 自动建库 + 清空
- 删除等不可逆操作必须先确认
- 功能开发严格按 Superpowers 工作流

## 子规则文件

- `.claude/rules/superpowers-workflow.md` — 开发流程
- `.claude/rules/go/gorm.md` — GORM 约定
- `.claude/rules/go/testing.md` — 测试约定
- `.claude/rules/project/conventions.md` — HTTP 响应、Context 注入、中文注释等约定
- `.claude/rules/project/architecture.md` — 分层架构、数据模型、路由表、前端
- `.claude/rules/project/agent.md` — Agent 设计、Tool、SSE 协议
- `.claude/rules/project/materials.md` — 资料/文档系统、编辑器、权限
