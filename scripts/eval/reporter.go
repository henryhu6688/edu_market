package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// generateReport 从评分结果生成评估报告。
func generateReport(results []ScoredResult) *EvalReport {
	report := &EvalReport{
		Timestamp:  time.Now().Format("2006-01-02 15:04:05"),
		TotalTasks: len(results),
		ByCategory: make(map[TaskCategory]*CategoryStats),
		Results:    results,
	}

	// 分类统计
	type catTemp struct {
		total, passed                             int
		sumComplete, sumReliable, sumEfficient, sumFailure float64
	}
	catMap := make(map[TaskCategory]*catTemp)
	for _, r := range results {
		if catMap[r.Category] == nil {
			catMap[r.Category] = &catTemp{}
		}
		ct := catMap[r.Category]
		ct.total++
		if r.Layer1Pass {
			report.PassedTasks++
			ct.passed++
		}
		if r.Layer2Scores != nil {
			ct.sumComplete += float64(r.Layer2Scores.TaskComplete)
			ct.sumReliable += float64(r.Layer2Scores.ProcessReliable)
			ct.sumEfficient += float64(r.Layer2Scores.Efficiency)
			ct.sumFailure += float64(r.Layer2Scores.FailureHandling)
		}
	}

	for cat, ct := range catMap {
		stats := &CategoryStats{Total: ct.total, Passed: ct.passed}
		if ct.total > 0 {
			stats.PassRate = float64(ct.passed) / float64(ct.total) * 100
			stats.AvgComplete = ct.sumComplete / float64(ct.total)
			stats.AvgReliable = ct.sumReliable / float64(ct.total)
			stats.AvgEfficient = ct.sumEfficient / float64(ct.total)
			stats.AvgFailure = ct.sumFailure / float64(ct.total)
		}
		report.ByCategory[cat] = stats
	}

	if report.TotalTasks > 0 {
		report.PassRate = float64(report.PassedTasks) / float64(report.TotalTasks) * 100
	}

	return report
}

// formatMarkdown 将报告格式化为 Markdown 文本。
func formatMarkdown(report *EvalReport) string {
	var sb strings.Builder
	sb.WriteString("# Agent 评估报告\n\n")
	sb.WriteString(fmt.Sprintf("**时间:** %s  \n", report.Timestamp))
	sb.WriteString(fmt.Sprintf("**总任务数:** %d  \n", report.TotalTasks))
	sb.WriteString(fmt.Sprintf("**通过数:** %d  \n", report.PassedTasks))
	sb.WriteString(fmt.Sprintf("**通过率:** %.1f%%\n\n", report.PassRate))

	// 分类汇总
	sb.WriteString("## 分类汇总\n\n")
	sb.WriteString("| 类别 | 总数 | 通过 | 通过率 | 任务完成 | 过程可靠 | 效率 | 兜底 |\n")
	sb.WriteString("|------|:---:|:---:|:-----:|:------:|:------:|:---:|:---:|\n")
	catOrder := []TaskCategory{CatNormal, CatMissingInfo, CatToolFailure, CatHighRisk, CatNoise}
	for _, cat := range catOrder {
		if s, ok := report.ByCategory[cat]; ok {
			sb.WriteString(fmt.Sprintf("| %s | %d | %d | %.0f%% | %.1f | %.1f | %.1f | %.1f |\n",
				cat, s.Total, s.Passed, s.PassRate, s.AvgComplete, s.AvgReliable, s.AvgEfficient, s.AvgFailure))
		}
	}

	// 详细结果
	sb.WriteString("\n## 详细结果\n\n")
	for i, r := range report.Results {
		sb.WriteString(fmt.Sprintf("### %d. [%s] %s — %s\n\n", i+1, r.Category, r.TaskID, r.Description))
		sb.WriteString(fmt.Sprintf("- **Layer 1:** %s\n", passFail(r.Layer1Pass)))
		for _, rule := range r.Layer1Rules {
			icon := "✅"
			if !rule.Passed {
				icon = "❌"
			}
			sb.WriteString(fmt.Sprintf("  - %s %s: %s\n", icon, rule.Rule, rule.Detail))
		}
		if r.Layer2Scores != nil {
			sb.WriteString(fmt.Sprintf("- **Layer 2:** 完成=%d 可靠=%d 效率=%d 兜底=%d\n",
				r.Layer2Scores.TaskComplete, r.Layer2Scores.ProcessReliable,
				r.Layer2Scores.Efficiency, r.Layer2Scores.FailureHandling))
			sb.WriteString(fmt.Sprintf("  - *%s*\n", r.Layer2Scores.Reason))
		}
		if r.Error != "" {
			sb.WriteString(fmt.Sprintf("- ⚠️ 执行错误: %s\n", r.Error))
		}
		sb.WriteString(fmt.Sprintf("- 步数: %d | Tokens: %d\n\n", r.TotalRounds, r.TotalTokens))
	}

	return sb.String()
}

func passFail(passed bool) string {
	if passed {
		return "✅ PASS"
	}
	return "❌ FAIL"
}

// saveTrace 保存单条 task trace 到独立 JSON 文件。
func saveTrace(traceData string, taskID, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	filename := fmt.Sprintf("%s/%s.json", dir, taskID)
	return os.WriteFile(filename, []byte(traceData), 0644)
}

// saveReport 保存报告到文件。
func saveReport(report *EvalReport, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("%s/report.md", dir)
	md := formatMarkdown(report)
	if err := os.WriteFile(filename, []byte(md), 0644); err != nil {
		return "", err
	}
	return filename, nil
}
