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

文档保存后完整链路（`service/rag/` 独立包）：

```
上传 → document_parser（PDF/DOCX/TXT，不支持PPTX）
  → 解析层清洗（cleanPDF/DOCX — 页眉页脚/页码/硬换行/水印/目录）
  → OCR 降级（PDF 不可读时 tesseract）
  → Markdown 清洗（cleaner.go — 图片/链接/格式符号/全角半角）
  → 结构切片（chunker.go — MD按#标题 / DOCX按pStyle / PDF按段落）
  → batch Embedding (bge-m3, 并发3)
  → Qdrant (向量 + payload) + MySQL document_chunks (备份)
```

检索链路：

```
search_documents(query, material_id)
  → L1 精确缓存(Redis) / L2 语义缓存(余弦≥0.85)
  → Qdrant 向量检索 + text 全文过滤(混合检索 RRF)
  → Rerank (bge-reranker-v2-m3, 10→3)
  → 元数据拼装（DocumentID→批量查标题 + SectionPath → 来源引用）
```

DocumentChunk 新增 `document_id`（来源文档）+ `section_path`（章节路径），检索结果直接带来源标注。

## 相关文件

- `model/material.go` `model/document.go` `model/document_chunk.go` — 数据模型
- `service/material_service.go` `service/document_service.go` — Service
- `service/document_parser.go` — 文件解析（清洗 + OCR 降级）
- `service/rag/` — RAG 检索服务（Qdrant + Embedding + Rerank + 切片 + 缓存 + 清洗）
- `controller/material_controller.go` `controller/document_controller.go` — Controller
- `web/src/views/DocumentEditor.vue` — 编辑器
- `web/src/views/DocumentView.vue` — 阅读器
- `web/src/views/MaterialList.vue` — 资料列表
