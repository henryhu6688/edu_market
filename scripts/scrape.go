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
	fmt.Printf("Material 创建: %d\n", totalCreated)
	fmt.Printf("跳过（已存在）: %d\n", totalSkipped)
}
