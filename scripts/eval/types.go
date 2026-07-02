//go:build ignore

package main

// TaskCategory 任务类别
type TaskCategory string

const (
	CatNormal      TaskCategory = "normal"
	CatMissingInfo TaskCategory = "missing_info"
	CatToolFailure TaskCategory = "tool_failure"
	CatHighRisk    TaskCategory = "high_risk"
	CatNoise       TaskCategory = "noise"
)

// UserProfile 评估任务中的模拟用户
type UserProfile struct {
	Role               string `json:"role"`
	PurchasedMaterials []uint `json:"purchased_materials"`
	PublishedMaterials []uint `json:"published_materials"`
}

// PassConditions 硬边界条件
type PassConditions struct {
	RequiredTools  []string `json:"required_tools"`
	ForbiddenTools []string `json:"forbidden_tools"`
	MaxSteps       int      `json:"max_steps"`
}

// ScoringWeights 四维度权重
type ScoringWeights struct {
	TaskComplete    float64 `json:"task_complete_weight"`
	ProcessReliable float64 `json:"process_reliable_weight"`
	Efficiency      float64 `json:"efficiency_weight"`
	FailureHandling float64 `json:"failure_handling_weight"`
}

// EvalTask 单条评估任务
type EvalTask struct {
	ID             string        `json:"id"`
	Category       TaskCategory  `json:"category"`
	Description    string        `json:"description"`
	User           UserProfile   `json:"user"`
	Input          string        `json:"input"`
	ContextHistory []ContextMsg   `json:"context_history,omitempty"`
	Setup          TaskSetup     `json:"setup"`
	PassConditions PassConditions `json:"pass_conditions"`
	Scoring        ScoringWeights `json:"scoring"`
}

// ContextMsg 预设上下文消息（噪音类任务用）
type ContextMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// TaskSetup 任务环境设定
type TaskSetup struct {
	MaterialID    uint   `json:"material_id"`
	MaterialTitle string `json:"material_title"`
	HasAccess     bool   `json:"has_access"`
}

// TraceStep 单步 trace 记录
type TraceStep struct {
	Round        int    `json:"round"`
	Step         string `json:"step"`
	ToolName     string `json:"tool_name,omitempty"`
	ToolArgs     string `json:"tool_args,omitempty"`
	ToolResult   string `json:"tool_result,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`
	Recoverable  bool   `json:"recoverable"`
	Content      string `json:"content,omitempty"`
	FinishReason string `json:"finish_reason,omitempty"`
	TokensUsed   int    `json:"tokens_used"`
}

// EvalTrace 完整执行轨迹
type EvalTrace struct {
	TaskID       string      `json:"task_id"`
	Steps        []TraceStep `json:"steps"`
	FinalAnswer  string      `json:"final_answer"`
	TotalRounds  int         `json:"total_rounds"`
	TotalTokens  int         `json:"total_tokens"`
	ModeSequence []string    `json:"mode_sequence"`
	Error        string      `json:"error,omitempty"`
}

// RuleResult 单条规则检查结果
type RuleResult struct {
	Rule   string `json:"rule"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

// JudgeScores LLM Judge 打分
type JudgeScores struct {
	TaskComplete    int    `json:"task_complete"`
	ProcessReliable int    `json:"process_reliable"`
	Efficiency      int    `json:"efficiency"`
	FailureHandling int    `json:"failure_handling"`
	Reason          string `json:"reason"`
}

// ScoredResult 单条任务的完整评分
type ScoredResult struct {
	TaskID       string       `json:"task_id"`
	Category     TaskCategory `json:"category"`
	Description  string       `json:"description"`
	Layer1Pass   bool         `json:"layer1_pass"`
	Layer1Rules  []RuleResult `json:"layer1_rules"`
	Layer2Scores *JudgeScores `json:"layer2_scores,omitempty"`
	TotalRounds  int          `json:"total_rounds"`
	TotalTokens  int          `json:"total_tokens"`
	Error        string       `json:"error,omitempty"`
}

// EvalReport 评估报告
type EvalReport struct {
	Timestamp      string                    `json:"timestamp"`
	TotalTasks     int                       `json:"total_tasks"`
	PassedTasks    int                       `json:"passed_tasks"`
	PassRate       float64                   `json:"pass_rate"`
	ByCategory     map[TaskCategory]*CategoryStats  `json:"by_category"`
	ByToolAccuracy map[string]*ToolAccuracy         `json:"by_tool_accuracy"`
	Results        []ScoredResult            `json:"results"`
}

// CategoryStats 分类统计
type CategoryStats struct {
	Total        int     `json:"total"`
	Passed       int     `json:"passed"`
	PassRate     float64 `json:"pass_rate"`
	AvgComplete  float64 `json:"avg_complete"`
	AvgReliable  float64 `json:"avg_reliable"`
	AvgEfficient float64 `json:"avg_efficient"`
	AvgFailure   float64 `json:"avg_failure"`
}

// ToolAccuracy Tool 准确率统计
type ToolAccuracy struct {
	Called          int     `json:"called"`
	InForbidden     int     `json:"in_forbidden"`
	MissingRequired int     `json:"missing_required"`
	Accuracy        float64 `json:"accuracy"`
}
