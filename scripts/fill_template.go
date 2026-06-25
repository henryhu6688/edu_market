//go:build ignore

package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/xuri/excelize/v2"
)

func main() {
	f, err := excelize.OpenFile("docs/udq_template.xlsx")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	file, err := os.Open("docs/mock-interview-v2.md")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	type QA struct{ q, a string }
	var qas []QA
	var current QA
	var mode string        // "main" = ## QN 格式，"supp" = **Q: / **追问： 格式
	inAnswer := false

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// ── 主问题开始：## QN: ... ──
		if strings.HasPrefix(line, "## Q") && strings.Contains(line, ":") {
			// 保存上一个 Q&A
			if current.q != "" {
				current.a = strings.TrimSpace(current.a)
				qas = append(qas, current)
			}
			// 去掉 ## 前缀，提取 "Q1: 简单介绍一下你的项目"
			q := strings.TrimPrefix(line, "## ")
			current = QA{q: q, a: ""}
			mode = "main"
			inAnswer = false
			continue
		}

		// ── 补充实战题 / 八股文 问题开始：**Q: ...** ──
		if strings.HasPrefix(line, "**Q:") {
			if current.q != "" {
				current.a = strings.TrimSpace(current.a)
				qas = append(qas, current)
			}
			// 去掉 ** 包裹，提取纯文本问题
			q := strings.TrimPrefix(line, "**Q:")
			q = strings.TrimSuffix(q, "**")
			q = strings.TrimSpace(q)
			current = QA{q: q, a: ""}
			mode = "supp"
			inAnswer = true // 补充题没有 **参考回答：** 标记，直接进入答案
			continue
		}

		// ── 追问处理：分两种场景 ──
		if strings.HasPrefix(line, "**追问：") || strings.HasPrefix(line, "**追问：**") {
			if mode == "main" && inAnswer {
				// 主问题内的追问：合并到当前答案
				q := strings.TrimPrefix(line, "**")
				q = strings.TrimSuffix(q, "**")
				current.a += "\n\n" + q
				continue
			} else if mode == "supp" {
				// 补充题里的追问：独立成一个新 Q&A
				if current.q != "" {
					current.a = strings.TrimSpace(current.a)
					qas = append(qas, current)
				}
				q := strings.TrimPrefix(line, "**追问：")
				q = strings.TrimSuffix(q, "**")
				q = strings.TrimSpace(q)
				current = QA{q: q, a: ""}
				mode = "supp"
				inAnswer = true // 直接进入答案
				continue
			}
			// 还没进入答案时遇到追问（兜底）：当新问题处理
			if current.q != "" {
				current.a = strings.TrimSpace(current.a)
				qas = append(qas, current)
			}
			q := strings.TrimPrefix(line, "**追问：")
			q = strings.TrimSuffix(q, "**")
			q = strings.TrimSpace(q)
			current = QA{q: q, a: ""}
			mode = "supp"
			inAnswer = true
			continue
		}

		// ── 跳过章节标题（### ... / ## ...）和分隔线 ──
		if strings.HasPrefix(line, "### ") || strings.HasPrefix(line, "# ") {
			continue
		}
		// ## 但不是 Q 开头（如 ## 项目相关八股文）
		if strings.HasPrefix(line, "## ") && !strings.Contains(line, "Q") {
			continue
		}

		// ── 答案开始标记（仅 main 模式）──
		if strings.HasPrefix(line, "**参考回答：**") || strings.HasPrefix(line, "**参考答案：**") {
			inAnswer = true
			continue
		}

		// ── main 模式的 --- 分隔线 = 答案结束 ──
		if strings.HasPrefix(line, "---") && mode == "main" && inAnswer {
			current.a = strings.TrimSpace(current.a)
			qas = append(qas, current)
			current = QA{}
			mode = ""
			inAnswer = false
			continue
		}

		// ── supp 模式的 --- 只是章节分隔，不是答案结束 ──
		if strings.HasPrefix(line, "---") && mode == "supp" {
			continue
		}

		// ── 收集答案内容 ──
		if inAnswer && line != "" {
			current.a += line + "\n"
		}
	}

	// 保存最后一个 Q&A
	if current.q != "" {
		current.a = strings.TrimSpace(current.a)
		qas = append(qas, current)
	}

	// ── 写入 xlsx（从第 2 行开始，保留表头）──
	ws := f.GetSheetName(0)
	for i, qa := range qas {
		row := i + 2
		f.SetCellValue(ws, fmt.Sprintf("A%d", row), i+1)
		f.SetCellValue(ws, fmt.Sprintf("B%d", row), qa.q)
		f.SetCellValue(ws, fmt.Sprintf("C%d", row), qa.a)
	}

	if err := f.SaveAs("docs/udq_filled.xlsx"); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Done: %d Q&A written to docs/udq_filled.xlsx\n", len(qas))

	// 打印前几条验证
	for i, qa := range qas {
		if i >= 5 {
			break
		}
		fmt.Printf("\n── Q%d: %.60s\n", i+1, qa.q)
		fmt.Printf("   A: %.80s\n", qa.a)
	}
}
