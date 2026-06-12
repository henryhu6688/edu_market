# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 启动与运行

```bash
# 后端 — 先杀旧进程（避免旧版占端口），确保 MySQL + Redis 运行
taskkill //F //IM edu_market.exe 2>/dev/null; taskkill //F //IM main.exe 2>/dev/null
go run .

# 前端开发服务器 (web/)
cd web && npm run dev
```

后端默认监听 `:8080`，前端 Vite 开发服务器在 `:5173` 并自动代理 `/api` 和 `/uploads` 到后端。

```bash
# 运行所有测试
go test ./...

# 运行单个包的测试
go test ./service/ -v
go test ./utils/ -v

# 运行单个测试用例
go test ./service/ -run TestCreateCourse -v
```

## 架构概览

在线学习资料售卖 + AI答疑平台，前后端大仓模式。

```
请求 → router (Gin) → middleware (CORS/Logger/JWT) → controller → service → model (GORM) → MySQL
                                                                          ↘ utils/captcha → Redis
```

| 层 | 职责 |
|---|---|
| `router/` | 路由注册，公开 `/api/*`、需认证 `/api/*`（JWT）、管理员 `/api/admin/*`（JWT+AdminOnly） |
| `middleware/` | CORS、请求日志、JWT认证（`user_id/username/role` 注入 ctx） |
| `controller/` | 绑定校验请求参数，调 service，用 `utils` 统一响应 |
| `service/` | 业务逻辑，通过 `database.DB` 操作数据库。Agent 引擎也在这层 |
| `model/` | GORM 模型：User、Course、Category、Order、Review、Material、Document、Session、Message、FAQ 等 |
| `config/` | Viper 读取 `config/app.yml`，全局 `config.App`，敏感字段可用环境变量覆盖 |

### 数据模型

- **User**: `user` | `admin`，bcrypt 密码，phone 登录/注册
- **Material**: 替代旧 Course 模型，属于 Category + User（发布者），`draft|published|off`
- **Document**: 在线 Markdown 文档，属于 Material，支持文档树 + 试读
- **Session / Message**: Agent 对话，一条 Session 包含多条 Message，支持 tool_calls
- **Order**: 订单号 `order_no`，`pending|paid|cancelled`，软删除
- **Review / Category / FAQ**: 评价、分类、常见问题

### API 路由一览

| 路由 | 认证 | 说明 |
|------|------|------|
| POST `/api/agent/chat` | JWT | Agent SSE 流式对话 |
| GET/POST `/api/materials` | 公开/JWT | 资料列表/发布 |
| GET `/api/materials/:id/documents` | 公开 | 文档目录树 |
| GET/PUT/DELETE `/api/documents/:id` | JWT | 文档查看/编辑/删除 |
| POST `/api/materials/:id/documents/upload` | JWT | 文件上传转文档 |
| GET/POST `/api/orders` | JWT | 订单列表/创建 |
| POST `/api/orders/:order_no/pay` | JWT | 支付 |
| POST `/api/reviews` | JWT | 发表评价 |
| `/api/admin/*` | Admin | 管理后台 |

旧路由 `/api/courses`、`/api/categories`、`/api/captcha/*`、`/api/login`、`/api/user/profile` 继续可用。

### Agent 系统

单一 Agent + 9 个 Tool，LLM 自主规划。Workflow 层只做安全兜底（关键词路由 + 购买校验 + 买前内容限制）。

详见 `.claude/rules/project/conventions.md`。

## 关键约定

- 新增功能按 **model → dto/request → service → controller → router** 顺序开发
- 所有 HTTP 响应走 `utils/response.go`，不直接 `c.JSON`
- Service 只返回 Go `error`，不引用 `gin.Context`
- JWT access_token 30min / refresh_token 24h，通过 `Authorization: Bearer <token>` 传递
- 验证码 6 位数字，Redis 存储，一次性消费
- `config/app.yml` 已 gitignore，用 `config/app.example.yml` 做模板
- 敏感字段用环境变量覆盖：`AI_API_KEY`、`JWT_SECRET`、`DB_PASSWORD`、`REDIS_PASSWORD`
- 测试文件与源码同目录，测试库独立（`edu_market_test`）
- 删除等不可逆操作必须先确认
- 功能开发严格按 Superpowers 工作流进行

## 子规则文件

- `.claude/rules/superpowers-workflow.md` — 开发流程
- `.claude/rules/go/gorm.md` — GORM 使用约定
- `.claude/rules/go/testing.md` — 测试约定
- `.claude/rules/project/conventions.md` — 项目约定（HTTP响应、开发流水线、Agent设计）
