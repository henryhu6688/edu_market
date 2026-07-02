//go:build ignore

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"edu_market/config"
	"edu_market/database"
	"edu_market/model"
	"edu_market/service/agent"
	"edu_market/service/rag"

	"github.com/spf13/viper"
	"gorm.io/gorm/logger"
)

func main() {
	// flags
	tasksFile := flag.String("tasks", "data/eval/tasks.json", "评估任务集 JSON 文件路径")
	categoryFilter := flag.String("category", "", "只跑指定类别 (normal|missing_info|tool_failure|high_risk|noise)")
	taskIDFilter := flag.String("id", "", "只跑指定任务 ID")
	layer1Only := flag.Bool("layer1-only", false, "只跑规则检查，跳过 LLM Judge")
	verbose := flag.Bool("verbose", false, "详细模式：实时打印每轮 LLM 请求/响应/Tool 执行")
	reportDir := flag.String("report-dir", "data/eval/reports", "报告输出目录")
	tracesDir := flag.String("traces-dir", "data/eval/traces", "单条 trace JSON 输出目录")
	flag.Parse()

	// 加载配置
	loadEvalConfig()

	// 初始化数据库（静默模式）
	database.InitLogLevel = logger.Silent
	database.Init()
	fmt.Printf("数据库就绪 (%s)\n", config.App.Database.DBName)

	// Redis 可选
	if err := database.InitRedis(); err != nil {
		fmt.Println("⚠️  Redis 不可用（不影响规则检查）")
	}

	// RAG 初始化（可选）
	initRAGSafely()

	// 加载任务
	tasks, err := loadTasks(*tasksFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载任务失败: %v\n", err)
		os.Exit(1)
	}

	// 过滤
	tasks = filterTasks(tasks, *categoryFilter, *taskIDFilter)
	fmt.Printf("加载 %d 个评估任务\n", len(tasks))
	if len(tasks) == 0 {
		fmt.Println("没有匹配的任务，退出")
		os.Exit(0)
	}

	// 准备测试用户
	evalUser := getOrCreateEvalUser()
	fmt.Printf("评估用户: %s (ID=%d)\n", evalUser.Username, evalUser.ID)

	// 初始化 AgentService + SearchFunc
	evalSvc := agent.NewAgentService(agent.NewAgentEngine())
	searchFunc := buildEvalSearchFunc()

	// 逐任务执行 + 评分
	var results []ScoredResult
	for i, task := range tasks {
		fmt.Printf("\n[%d/%d] %s [%s] %s\n", i+1, len(tasks), task.ID, task.Category, task.Description)

		// 执行
		requestID := fmt.Sprintf("eval-%s", task.ID)
		trace, err := runTask(&task, evalUser.ID, evalSvc, searchFunc, requestID, *verbose)
		if err != nil {
			fmt.Printf("  ⚠️ 执行异常: %v\n", err)
		}

		if trace == nil {
			trace = &EvalTrace{TaskID: task.ID, Error: "执行失败：trace 为空"}
		}

		// 保存单条 trace JSON
		traceBytes, _ := json.MarshalIndent(trace, "", "  ")
		if err := saveTrace(string(traceBytes), task.ID, *tracesDir); err != nil {
			fmt.Printf("  ⚠️ 保存 trace 失败: %v\n", err)
		}

		// Layer 1
		rules := scoreTask(&task, trace)
		allPass := true
		for _, r := range rules {
			if !r.Passed {
				allPass = false
				fmt.Printf("  ❌ %s: %s\n", r.Rule, r.Detail)
			}
		}
		if allPass {
			fmt.Println("  ✅ Layer 1 全部通过")
		}

		result := ScoredResult{
			TaskID:      task.ID,
			Category:    task.Category,
			Description: task.Description,
			Layer1Pass:  allPass,
			Layer1Rules: rules,
			TotalRounds: trace.TotalRounds,
			TotalTokens: trace.TotalTokens,
			Error:       trace.Error,
		}

		// Layer 2（可选）
		if !*layer1Only {
			scores, err := judgeTask(&task, trace)
			if err != nil {
				fmt.Printf("  ⚠️ Judge 打分失败: %v\n", err)
			} else {
				result.Layer2Scores = scores
				fmt.Printf("  📊 完成=%d 可靠=%d 效率=%d 兜底=%d | %s\n",
					scores.TaskComplete, scores.ProcessReliable,
					scores.Efficiency, scores.FailureHandling, scores.Reason)
			}
		}

		results = append(results, result)
	}

	// 生成报告
	report := generateReport(results)
	path, err := saveReport(report, *reportDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "保存报告失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n========================================\n")
	fmt.Printf("评估完成\n")
	fmt.Printf("通过率: %.1f%% (%d/%d)\n", report.PassRate, report.PassedTasks, report.TotalTasks)
	fmt.Printf("报告: %s\n", path)
}

// ======================== 辅助函数 ========================

func loadEvalConfig() {
	config.App = &config.Config{
		Server:   config.ServerConfig{Port: 8080, Mode: "test"},
		Database: config.DatabaseConfig{Host: "127.0.0.1", Port: 3306, User: "root", Password: readEvalYAML("database.password"), DBName: readEvalYAML("database.dbname"), Charset: "utf8mb4", MaxIdleConns: 5, MaxOpenConns: 10},
		Redis:    config.RedisConfig{Addr: "127.0.0.1:6379", Password: "", DB: 0},
		JWT:      config.JWTConfig{Secret: "eval-secret", AccessTTLMin: 30, RefreshTTLHours: 24},
		AI: config.AIConfig{
			Provider: "deepseek",
			APIKey:   readEvalYAML("ai.api_key"),
			APIURL:   readEvalYAML("ai.api_url"),
			Model:    readEvalYAML("ai.model"),
		},
		Agent: config.AgentConfig{
			MaxToolRounds:   10,
			ContextMaxMsg:   20,
			ChunkSize:       500,
			ChunkOverlap:    50,
			EmbeddingModel:  "BAAI/bge-m3",
			EmbeddingAPIURL: "https://api.siliconflow.cn/v1/embeddings",
			EmbeddingAPIKey: readEvalYAML("agent.embedding_api_key"),
		},
	}
	if config.App.AI.Model == "" {
		config.App.AI.Model = "deepseek-chat"
	}
	if config.App.AI.APIURL == "" {
		config.App.AI.APIURL = "https://api.deepseek.com/v1/chat/completions"
	}
	if config.App.Database.DBName == "" {
		config.App.Database.DBName = "edu_market"
	}
	if config.App.Database.Password == "" {
		config.App.Database.Password = readEvalYAML("database.password")
	}
}

func readEvalYAML(key string) string {
	v := viper.New()
	v.SetConfigName("app")
	v.SetConfigType("yml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	v.AddConfigPath("../../config")
	if err := v.ReadInConfig(); err != nil {
		return ""
	}
	return v.GetString(key)
}

func initRAGSafely() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("⚠️  RAG 初始化失败（Qdrant 未启动？）: %v\n", r)
		}
	}()
	rag.Init()
	fmt.Println("RAG 服务就绪")
}

func loadTasks(path string) ([]EvalTask, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tasks []EvalTask
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func filterTasks(tasks []EvalTask, category, id string) []EvalTask {
	if category == "" && id == "" {
		return tasks
	}
	var filtered []EvalTask
	for _, t := range tasks {
		if id != "" && t.ID != id {
			continue
		}
		if category != "" && string(t.Category) != category {
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered
}

func getOrCreateEvalUser() *model.User {
	var user model.User
	result := database.DB.Where("username = ?", "eval_test_user").First(&user)
	if result.Error == nil {
		return &user
	}
	user = model.User{
		Username:     "eval_test_user",
		Role:         "user",
		PasswordHash: "$2a$10$evalplaceholder",
	}
	database.DB.Create(&user)
	return &user
}

func buildEvalSearchFunc() agent.SearchFunc {
	ragSvc := rag.Get()
	if ragSvc == nil {
		return nil
	}
	return func(materialID uint, query string, topK int, hasAccess bool) (string, error) {
		results, err := ragSvc.Search(materialID, query, topK, hasAccess)
		if err != nil {
			return "", err
		}
		if len(results) == 0 {
			return "", nil
		}
		var parts []string
		for _, r := range results {
			parts = append(parts, r.Content)
		}
		b, _ := json.Marshal(parts)
		return string(b), nil
	}
}
