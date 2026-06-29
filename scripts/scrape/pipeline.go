package scrape

import (
	"fmt"
	"log/slog"
	"strings"

	"edu_market/database"
	"edu_market/model"
	"edu_market/service/rag"

	"gorm.io/gorm"
)

// Pipeline 核心管线：Article → Material → Document → RAG 索引。
type Pipeline struct {
	db      *gorm.DB
	rag     *rag.RAGService
	userID  uint
	cats    map[string]uint // name → id
	force   bool
	dryRun  bool
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
		// force 模式：清理旧数据后删除 Material
		// 1. 删 Qdrant 向量
		if p.rag != nil {
			// 通过 IndexCourse 的删除逻辑：手动清 chunks + vectors
			p.db.Where("course_id = ?", existing.ID).Delete(&model.DocumentChunk{})
			// VectorStore.Delete 不可直接访问，IndexCourse 内部会清理，这里额外清 MySQL
		}
		// 2. DB 级联删 Material → Documents（FK CASCADE）
		p.db.Delete(&existing)
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

		// 7. RAG 索引：传原始 Markdown，structural_chunk 利用 # 标题切片
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
