# 登录注册功能重构设计

## 背景

原系统有 4 条认证路径（密码登录/验证码登录/用户名注册/手机号注册），前端两个页面各带 tab 切换，后端 6 条路由。本次重构统一为手机号+验证码单一入口，新用户自动注册，加入双 Token 机制和图形验证码防刷。

## 前端 — 统一登录页 + 双 Token 管理

**登录流程**：

```
输手机号 → 点"发送验证码" → 弹出图形验证码 → 解图形码
→ 短信码发到 Redis → 输入短信码 → 登录/自动注册 → 返回双 Token
```

- 删除 Register.vue，Login.vue 改为统一入口
- 图形验证码弹出覆盖层，点击图片可刷新
- Navbar 从"登录""注册"两个链接改为一个"登录 / 注册"

**Pinia Store**（`stores/user.js`）：

| 字段 | 存储位置 | 用途 |
|------|---------|------|
| `accessToken` | localStorage | 每次请求带在 Authorization header |
| `refreshToken` | localStorage | access 过期时调 /api/refresh |
| `user` | localStorage | 显示用户名/头像/角色 |

路由守卫用 `userStore.accessToken` 判断登录态（`meta.auth`），用 `user.role` 判断管理员（`meta.admin`）。

**401 自动刷新**（`api/index.js` 响应拦截器）：收到 401 时自动调 `/api/refresh` 获取新 access_token 并重试原请求。并发 401 共享一次刷新调用（`isRefreshing` 排队机制），不重复请求。刷新失败则清空 localStorage 并跳转 `/login`。

## 后端 API — 4 条公开路由

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/captcha/image` | 返回图形验证码（captcha_id + base64 图片） |
| POST | `/api/send-code` | 传 phone + captcha_id + captcha_code，过图形码后发短信码 |
| POST | `/api/login` | 传 phone + code，查用户 → 存在则登录，不存在则自动注册 |
| POST | `/api/refresh` | 传 refresh_token，返回新双 Token（滚动刷新） |

**统一入口逻辑**（`POST /api/login`）：先验证短信码（Redis 查→比对→删），再查 `users` 表——找到则直接登录，没找到则自动注册（`username=user_<手机尾4位>`，随机 bcrypt 密码，`role=student`）。最后生成双 Token 并写回 `users.refresh_token` / `refresh_expires_at`。

## 双 Token 机制

| | access_token | refresh_token |
|---|-------------|---------------|
| 格式 | JWT (HS256) | 32 字节随机 hex 字符串 |
| 载荷 | `{ user_id, username, role, exp, iat, iss }` | 无（仅存 DB） |
| 有效期 | 30 分钟（`jwt.access_ttl_minutes`） | 24 小时（`jwt.refresh_ttl_hours`） |
| 传输方式 | `Authorization: Bearer <token>` | `POST /api/refresh` body |
| 存储 | localStorage（后端不存） | localStorage + `users.refresh_token` |
| 刷新 | 401 时自动 | 滚动更新（新旧都换，存新值+新过期时间） |

`RefreshToken` / `RefreshExpiresAt` / `PasswordHash` 三个字段带 `json:"-"`，永不序列化到客户端。

## 验证码体系

### 图形验证码

| 项 | 值 |
|----|-----|
| 库 | `github.com/mojocn/base64Captcha` |
| 类型 | 4 位字母+数字混合，含干扰线（正弦线+噪点线） |
| 字符集 | 排除了 `0O1Il`（共 34 个可用字符） |
| 存储 | 内存（`base64Captcha.DefaultMemStore`），2 分钟过期 |
| 校验 | 一次性，成功后删除 |

### 短信验证码

| 项 | 值 |
|----|-----|
| 格式 | 6 位数字 |
| 全局实例 | `utils.CaptchaStore`（`InitCaptcha()` 初始化） |
| 配置 | `config.App.Captcha`（length / expire_seconds / resend_seconds） |
| 入口 | `POST /api/send-code`（先过图形验证码，再进短信发送逻辑） |

**Redis 存储**：

| Key | 值 | TTL | 作用 |
|-----|-----|-----|------|
| `captcha:sms:<phone>` | 6 位数字 | 300s | 验证码 |
| `captcha:smslimit:<phone>` | "1" | 60s | 限频，同手机号 60s 内不可重发 |

校验时一次性消费（GET 比对成功后 DEL 两个 key）。

## 数据模型变更

### User 表

| 字段 | 变更 |
|------|------|
| Email | `not null` → `default:null`（手机号注册不需要邮箱） |
| RefreshToken | 新增 `varchar(255)`, `json:"-"` |
| RefreshExpiresAt | 新增 `datetime`, `json:"-"` |

### 模型外键

所有父子关联外键统一加 `constraint:OnDelete:CASCADE`（Course→Category, Course→User, Order→User, Order→Course, Review→User, Review→Course, Conversation→User）。删父记录时自动删子记录。

## 结构化日志

`utils/logger.go` 基于 `slog` + `lumberjack`，开发模式双写（控制台文本 + 文件），生产模式 JSON 格式只写文件，自动滚动（10MB / 保留 30 个 / 7 天过期 / gzip 压缩）。`logs/` 目录已 gitignore。启动顺序中 `InitLogger()` 放在 `InitRedis` 之前。

## 删除的代码

- 前端: Register.vue，Login.vue 的密码/用户名 tab
- 后端: Register/Login/PhoneRegister/PhoneLogin 方法及对应 DTO
- 路由: /api/register, /api/register/phone, /api/login/phone, /api/login（改为统一入口）

## 验证方式

1. `go build ./...` 编译通过
2. `go test ./...` 全部测试通过（中间件/服务/工具共 60+ 用例）
3. 启动服务 → 浏览器访问 /login → 输入手机号 → 输入图形验证码 → 输入短信验证码 → 自动登录
