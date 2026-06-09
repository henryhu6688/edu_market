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

### 日志系统

`utils/logger.go` 基于 `slog` + `lumberjack`，开发模式双写（控制台彩色 + `logs/app.log`），生产模式 JSON 格式只写文件，自动滚动（10MB 切分，保留 30 个，7 天过期，gzip 压缩）。

```bash
# 查看日志
tail -f logs/app.log
```

`logs/` 目录已 gitignore。

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
| `utils/` | JWT 生成/解析 (HS256)、统一响应函数、结构化日志(`slog`+`lumberjack`)、验证码存储 `CodeStore` (Redis 版，支持限频/过期/一次性) |

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
| GET | `/api/captcha/image` | 无 | 获取图形验证码（base64图片+`captcha_id`） |
| POST | `/api/send-code` | 无 | 发送手机验证码（需传图形验证码 `captcha_id`+`captcha_code`） |
| POST | `/api/login` | 无 | 手机号+短信验证码登录/注册（新用户自动注册） |
| POST | `/api/refresh` | 无 | 刷新 Token（用 `refresh_token` 换新 `access_token`） |
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

### 认证流程

```
1. GET /api/captcha/image  →  获取图形验证码（base64图片 + captcha_id）
2. POST /api/send-code     →  传 phone + captcha_id + captcha_code → Redis 存 6 位短信码
3. POST /api/login          →  传 phone + 短信验证码
   ├─ 新用户 → 自动注册（用户名 user_XXXX，随机密码）
   └─ 老用户 → 直接登录
   返回: { access_token, refresh_token, user }
4. 后续请求 → Authorization: Bearer <access_token>
5. access_token 过期 → POST /api/refresh { refresh_token } → 刷新 token
```

### 双 Token 机制

| Token | 类型 | TTL | 用途 |
|-------|------|-----|------|
| `access_token` | JWT (HS256) | 30 分钟（`jwt.access_ttl_minutes`） | 接口认证 |
| `refresh_token` | 随机 hex 字符串 | 24 小时（`jwt.refresh_ttl_hours`） | 静默刷新 access_token |

### 前端 (`web/`)

- Vue3 + Vite + Pinia + Vue Router + Axios
- 登录/注册统一入口：手机号 + 验证码，新用户自动注册
- 图形验证码弹窗：点"发送验证码"先弹出 4 位图形验证码，通过后才发短信
- Pinia store: `accessToken` / `refreshToken` / `user`（路由守卫用 `userStore.accessToken` 判断登录态）
- 路由守卫：`meta.auth` 要求登录（检查 `userStore.accessToken`），`meta.admin` 要求管理员角色
- Vite 代理：`/api` → `localhost:8080`，`/uploads` → `localhost:8080`

## 关键约定

- 新增功能按 **model → dto → service → controller → router** 这条流水线开发
- 所有 HTTP 响应统一走 `utils/response.go` 的响应函数，不直接 `c.JSON`
- JWT access_token 通过 `Authorization: Bearer <token>` 传递，access 30min / refresh 24h
- 验证码 6 位数字，Redis 存储自动过期，限频 60s（`utils/codeStore`），开发阶段控制台打印
- 发送短信前需过图形验证码（`utils/captcha/image.go`，4位字母数字，存 Redis）
- 启动顺序：`config.Load` → `utils.InitLogger` → `database.InitRedis` → `utils.InitCaptcha` → `database.Init`
- 验证码校验后立即删除（一次性），同时释放同手机号限频 key 方便连续操作
- 文件上传存在 `uploads/`（gitignore 了除 `.gitkeep` 外的所有文件）
- 开发阶段启动时自动 `AutoMigrate`，生产注意关闭
- 所有 Model 外键设置 `OnDelete:CASCADE`，删父记录自动删子记录
- 测试文件与源码同目录放（`*_test.go`），`go test ./...` 一键运行
- service 测试用独立数据库 `edu_market_test`（`TestMain` 自动创建），与开发库隔离，跑完自动清空
- 开发阶段查验证码：`redis-cli GET captcha:sms:<phone>`（Redis 存的是真实验证码值）
- 项目级 Rules 在 `.claude/rules/` 目录，子 Agent 受限时主会话直接遵循这些规则（当前后端 `deepseek-v4-pro` 不支持 spawn 子 Agent）
