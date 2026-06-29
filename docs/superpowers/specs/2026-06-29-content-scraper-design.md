# 在线资料爬取 + RAG 入库工具

> 2026-06-29 | brainstorming 产出

## 目标

一个可复用的 Go CLI 工具，从多个在线源爬取编程技术文档，转为 Markdown，创建 Material/Document 记录，走完整 RAG 离线管线入库（清洗 → 切片 → Embedding → Qdrant + MySQL document_chunks）。

## 使用场景

- 初始导入：空库灌几十篇教学资料，把平台跑起来
- 后续追加：实现新的 `Scraper` 适配器即可加新源

## CLI 接口

```bash
go run scripts/scrape.go --source=github     # 仅 GitHub 源
go run scripts/scrape.go --source=runoob     # 仅菜鸟教程
go run scripts/scrape.go --source=all        # 全部源
go run scripts/scrape.go --source=all --dry-run     # 只打印，不写库
go run scripts/scrape.go --source=all --skip-rag    # 入库但不向量化
go run scripts/scrape.go --source=all --force       # 覆盖已存在 Material
```

默认行为：增量模式，已存在同名 Material 则跳过。

## 架构

### 文件组织

```
scripts/scrape.go                 CLI 入口（flag 解析、初始化、调度）
scripts/scrape/
    scraper.go                    Scraper 接口 + Article 结构体
    github.go                     GitHub 文档源
    runoob.go                     菜鸟教程源
    pipeline.go                   核心管线：Article → Material → Document → IndexCourse
```

### 核心抽象

```go
// Scraper 数据源适配接口
type Scraper interface {
    Name() string
    Fetch(ctx context.Context) ([]*Article, error)
}

// Article 一篇被抓取的内容
type Article struct {
    Title       string      // Material/Document 标题
    Content     string      // Markdown 正文，存 Document.Content
    Description string      // Material.Description
    Category    string      // 分类名，映射到 categories 表
    Price       float64
    Sections    []*Article  // 子章节 → 同一个 Material 下的多个 Document
}
```

每个源实现 `Scraper` 接口。加新站 = 写一个新适配器文件。核心管线完全复用。

### 核心管线

```
Article
  → 查 CategoryID（按 Name 匹配 categories 表）
  → 查 UserID（按 phone 查 users 表）
  → 增量检查（同名 Material 已存在 → 跳过或覆盖）
  → database.DB.Create(&Material)
  → 遍历 Sections（或自身）: database.DB.Create(&Document)
  → rag.Get().IndexCourse(materialID, documentID, plainText)
      内部: cleanMarkdown → structuralChunk → embedTexts(bge-m3) → Qdrant + MySQL
```

### 错误处理

| 层级 | 策略 |
|------|------|
| 单篇文章抓取失败 | 日志告警，跳过，继续下一篇 |
| 整个源初始化失败 | 日志告警，跳过该源 |
| DB 写入失败 | 日志告警，跳过该条 |
| RAG 索引失败 | 日志告警，Document 已存，可后续用 `index_docs.go` 补救 |

不中断原则：任何错误不整体中断。最终输出汇总：成功 X / 失败 Y / 跳过（已存在）Z。

## 数据源

### 源一：GitHub 项目文档

**目标仓库：** 8-10 个中文/编程项目（gin、gorm、vue、kubernetes、python、rust 等），每个项目 `docs/` 目录或根目录下的 `.md` 文件。

**抓取策略：**
1. GitHub REST API 获取仓库文件树（`GET /repos/{owner}/{repo}/git/trees/{branch}?recursive=1`）
2. 筛选 .md 文件，排除 node_modules、vendor 等
3. GitHub raw URL 直接下载（纯文本，零解析成本）
4. 一个仓库 = 一个 Material，每个 .md = 一个 Document
5. 文件名为 Document.Title，`#` 一级标题优先
6. 去重键：repo_full_name + Material.Title

**Content 处理：** Markdown 原文直接存，不需要转码。

### 源二：菜鸟教程

**目标入口：** `https://www.runoob.com/` 下 3-5 个技术栈教程（python3、go、linux、docker、git）。

**页面结构：** 左侧目录树（`<div class="design">` 中的链接列表），正文区 `<div class="article-body">`。一个教程 30-80 篇文章。

**抓取策略：**
1. 访问教程首页，解析左侧目录树得到所有章节 URL
2. 并发抓取每篇文章（并发限制 3）
3. `goquery` 提取 `.article-body` HTML
4. `html-to-markdown` 转 Markdown
5. 过滤导航/广告/翻页噪声
6. 一个教程 = 一个 Material，每篇文章 = 一个 Document
7. 目录树层级 → SortOrder 保持顺序
8. 代码块（`<pre>`）保留为 Markdown 代码块

## 配置约定

- UserID：通过手机号 `13620996835` 查 `users` 表获取
- 定价：Material.Price = 0（免费）
- 状态：Material.Status = "published"
- 分类：Article.Category 映射到 `categories.name`

## 依赖新增

- `github.com/PuerkitoBio/goquery` — HTML 解析（菜鸟教程）
- `github.com/JohannesKaufmann/html-to-markdown` — HTML → Markdown

## 初始化顺序

```
1. config.Load()           ← config/app.yml
2. database.InitRedis()    ← 可选，失败不 Fatal
3. database.Init()         ← MySQL + AutoMigrate
4. rag.Init()              ← Qdrant + Collection 检查
5. 查询 user_id + categories 缓存
6. 逐个源 Scraper.Fetch() → pipeline.Process()
```

全部复用项目现有模块（config、database、model、service/rag）。

## 产出预期

| 源 | 预计 Material 数 | 预计 Document 数 |
|----|:---:|:---:|
| GitHub (8-10 仓库) | 8-10 | 80-150 |
| 菜鸟教程 (3-5 教程) | 3-5 | 90-200 |
| **合计** | **11-15** | **170-350** |

## 验证方式

1. `go run scripts/scrape.go --source=all --dry-run` — 确认抓取逻辑和文章数
2. `go run scripts/scrape.go --source=all` — 实际入库
3. 检查 MySQL `materials`、`documents`、`document_chunks` 表有数据
4. 检查 Qdrant collection 点数增长
5. 启动前端，在 Agent 对话中搜索爬到的内容，验证 RAG 检索可用
