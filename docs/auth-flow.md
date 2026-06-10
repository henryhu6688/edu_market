# 登录注册流程参考文档

> 当前状态参考（非设计稿），基于 2026-06-09 重构后的代码。

## 技术栈

| 组件 | 选型 | 说明 |
|------|------|------|
| HTTP 框架 | Gin | `router/router.go` |
| ORM | GORM | 全局 `database.DB`，直接操作 |
| 数据库 | MySQL | `edu_market` 库，`users` 表 |
| 缓存 | Redis | 验证码存储 + 限频 |
| JWT | golang-jwt/v5 | HS256 签名 |
| 图形验证码 | base64Captcha | `github.com/mojocn/base64Captcha`，内存存储 |
| 前端 | Vue3 + Pinia + Axios | localStorage 存 token |

## 相关文件索引

```
后端:
  router/router.go           — 路由注册（公开/需认证/管理员三组）
  controller/auth_controller.go — 4 个 Handler
  service/auth_service.go    — 业务逻辑
  dto/request/auth.go        — 请求体 + binding 校验
  utils/jwt.go               — JWT 生成/解析
  utils/captcha.go           — 短信验证码 (CodeStore) + 图形验证码
  middleware/jwt.go           — JWT 认证中间件
  model/user.go              — 用户模型

前端:
  web/src/api/auth.js        — 4 个 API 调用
  web/src/api/index.js       — axios 实例 + 401 自动刷新
  web/src/stores/user.js     — Pinia store (双 token)
  web/src/router/index.js    — 路由守卫
  web/src/views/Login.vue    — 统一登录页
```

## 完整流程

```
┌───────────────────────────────────────────────────┐
│  前端                                            │
│  localStorage                                    │
│    access_token ←─── JWT (30min)                 │
│    refresh_token ←── 随机hex (24h)               │
│    user          ←── {id, username, phone, role} │
└──────────┬───────────────────────────────────────┘
           │ ① GET /api/captcha/image
           ▼
┌──────────────────────────────────────┐
│  图形验证码                           │
│  库: base64Captcha                   │
│  类型: 4位字母数字，含干扰线           │
│  字符集: 排除 0O1Il (34个字符)        │
│  存储: 内存 DefaultMemStore           │
│  TTL: 2 分钟                          │
│  校验: 一次性，成功后删除              │
└──────────┬─────────────────────────────┘
           │ ② POST /api/send-code
           │    { phone, captcha_id, captcha_code }
           ▼
┌──────────────────────────────────────┐
│  短信验证码 (CodeStore 全局单例)       │
│                                      │
│  Redis key:                          │
│    captcha:sms:<phone>               │
│      值: 6位数字字符                  │
│      TTL: 300秒 (captcha.expire)     │
│                                      │
│    captcha:smslimit:<phone>          │
│      值: "1"                         │
│      TTL: 60秒 (captcha.interval)    │
│                                      │
│  校验成功后: DEL 两个 key             │
└──────────┬─────────────────────────────┘
           │ ③ POST /api/login
           │    { phone, code }
           ▼
┌──────────────────────────────────────┐
│  AuthService.LoginByCode(phone)      │
│                                      │
│  SELECT * FROM users WHERE phone=?   │
│         │                            │
│    找到 │ 没找到                       │
│     ↓   ↓                            │
│   老用户 新用户                         │
│         username=user_<手机尾4位>       │
│         password=bcrypt(8位随机数)       │
│         role=student                  │
│         INSERT INTO users             │
│         │                            │
│         └──── 合并 ────┘              │
│              ↓                       │
│  + GenerateAccessToken(id,name,role) │
│    JWT HS256, Claims:                │
│      { user_id, username, role,      │
│        exp, iat, iss:"edu_market" } │
│    TTL: 30分钟                        │
│                                      │
│  + GenerateRefreshToken()            │
│    32字节随机 hex 字符串               │
│    TTL: 24小时                        │
│                                      │
│  + UPDATE users SET                  │
│      refresh_token=?,                │
│      refresh_expires_at=?            │
│    (存到用户记录，用于后续刷新校验)      │
└──────────┬─────────────────────────────┘
           │ 返回 { access_token, refresh_token, user }
           ▼
┌──────────────────────────────────────┐
│  前端存入 localStorage                │
│  跳转首页，登录完成                    │
└──────────────────────────────────────┘
```

## 双 Token 机制

| | access_token | refresh_token |
|---|-------------|---------------|
| 格式 | JWT (HS256) | 32字节随机 hex |
| 载荷 | `{ user_id, username, role }` | 无（存 DB 查） |
| 有效期 | 30 min (`jwt.access_ttl_minutes`) | 24 h (`jwt.refresh_ttl_hours`) |
| 传输 | `Authorization: Bearer <token>` | POST body `{ refresh_token }` |
| 存储 | localStorage + MySQL (不存) | localStorage + MySQL (`users.refresh_token`) |
| 刷新 | 401 时自动 | 滚动更新（新旧都换） |

## 请求认证

```
请求 → 中间件链 → Controller
         │
    Cors() → Logger() → [JWTAuth()] → [AdminOnly()]
                           │
                    提取 Authorization 头
                    分割 "Bearer <token>"
                    utils.ParseToken(token)
                      → HMAC-SHA256 验签
                      → 解析 Claims
                    c.Set("user_id", ...)
                    c.Set("username", ...)
                    c.Set("role", ...)
                    失败 → 401 "Token无效或已过期"
```

**三组路由**：

| 组 | 中间件 | 示例 |
|----|--------|------|
| 公开 | 无 | `/api/login`, `/api/courses` |
| 需登录 | JWTAuth | `/api/user/profile`, `/api/orders` |
| 管理员 | JWTAuth + AdminOnly | `/api/admin/courses` |

## Token 刷新机制

### 后端刷新 (auth_service.go:67)

```go
// 查用户
SELECT * FROM users WHERE refresh_token = ?

// 检查过期
if time.Now().After(refresh_expires_at)
    → 401 "refresh_token已过期，请重新登录"

// 滚动更新
生成新 access_token + 新 refresh_token
UPDATE users SET refresh_token=新, refresh_expires_at=新
```

### 前端自动刷新 (api/index.js:25-70)

```
请求A → 401 → 调 refresh
请求B → 同时401 → isRefreshing=true → 排队等待
请求C → 同时401 → 排队等待
         ↓
    refresh 成功 → 通知排队 → 重试各请求 (新 token)
    refresh 失败 → 全部 reject → logout → 跳 /login
```

> 刷新期间 `isRefreshing` 上锁，其他 401 请求不重复调 refresh。

## 退出登录

纯前端操作，无后端端点：

```
userStore.logout()
  → localStorage.removeItem('access_token')
  → localStorage.removeItem('refresh_token')
  → localStorage.removeItem('user')
  → store 值清空
  → 跳转 /login
```

JWT 无状态，不需要通知后端。access_token 30 分钟自然过期，refresh_token 留在数据库也不影响安全（24h 后过期）。

## User 模型敏感字段

```go
type User struct {
    PasswordHash     string     `json:"-"`  // 不返回
    RefreshToken     string     `json:"-"`  // 不返回
    RefreshExpiresAt *time.Time `json:"-"`  // 不返回
}
```

三个字段带 `json:"-"` 标签，**任何 JSON 序列化都排除**，包括登录成功响应。

## 开发调试

```bash
# 查看 Redis 中的短信验证码
redis-cli GET "captcha:sms:13800138000"

# 查看限频状态
redis-cli GET "captcha:smslimit:13800138000"

# 查看用户 refresh_token
mysql -u root -p123456 edu_market -e "SELECT id,phone,refresh_token,refresh_expires_at FROM users WHERE phone='13800138000'"
```

## Redis Key 一览

| Key 模式 | 内容 | TTL | 读写位置 |
|----------|------|-----|----------|
| `captcha:img:<id>` | 图形验证码答案 | 2 min | base64Captcha 内存 Store |
| `captcha:sms:<phone>` | 6位短信码 | 300 s | `utils/captcha.go:51` |
| `captcha:smslimit:<phone>` | 限频标记 "1" | 60 s | `utils/captcha.go:54` |

## Config 关键配置

```yaml
# config/app.yml
jwt:
  secret: edu-market-secret-key-change-in-production
  access_ttl_minutes: 30
  refresh_ttl_hours: 24

captcha:
  length: 6           # 短信码位数
  expire_seconds: 300  # 短信码有效期
  interval_seconds: 60 # 重发间隔
  image_length: 4     # 图形码位数（代码硬编码）
```
