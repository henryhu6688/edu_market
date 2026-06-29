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
	Name string
	URL  string
	Cat  string
}

// RunoobScraper 从 runoob.com 抓取教程。
type RunoobScraper struct {
	client    *http.Client
	tutorials []tutorial
	converter *md.Converter
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
	var wg sync.WaitGroup

	for _, link := range links {
		wg.Add(1)
		go func(l sidebarLink) {
			defer wg.Done()
			sem <- struct{}{}
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
	wg.Wait()

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
	doc.Find(".design a, .left-column a, .sidebar a, #leftcolumn a").Each(func(_ int, s *goquery.Selection) {
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if err != nil {
		return "", err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}

	// 提取正文区
	contentSel := doc.Find(".article-body, .content, article, .article, main")
	if contentSel.Length() == 0 {
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

	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return "", fmt.Errorf("页面内容为空")
	}

	return markdown, nil
}
