# 登录注册功能重构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 4 条认证路径统一为手机号+验证码单入口，新用户自动注册，加入双 token + 图形验证码

**Architecture:** 前端 Login.vue 单体页面，后端 4 条精简路由（captcha/image / send-code / login / refresh），access_token(30min JWT) + refresh_token(1d 随机字符串) 双 token 滚动刷新

**Tech Stack:** Go + Gin + GORM + Redis + base64Captcha / Vue3 + Vite + Pinia

---

### Task 1: Config — 双 Token TTL

**Files:**
- Modify: `config/config.go:52-56`
- Modify: `config/app.yml:20-22`

- [ ] **Step 1: 给 JWTConfig 加 access_ttl_minutes 和 refresh_ttl_hours**

```go
// JWTConfig JWT 配置
type JWTConfig struct {
    Secret          string `mapstructure:"secret"`
    ExpireHours     int    `mapstructure:"expire_hours"`
    AccessTTLMin    int    `mapstructure:"access_ttl_minutes"`
    RefreshTTLHours int    `mapstructure:"refresh_ttl_hours"`
}
```

- [ ] **Step 2: 更新 app.yml**

```yaml
jwt:
  secret: edu-market-secret-key-change-in-production
  expire_hours: 24
  access_ttl_minutes: 30
  refresh_ttl_hours: 24
```

- [ ] **Step 3: 编译验证**

```bash
go build ./...  # 预期：编译通过
```

---

### Task 2: Model — User 表加 refresh_token 字段

**Files:**
- Modify: `model/user.go:6-19`

- [ ] **Step 1: User 结构体改动**

- Email: `not null` → `default:null`
- 新增 `RefreshToken string` 和 `RefreshExpiresAt *time.Time`

```go
type User struct {
    ID               uint       `gorm:"primaryKey;autoIncrement" json:"id"`
    Username         string     `gorm:"type:varchar(50);uniqueIndex;not null" json:"username"`
    Email            string     `gorm:"type:varchar(100);uniqueIndex;default:null" json:"email"`
    Phone            string     `gorm:"type:varchar(20);uniqueIndex;default:null" json:"phone"`
    PasswordHash     string     `gorm:"type:varchar(255);not null" json:"-"`
    Role             string     `gorm:"type:varchar(20);default:student;not null" json:"role"`
    Avatar           string     `gorm:"type:varchar(255)" json:"avatar"`
    RefreshToken     string     `gorm:"type:varchar(255)" json:"-"`
    RefreshExpiresAt *time.Time `gorm:"default:null" json:"-"`
    CreatedAt        time.Time  `gorm:"autoCreateTime" json:"created_at"`
    UpdatedAt        time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}
```

- [ ] **Step 2: 编译验证**

```bash
go build ./...  # 预期：通过（AutoMigrate 会加新列）
```

---

### Task 3: Utils — JWT 双 Token + 图形验证码

**Files:**
- Modify: `utils/jwt.go`
- Modify: `utils/captcha.go`

- [ ] **Step 1: jwt.go — 拆为 GenerateAccessToken + GenerateRefreshToken**

```go
// GenerateAccessToken 生成短期 Access Token (JWT)
func GenerateAccessToken(userID uint, username, role string) (string, error) {
    ttl := config.App.JWT.AccessTTLMin
    if ttl <= 0 { ttl = 30 }
    claims := Claims{
        UserID:   userID,
        Username: username,
        Role:     role,
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(ttl) * time.Minute)),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
            Issuer:    "edu_market",
        },
    }
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(config.App.JWT.Secret))
}

// GenerateRefreshToken 生成 Refresh Token（32字节随机hex）
func GenerateRefreshToken() (string, error) {
    b := make([]byte, 32)
    if _, err := rand.Read(b); err != nil { return "", err }
    return hex.EncodeToString(b), nil
}
```

- [ ] **Step 2: captcha.go — 加图形验证码（base64Captcha）**

```go
import "github.com/mojocn/base64Captcha"

var imgCaptchaStore = base64Captcha.DefaultMemStore

func GenerateImageCaptcha() (captchaID string, b64s string, err error) {
    imgCaptchaStore = base64Captcha.NewMemoryStore(100, 2*time.Minute)
    driver := base64Captcha.NewDriverString(36, 120, 0,
        base64Captcha.OptionShowSlimeLine|base64Captcha.OptionShowSineLine,
        4, "123456789abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ",
        nil, nil, nil)
    c := base64Captcha.NewCaptcha(driver, imgCaptchaStore)
    id, b64s, _, err := c.Generate()
    return id, b64s, err
}

func VerifyImageCaptcha(id, code string) bool {
    if id == "" || code == "" { return false }
    return imgCaptchaStore.Verify(id, code, true)
}
```

- [ ] **Step 3: 安装依赖 + 编译**

```bash
go get github.com/mojocn/base64Captcha
go build ./...
```

---

### Task 4: DTO — 精简请求结构

**Files:**
- Modify: `dto/request/auth.go`

- [ ] **Step 1: 替换为 3 个结构体**

```go
package request

type SendCodeReq struct {
    Phone       string `json:"phone" binding:"required,len=11"`
    CaptchaID   string `json:"captcha_id" binding:"required"`
    CaptchaCode string `json:"captcha_code" binding:"required"`
}

type LoginByCodeReq struct {
    Phone string `json:"phone" binding:"required,len=11"`
    Code  string `json:"code" binding:"required,len=6"`
}

type RefreshReq struct {
    RefreshToken string `json:"refresh_token" binding:"required"`
}
```

---

### Task 5: Service — 统一入口 + Refresh

**Files:**
- Modify: `service/auth_service.go`

- [ ] **Step 1: LoginByCode — 查手机号，没注册就自动注册**

```go
func (s *AuthService) LoginByCode(phone string) (accessToken, refreshToken string, user *model.User, err error) {
    var u model.User
    result := database.DB.Where("phone = ?", phone).First(&u)
    if errors.Is(result.Error, gorm.ErrRecordNotFound) {
        u = model.User{
            Username:     fmt.Sprintf("user_%s", phone[7:]),
            Phone:        phone,
            PasswordHash: randomBcryptHash(),
            Role:         "student",
        }
        database.DB.Create(&u)
    } else if result.Error != nil {
        return "", "", nil, result.Error
    }

    accessToken, _ = utils.GenerateAccessToken(u.ID, u.Username, u.Role)
    refreshToken, _ = utils.GenerateRefreshToken()
    expiresAt := time.Now().Add(utils.RefreshTTL())
    database.DB.Model(&u).Updates(map[string]interface{}{
        "refresh_token": refreshToken, "refresh_expires_at": &expiresAt,
    })
    return accessToken, refreshToken, &u, nil
}
```

- [ ] **Step 2: Refresh — 验证旧 token，生成新双 token**

```go
func (s *AuthService) Refresh(oldRefreshToken string) (accessToken, newRefreshToken string, err error) {
    var u model.User
    if err := database.DB.Where("refresh_token = ?", oldRefreshToken).First(&u).Error; err != nil {
        return "", "", errors.New("无效的refresh_token")
    }
    if u.RefreshExpiresAt == nil || time.Now().After(*u.RefreshExpiresAt) {
        return "", "", errors.New("refresh_token已过期，请重新登录")
    }

    accessToken, _ = utils.GenerateAccessToken(u.ID, u.Username, u.Role)
    newRefreshToken, _ = utils.GenerateRefreshToken()
    expiresAt := time.Now().Add(utils.RefreshTTL())
    database.DB.Model(&u).Updates(map[string]interface{}{
        "refresh_token": newRefreshToken, "refresh_expires_at": &expiresAt,
    })
    return accessToken, newRefreshToken, nil
}
```

---

### Task 6: Controller — 4 个 Handler

**Files:**
- Modify: `controller/auth_controller.go`

- [ ] **Step 1: GenerateCaptcha — GET /api/captcha/image**

```go
func (ctr *AuthController) GenerateCaptcha(c *gin.Context) {
    id, b64s, err := utils.GenerateImageCaptcha()
    if err != nil {
        utils.InternalError(c, "图形验证码生成失败")
        return
    }
    utils.Success(c, gin.H{"captcha_id": id, "captcha_image": b64s})
}
```

- [ ] **Step 2: SendCode — 先验图形码再发短信码**

```go
func (ctr *AuthController) SendCode(c *gin.Context) {
    var req request.SendCodeReq
    // ... ShouldBindJSON 校验 ...
    if !utils.VerifyImageCaptcha(req.CaptchaID, req.CaptchaCode) {
        utils.BadRequest(c, "图形验证码错误或已过期")
        return
    }
    if err := ctr.svc.SendCode(req.Phone); err != nil {
        utils.BadRequest(c, err.Error())
        return
    }
    utils.Success(c, gin.H{"message": "验证码已发送"})
}
```

- [ ] **Step 3: LoginByCode — 统一登录入口**

```go
func (ctr *AuthController) LoginByCode(c *gin.Context) {
    var req request.LoginByCodeReq
    // ... ShouldBindJSON ...
    if !utils.CaptchaStore.Verify(req.Phone, req.Code) {
        utils.BadRequest(c, "验证码错误或已过期")
        return
    }
    accessToken, refreshToken, user, err := ctr.svc.LoginByCode(req.Phone)
    // ... 返回 { access_token, refresh_token, user } ...
}
```

- [ ] **Step 4: Refresh — 刷新双 Token**

```go
func (ctr *AuthController) Refresh(c *gin.Context) {
    var req request.RefreshReq
    // ... ShouldBindJSON ...
    accessToken, refreshToken, err := ctr.svc.Refresh(req.RefreshToken)
    if err != nil {
        utils.Unauthorized(c, err.Error())
        return
    }
    utils.Success(c, gin.H{"access_token": accessToken, "refresh_token": refreshToken})
}
```

---

### Task 7: Router — 精简为 4 条公开路由

**Files:**
- Modify: `router/router.go:30-46`

- [ ] **Step 1: 替换公开路由区域**

```go
// 公开接口
api.GET("/captcha/image", authCtrl.GenerateCaptcha)
api.POST("/send-code", authCtrl.SendCode)
api.POST("/login", authCtrl.LoginByCode)
api.POST("/refresh", authCtrl.Refresh)
api.GET("/courses", courseCtrl.List)
api.GET("/courses/:id", courseCtrl.GetByID)
api.GET("/courses/:id/reviews", reviewCtrl.ListByCourse)
api.GET("/categories", categoryCtrl.List)
```

删除的旧路由：
- `POST /api/register`
- `POST /api/register/phone`
- `POST /api/login/phone`
- 旧 `POST /api/login`（密码登录）

---

### Task 8: 前端 — Login.vue 统一页面

**Files:**
- Modify: `web/src/views/Login.vue`（全部重写）
- Delete: `web/src/views/Register.vue`
- Modify: `web/src/router/index.js`（删除 register 路由）
- Modify: `web/src/components/Navbar.vue`（"登录"+"注册"→"登录/注册"）
- Modify: `web/src/api/auth.js`（重写 API）
- Modify: `web/src/stores/user.js`（单 token → 双 token）
- Modify: `web/src/api/index.js`（加 401 自动刷新拦截器）

- [ ] **Step 1: api/auth.js — 4 个新 API**

```javascript
export function getCaptcha() { return api.get('/captcha/image') }
export function sendCode(data) { return api.post('/send-code', data) }
export function loginByCode(data) { return api.post('/login', data) }
export function refreshToken(token) { return api.post('/refresh', { refresh_token: token }) }
```

- [ ] **Step 2: stores/user.js — 双 token 存储**

```javascript
const accessToken = ref(localStorage.getItem('access_token') || '')
const refreshToken = ref(localStorage.getItem('refresh_token') || '')

function setAuth(access, refresh, newUser) {
    accessToken.value = access
    refreshToken.value = refresh
    user.value = newUser
    localStorage.setItem('access_token', access)
    localStorage.setItem('refresh_token', refresh)
    localStorage.setItem('user', JSON.stringify(newUser))
}
```

- [ ] **Step 3: api/index.js — 401 自动刷新拦截器**

核心逻辑：当接口返回 401 时，用 refresh_token 调 /api/refresh，拿到新 access_token 后重试原请求。如果正在刷新中（isRefreshing），后续请求排队等待。

```javascript
if (response?.status === 401 && !config._retry) {
    config._retry = true
    const data = await refreshToken(userStore.refreshToken)
    userStore.updateAccessToken(data.data.access_token, data.data.refresh_token)
    config.headers.Authorization = `Bearer ${userStore.accessToken}`
    return api(config)
}
```

- [ ] **Step 4: Login.vue — 单页面 + 图形验证码弹窗**

流程：
1. 用户输入手机号 → 点"发送验证码"
2. 弹出图形验证码 overlay（点击图片可刷新）
3. 输入正确图形码 → 后端发短信验证码 → 开始 60s 倒计时
4. 用户输入短信验证码 → 点"登录 / 注册"
5. 后端返回双 token + user → 存入 localStorage → 跳转首页

- [ ] **Step 5: 删除 Register.vue，更新路由和 Navbar**

```bash
rm web/src/views/Register.vue
```

Navbar 改为单个 "登录 / 注册" 链接，router 删除 register 路由。

- [ ] **Step 6: 启动验证**

```bash
cd web && npm run dev
# 访问 http://localhost:5173/login
# 流程：手机号 → 图形验证码 → 短信验证码 → 登录成功
```

---

### Task 9: 测试 — 适配新 API

**Files:**
- Modify: `service/auth_service_test.go`（全部重写）
- Modify: `service/setup_test.go`（加双 token TTL 配置）
- Modify: `utils/jwt_test.go`（GenerateToken → GenerateAccessToken）
- Modify: `middleware/jwt_test.go`（同上）

- [ ] **Step 1: auth_service_test.go — 5 个新测试**

```
TestSendCodeSuccess       - 发验证码
TestLoginByCodeNewUser    - 新手机号自动注册+双token
TestLoginByCodeExistingUser - 已有用户再次登录
TestRefreshToken          - 用旧refresh换新双token
TestRefreshTokenInvalid   - 无效refresh被拒
```

- [ ] **Step 2: 运行全部测试**

```bash
go test ./... -count=1 -timeout 60s
# 预期: ok  edu_market/middleware
#       ok  edu_market/service
#       ok  edu_market/utils
```
