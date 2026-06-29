# 在线资料爬取 + RAG 入库工具 — 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建可复用 CLI 工具，从 GitHub + 菜鸟教程爬取编程文档，自动创建 Material/Document 并走完整 RAG 管线入库。

**Architecture:** 5 个文件，`scrape` 库包 + `scrape.go` CLI 入口。Scraper 接口适配多源，Pipeline 统一处理 Article → Material → Document → IndexCourse。复用项目 config/database/model/service/rag 全栈模块。

**Tech Stack:** Go 1.25.6, goquery (HTML 解析), html-to-markdown (HTML→MD), GORM, Qdrant REST API

## Global Constraints

- UserID: 通过手机号 `13620996835` 查 users 表
- 定价: 按内容总字数自适应 — 短篇(<5000字) ¥9.90, 中篇(5000-15000字) ¥19.90, 长篇(>15000字) ¥29.90
- 状态: Material.Status = "published", 所有 Document 也为 published
- 分类: Article.Category 映射到 categories.name
- 错误处理: 单篇/单源失败不中断整体流程，最终输出汇总
- 增量: 默认按 Material.Title + UserID 去重跳过，`--force` 覆盖
- RAG: 传原始 Markdown 给 IndexCourse（利用 structural_chunk 按 # 标题切片）

## File Map

| 文件 | 职责 | 新建/修改 |
|------|------|----------|
| `scripts/scrape/scraper.go` | Scraper 接口 + Article 结构体 + 定价函数 | 新建 |
| `scripts/scrape/pipeline.go` | 核心管线：Article → Material → Document → IndexCourse | 新建 |
| `scripts/scrape/github.go` | GitHub 源：API 获取文件树 → 下载 .md → 组装 Article | 新建 |
| `scripts/scrape/runoob.go` | 菜鸟教程源：解析目录树 → 抓取正文 → HTML 转 MD | 新建 |
| `scripts/scrape.go` | CLI 入口：flag 解析、初始化、调度、汇总 | 新建 |
| `go.mod` | 新增 goquery + html-to-markdown 依赖 | 修改 |

---

### Task 1: 新增依赖

**Files:**
- Modify: `go.mod`

**Interfaces:**
- Produces: `github.com/PuerkitoBio/goquery` (HTML 解析), `github.com/JohannesKaufmann/html-to-markdown` (HTML→MD)

- [ ] **Step 1: 添加依赖**

```bash
cd d:\Vscoding\edu_market
go get github.com/PuerkitoBio/goquery
go get github.com/JohannesKaufmann/html-to-markdown
```

- [ ] **Step 2: 验证编译**

```bash
go build ./...
```
Expected: 编译成功（新包未被主代码引用，不影响现有构建）

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add goquery + html-to-markdown for scraper"
```

---

### Task 2: Scraper 接口 + Article 结构体 + 定价函数

**Files:**
- Create: `scripts/scrape/scraper.go`

**Interfaces:**
- Produces: `Scraper` interface, `Article` struct, `calculatePrice(contentLen int) float64`

- [ ] **Step 1: 创建 scraper.go**

```go
// Package scrape 提供可复用的在线资料爬取 + RAG 入库管线。
package scrape

import "context"

// Scraper 数据源适配接口。每个数据源实现此接口，核心管线完全复用。
type Scraper interface {
	// Name 返回数据源名称（用于日志和统计）。
	Name() string
	// Fetch 从数据源抓取所有文章。ctx 用于取消和超时控制。
	Fetch(ctx context.Context) ([]*Article, error)
}

// Article 一篇被抓取的内容。可以包含子章节（Section），
// 有子章节时自身不生成 Document，只有 Section 生成。
type Article struct {
	Title       string     // Material/Document 标题
	Content     string     // Markdown 正文，直接存 Document.Content
	Description string     // Material.Description（取 Content 前 200 字）
	Category    string     // 分类名，映射到 categories.name
	Price       float64    // 若为 0 则管线自动按长度计算
	Sections    []*Article // 子章节 → 同一个 Material 下的多个 Document
}

// calculatePrice 按内容总长度自适应定价。
// 短篇（<5000字）¥9.90，中篇（5000-15000字）¥19.90，长篇（>15000字）¥29.90。
func calculatePrice(totalLen int) float64 {
	if totalLen < 5000 {
		return 9.90
	}
	if totalLen < 15000 {
		return 19.90
	}
	return 29.90
}
```

- [ ] **Step 2: 验证编译**

```bash
cd d:\Vscoding\edu_market
go build ./scripts/scrape/
```
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add scripts/scrape/scraper.go
git commit -m "feat: Scraper 接口 + Article 结构体 + 自适应定价"
```

---

### Task 3: Pipeline 核心管线

**Files:**
- Create: `scripts/scrape/pipeline.go`

**Interfaces:**
- Consumes: `Scraper` interface, `Article` struct, `calculatePrice` from scraper.go
- Consumes: `config.App`, `database.DB`, `rag.Get()`, `model.Material`, `model.Document`
- Produces: `Pipeline` struct with `Process(*Article) (created, skipped int, error)`

- [ ] **Step 1: 创建 pipeline.go**

```go
package scrape

import (
	"fmt"
	"log/slog"
	"strings"

	"edu_market/config"
	"edu_market/database"
	"edu_market/model"
	"edu_market/service/rag"

	"gorm.io/gorm"
)

// Pipeline 核心管线：Article → Material → Document → RAG 索引。
type Pipeline struct {
	db     *gorm.DB
	rag    *rag.RAGService
	userID uint
	cats   map[string]uint // name → id
	force  bool
	dryRun bool
	skipRAG bool
}

// NewPipeline 创建管线。force=true 时覆盖已存在的 Material。
func NewPipeline(force, dryRun, skipRAG bool) (*Pipeline, error) {
	// 查 UserID
	var user model.User
	if err := database.DB.Where("phone = ?", "13620996835").First(&user).Error; err != nil {
		return nil, fmt.Errorf("用户不存在 (phone=13620996835): %w", err)
	}

	// 缓存分类映射
	var categories []model.Category
	database.DB.Find(&categories)
	cats := make(map[string]uint, len(categories))
	for _, c := range categories {
		cats[c.Name] = c.ID
	}

	return &Pipeline{
		db:      database.DB,
		rag:     rag.Get(),
		userID:  user.ID,
		cats:    cats,
		force:   force,
		dryRun:  dryRun,
		skipRAG: skipRAG,
	}, nil
}

// Process 处理一篇文章（及其子章节），返回 (created, skipped, error)。
func (p *Pipeline) Process(article *Article) (int, int, error) {
	if p.dryRun {
		p.logDryRun(article)
		return 0, 0, nil
	}

	// 1. 查分类
	catID, ok := p.cats[article.Category]
	if !ok {
		return 0, 0, fmt.Errorf("分类不存在: %s", article.Category)
	}

	// 2. 增量检查
	var existing model.Material
	if err := p.db.Where("title = ? AND user_id = ?", article.Title, p.userID).First(&existing).Error; err == nil {
		if !p.force {
			slog.Info("跳过已存在", "title", article.Title)
			return 0, 1, nil
		}
		// force 模式：级联删除旧 Material（Documents + chunks 自动清理）
		p.db.Select("Documents").Delete(&existing)
		slog.Info("覆盖已存在", "title", article.Title)
	}

	// 3. 计算价格
	price := article.Price
	if price == 0 {
		totalLen := len([]rune(article.Content))
		for _, s := range article.Sections {
			totalLen += len([]rune(s.Content))
		}
		price = calculatePrice(totalLen)
	}

	// 4. 确保 Description 有值
	desc := article.Description
	if desc == "" {
		desc = truncate(article.Content, 200)
	}

	// 5. 创建 Material
	m := &model.Material{
		Title:       article.Title,
		Description: desc,
		Price:       price,
		CategoryID:  catID,
		UserID:      p.userID,
		Status:      "published",
	}
	if err := p.db.Create(m).Error; err != nil {
		return 0, 0, fmt.Errorf("创建 Material 失败: %w", err)
	}

	// 6. 构建 Document 列表
	docs := article.Sections
	if len(docs) == 0 {
		// 无子章节 → 自身作为一个 Document
		docs = []*Article{article}
	}

	created := 0
	for i, sec := range docs {
		doc := &model.Document{
			MaterialID: m.ID,
			Title:      sec.Title,
			Content:    sec.Content,
			SortOrder:  i,
			Status:     "published",
		}
		if err := p.db.Create(doc).Error; err != nil {
			slog.Warn("创建 Document 失败", "title", sec.Title, "error", err)
			continue
		}
		created++

		// 7. RAG 索引（传原始 Markdown，structural_chunk 利用 # 标题）
		if !p.skipRAG && p.rag != nil {
			if err := p.rag.IndexCourse(m.ID, doc.ID, sec.Content); err != nil {
				slog.Warn("RAG 索引失败（Document 已存，可后续 index_docs.go 补救）",
					"doc_id", doc.ID, "error", err)
			}
		}
	}

	slog.Info("Material 创建完成", "title", m.Title, "docs", created, "price", price)
	return created, 0, nil
}

func (p *Pipeline) logDryRun(article *Article) {
	n := 1
	if len(article.Sections) > 0 {
		n = len(article.Sections)
	}
	fmt.Printf("[DRY-RUN] %s | %s | %d doc(s)\n", article.Title, article.Category, n)
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(string(runes[:n])) + "..."
}
```

- [ ] **Step 2: 验证编译**

```bash
cd d:\Vscoding\edu_market
go build ./scripts/scrape/
```
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add scripts/scrape/pipeline.go
git commit -m "feat: Pipeline 核心管线 Article→Material→Document→RAG"
```

---

### Task 4: GitHub 文档源

**Files:**
- Create: `scripts/scrape/github.go`

**Interfaces:**
- Consumes: `Scraper` interface, `Article` struct from scraper.go
- Produces: `NewGitHubScraper() *GitHubScraper`

- [ ] **Step 1: 创建 github.go**

```go
package scrape

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// repo 一个 GitHub 仓库抓取目标。
type repo struct {
	Owner  string
	Name   string
	Branch string // 默认 "main"
	Cat    string // 分类名
}

// GitHubScraper 从 GitHub 仓库抓取 Markdown 文档。
type GitHubScraper struct {
	client  *http.Client
	repos   []repo
}

// NewGitHubScraper 创建 GitHub 源适配器。
func NewGitHubScraper() *GitHubScraper {
	return &GitHubScraper{
		client: &http.Client{Timeout: 30 * time.Second},
		repos: []repo{
			{Owner: "gin-gonic", Name: "gin", Branch: "master", Cat: "编程开发"},
			{Owner: "go-gorm", Name: "gorm", Branch: "master", Cat: "编程开发"},
			{Owner: "golang", Name: "go", Branch: "master", Cat: "编程开发"},
			{Owner: "vuejs", Name: "core", Branch: "main", Cat: "编程开发"},
			{Owner: "kubernetes", Name: "website", Branch: "main", Cat: "编程开发"},
			{Owner: "rust-lang", Name: "book", Branch: "main", Cat: "编程开发"},
			{Owner: "python", Name: "cpython", Branch: "main", Cat: "编程开发"},
			{Owner: "torvalds", Name: "linux", Branch: "master", Cat: "编程开发"},
			{Owner: "gohugoio", Name: "hugo", Branch: "master", Cat: "编程开发"},
			{Owner: "avelino", Name: "awesome-go", Branch: "main", Cat: "编程开发"},
		},
	}
}

func (g *GitHubScraper) Name() string { return "github" }

func (g *GitHubScraper) Fetch(ctx context.Context) ([]*Article, error) {
	var all []*Article
	for _, r := range g.repos {
		articles, err := g.fetchRepo(ctx, r)
		if err != nil {
			slog.Warn("GitHub 仓库抓取失败，跳过", "repo", r.Owner+"/"+r.Name, "error", err)
			continue
		}
		all = append(all, articles...)
	}
	return all, nil
}

// ghTreeItem GitHub API tree 响应中的单条。
type ghTreeItem struct {
	Path string `json:"path"`
	Type string `json:"type"` // "blob" or "tree"
	URL  string `json:"url"`
}

// ghTreeResp GitHub API tree 响应。
type ghTreeResp struct {
	Tree []ghTreeItem `json:"tree"`
}

func (g *GitHubScraper) fetchRepo(ctx context.Context, r repo) ([]*Article, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/%s?recursive=1",
		r.Owner, r.Name, r.Branch)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "edu-market-scraper")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("GitHub API %d: %s", resp.StatusCode, string(body))
	}

	var tree ghTreeResp
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		return nil, err
	}

	// 筛选 docs/ 和根目录 .md 文件
	var mdFiles []string
	for _, item := range tree.Tree {
		if item.Type != "blob" || !strings.HasSuffix(item.Path, ".md") {
			continue
		}
		// 排除 vendored/test 目录
		if strings.Contains(item.Path, "vendor/") ||
			strings.Contains(item.Path, "node_modules/") ||
			strings.Contains(item.Path, "testdata/") {
			continue
		}
		// 仅取 docs/ 和根目录
		if strings.HasPrefix(item.Path, "docs/") || !strings.Contains(item.Path, "/") {
			mdFiles = append(mdFiles, item.Path)
		}
	}

	if len(mdFiles) == 0 {
		return nil, fmt.Errorf("无 .md 文件")
	}

	// 每个 .md 文件 → Article Section
	var sections []*Article
	for _, path := range mdFiles {
		rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s",
			r.Owner, r.Name, r.Branch, path)
		content, err := g.download(ctx, rawURL)
		if err != nil {
			slog.Warn("下载失败，跳过", "url", rawURL, "error", err)
			continue
		}
		title := mdTitle(path, content)
		sections = append(sections, &Article{
			Title:   title,
			Content: content,
		})
	}

	if len(sections) == 0 {
		return nil, fmt.Errorf("所有文件下载失败")
	}

	slog.Info("GitHub 仓库抓取完成", "repo", r.Owner+"/"+r.Name, "files", len(sections))

	// 一个仓库 = 一个 Material，每个 .md = 一个 Document
	return []*Article{{
		Title:       r.Owner + "/" + r.Name + " 文档",
		Description: fmt.Sprintf("%s/%s 项目文档，共 %d 篇", r.Owner, r.Name, len(sections)),
		Category:    r.Cat,
		Sections:    sections,
	}}, nil
}

func (g *GitHubScraper) download(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "edu-market-scraper")
	resp, err := g.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024)) // 512KB limit
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// mdTitle 从文件路径和内容提取标题：优先用第一个 # 标题，否则用文件名。
func mdTitle(path string, content string) string {
	lines := strings.SplitN(content, "\n", 10)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return strings.TrimSuffix(filepath.Base(path), ".md")
}
```

- [ ] **Step 2: 验证编译**

```bash
cd d:\Vscoding\edu_market
go build ./scripts/scrape/
```
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add scripts/scrape/github.go
git commit -m "feat: GitHub 文档源（10仓库 .md → Article）"
```

---

### Task 5: 菜鸟教程源

**Files:**
- Create: `scripts/scrape/runoob.go`

**Interfaces:**
- Consumes: `Scraper` interface, `Article` struct from scraper.go
- Consumes: `goquery`, `html-to-markdown` (new deps from Task 1)
- Produces: `NewRunoobScraper() *RunoobScraper`

- [ ] **Step 1: 创建 runoob.go**

```go
package scrape

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
)

// tutorial 菜鸟教程一个技术栈教程。
type tutorial struct {
	Name string   // 教程名，如 "Python3 教程"
	URL  string   // 首页 URL
	Cat  string   // 分类名
}

// RunoobScraper 从 runoob.com 抓取教程。
type RunoobScraper struct {
	client      *http.Client
	tutorials   []tutorial
	converter   *md.Converter
}

// NewRunoobScraper 创建菜鸟教程源适配器。
func NewRunoobScraper() *RunoobScraper {
	converter := md.NewConverter("", true, nil)
	return &RunoobScraper{
		client:    &http.Client{Timeout: 30 * time.Second},
		converter: converter,
		tutorials: []tutorial{
			{Name: "Python3 教程", URL: "https://www.runoob.com/python3/python3-tutorial.html", Cat: "编程开发"},
			{Name: "Go 语言教程", URL: "https://www.runoob.com/go/go-tutorial.html", Cat: "编程开发"},
			{Name: "Linux 教程", URL: "https://www.runoob.com/linux/linux-tutorial.html", Cat: "编程开发"},
			{Name: "Docker 教程", URL: "https://www.runoob.com/docker/docker-tutorial.html", Cat: "编程开发"},
			{Name: "Git 教程", URL: "https://www.runoob.com/git/git-tutorial.html", Cat: "编程开发"},
			{Name: "MySQL 教程", URL: "https://www.runoob.com/mysql/mysql-tutorial.html", Cat: "编程开发"},
			{Name: "Redis 教程", URL: "https://www.runoob.com/redis/redis-tutorial.html", Cat: "编程开发"},
		},
	}
}

func (r *RunoobScraper) Name() string { return "runoob" }

func (r *RunoobScraper) Fetch(ctx context.Context) ([]*Article, error) {
	var all []*Article
	for _, t := range r.tutorials {
		article, err := r.fetchTutorial(ctx, t)
		if err != nil {
			slog.Warn("菜鸟教程抓取失败，跳过", "tutorial", t.Name, "error", err)
			continue
		}
		all = append(all, article)
	}
	return all, nil
}

// sidebarLink 左侧目录树中的链接。
type sidebarLink struct {
	Title string
	URL   string
}

func (r *RunoobScraper) fetchTutorial(ctx context.Context, t tutorial) (*Article, error) {
	// 1. 获取首页，解析左侧目录树
	links, err := r.parseSidebar(ctx, t.URL)
	if err != nil {
		return nil, fmt.Errorf("解析目录失败: %w", err)
	}
	if len(links) == 0 {
		return nil, fmt.Errorf("目录为空")
	}

	slog.Info("菜鸟教程目录解析完成", "tutorial", t.Name, "chapters", len(links))

	// 2. 并发抓取每篇文章（限并发 3）
	sem := make(chan struct{}, 3)
	var mu sync.Mutex
	var sections []*Article

	for _, link := range links {
		sem <- struct{}{}
		go func(l sidebarLink) {
			defer func() { <-sem }()
			content, err := r.fetchPage(ctx, l.URL)
			if err != nil {
				slog.Warn("文章抓取失败", "title", l.Title, "url", l.URL, "error", err)
				return
			}
			mu.Lock()
			sections = append(sections, &Article{
				Title:   l.Title,
				Content: content,
			})
			mu.Unlock()
		}(link)
	}
	// 等待所有 goroutine 完成
	for i := 0; i < 3; i++ {
		sem <- struct{}{}
	}

	slog.Info("菜鸟教程章节抓取完成", "tutorial", t.Name, "sections", len(sections))

	if len(sections) == 0 {
		return nil, fmt.Errorf("所有文章抓取失败")
	}

	return &Article{
		Title:       t.Name,
		Description: fmt.Sprintf("%s，共 %d 篇", t.Name, len(sections)),
		Category:    t.Cat,
		Sections:    sections,
	}, nil
}

// parseSidebar 解析菜鸟教程左侧目录树，提取所有章节链接。
func (r *RunoobScraper) parseSidebar(ctx context.Context, indexURL string) ([]sidebarLink, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", indexURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "edu-market-scraper")
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var links []sidebarLink
	seen := make(map[string]bool)

	// 菜鸟教程左侧目录树在 .design 或 .left-column 中
	doc.Find(".design a, .left-column a, .sidebar a, #leftcolumn a").Each(func(i int, s *goquery.Selection) {
		href, ok := s.Attr("href")
		if !ok {
			return
		}
		title := strings.TrimSpace(s.Text())
		if title == "" || seen[href] {
			return
		}
		seen[href] = true

		// 补全相对 URL
		fullURL := href
		if !strings.HasPrefix(href, "http") {
			if strings.HasPrefix(href, "/") {
				fullURL = "https://www.runoob.com" + href
			} else {
				// 相对路径，基于 indexURL
				base := indexURL[:strings.LastIndex(indexURL, "/")+1]
				fullURL = base + href
			}
		}
		links = append(links, sidebarLink{Title: title, URL: fullURL})
	})

	return links, nil
}

// fetchPage 抓取单篇文章并转 Markdown。
func (r *RunoobScraper) fetchPage(ctx context.Context, pageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", pageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "edu-market-scraper")
	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024)) // 1MB limit
	if err != nil {
		return "", err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}

	// 提取正文区（菜鸟教程正文在 .article-body）
	contentSel := doc.Find(".article-body, .content, article, .article, main")
	if contentSel.Length() == 0 {
		// 回退到 <body>
		contentSel = doc.Find("body")
	}

	// 删除噪声元素
	contentSel.Find("script, style, .ads, .advertisement, nav, .nav, " +
		".prevnext, .pagination, .page-nav, .next-link, .prev-link, " +
		".footer, .header, .reply, #respond, .comment, .sidebar").Remove()

	// HTML → Markdown
	html, _ := contentSel.Html()
	markdown, err := r.converter.ConvertString(html)
	if err != nil {
		return "", fmt.Errorf("HTML→MD 转换失败: %w", err)
	}

	// 基础清洗
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return "", fmt.Errorf("页面内容为空")
	}

	return markdown, nil
}
```

- [ ] **Step 2: 验证编译**

```bash
cd d:\Vscoding\edu_market
go build ./scripts/scrape/
```
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add scripts/scrape/runoob.go
git commit -m "feat: 菜鸟教程源（目录树解析 + 并发抓取 + HTML→MD）"
```

---

### Task 6: CLI 入口

**Files:**
- Create: `scripts/scrape.go`

**Interfaces:**
- Consumes: `scrape.Scraper`, `scrape.Pipeline`, `scrape.NewGitHubScraper`, `scrape.NewRunoobScraper`
- Consumes: `config.Load()`, `database.Init()`, `database.InitRedis()`, `rag.Init()`
- Produces: 可执行的 CLI 工具

- [ ] **Step 1: 创建 scrape.go**

```go
//go:build ignore

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"edu_market/config"
	"edu_market/database"
	"edu_market/scripts/scrape"
	"edu_market/service/rag"
)

func main() {
	source := flag.String("source", "all", "数据源: github, runoob, all")
	dryRun := flag.Bool("dry-run", false, "仅预览，不写库")
	skipRAG := flag.Bool("skip-rag", false, "入库但不向量化")
	force := flag.Bool("force", false, "覆盖已存在的 Material")
	flag.Parse()

	// === 初始化 ===
	config.Load()
	database.InitRedis()
	database.Init()
	rag.Init()

	if rag.Get() == nil && !*skipRAG && !*dryRun {
		slog.Warn("RAG 未初始化，自动跳过向量化")
		*skipRAG = true
	}

	// === 创建管线 ===
	pipeline, err := scrape.NewPipeline(*force, *dryRun, *skipRAG)
	if err != nil {
		fmt.Fprintf(os.Stderr, "管线初始化失败: %v\n", err)
		os.Exit(1)
	}

	// === 收集数据源 ===
	var scrapers []scrape.Scraper
	switch *source {
	case "github":
		scrapers = append(scrapers, scrape.NewGitHubScraper())
	case "runoob":
		scrapers = append(scrapers, scrape.NewRunoobScraper())
	case "all":
		scrapers = append(scrapers, scrape.NewGitHubScraper(), scrape.NewRunoobScraper())
	default:
		fmt.Fprintf(os.Stderr, "未知数据源: %s (可用: github, runoob, all)\n", *source)
		os.Exit(1)
	}

	// === 执行 ===
	ctx := context.Background()
	totalCreated := 0
	totalSkipped := 0

	for _, s := range scrapers {
		fmt.Printf("\n=== %s ===\n", s.Name())
		articles, err := s.Fetch(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] 抓取失败: %v\n", s.Name(), err)
			continue
		}
		fmt.Printf("[%s] 获取 %d 篇文章\n", s.Name(), len(articles))

		for _, a := range articles {
			created, skipped, err := pipeline.Process(a)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%s] %s 处理失败: %v\n", s.Name(), a.Title, err)
				continue
			}
			totalCreated += created
			totalSkipped += skipped
		}
	}

	// === 汇总 ===
	fmt.Println("\n=== DONE ===")
	fmt.Printf("新建 Material: %d\n", totalCreated)
	fmt.Printf("跳过（已存在）: %d\n", totalSkipped)
}
```

- [ ] **Step 2: 验证编译**

```bash
cd d:\Vscoding\edu_market
go build ./scripts/scrape/
go vet ./scripts/scrape/
```
Expected: 编译成功，无 vet 警告

- [ ] **Step 3: Dry-run 测试**

```bash
go run scripts/scrape.go --source=all --dry-run
```
Expected: 输出所有文章的 DRY-RUN 预览，不写库

- [ ] **Step 4: Commit**

```bash
git add scripts/scrape.go
git commit -m "feat: CLI 入口 scrape.go（flag 解析 + 初始化 + 调度）"
```

---

### Task 7: 实机测试 + 调修

- [ ] **Step 1: 真实入库测试**

```bash
go run scripts/scrape.go --source=all
```
Expected: 
- materials 表有新纪录（11-15 条）
- documents 表有记录（170-350 条）
- document_chunks 表有切片数据
- Qdrant collection 点数增加
- 日志输出每个 Material 的创建情况

- [ ] **Step 2: 增量模式测试**

```bash
go run scripts/scrape.go --source=all
```
Expected: 所有 Material 显示"跳过已存在"

- [ ] **Step 3: Force 覆盖测试**

```bash
go run scripts/scrape.go --source=github --force
```
Expected: GitHub 源 Material 被删除后重建，document_chunks 刷新

- [ ] **Step 4: RAG 检索验证**

启动服务后，在 Agent 对话中问技术问题（如"Python 怎么定义函数"），验证能从爬取内容中检索到答案。

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: 实机验证爬虫全链路 + 增量/force/RAG 检索"
```

---

## Self-Review

1. **Spec coverage:** 全部需求已覆盖 — 多源接口(✓) GitHub(✓) 菜鸟(✓) Pipeline(✓) CLI(✓) 定价自适应(✓) 增量/force(✓) skip-rag(✓) dry-run(✓)
2. **Placeholder scan:** 无 TBD/TODO/占位符
3. **Type consistency:** `Scraper` 接口 → `GitHubScraper`/`RunoobScraper` → `Pipeline.Process(*Article)` 类型链一致
