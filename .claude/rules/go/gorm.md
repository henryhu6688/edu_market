# GORM 使用约定

> 提炼自 service 层和 model 层代码

## 数据库访问

全局使用 `database.DB`，无 Repository 层包装。Service 直接操作：

```go
database.DB.Where("phone = ?", phone).First(&u)
database.DB.Create(&course).Error
```

## GORM 错误处理

必须区分 "记录不存在" 与其他数据库错误：

```go
result := database.DB.Where("phone = ?", phone).First(&u)
if errors.Is(result.Error, gorm.ErrRecordNotFound) {
    // 记录不存在 — 通常是正常分支（新用户注册等）
} else if result.Error != nil {
    // 真正的数据库错误
    return result.Error
}
```

- **用 `errors.Is(err, gorm.ErrRecordNotFound)`**，不用字符串比较
- 其他 GORM 错误统一当数据库异常处理

## 更新操作

使用 `Updates(map[string]interface{})` 做部分更新，避免零值覆盖问题：

```go
database.DB.Model(&u).Updates(map[string]interface{}{
    "refresh_token":      newToken,
    "refresh_expires_at": &expiresAt,
})
```

- 不用 `Save()`（会覆盖所有字段）
- 不用 `UpdateColumn()`（除非要绕过 Hook）
- `struct` 更新会跳过零值字段，注意布尔/整数零值场景

## 查询模式

```go
// 单条查询 — 用 First
database.DB.Where("phone = ?", phone).First(&u)

// 列表查询 — 用 Find，先 Order 后 Find
database.DB.Order("id ASC").Find(&categories)

// 分页 — 先 Count 再 Offset/Limit
database.DB.Model(&model.Course{}).Where(...).Count(&total)
database.DB.Where(...).Offset(offset).Limit(limit).Find(&list)

// 预加载 — 用 Preload
database.DB.Preload("Category").Preload("User").First(&course, id)
```

## 模型定义

```go
type User struct {
    ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
    Username  string    `gorm:"type:varchar(50);uniqueIndex;not null" json:"username"`
    Role      string    `gorm:"type:varchar(20);default:student;not null" json:"role"`
    CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
    UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (User) TableName() string { return "users" }
```

- 每个模型必须定义 `TableName()`，表名用蛇形复数
- 时间字段用 `autoCreateTime` / `autoUpdateTime`
- 唯一约束用 `uniqueIndex`，非空用 `not null`
- 敏感字段 `json:"-"` 排序列化
- 外键关联需设置 `OnDelete:CASCADE`（在父模型中）

## 软删除

需保留记录的实体使用 GORM 软删除（如 Order）：

```go
import "gorm.io/gorm"

type Order struct {
    ...
    DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
```

- `database.DB.Delete(&order)` → 设 `deleted_at`，不真删
- 查询自动过滤已软删除记录
- 清理测试数据时用 `Where("1=1").Delete(&model.Order{})` 可避开软删除
