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
	Branch string
	Cat    string
}

// GitHubScraper 从 GitHub 仓库抓取 Markdown 文档。
type GitHubScraper struct {
	client *http.Client
	repos  []repo
}

// NewGitHubScraper 创建 GitHub 源适配器。
func NewGitHubScraper() *GitHubScraper {
	return &GitHubScraper{
		client: &http.Client{Timeout: 60 * time.Second},
		repos: []repo{
			{Owner: "gin-gonic", Name: "gin", Branch: "master", Cat: "编程开发"},
			{Owner: "go-gorm", Name: "gorm", Branch: "master", Cat: "编程开发"},
			{Owner: "golang", Name: "go", Branch: "master", Cat: "编程开发"},
			{Owner: "vuejs", Name: "core", Branch: "main", Cat: "编程开发"},
			{Owner: "rust-lang", Name: "book", Branch: "main", Cat: "编程开发"},
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
	var lastErr error
	for retry := 0; retry < 3; retry++ {
		if retry > 0 {
			time.Sleep(time.Duration(retry) * time.Second)
		}
		content, err := g.downloadOnce(ctx, url)
		if err == nil {
			return content, nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("下载失败（3次重试后）: %w", lastErr)
}

func (g *GitHubScraper) downloadOnce(ctx context.Context, url string) (string, error) {
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
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
