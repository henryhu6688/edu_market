# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 启动与运行

```bash
# 后端 — 先杀旧进程（避免旧版占端口），确保 MySQL + Redis 运行
taskkill //F //IM edu_market.exe 2>/dev/null; taskkill //F //IM main.exe 2>/dev/null
go run .

# 验证后端是否成功启动（看是否 Redis 连接成功）
# 日志应出现: "Redis 连接成功" 和 "验证码存储器初始化完成"

# 前端开发服务器 (web/)
cd web && npm run dev
```

> **常见坑**：`go run .` 编译失败或被旧进程占 8080 端口时不会报明显错误，curl 打到了旧版上。
> 解决：`netstat -ano | grep ":8080 " | grep LISTENING` 查 PID → `taskkill //F //PID <pid>` 杀掉 → 重试。

后端默认监听 `:8080`，前端 Vite 开发服务器在 `:5173` 并自动代理 `/api` 和 `/uploads` 到后端。

```bash
# 运行所有测试（需要 MySQL + Redis）
go test ./...

# 运行单个包的测试
go test ./utils/ -v
go test ./middleware/ -v
go test ./service/ -v

# 运行单个测试用例
go test ./utils/ -run TestGenerateCode -v
```

## 架构概览

**在线学习资料售卖 + AI答疑平台**，前后端大仓模式。

```
请求 → router (Gin) → middleware (CORS/Logger/JWT) → controller → service → model (GORM) → MySQL
                                                                          ↘ utils/captcha → Redis
```

| 层 | 职责 |
|---|---|
| `router/` | 路由注册，三组：公开 `/api/*`、需认证 `/api/*`（JWT）、管理员 `/api/admin/*`（JWT+AdminOnly） |
| `middleware/` | CORS跨域、请求日志、JWT认证（`user_id/username/role` 注入 ctx）、管理员权限 |
| `controller/` | 请求参数绑定与校验（`c.ShouldBindJSON`），调用 service，用 `utils` 统一响应 |
| `service/` | 业务逻辑，通过 `database.DB` 操作数据库 |
| `model/` | GORM 模型：User、Course、Category、Order、Review、Conversation |
| `dto/request/` | 请求体结构 + Gin binding 校验规则 |
| `dto/response/` | 统一响应 `{code, message, data}` + 分页 `PageData` |
| `config/` | Viper 读取 `config/app.yml`，全局 `config.App` 可用 |
| `database/` | GORM + MySQL 初始化(自动迁移) + Redis 客户端初始化 |
| `utils/` | JWT 生成/解析 (HS256)、统一响应函数、验证码存储 `CodeStore` (Redis 版，支持限频/过期/一次性) |

### 数据模型

- **User**: `student` | `admin`，bcrypt 密码，支持 `Phone` 字段（手机号注册/登录）
- **Course**: 属于 Category + User（发布者），状态 `draft|published|off`，有价格/封面/文件URL
- **Category**: 支持二级分类（`ParentID` 可为 nil）
- **Order**: 订单号 `order_no`，状态 `pending|paid|cancelled`，软删除
- **Review**: 1-5星评分，关联 User + Course
- **Conversation**: AI对话记录，存 question/answer/model/tokens_used

### API 路由一览

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| POST | `/api/send-code` | 无 | 发送手机验证码（Redis存储，5分钟有效，60s限频） |
| POST | `/api/register` | 无 | 用户名+邮箱+密码注册 |
| POST | `/api/register/phone` | 无 | 手机号+验证码注册（自动生成用户名和随机密码） |
| POST | `/api/login` | 无 | 用户名+密码登录，返回 JWT |
| POST | `/api/login/phone` | 无 | 手机号+验证码登录，返回 JWT |
| GET | `/api/courses` | 无 | 课程列表 |
| GET | `/api/courses/:id` | 无 | 课程详情 |
| GET | `/api/courses/:id/reviews` | 无 | 课程评论 |
| GET | `/api/categories` | 无 | 分类列表 |
| GET | `/api/user/profile` | JWT | 个人信息 |
| PUT | `/api/user/profile` | JWT | 修改个人信息 |
| POST | `/api/orders` | JWT | 创建订单 |
| GET | `/api/orders` | JWT | 订单列表 |
| POST | `/api/orders/:order_no/pay` | JWT | 支付 |
| POST | `/api/ai/chat` | JWT | AI 答疑 |
| GET | `/api/ai/history` | JWT | AI 对话历史 |
| POST | `/api/reviews` | JWT | 发表评论 |
| POST | `/api/admin/courses` | Admin | 创建课程 |
| PUT | `/api/admin/courses/:id` | Admin | 更新课程 |
| DELETE | `/api/admin/courses/:id` | Admin | 删除课程 |
| POST | `/api/admin/categories` | Admin | 创建分类 |
| PUT | `/api/admin/categories/:id` | Admin | 更新分类 |
| DELETE | `/api/admin/categories/:id` | Admin | 删除分类 |

### 前端 (`web/`)

- Vue3 + Vite + Pinia + Vue Router + Axios
- `web/src/api/` 按模块封装 HTTP 调用（auth/course/order/ai/review/category）
- Pinia store: `web/src/stores/user.js` 管理登录状态和 token
- 路由守卫：`meta.auth` 要求登录，`meta.admin` 要求管理员角色，`meta.guest` 登录后自动跳首页
- Vite 代理：`/api` → `localhost:8080`，`/uploads` → `localhost:8080`

## 关键约定

- 新增功能按 **model → dto → service → controller → router** 这条流水线开发
- 所有 HTTP 响应统一走 `utils/response.go` 的响应函数，不直接 `c.JSON`
- JWT token 通过 `Authorization: Bearer <token>` 传递，24h 过期，HS256 签名
- 验证码 6 位数字，Redis 存储自动过期，限频 60s（`utils/codeStore`），开发阶段控制台打印
- 启动顺序：`config.Load` → `database.InitRedis` → `utils.InitCaptcha` → `database.Init`
- 验证码校验后立即删除（一次性），同时释放同手机号限频 key 方便连续操作
- 文件上传存在 `uploads/`（gitignore 了除 `.gitkeep` 外的所有文件）
- 开发阶段启动时自动 `AutoMigrate`，生产注意关闭
- 测试文件与源码同目录放（`*_test.go`），`go test ./...` 一键运行
- service 测试用独立数据库 `edu_market_test`（`TestMain` 自动创建），与开发库隔离，跑完自动清空
- 开发阶段查验证码：`redis-cli GET captcha:code:<phone>`（Redis 存的是真实验证码值）
