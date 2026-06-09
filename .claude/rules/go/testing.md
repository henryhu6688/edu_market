# Go 测试约定

> 提炼自 service/*_test.go、utils/*_test.go、middleware/*_test.go

## 测试数据库

所有集成测试使用独立数据库 `edu_market_test`，不与开发库混用：

```go
const testDBName = "edu_market_test"
```

TestMain 负责创建和销毁：

```go
func TestMain(m *testing.M) {
    // 1. 手动构造 config（不依赖 config.Load）
    config.App = &config.Config{ ... Database.DBName: testDBName ... }
    
    // 2. 初始化 Redis（测试用独立 DB，如 DB 2）
    database.InitRedis()
    utils.InitCaptcha()
    
    // 3. 创建测试库（CREATE DATABASE IF NOT EXISTS）
    createTestDB()
    
    // 4. AutoMigrate
    database.Init()
    
    // 5. 跑测试
    code := m.Run()
    
    // 6. 清理：按外键顺序删除
    cleanAllTestData()
    
    // 7. 清空 Redis 测试库
    database.RDB.FlushDB(context.Background())
}
```

## 数据清理顺序

按**外键依赖顺序**从子到父删除，先关 FK 检查再清理：

```go
func cleanAllTestData() {
    database.DB.Exec("SET FOREIGN_KEY_CHECKS = 0")
    database.DB.Where("1=1").Delete(&model.Review{})
    database.DB.Where("1=1").Delete(&model.Order{})
    database.DB.Where("1=1").Delete(&model.Conversation{})
    database.DB.Where("1=1").Delete(&model.Course{})
    database.DB.Where("1=1").Delete(&model.Category{})
    database.DB.Where("1=1").Delete(&model.User{})
    database.DB.Exec("SET FOREIGN_KEY_CHECKS = 1")
}
```

- `Where("1=1").Delete()` 用于硬删除（避开软删除的模型）
- 每个测试函数开始前先删自己的关联数据，避免残留影响

## Redis 测试约定

- 测试环境用 **独立 DB**（如 DB 2），`config.Redis.DB: 2`
- Redis 不可用时打印警告，跳过验证码相关测试，不阻塞其他测试
- 测试结束 `FlushDB()` 清空

## 测试配置

测试不依赖 `config/app.yml`，直接在 TestMain 中手动构造：

```go
config.App = &config.Config{
    Server:   config.ServerConfig{Port: 8080, Mode: "test"},
    Database: config.DatabaseConfig{
        Host: "127.0.0.1", Port: 3306, User: "root", Password: "123456",
        DBName: testDBName, Charset: "utf8mb4",
    },
    Redis: config.RedisConfig{
        Addr: "127.0.0.1:6379", Password: "", DB: 2,
    },
    JWT: config.JWTConfig{Secret: "test-secret-key", ...},
}
```

## 测试文件组织

- 测试文件与源码同目录：`service/user_service_test.go`
- `setup_test.go` 放 TestMain（一个包一个）
- 运行：`go test ./...` 一键全跑
