# 登录注册功能重构设计

## 背景

原系统有 4 条认证路径（密码登录/验证码登录/用户名注册/手机号注册），前端两个页面各带 tab 切换，后端 6 条路由。用户要求统一为手机号+验证码单入口，新用户自动注册，加 refresh token 双 token 机制，加图形验证码防刷。

## 设计决策

### 前端 — 一个页面搞定一切

```
打开 → 输手机号 → 图形验证码弹窗 → 解图形码 → 短信码发到 Redis
     → 输短信码 → 登录/自动注册 → 返回双 token
```

- 删除 Register.vue，Login.vue 改为统一入口
- 图形验证码弹出覆盖层，点击图片可刷新
- Navbar 从"登录""注册"两个链接改为一个"登录 / 注册"

### 后端 — 统一入口 + 双 Token

| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/captcha/image` | GET | 获取图形验证码（base64 png + captcha_id） |
| `/api/send-code` | POST | 传 phone + captcha_id + captcha_code，过图形码后发短信码 |
| `/api/login` | POST | 传 phone + code，查手机号 → 已存在则登录，不存在则自动注册 |
| `/api/refresh` | POST | 传 refresh_token，返回新的双 token（滚动刷新） |

### Token 设计

| | access_token | refresh_token |
|----|---------------|----------------|
| 格式 | JWT (HS256) | 随机 hex 字符串 |
| 有效期 | 30 分钟 | 1 天 |
| 存储 | localStorage | localStorage |
| 使用 | 每次请求带在 Authorization header | 仅 /api/refresh 时传 |
| 刷新 | 401 时自动调用 /api/refresh | 每次刷新时滚动更新 |

### 图形验证码

| 项 | 值 |
|----|-----|
| 库 | `github.com/mojocn/base64Captcha` |
| 类型 | 4 位数字+字母混合，含干扰线 |
| 存储 | 内存（base64Captcha.DefaultMemStore），2 分钟过期 |
| 校验 | 一次性，成功后删除 |

### 数据模型变更

User 表：
- Email: `not null` → `default:null`（手机号注册不需要邮箱）
- 新增 `refresh_token` varchar(255)
- 新增 `refresh_expires_at` datetime

### 删除的代码

- 前端: Register.vue，Login.vue 的密码/用户名 tab
- 后端: Register/Login/PhoneRegister/PhoneLogin 方法及对应 DTO
- 路由: /api/register, /api/register/phone, /api/login/phone, /api/login（改为统一入口）

## 验证方式

1. `go build ./...` 编译通过
2. `go test ./...` 全部测试通过（中间件/服务/工具共 60+ 用例）
3. 启动服务 → 浏览器访问 /login → 输入手机号 → 输入图形验证码 → 输入短信验证码 → 自动登录
