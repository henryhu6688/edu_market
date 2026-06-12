# edu_market 项目约定

> 自动提炼自代码库跨文件分析（controller/service/model/dto/middleware/utils）

## HTTP 响应规范

所有 API 响应必须走 `utils/response.go` 提供的函数，**禁止直接调用 `c.JSON()`**。

| 函数 | HTTP 状态码 | 用途 |
|------|------------|------|
| `Success(c, data)` | 200 | 普通成功响应 |
| `Created(c, data)` | 201 | 创建资源成功 |
| `PageSuccess(c, list, total, page, pageSize)` | 200 | 分页列表 |
| `BadRequest(c, msg)` | 400 | 参数校验失败 |
| `Unauthorized(c, msg)` | 401 | 未登录/Token 无效 |
| `Forbidden(c, msg)` | 403 | 权限不足 |
| `NotFound(c, msg)` | 404 | 资源不存在 |
| `InternalError(c, msg)` | 500 | 服务器内部错误 |

**违例风险**：直接 `c.JSON` 导致响应格式 `{code, message, data}` 不一致，前端解析失败。

## 开发流水线

新增功能**严格按此顺序**开发：

```
1. model/        → GORM 模型定义 + TableName()
2. dto/request/  → 请求体 + binding 校验标签
3. dto/response/ → 响应体（如需新结构体）
4. service/      → 业务逻辑，操作 database.DB
5. controller/   → 参数绑定 + 调用 service + utils 响应
6. router/       → 注册路由 + 指定中间件
```

## Service 层不碰 HTTP

- Service 只返回 Go `error`，**不引用 `gin.Context`**
- HTTP 状态码的选择权在 Controller
- Service 错误信息用中文，可直接透传给用户

```go
// ✅ 正确
func (s *SomeService) DoSomething() error {
    return errors.New("操作失败")
}

// ❌ 错误
func (s *SomeService) DoSomething(c *gin.Context) {
    utils.BadRequest(c, "操作失败")
}
```

## 请求校验用 binding 标签

所有请求参数校验通过 `binding` 标签在 DTO 中声明，Controller 用 `c.ShouldBindJSON(&req)` 统一校验。

```go
type LoginByCodeReq struct {
    Phone string `json:"phone" binding:"required,len=11"`
    Code  string `json:"code" binding:"required,len=6"`
}
```

- 常用标签：`required`、`len`、`min`、`max`、`oneof`、`email`
- **不在 Controller 里写手动 if 校验**

## Context 注入约定

JWT 中间件向 `gin.Context` 注入以下 key，后续通过 `c.Get()` 获取：

| Key | 类型 | 说明 |
|-----|------|------|
| `user_id` | `uint` | 用户 ID |
| `username` | `string` | 用户名 |
| `role` | `string` | `user` 或 `admin` |

```go
userID, _ := c.Get("user_id")
role, _ := c.Get("role")
```

## JSON 安全 — 敏感字段保护

密码、Token 等敏感字段必须使用 `json:"-"` 标签，**永不序列化到客户端**。

```go
type User struct {
    PasswordHash    string `json:"-"`  // ✅ 不会输出到 JSON
    RefreshToken    string `json:"-"`  // ✅
}
```

## 中文注释规范

所有**导出函数和导出结构体**必须有中文注释，紧贴声明上方：

```go
// User 用户模型
type User struct { ... }

// GenerateAccessToken 生成短期 Access Token
func GenerateAccessToken(userID uint, username, role string) (string, error) { ... }
```

## 配置文件同步

`config/app.example.yml` 和 `config/app.yml` 必须保持**结构完全一致**（字段、缩进、注释），唯一区别是敏感字段的值：

| 字段 | example | 真实 |
|------|---------|------|
| `database.password` | `""` | `"实际密码"` |
| `jwt.secret` | `""` | `"实际secret"` |
| `ai.api_key` | `""` | `"实际key"` |

改配置结构时，两个文件**必须同时同步更新**。

## 敏感数据保护

**禁止将任何敏感数据（API Key、密码、Token、Secret）硬编码在代码或测试文件中提交到 git。** 所有敏感数据必须通过引用配置文件来使用：

- 测试代码用 `readConfigYAML("key")` 从本地 `config/app.yml` 读取
- 生产代码用 `config.App.XXX` 或环境变量

`config/app.yml` 已在 `.gitignore` 中，不会被提交。`config/app.example.yml` 可以提交，但敏感字段必须为空字符串。

## Git 分支规则

**所有代码改动必须先创建新分支，禁止直接在 master 提交。** 修改前执行 `git checkout -b vN_featureName`。
