//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"edu_market/config"
)

// judgeSystemPrompt Judge 的系统提示
const judgeSystemPrompt = `你是一个 Agent 评估专家。你会收到一个评估任务的定义和 Agent 实际执行的完整轨迹。
你需要对四个维度分别打分（1-5分整数），并给出简短理由。

输出必须是严格的 JSON 格式：
{"task_complete": N, "process_reliable": N, "efficiency": N, "failure_handling": N, "reason": "..."}`

// judgeTask 调用 LLM Judge 对单条评估任务打分（四维度 1-5 分）。
func judgeTask(task *EvalTask, trace *EvalTrace) (*JudgeScores, error) {
	cfg := config.App.AI
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("AI API Key 未配置，跳过 Judge 打分")
	}

	prompt := buildJudgePrompt(task, trace)

	model := cfg.Model
	if model == "" {
		model = "deepseek-chat"
	}

	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": judgeSystemPrompt},
			{"role": "user", "content": prompt},
		},
		"stream":      false,
		"max_tokens":  512,
		"temperature": 0, // 保证评分一致性
	}

	jsonBytes, _ := json.Marshal(reqBody)

	ctx := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("POST", cfg.APIURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("创建 Judge 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := ctx.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Judge API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Judge API 返回状态 %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析 Judge 响应失败: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("Judge 返回空 choices")
	}

	// 解析 Judge 输出的 JSON
	content := result.Choices[0].Message.Content
	// 清理 markdown code block
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	scores := &JudgeScores{}
	if err := json.Unmarshal([]byte(content), scores); err != nil {
		// 尝试从文本中提取数字
		return parseJudgeFallback(content), nil
	}

	return scores, nil
}

// buildJudgePrompt 构造 Judge 的用户提示
func buildJudgePrompt(task *EvalTask, trace *EvalTrace) string {
	var sb strings.Builder
	sb.WriteString("## 评估任务\n\n")
	sb.WriteString(fmt.Sprintf("- 类别: %s\n", task.Category))
	sb.WriteString(fmt.Sprintf("- 描述: %s\n", task.Description))
	sb.WriteString(fmt.Sprintf("- 用户输入: %s\n", task.Input))
	sb.WriteString(fmt.Sprintf("- 用户权限: has_access=%v\n\n", task.Setup.HasAccess))

	sb.WriteString("## Agent 执行轨迹\n\n")
	for i, s := range trace.Steps {
		if s.ToolName != "" {
			sb.WriteString(fmt.Sprintf("步骤 %d: 调用 %s(%s)\n", i+1, s.ToolName, s.ToolArgs))
			if s.ErrorCode != "" {
				sb.WriteString(fmt.Sprintf("  错误码: %s, 可恢复: %v\n", s.ErrorCode, s.Recoverable))
			}
		}
	}

	sb.WriteString("\n## Agent 最终回答\n\n")
	finalAnswer := trace.FinalAnswer
	if len([]rune(finalAnswer)) > 800 {
		finalAnswer = string([]rune(finalAnswer)[:800]) + "...(截断)"
	}
	sb.WriteString(finalAnswer)

	sb.WriteString("\n\n## 评分标准\n\n")
	sb.WriteString("- 任务完成度(1-5): 用户目标是否完全解决\n")
	sb.WriteString("- 过程可靠度(1-5): Tool 选择是否合理、有无越权、有无忽略失败\n")
	sb.WriteString("- 效率(1-5): 是否最短路径，有无明显冗余调用\n")
	sb.WriteString("- 失败兜底(1-5): 错误后是否正确恢复/追问/诚实告知\n\n")
	sb.WriteString("请输出 JSON 评分。")

	return sb.String()
}

// parseJudgeFallback 当 Judge 输出不是严格的 JSON 时，尝试从文本中提取评分。
func parseJudgeFallback(content string) *JudgeScores {
	scores := &JudgeScores{Reason: content}
	// 尝试正则提取 "task_complete": N 模式
	patterns := map[string]*int{
		`"task_complete"\s*:\s*(\d)`:        &scores.TaskComplete,
		`"process_reliable"\s*:\s*(\d)`:     &scores.ProcessReliable,
		`"efficiency"\s*:\s*(\d)`:           &scores.Efficiency,
		`"failure_handling"\s*:\s*(\d)`:     &scores.FailureHandling,
	}
	for pat, target := range patterns {
		re := regexp.MustCompile(pat)
		if matches := re.FindStringSubmatch(content); len(matches) >= 2 {
			if v, err := strconv.Atoi(matches[1]); err == nil {
				*target = v
			}
		}
	}
	return scores
}
