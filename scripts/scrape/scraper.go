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
