# 学习资料 + 在线文档编辑器 设计

> 日期: 2026-06-11
> 状态: 设计完成
> 分支: v2_RAG

## 概述

三个核心改动：

- **角色放开** — student 改为 user，所有人可发布和购买学习资料
- **在线文档** — 语雀式所见即所得编辑器，文档目录树，自动保存
- **RAG 集成** — 文档内容自动切片入库，Agent 可检索回答

## 数据模型

### User 角色

```go
// role 字段不变，只改含义
Role string `gorm:"type:varchar(20);default:user;not null" json:"role"`
// user  | admin
// user  = 可发布资料 + 可购买资料
// admin = user 全部权限 + 管理后台
```

### materials（重命名自 courses）

```go
type Material struct {
    ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
    Title       string    `gorm:"type:varchar(200);not null;index" json:"title"`
    Description string    `gorm:"type:text" json:"description"`
    Price       float64   `gorm:"type:decimal(10,2);not null;default:0" json:"price"`
    CoverImage  string    `gorm:"type:varchar(255)" json:"cover_image"`
    CategoryID  uint      `gorm:"not null;index" json:"category_id"`
    UserID      uint      `gorm:"not null;index" json:"user_id"` // 发布者
    Status      string    `gorm:"type:varchar(20);default:draft;not null" json:"status"` // draft | published | off
    ViewCount   int       `gorm:"default:0" json:"view_count"`
    BuyCount    int       `gorm:"default:0" json:"buy_count"`
    CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
    UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`

    Category Category   `gorm:"foreignKey:CategoryID;constraint:OnDelete:CASCADE" json:"category,omitempty"`
    User     User       `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
    Documents []Document `gorm:"foreignKey:MaterialID;constraint:OnDelete:CASCADE" json:"documents,omitempty"`
}

func (Material) TableName() string { return "materials" }
```

### documents（新增）

```go
type Document struct {
    ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
    MaterialID    uint      `gorm:"not null;index" json:"material_id"`
    ParentID      *uint     `gorm:"index;default:null" json:"parent_id"` // nil=顶层
    Title         string    `gorm:"type:varchar(200);not null" json:"title"`
    Content       string    `gorm:"type:longtext" json:"content"`        // Tiptap JSON
    SortOrder     int       `gorm:"default:0" json:"sort_order"`
    IsFreePreview bool      `gorm:"default:false" json:"is_free_preview"`
    Status        string    `gorm:"type:varchar(20);default:draft" json:"status"` // draft | published
    CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
    UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`

    Material Material  `gorm:"foreignKey:MaterialID;constraint:OnDelete:CASCADE" json:"-"`
    Children []Document `gorm:"foreignKey:ParentID;constraint:OnDelete:SET NULL" json:"children,omitempty"`
}

func (Document) TableName() string { return "documents" }
```

### 迁移说明

- 旧 `courses` 表重命名为 `materials`（保留数据，新增字段被忽略）
- 旧 `FileURL` 字段保留但不再使用（向后兼容）
- `DocumentChunk` 保持不变，从 `document.content` 提取纯文本再切片

## 权限模型

| 角色 | 买资料 | 发资料 | 管理后台 | 看文档（自己发的） | 看文档（买的） |
|------|--------|--------|----------|-------------------|---------------|
| user | ✅ | ✅ | ❌ | ✅ | ✅ |
| admin | ✅ | ✅ | ✅ | ✅ | ✅ |

文档访问权限：
```
GET /api/documents/:id
  ├─ doc.IsFreePreview    → 任何人可看（登录即可）
  ├─ material.UserID == me → 发布者，可看
  ├─ hasPurchased(me)     → 已购买，可看
  └─ else                 → 403 "请先购买"
```

## 在线文档编辑器

### 技术选型

**Tiptap**（基于 ProseMirror，Vue 3 原生支持）

- 所见即所得富文本：标题、列表、代码块、图片、表格
- 内容存储为 JSON（`documents.content` 字段，MySQL LONGTEXT）
- MIT 开源，社区活跃

### 编辑页面布局

```
┌──────────────┬──────────────────────────────┐
│  文档目录      │     编辑区                     │
│              │                              │
│ 📁 第一章     │  # 第一章 快速入门              │
│  ├ 📄 1.1概述 │                              │
│  ├ 📄 1.2安装 │  这是第一章的内容...            │
│  └ 📄 1.3配置 │                              │
│ 📁 第二章     │                              │
│  ├ 📄 2.1语法 │                              │
│  └ 📄 2.2实战 │                              │
│              │                              │
│ [+ 新建文档]  │  [自动保存] [已保存] [发布]    │
└──────────────┴──────────────────────────────┘
```

### 自动保存

- 前端监听 Tiptap `onUpdate` 事件
- 2 秒 debounce，发送 `PUT /api/documents/:id`（仅 content 字段）
- 后端保存后触发 RAG 重新切片（异步或同步）
- 状态指示：💾 保存中... / ✅ 已保存

### RAG 集成

```
文档保存
  → 从 Tiptap JSON 提取纯文本
  → 清空该文档旧 chunks
  → 切片（500字/块，50字重叠）
  → 调 Embedding API 生成向量
  → 存 document_chunks 表 + 向量索引
```

Agent 问答时 `search_course_materials` 参数从 `course_id` 改为 `material_id`。

## API 设计

### 资料（materials）

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/api/materials` | 无 | 资料列表（替代 /api/courses） |
| GET | `/api/materials/:id` | 无 | 资料详情 |
| POST | `/api/materials` | JWT | 发布资料（user/admin 均可） |
| PUT | `/api/materials/:id` | JWT | 编辑资料（仅发布者/admin） |
| DELETE | `/api/materials/:id` | JWT | 删除资料（仅发布者/admin） |
| GET | `/api/materials/:id/reviews` | 无 | 资料评价 |

旧 `/api/courses` 路由保留做 301 重定向或兼容代理。

### 文档（documents）

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/api/materials/:mid/documents` | 无 | 文档目录树（不含 content） |
| GET | `/api/documents/:id` | JWT | 文档详情 + content（校验购买） |
| POST | `/api/materials/:mid/documents` | JWT | 新建文档（仅发布者） |
| PUT | `/api/documents/:id` | JWT | 编辑文档（仅发布者，自动保存） |
| DELETE | `/api/documents/:id` | JWT | 删除文档（仅发布者） |

### 管理后台

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| ... | `/api/admin/*` | Admin | 不变，admin 独有 |

## 前端改造

### 新增页面

| 页面 | 路由 | 说明 |
|------|------|------|
| MaterialList | `/materials` | 资料列表（原 Course 页面改） |
| MaterialDetail | `/materials/:id` | 资料详情 + 购买 |
| MaterialEditor | `/materials/:id/edit` | 资料信息编辑 |
| DocumentEditor | `/materials/:id/docs` | 文档编辑器（左侧树 + 右侧编辑） |
| DocumentView | `/materials/:id/docs/:did` | 文档阅读视图（购买后） |
| PublishMaterial | `/materials/new` | 发布新资料 |

### 修改页面

| 页面 | 改动 |
|------|------|
| Navbar | "发布资料"入口对 user 可见（原 admin 后台入口不动） |
| Home | "课程" → "学习资料" |
| Profile | "我的资料"tab（已发布 + 已购买） |

### 依赖

```bash
npm install @tiptap/vue-3 @tiptap/starter-kit @tiptap/extension-placeholder @tiptap/extension-image @tiptap/extension-table @tiptap/extension-code-block
```

## 配置

```yaml
# config/app.yml 新增
document:
  auto_save_delay: 2  # 自动保存延迟（秒）
  rag_sync: true       # 保存时同步触发 RAG 切片
```

## 测试策略

| 测试文件 | 内容 |
|---------|------|
| `service/material_service_test.go` | 资料 CRUD、权限校验 |
| `service/document_service_test.go` | 文档 CRUD、权限校验（购买后/试读/发布者） |
| `service/document_rag_test.go` | 文档保存触发 RAG 切片 |

## 向后兼容

- 旧 `courses` 表 → 保留，数据不动（新功能走 `materials`）
- 旧 `/api/courses` 路由 → 保留但不推荐使用
- `Conversation` 模型已删除，旧 AI 功能不可用（v2 Agent 替代）
- `DocumentChunk` 表不变，RAG 继续工作

## 文件上传转在线文档

### 流程

```
用户上传 PDF/PPTX/DOCX/MD/TXT
  → 后端提取纯文本
  → 文本按段落转为 Tiptap JSON
  → 自动创建一篇 Document
  → 跳转到编辑器，用户可继续修改
```

### 支持的格式与 Go 库

| 格式 | 库 | 说明 |
|------|-----|------|
| `.txt` `.md` | 标准库 | 直接读取 |
| `.pdf` | `ledongthuc/pdf` | 提取文本内容 |
| `.pptx` | `baliance/gooxml` | 提取幻灯片文本 |
| `.docx` | `baliance/gooxml` | 提取段落文本 |

### 文本 → Tiptap JSON 转换

按双换行拆段，每段一个 paragraph：

```
输入文本:
"第一章\n\n这是内容。"

输出 Tiptap JSON:
{
  "type": "doc",
  "content": [
    { "type": "paragraph", "content": [{ "type": "text", "text": "第一章" }] },
    { "type": "paragraph", "content": [{ "type": "text", "text": "这是内容。" }] }
  ]
}
```

单换行在同一 paragraph 内用 `hardBreak` 处理。

### 上传入口

文档编辑器页面左侧文档树顶部加"📎 导入文件"按钮。

### API

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| POST | `/api/materials/:mid/documents/upload` | JWT | 上传文件转文档（仅发布者） |

请求：`multipart/form-data`，字段 `file`。
响应：创建的 Document 对象（含 ID），前端拿到 ID 跳转编辑器。

### 配置

```yaml
document:
  max_upload_size: 20971520  # 20MB
  allowed_formats: [".pdf", ".pptx", ".docx", ".md", ".txt"]
```

---

## 已知限制

| 限制 | 后续方向 |
|------|---------|
| 无多人实时协作 | v3 |
| 无版本历史 | v3 |
| 文档内图片上传待定 | 先用外链图片，后续加图床 |
| Tiptap JSON 与 RAG 纯文本转换 | 需写 extractor |
