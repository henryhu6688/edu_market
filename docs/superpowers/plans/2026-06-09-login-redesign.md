# 登录注册功能重构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 4 条认证路径统一为手机号+验证码单入口，新用户自动注册，加入双 token + 图形验证码

**Architecture:** 前端 Login.vue 单体页面，后端 4 条精简路由（captcha/image / send-code / login / refresh），access_token(30min JWT) + refresh_token(24h 随机字符串) 双 token 滚动刷新

**Tech Stack:** Go + Gin + GORM + MySQL + Redis + base64Captcha / Vue3 + Vite + Pinia + Axios / slog + lumberjack

---

### Task 1: Config — 双 Token TTL + 验证码配置

**Files:**
- Modify: `config/config.go:15-19, 52-73`
- Modify: `config/app.yml:20-35`

- [ ] **Step 1: JWTConfig 加 access_ttl_minutes 和 refresh_ttl_hours**

```go
// JWTConfig JWT 配置
type JWTConfig struct {
    Secret          string `mapstructure:"secret"`
    ExpireHours     int    `mapstructure:"expire_hours"`
    AccessTTLMin    int    `mapstructure:"access_ttl_minutes"`
    RefreshTTLHours int    `mapstructure:"refresh_ttl_hours"`
}
```

- [ ] **Step 2: 新增 CaptchaConfig**

```go
// CaptchaConfig 验证码配置
type CaptchaConfig struct {
    Length         int `mapstructure:"length"`
    ExpireSeconds  int `mapstructure:"expire_seconds"`
    ResendSeconds  int `mapstructure:"resend_seconds"`
}
```

- [ ] **Step 3: Config 结构体加 Captcha 字段**

```go
type Config struct {
    Server   ServerConfig   `mapstructure:"server"`
    Database DatabaseConfig `mapstructure:"database"`
    Redis    RedisConfig    `mapstructure:"redis"`
    JWT      JWTConfig      `mapstructure:"jwt"`
    AI       AIConfig       `mapstructure:"ai"`
    Upload   UploadConfig   `mapstructure:"upload"`
    Captcha  CaptchaConfig  `mapstructure:"captcha"`
}
```

- [ ] **Step 4: 更新 app.yml**

```yaml
jwt:
  secret: edu-market-secret-key-change-in-production
  expire_hours: 24
  access_ttl_minutes: 30
  refresh_ttl_hours: 24

captcha:
  length: 6
  expire_seconds: 300
  resend_seconds: 60
```

- [ ] **Step 5: 编译验证**

```bash
go build ./...
```

---

### Task 2: Model — User 加 RefreshToken + 全模型 CASCADE 外键

**Files:**
- Modify: `model/user.go:6-18`
- Modify: `model/course.go:22-23`
- Modify: `model/order.go:23-24`
- Modify: `model/review.go:15-16`
- Modify: `model/conversation.go:16`

- [ ] **Step 1: User 结构体 — Email 改 nullable，加双字段**

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

- [ ] **Step 2: 所有模型外键加 OnDelete:CASCADE**

Course:
```go
Category Category `gorm:"foreignKey:CategoryID;constraint:OnDelete:CASCADE" json:"category,omitempty"`
User     User     `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
```

Order:
```go
User   User   `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
Course Course `gorm:"foreignKey:CourseID;constraint:OnDelete:CASCADE" json:"course,omitempty"`
```

Review:
```go
User   User   `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
Course Course `gorm:"foreignKey:CourseID;constraint:OnDelete:CASCADE" json:"course,omitempty"`
```

Conversation:
```go
User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
```

- [ ] **Step 3: 编译验证**

```bash
go build ./...  # AutoMigrate 会加新列 + 更新外键约束
```

---

### Task 3: Utils — JWT 双 Token + 图形验证码 + 短信验证码

**Files:**
- Modify: `utils/jwt.go`
- Modify: `utils/captcha.go`（短信 CodeStore + 图形验证码）
- Create: `utils/logger.go`

- [ ] **Step 1: jwt.go — GenerateAccessToken + ParseToken + GenerateRefreshToken + RefreshTTL**

```go
type Claims struct {
    UserID   uint   `json:"user_id"`
    Username string `json:"username"`
    Role     string `json:"role"`
    jwt.RegisteredClaims
}

func GenerateAccessToken(userID uint, username, role string) (string, error) {
    ttl := config.App.JWT.AccessTTLMin
    if ttl <= 0 { ttl = 30 }
    claims := Claims{
        UserID:   userID, Username: username, Role: role,
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(ttl) * time.Minute)),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
            Issuer:    "edu_market",
        },
    }
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(config.App.JWT.Secret))
}

func ParseToken(tokenString string) (*Claims, error) {
    token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
        return []byte(config.App.JWT.Secret), nil
    })
    if err != nil { return nil, err }
    claims, ok := token.Claims.(*Claims)
    if !ok || !token.Valid { return nil, errors.New("无效的Token") }
    return claims, nil
}

func GenerateRefreshToken() (string, error) {
    b := make([]byte, 32)
    if _, err := rand.Read(b); err != nil { return "", err }
    return hex.EncodeToString(b), nil
}

func RefreshTTL() time.Duration {
    ttl := config.App.JWT.RefreshTTLHours
    if ttl <= 0 { ttl = 24 }
    return time.Duration(ttl) * time.Hour
}
```

- [ ] **Step 2: captcha.go — CodeStore（含限频）**

```go
type CodeStore struct {
    codeLen  int
    ttl      time.Duration
    interval time.Duration
}

func NewCodeStore(codeLen int, ttlSeconds, intervalSeconds int) *CodeStore {
    return &CodeStore{
        codeLen:  codeLen,
        ttl:      time.Duration(ttlSeconds) * time.Second,
        interval: time.Duration(intervalSeconds) * time.Second,
    }
}

func (s *CodeStore) codeKey(phone string) string  { return fmt.Sprintf("captcha:sms:%s", phone) }
func (s *CodeStore) limitKey(phone string) string  { return fmt.Sprintf("captcha:smslimit:%s", phone) }

func (s *CodeStore) Generate(phone string) (string, error) {
    if database.RDB == nil { return "", fmt.Errorf("Redis 未连接") }
    ctx := context.Background()
    // 检查限频
    exists, _ := database.RDB.Exists(ctx, s.limitKey(phone)).Result()
    if exists > 0 { return "", fmt.Errorf("验证码发送过于频繁，请%d秒后再试", int(s.interval.Seconds())) }
    // 生成 + 存储
    code := s.randomCode()
    database.RDB.Set(ctx, s.codeKey(phone), code, s.ttl)
    database.RDB.Set(ctx, s.limitKey(phone), "1", s.interval)
    log.Printf("[短信验证码] 手机号 %s 的验证码: %s", phone, code)
    return code, nil
}

func (s *CodeStore) Verify(phone, code string) bool {
    if database.RDB == nil { return false }
    ctx := context.Background()
    stored, err := database.RDB.Get(ctx, s.codeKey(phone)).Result()
    if err != nil || stored != code { return false }
    database.RDB.Del(ctx, s.codeKey(phone))
    database.RDB.Del(ctx, s.limitKey(phone))
    return true
}
```

- [ ] **Step 3: captcha.go — 图形验证码（base64Captcha）**

```go
import "github.com/mojocn/base64Captcha"

const imgCaptchaTTL = 2 * time.Minute
var imgCaptchaStore = base64Captcha.DefaultMemStore

func GenerateImageCaptcha() (captchaID string, b64s string, err error) {
    imgCaptchaStore = base64Captcha.NewMemoryStore(100, imgCaptchaTTL)
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

- [ ] **Step 4: captcha.go — 全局初始化**

```go
var CaptchaStore *CodeStore

func InitCaptcha() {
    cfg := config.App.Captcha
    if cfg.Length == 0 { cfg.Length = 6 }
    if cfg.ExpireSeconds == 0 { cfg.ExpireSeconds = 300 }
    if cfg.ResendSeconds == 0 { cfg.ResendSeconds = 60 }
    CaptchaStore = NewCodeStore(cfg.Length, cfg.ExpireSeconds, cfg.ResendSeconds)
    log.Printf("验证码存储器初始化完成 (长度:%d 有效期:%ds 间隔:%ds)", cfg.Length, cfg.ExpireSeconds, cfg.ResendSeconds)
}
```

- [ ] **Step 5: logger.go — slog + lumberjack 结构化日志**

```go
func InitLogger() {
    logDir := "logs"
    os.MkdirAll(logDir, 0755)
    fileWriter := &lumberjack.Logger{
        Filename:   filepath.Join(logDir, "app.log"),
        MaxSize:    10, MaxBackups: 30, MaxAge: 7, Compress: true,
    }
    // 开发: 文本双写; 生产: JSON 写文件
    var handler slog.Handler
    if config.App.Server.Mode == "release" {
        handler = slog.NewJSONHandler(fileWriter, &slog.HandlerOptions{Level: slog.LevelInfo})
    } else {
        handler = slog.NewTextHandler(io.MultiWriter(os.Stdout, fileWriter), &slog.HandlerOptions{Level: slog.LevelDebug})
    }
    logger := slog.New(handler)
    slog.SetDefault(logger)
    log.SetOutput(io.MultiWriter(os.Stdout, fileWriter))
    log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}
```

- [ ] **Step 6: 安装依赖 + 编译**

```bash
go get github.com/mojocn/base64Captcha
go get gopkg.in/natefinch/lumberjack.v2
go build ./...
```

---

### Task 4: DTO — 精简为 3 个请求结构体

**Files:**
- Modify: `dto/request/auth.go`（全部重写）

- [ ] **Step 1: 删除旧 DTO，写 3 个新结构体**

```go
package request

type SendCodeReq struct {
    Phone      string `json:"phone" binding:"required,len=11"`
    CaptchaID  string `json:"captcha_id" binding:"required"`
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

- [ ] **Step 2: 编译**

```bash
go build ./...
```

---

### Task 5: Service — 统一入口 + Refresh

**Files:**
- Modify: `service/auth_service.go`（全部重写）

- [ ] **Step 1: SendCode — 调 CodeStore 生成验证码**

```go
type AuthService struct{}

func (s *AuthService) SendCode(phone string) error {
    _, err := utils.CaptchaStore.Generate(phone)
    return err
}
```

- [ ] **Step 2: LoginByCode — 查手机号，新用户自动注册**

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
        if err := database.DB.Create(&u).Error; err != nil {
            return "", "", nil, errors.New("自动注册失败")
        }
    } else if result.Error != nil {
        return "", "", nil, result.Error
    }

    accessToken, err = utils.GenerateAccessToken(u.ID, u.Username, u.Role)
    if err != nil { return "", "", nil, errors.New("Token生成失败") }
    refreshToken, err = utils.GenerateRefreshToken()
    if err != nil { return "", "", nil, errors.New("RefreshToken生成失败") }

    expiresAt := time.Now().Add(utils.RefreshTTL())
    database.DB.Model(&u).Updates(map[string]interface{}{
        "refresh_token": refreshToken, "refresh_expires_at": &expiresAt,
    })
    return accessToken, refreshToken, &u, nil
}

func randomBcryptHash() string {
    raw := fmt.Sprintf("%08d", rand.Intn(100000000))
    hash, _ := bcrypt.GenerateFromPassword([]byte(raw), bcrypt.DefaultCost)
    return string(hash)
}
```

- [ ] **Step 3: Refresh — 验证旧 token，滚动更新**

```go
func (s *AuthService) Refresh(oldRefreshToken string) (accessToken, newRefreshToken string, err error) {
    var u model.User
    if err := database.DB.Where("refresh_token = ?", oldRefreshToken).First(&u).Error; err != nil {
        return "", "", errors.New("无效的refresh_token")
    }
    if u.RefreshExpiresAt == nil || time.Now().After(*u.RefreshExpiresAt) {
        return "", "", errors.New("refresh_token已过期，请重新登录")
    }

    accessToken, err = utils.GenerateAccessToken(u.ID, u.Username, u.Role)
    if err != nil { return "", "", errors.New("Token生成失败") }
    newRefreshToken, err = utils.GenerateRefreshToken()
    if err != nil { return "", "", errors.New("RefreshToken生成失败") }

    expiresAt := time.Now().Add(utils.RefreshTTL())
    database.DB.Model(&u).Updates(map[string]interface{}{
        "refresh_token": newRefreshToken, "refresh_expires_at": &expiresAt,
    })
    return accessToken, newRefreshToken, nil
}
```

- [ ] **Step 4: 编译**

```bash
go build ./...
```

---

### Task 6: Controller — 4 个 Handler

**Files:**
- Modify: `controller/auth_controller.go`（全部重写）

- [ ] **Step 1: GenerateCaptcha — GET /api/captcha/image**

```go
type AuthController struct {
    svc service.AuthService
}

func (ctr *AuthController) GenerateCaptcha(c *gin.Context) {
    id, b64s, err := utils.GenerateImageCaptcha()
    if err != nil {
        utils.InternalError(c, "图形验证码生成失败")
        return
    }
    utils.Success(c, gin.H{"captcha_id": id, "captcha_image": b64s})
}
```

- [ ] **Step 2: SendCode — 先验图形码，再发短信码**

```go
func (ctr *AuthController) SendCode(c *gin.Context) {
    var req request.SendCodeReq
    if err := c.ShouldBindJSON(&req); err != nil {
        utils.BadRequest(c, "参数校验失败: "+err.Error())
        return
    }
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
    if err := c.ShouldBindJSON(&req); err != nil {
        utils.BadRequest(c, "参数校验失败: "+err.Error())
        return
    }
    if !utils.CaptchaStore.Verify(req.Phone, req.Code) {
        utils.BadRequest(c, "验证码错误或已过期")
        return
    }
    accessToken, refreshToken, user, err := ctr.svc.LoginByCode(req.Phone)
    if err != nil {
        utils.BadRequest(c, err.Error())
        return
    }
    utils.Success(c, gin.H{
        "access_token": accessToken, "refresh_token": refreshToken,
        "user": gin.H{"id": user.ID, "username": user.Username, "phone": user.Phone, "role": user.Role, "avatar": user.Avatar},
    })
}
```

- [ ] **Step 4: Refresh — 刷新双 Token**

```go
func (ctr *AuthController) Refresh(c *gin.Context) {
    var req request.RefreshReq
    if err := c.ShouldBindJSON(&req); err != nil {
        utils.BadRequest(c, "参数校验失败: "+err.Error())
        return
    }
    accessToken, refreshToken, err := ctr.svc.Refresh(req.RefreshToken)
    if err != nil {
        utils.Unauthorized(c, err.Error())
        return
    }
    utils.Success(c, gin.H{"access_token": accessToken, "refresh_token": refreshToken})
}
```

- [ ] **Step 5: 编译**

```bash
go build ./...
```

---

### Task 7: Router — 公开路由精简 + Main 启动顺序

**Files:**
- Modify: `router/router.go:30-46`
- Modify: `main.go:14-41`

- [ ] **Step 1: router.go — 4 条公开路由替换旧 6 条**

```go
api := r.Group("/api")
{
    // 公开接口
    api.GET("/captcha/image", authCtrl.GenerateCaptcha)
    api.POST("/send-code", authCtrl.SendCode)
    api.POST("/login", authCtrl.LoginByCode)
    api.POST("/refresh", authCtrl.Refresh)
    api.GET("/courses", courseCtrl.List)
    api.GET("/courses/:id", courseCtrl.GetByID)
    api.GET("/courses/:id/reviews", reviewCtrl.ListByCourse)
    api.GET("/categories", categoryCtrl.List)

    // 需认证...
    // 管理员...
}
```

删除的旧路由：`POST /api/register`, `POST /api/register/phone`, `POST /api/login/phone`, 旧 `POST /api/login`

- [ ] **Step 2: main.go — 启动顺序加 InitLogger 和 InitCaptcha**

```go
func main() {
    config.Load()
    utils.InitLogger()
    if err := database.InitRedis(); err != nil {
        log.Fatalf("Redis 连接失败: %v", err)
    }
    utils.InitCaptcha()
    database.Init()
    r := router.Setup()
    // ...
}
```

- [ ] **Step 3: .gitignore — 排除 logs/**

```
# --- Logs ---
logs/
```

- [ ] **Step 4: 编译**

```bash
go build ./...
```

---

### Task 8: 前端 — 统一登录页 + 双 Token Store + 401 刷新

**Files:**
- Create: `web/src/views/Login.vue`（重写为统一入口）
- Delete: `web/src/views/Register.vue`
- Modify: `web/src/api/auth.js`（4 个 API）
- Modify: `web/src/api/index.js`（加 401 刷新拦截器）
- Modify: `web/src/stores/user.js`（单 token → 双 token）
- Modify: `web/src/router/index.js`（删除 register 路由，accessToken 守卫）
- Modify: `web/src/components/Navbar.vue`（"登录"+"注册"→"登录/注册"）

- [ ] **Step 1: api/auth.js — 4 个 API**

```javascript
import api from './index'

export function getCaptcha() { return api.get('/captcha/image') }
export function sendCode(data) { return api.post('/send-code', data) }
export function loginByCode(data) { return api.post('/login', data) }
export function refreshToken(refreshToken) { return api.post('/refresh', { refresh_token: refreshToken }) }
```

- [ ] **Step 2: stores/user.js — 双 token**

```javascript
export const useUserStore = defineStore('user', () => {
  const accessToken = ref(localStorage.getItem('access_token') || '')
  const refreshToken = ref(localStorage.getItem('refresh_token') || '')
  const user = ref(JSON.parse(localStorage.getItem('user') || 'null'))

  const isLoggedIn = computed(() => !!accessToken.value)
  const isAdmin = computed(() => user.value?.role === 'admin')

  function setAuth(access, refresh, newUser) {
    accessToken.value = access; refreshToken.value = refresh; user.value = newUser
    localStorage.setItem('access_token', access)
    localStorage.setItem('refresh_token', refresh)
    localStorage.setItem('user', JSON.stringify(newUser))
  }

  function updateAccessToken(access, refresh) {
    accessToken.value = access; refreshToken.value = refresh
    localStorage.setItem('access_token', access)
    if (refresh) localStorage.setItem('refresh_token', refresh)
  }

  function logout() {
    accessToken.value = ''; refreshToken.value = ''; user.value = null
    localStorage.removeItem('access_token')
    localStorage.removeItem('refresh_token')
    localStorage.removeItem('user')
  }

  return { accessToken, refreshToken, user, isLoggedIn, isAdmin, setAuth, updateAccessToken, logout }
})
```

- [ ] **Step 3: api/index.js — 401 自动刷新拦截器**

```javascript
// 请求拦截器 — 自动带 accessToken
api.interceptors.request.use(config => {
  const userStore = useUserStore()
  if (userStore.accessToken) {
    config.headers.Authorization = `Bearer ${userStore.accessToken}`
  }
  return config
})

// 响应拦截器 — 401 自动刷新，并发排队
let isRefreshing = false
let refreshQueue = []

api.interceptors.response.use(
  response => response.data,
  async error => {
    const { config, response } = error
    if (response?.status === 401 && !config._retry) {
      const userStore = useUserStore()
      if (userStore.refreshToken) {
        config._retry = true
        if (isRefreshing) {
          return new Promise((resolve, reject) => {
            refreshQueue.push({ resolve, reject })
          }).then(() => {
            config.headers.Authorization = `Bearer ${userStore.accessToken}`
            return api(config)
          })
        }
        isRefreshing = true
        try {
          const data = await refreshToken(userStore.refreshToken)
          userStore.updateAccessToken(data.data.access_token, data.data.refresh_token)
          refreshQueue.forEach(q => q.resolve()); refreshQueue = []
          config.headers.Authorization = `Bearer ${userStore.accessToken}`
          return api(config)
        } catch (e) {
          refreshQueue.forEach(q => q.reject(e)); refreshQueue = []
          userStore.logout(); router.push('/login')
          return Promise.reject(e)
        } finally { isRefreshing = false }
      } else { userStore.logout(); router.push('/login') }
    }
    return Promise.reject(response?.data || error)
  }
)
```

- [ ] **Step 4: router/index.js — 守卫用 accessToken**

```javascript
router.beforeEach((to, from, next) => {
  const userStore = useUserStore()
  if (to.meta.auth && !userStore.accessToken) return next('/login')
  if (to.meta.admin && userStore.user?.role !== 'admin') return next('/')
  if (to.meta.guest && userStore.accessToken) return next('/')
  next()
})
```

- [ ] **Step 5: Login.vue — 统一入口 + 图形验证码弹窗**

流程：输手机号 → 点"发送验证码" → 弹出图形验证码 overlay（点击可刷新） → 解图形码 → 后端存短信码到 Redis → 开始 60s 倒计时 → 输短信码 → 点"登录/注册" → 返回双 token → 存 localStorage → 跳首页

- [ ] **Step 6: 删除 Register.vue，更新 Navbar**

```bash
rm web/src/views/Register.vue
```

Navbar 改为单个"登录 / 注册"链接。

---

### Task 9: 测试 — 适配新 API + TestMain 调整

**Files:**
- Modify: `service/setup_test.go`
- Modify: `service/auth_service_test.go`
- Modify: `utils/jwt_test.go`
- Modify: `middleware/jwt_test.go`

- [ ] **Step 1: setup_test.go — TestMain 加 CaptchaConfig 和 Redis 降级**

```go
func TestMain(m *testing.M) {
    config.App = &config.Config{
        Server:   config.ServerConfig{Port: 8080, Mode: "test"},
        Database: config.DatabaseConfig{Host: "127.0.0.1", Port: 3306, User: "root", Password: "123456", DBName: testDBName, Charset: "utf8mb4"},
        Redis:    config.RedisConfig{Addr: "127.0.0.1:6379", Password: "", DB: 2},
        JWT:      config.JWTConfig{Secret: "test-secret-key", AccessTTLMin: 30, RefreshTTLHours: 24},
        Captcha:  config.CaptchaConfig{Length: 6, ExpireSeconds: 300, ResendSeconds: 1},
    }
    // Redis 不可用时打印警告，不阻塞测试
    if err := database.InitRedis(); err != nil {
        log.Printf("警告: Redis 未连接 (%v)，验证码相关测试将跳过", err)
    }
    utils.InitCaptcha()
    createTestDB()
    database.Init()
    cleanAllTestData()
    code := m.Run()
    cleanAllTestData()
    if database.RDB != nil { database.RDB.FlushDB(context.Background()) }
}
```

- [ ] **Step 2: auth_service_test.go — 5 个测试**

```
TestSendCodeSuccess        — 发送短信验证码成功
TestSendCodeRateLimit      — 60s 内重复发送被拒
TestLoginByCodeNewUser     — 新手机号自动注册 + 返回双 token
TestLoginByCodeExistingUser — 已有用户登录 + 返回双 token
TestRefreshToken           — 用旧 refresh 换新双 token（旧 token 失效）
```

- [ ] **Step 3: jwt_test.go 和 middleware/jwt_test.go — 适配新函数名**

```go
// 旧: utils.GenerateToken(...)
// 新: utils.GenerateAccessToken(userID, username, role)
```

- [ ] **Step 4: 运行全部测试**

```bash
go test ./... -count=1 -timeout 60s
# 预期: ok  edu_market/middleware
#       ok  edu_market/service
#       ok  edu_market/utils
```

---

### Task 10: 文档 + 最终验证

- [ ] **Step 1: 更新 CLAUDE.md** — 加入 InitLogger 启动顺序、logs/ 目录说明、accessToken 路由守卫

- [ ] **Step 2: 全栈启动验证**

```bash
# 后端
taskkill //F //IM edu_market.exe 2>/dev/null; taskkill //F //IM main.exe 2>/dev/null
go run .
# 确认日志出现: "Redis 连接成功" + "验证码存储器初始化完成"

# 前端
cd web && npm run dev

# 浏览器: http://localhost:5173/login
# 流程: 手机号 → 图形验证码 → 短信验证码 → 登录成功 → 首页
```

- [ ] **Step 3: 最终测试**

```bash
go test ./... -count=1
```

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "feat: 重构登录注册 — 统一手机号入口 + 双Token + 图形验证码"
```
