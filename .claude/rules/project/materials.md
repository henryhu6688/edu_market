# 学习资料 + 在线文档

## 数据模型

```
materials                     documents
┌──────────────────┐         ┌──────────────────┐
│ id               │         │ id               │
│ title            │         │ material_id      │── FK → materials
│ description      │         │ parent_id        │── FK → documents (null=顶层)
│ price            │         │ title            │
│ cover_image      │         │ content          │── Markdown 文本
│ category_id      │         │ sort_order       │
│ user_id (发布者)  │         │ is_free_preview  │── 试读标记
│ status           │         │ status           │
│ view_count       │         │ created_at       │
│ buy_count        │         │ updated_at       │
└──────────────────┘         └──────────────────┘
```

## 编辑器

ByteMD (Vue3)，所见即所得 Markdown 编辑，类似 Typora。

- 左侧文档树（parent_id 层级）
- 右侧编辑区实时预览
- 2s 防抖自动保存 `PUT /api/documents/:id`
- 支持新建、删除、导入文件

## 文档解析器

| 格式 | 解析方式 |
|------|---------|
| .txt .md | 直接读取 |
| .docx | nguyenthenguyen/docx → 从 `<w:t>` 标签提取文字 |
| .pdf | 系统 pdftotext 命令 (-layout -enc UTF-8) |
| .pptx | archive/zip 解压 → 从 slide XML `<a:t>` 提取文字 |

上传后自动转为 Markdown 存入 `content` 字段。

## 权限模型

| 用户类型 | 查看文档 | 编辑文档 | 说明 |
|---------|---------|---------|------|
| 发布者 | ✅ | ✅ | 自己的资料 |
| 已购买 | ✅ | ❌ | 买后可看全文 |
| 未购买(试读) | ✅ 部分 | ❌ | `is_free_preview=true` 的文档 |
| 未购买 | ❌ | ❌ | 返回 "请先购买" |

## RAG 集成

文档保存 → `extractTextFromMarkdown()` 提取纯文本 → 切片(500字/块) → Embedding → 存 `document_chunks` 表。Agent 通过 `search_documents` tool 检索。

## 相关文件

- `model/material.go` `model/document.go` — 数据模型
- `service/material_service.go` `service/document_service.go` — Service
- `service/document_parser.go` — 文件解析
- `controller/material_controller.go` `controller/document_controller.go` — Controller
- `web/src/views/DocumentEditor.vue` — 编辑器
- `web/src/views/DocumentView.vue` — 阅读器
- `web/src/views/MaterialList.vue` — 资料列表
