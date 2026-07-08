package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// generateReport 从评测结果生成完整报告。
func generateReport(groups []EvalGroupResult, topK int) *RAGEvalReport {
	report := &RAGEvalReport{
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		TopK:      topK,
	}

	if len(groups) == 0 {
		return report
	}

	report.TotalQueries = len(groups[0].Results)

	for _, g := range groups {
		report.Configs = append(report.Configs, g.Config)
		summary := configSummary(g.Config.Name, g.Results)

		// 收集所有延迟
		var lats []float64
		for _, r := range g.Results {
			lats = append(lats, r.LatencyMs)
		}
		sort.Float64s(lats)
		avg, p50, p95, p99 := latencyPercentiles(lats)
		summary.AvgLatencyMs = avg

		report.Summaries = append(report.Summaries, summary)
		report.LatencyStats = append(report.LatencyStats, LatencyStats{
			ConfigName: g.Config.Name,
			Avg:        avg,
			P50:        p50,
			P95:        p95,
			P99:        p99,
		})
		report.Results = append(report.Results, g.Results...)
	}
	return report
}

// formatMarkdown 将报告格式化为 Markdown。
func formatMarkdown(report *RAGEvalReport) string {
	var sb strings.Builder
	sb.WriteString("# RAG 检索质量评测报告\n\n")
	sb.WriteString(fmt.Sprintf("**时间:** %s  |  **查询数:** %d  |  **TopK:** %d\n\n",
		report.Timestamp, report.TotalQueries, report.TopK))

	// 分层对比表
	sb.WriteString("## 分层对比\n\n")
	sb.WriteString("| 配置 | Precision@K | Recall@K | TopK命中率 | 平均延迟 | P50 | P95 | P99 |\n")
	sb.WriteString("|------|:----------:|:--------:|:---------:|:------:|:---:|:---:|:---:|\n")
	for i, s := range report.Summaries {
		ls := report.LatencyStats[i]
		sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %.1f%% | %.1f%% | %.1fms | %.1fms | %.1fms | %.1fms |\n",
			s.ConfigName,
			s.AvgPrecision*100, s.AvgRecall*100, s.TopKHitRate*100,
			ls.Avg, ls.P50, ls.P95, ls.P99))
	}

	// 各层贡献分析
	sb.WriteString("\n## 各层贡献分析\n\n")
	sb.WriteString("| 优化项 | Precision变化 | Recall变化 | 延迟变化 |\n")
	sb.WriteString("|--------|:----------:|:--------:|:------:|\n")
	if len(report.Summaries) >= 2 {
		sb.WriteString(fmt.Sprintf("| BM25混合检索 | %+.1fpp | %+.1fpp | %+.1fms |\n",
			(report.Summaries[1].AvgPrecision-report.Summaries[0].AvgPrecision)*100,
			(report.Summaries[1].AvgRecall-report.Summaries[0].AvgRecall)*100,
			report.LatencyStats[1].Avg-report.LatencyStats[0].Avg))
	}
	if len(report.Summaries) >= 3 {
		sb.WriteString(fmt.Sprintf("| Rerank精排 | %+.1fpp | %+.1fpp | %+.1fms |\n",
			(report.Summaries[2].AvgPrecision-report.Summaries[1].AvgPrecision)*100,
			(report.Summaries[2].AvgRecall-report.Summaries[1].AvgRecall)*100,
			report.LatencyStats[2].Avg-report.LatencyStats[1].Avg))
	}
	if len(report.Summaries) >= 4 {
		if report.LatencyStats[2].Avg > 0 {
			latChange := (report.LatencyStats[3].Avg - report.LatencyStats[2].Avg) / report.LatencyStats[2].Avg * 100
			sb.WriteString(fmt.Sprintf("| 两级缓存（命中时） | — | — | %.0f%% |\n", latChange))
		}
	}

	sb.WriteString("\n## 局限说明\n\n")
	sb.WriteString("- Recall 依赖 LLM 判断 ground truth，存在 5-10% 的标注误差\n")
	sb.WriteString("- 缓存组测的是 L1 精确缓存命中（重复查询），非生产混合命中率\n")

	return sb.String()
}

// saveReport 保存 Markdown 报告。
func saveReport(report *RAGEvalReport, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	filename := dir + "/report.md"
	if err := os.WriteFile(filename, []byte(formatMarkdown(report)), 0644); err != nil {
		return "", err
	}
	return filename, nil
}
