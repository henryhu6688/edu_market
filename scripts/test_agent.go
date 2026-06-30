//go:build ignore

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"edu_market/config"
	"edu_market/database"
	"edu_market/model"
	"edu_market/service/agent"
	"edu_market/service/rag"

	"github.com/spf13/viper"
	"gorm.io/gorm/logger"
)

// ANSI 颜色
const (
	cReset  = "\033[0m"
	cRed    = "\033[31m"
	cGreen  = "\033[32m"
	cYellow = "\033[33m"
	cCyan   = "\033[36m"
	cBold   = "\033[1m"
	cDim    = "\033[2m"
)

var (
	currentSessionID uint
)

func main() {
	fmt.Println("⏳ 加载配置 + 初始化数据库...")

	loadConfig()
	database.InitLogLevel = logger.Silent // REPL 不需要 SQL 日志
	database.Init()
	fmt.Printf("   %s✓%s 数据库就绪 (%s)\n", cGreen, cReset, config.App.Database.DBName)

	// Redis 可选
	if err := database.InitRedis(); err != nil {
		fmt.Printf("   %s⚠%s  Redis 不可用，VectorStore 降级到内存模式\n", cYellow, cReset)
	}

	// 初始化 RAG
	rag.Init()

	user := getOrCreateUser()
	fmt.Printf("   %s✓%s 用户就绪 (id=%d)\n", cGreen, cReset, user.ID)

	engine := agent.NewAgentEngine()
	engine.SetTraceHandler(tracePrinter)

	svc := agent.NewAgentService(engine)
	searchFunc := buildSearchFunc()

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║        🤖  Agent  全链路调测  REPL               ║")
	fmt.Println("╠══════════════════════════════════════════════════╣")
	fmt.Println("║  输入问题 → 查看每一步：                         ║")
	fmt.Println("║    ① System Prompt      ② LLM 请求体            ║")
	fmt.Println("║    ③ LLM 决策(Tool/Text)④ Tool 完整返回         ║")
	fmt.Println("║    ⑤ 最终流式回复                                ║")
	fmt.Println("╠══════════════════════════════════════════════════╣")
	fmt.Println("║  :q 退出   :n 新会话   :s 查看当前会话ID        ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\n" + cBold + "🤖 > " + cReset)
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		switch input {
		case ":q", ":quit":
			fmt.Println("👋 再见")
			return
		case ":n", ":new":
			currentSessionID = 0
			fmt.Printf("   %s✓%s 已切换到新会话\n", cGreen, cReset)
			continue
		case ":s", ":session":
			fmt.Printf("   当前会话ID: %d (0=下次自动创建)\n", currentSessionID)
			continue
		}

		runChat(svc, user.ID, input, searchFunc)
	}
}

// ======================== 配置加载 ========================

func loadConfig() {
	config.App = &config.Config{
		Server:   config.ServerConfig{Port: 8080, Mode: "debug"},
		Database: config.DatabaseConfig{Host: "127.0.0.1", Port: 3306, User: "root", Password: readYAML("database.password"), DBName: readYAML("database.dbname"), Charset: "utf8mb4", MaxIdleConns: 5, MaxOpenConns: 10},
		Redis:    config.RedisConfig{Addr: "127.0.0.1:6379", Password: "", DB: 0},
		JWT:      config.JWTConfig{Secret: readYAML("jwt.secret"), AccessTTLMin: 30, RefreshTTLHours: 24},
		AI:       config.AIConfig{Provider: "deepseek", APIKey: readYAML("ai.api_key"), APIURL: "https://api.deepseek.com/v1/chat/completions", Model: readYAML("ai.model")},
		Captcha:  config.CaptchaConfig{Length: 6, ExpireSeconds: 300, ResendSeconds: 1},
		Agent:    config.AgentConfig{MaxToolRounds: 10, ContextMaxMsg: 20, ChunkSize: 500, ChunkOverlap: 50, EmbeddingModel: "BAAI/bge-large-zh-v1.5", EmbeddingAPIURL: "https://api.siliconflow.cn/v1/embeddings", EmbeddingAPIKey: readYAML("agent.embedding_api_key")},
		Document: config.DocumentConfig{AutoSaveDelay: 2, RagSync: true, MaxUploadSize: 20 << 20, AllowedFormats: []string{".pdf", ".pptx", ".docx", ".md", ".txt"}},
	}

	if config.App.AI.Model == "" {
		config.App.AI.Model = "deepseek-chat"
	}
	if config.App.Database.DBName == "" {
		config.App.Database.DBName = "edu_market"
	}
}

func readYAML(key string) string {
	v := viper.New()
	v.SetConfigName("app")
	v.SetConfigType("yml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	v.AddConfigPath("../config")
	v.AddConfigPath("../../config")
	if err := v.ReadInConfig(); err != nil {
		return ""
	}
	return v.GetString(key)
}

// ======================== 数据库 ========================

func getOrCreateUser() *model.User {
	var user model.User

	// 优先用已存在的 admin 用户
	result := database.DB.Where("role = ?", "admin").First(&user)
	if result.Error == nil {
		return &user
	}

	// 尝试用已存在的普通用户
	result = database.DB.First(&user)
	if result.Error == nil {
		return &user
	}

	// 创建一个新用户
	user = model.User{Username: "agent_repl_user", Role: "user"}
	if err := database.DB.Create(&user).Error; err != nil {
		// 可能已存在（前次运行残留），直接用
		if err := database.DB.Where("username = ?", "agent_repl_user").First(&user).Error; err != nil {
			log.Fatalf("无法获取/创建用户: %v", err)
		}
	}
	fmt.Printf("   创建测试用户: agent_repl_user (id=%d)\n", user.ID)
	return &user
}

// ======================== RAG ========================

func buildSearchFunc() agent.SearchFunc {
	ragSvc := rag.Get()
	if ragSvc == nil {
		return nil
	}
	return func(courseID uint, query string, topK int, hasAccess bool) (string, error) {
		results, err := ragSvc.Search(courseID, query, topK, hasAccess)
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
		bytes, _ := json.Marshal(parts)
		return string(bytes), nil
	}
}

// ======================== Agent 调用 ========================

func runChat(svc *agent.AgentService, userID uint, question string, searchFunc agent.SearchFunc) {
	var sid *uint
	if currentSessionID != 0 {
		sid = &currentSessionID
	}

	sseHandler := func(event, data string) error {
		printSSE(event, data)
		return nil
	}

	session, err := svc.Chat(userID, sid, question, searchFunc, sseHandler, "repl")
	if err != nil {
		fmt.Printf("\n  %s❌ 错误: %v%s\n", cRed, err, cReset)
		return
	}

	if session != nil {
		currentSessionID = session.ID
	}
}

// ======================== Trace 输出 ========================

func tracePrinter(evt agent.TraceEvent) {
	switch evt.Step {
	case "context_loaded":
		printContextLoaded(evt)
	case "llm_request":
		printLLMRequest(evt)
	case "llm_response":
		printLLMResponse(evt)
	case "tool_result":
		printToolResult(evt)
	case "final_answer":
		printFinalAnswer(evt)
	}
}

func printContextLoaded(evt agent.TraceEvent) {
	prompt, _ := evt.Data["system_prompt"].(string)
	count := len([]rune(prompt))
	promptPreview := prompt
	if count > 500 {
		promptPreview = string([]rune(prompt)[:500]) + "..."
	}

	fmt.Printf("\n%s┌─ ① 上下文加载 ─────────────────────────────%s\n", cCyan, cReset)
	fmt.Printf("%s│%s System Prompt (%d字):\n", cCyan, cReset, count)
	fmt.Printf("%s│%s   %s\n", cCyan, cReset, promptPreview)
	fmt.Printf("%s└──────────────────────────────────────────────%s\n", cCyan, cReset)
}

func printLLMRequest(evt agent.TraceEvent) {
	round := evt.Round
	model, _ := evt.Data["model"].(string)
	stream, _ := evt.Data["stream"].(bool)
	toolsCount, _ := evt.Data["tools_count"].(int)

	mode := "非流式 (检查 tool_calls)"
	if stream {
		mode = "流式 (生成最终回复)"
	}

	fmt.Printf("\n%s┌─ ② LLM 请求 (第%d轮) ────────────────────────%s\n", cYellow, round, cReset)
	fmt.Printf("%s│%s Model: %s  Mode: %s  Tools: %d个\n", cYellow, cReset, model, mode, toolsCount)

	// 消息列表是 []interface{} → 每个元素是 map[string]interface{}
	if msgsRaw, ok := evt.Data["messages"].([]interface{}); ok {
		for i, mRaw := range msgsRaw {
			m, _ := mRaw.(map[string]interface{})
			role, _ := m["role"].(string)
			content, _ := m["content"].(string)

			roleFmt := formatRole(role)
			contentPreview := truncateStr(content, 200)

			// tool_calls 信息
			extra := ""
			if tcsRaw, ok := m["tool_calls"].([]interface{}); ok && len(tcsRaw) > 0 {
				names := make([]string, 0, len(tcsRaw))
				for _, tcRaw := range tcsRaw {
					tc, _ := tcRaw.(map[string]interface{})
					if fn, ok := tc["function"].(map[string]interface{}); ok {
						if name, ok := fn["name"].(string); ok {
							names = append(names, name)
						}
					}
				}
				extra = fmt.Sprintf(" %s→ tool_calls: %s%s", cDim, strings.Join(names, ", "), cReset)
			}
			if toolCallID, ok := m["tool_call_id"].(string); ok && toolCallID != "" {
				extra = fmt.Sprintf(" %s← tool_result (call_id=%s)%s", cDim, toolCallID, cReset)
			}

			fmt.Printf("%s│%s   [%d] %s %s%s%s\n", cYellow, cReset, i, roleFmt, contentPreview, extra, cReset)
		}
	}
	fmt.Printf("%s└──────────────────────────────────────────────%s\n", cYellow, cReset)
}

func printLLMResponse(evt agent.TraceEvent) {
	round := evt.Round
	finish, _ := evt.Data["finish_reason"].(string)
	hasToolCalls, _ := evt.Data["has_tool_calls"].(bool)
	tokens, _ := evt.Data["tokens"].(int)
	content, _ := evt.Data["content"].(string)

	fmt.Printf("\n%s┌─ ③ LLM 响应 (第%d轮) ────────────────────────%s\n", cGreen, round, cReset)
	fmt.Printf("%s│%s Finish: %s  Tokens: %d\n", cGreen, cReset, finish, tokens)

	if hasToolCalls {
		if tcsRaw, ok := evt.Data["tool_calls"].([]interface{}); ok {
			fmt.Printf("%s│%s LLM 决定调用 %d 个工具:\n", cGreen, cReset, len(tcsRaw))
			for _, tcRaw := range tcsRaw {
				tc, _ := tcRaw.(map[string]interface{})
				if fn, ok := tc["function"].(map[string]interface{}); ok {
					name, _ := fn["name"].(string)
					args, _ := fn["arguments"].(string)
					fmt.Printf("%s│%s   🔧 %s(%s)\n", cGreen, cReset, name, args)
				}
			}
		}
	} else if content != "" {
		fmt.Printf("%s│%s 文本回复: %s\n", cGreen, cReset, truncateStr(content, 300))
	}
	fmt.Printf("%s└──────────────────────────────────────────────%s\n", cGreen, cReset)
}

func printToolResult(evt agent.TraceEvent) {
	round := evt.Round
	toolName, _ := evt.Data["tool"].(string)
	args, _ := evt.Data["args"].(string)
	success, _ := evt.Data["success"].(bool)
	result, _ := evt.Data["result"].(string)

	status := fmt.Sprintf("%s✓ 成功%s", cGreen, cReset)
	if !success {
		status = fmt.Sprintf("%s✗ 失败%s", cRed, cReset)
	}

	resultDisplay := result
	if len([]rune(resultDisplay)) > 1000 {
		resultDisplay = string([]rune(resultDisplay)[:1000]) + fmt.Sprintf("\n  ... (%d字截断)", len([]rune(result)))
	}

	fmt.Printf("\n%s┌─ ④ Tool 执行 (第%d轮) ───────────────────────%s\n", cCyan, round, cReset)
	fmt.Printf("%s│%s 🔧 %s(%s) %s\n", cCyan, cReset, toolName, args, status)
	fmt.Printf("%s│%s 返回 (%d字):\n", cCyan, cReset, len([]rune(result)))
	for _, line := range strings.Split(resultDisplay, "\n") {
		fmt.Printf("%s│%s   %s\n", cCyan, cReset, line)
	}
	fmt.Printf("%s└──────────────────────────────────────────────%s\n", cCyan, cReset)
}

func printFinalAnswer(evt agent.TraceEvent) {
	raw, _ := evt.Data["raw"].(string)
	cleaned, _ := evt.Data["cleaned"].(string)
	tokens, _ := evt.Data["tokens"].(int)

	wasCleaned := raw != cleaned

	fmt.Printf("\n%s┌─ ⑤ 最终回复 ─────────────────────────────────%s\n", cGreen, cReset)
	fmt.Printf("%s│%s 总 tokens: %d\n", cGreen, cReset, tokens)
	if wasCleaned {
		fmt.Printf("%s│%s 原始 (%d字): %s\n", cGreen, cReset, len([]rune(raw)), truncateStr(raw, 200))
		fmt.Printf("%s│%s 清洗 (%d字): %s\n", cGreen, cReset, len([]rune(cleaned)), truncateStr(cleaned, 200))
	} else {
		fmt.Printf("%s│%s 内容 (%d字): %s\n", cGreen, cReset, len([]rune(cleaned)), truncateStr(cleaned, 300))
	}
	fmt.Printf("%s└──────────────────────────────────────────────%s\n", cGreen, cReset)
}

// ======================== SSE 输出 ========================

func printSSE(event, data string) {
	switch event {
	case "thinking":
		var payload struct {
			Tool   string `json:"tool"`
			Status string `json:"status"`
		}
		if json.Unmarshal([]byte(data), &payload) == nil {
			fmt.Printf("\n  %s💭 thinking: %s %s%s\n", cDim, payload.Tool, payload.Status, cReset)
		}
	case "delta":
		var payload struct {
			Content string `json:"content"`
		}
		if json.Unmarshal([]byte(data), &payload) == nil {
			fmt.Print(payload.Content)
		}
	case "action":
		fmt.Printf("\n  %s🛒 action: %s%s\n", cYellow, data, cReset)
	case "done":
		fmt.Printf("\n\n  %s✅ 完成%s\n", cGreen, cReset)
	case "error":
		fmt.Printf("\n  %s❌ error: %s%s\n", cRed, data, cReset)
	default:
		fmt.Printf("\n  %s[%s] %s%s\n", cDim, event, data, cReset)
	}
}

// ======================== 辅助函数 ========================

func formatRole(role string) string {
	switch role {
	case "system":
		return fmt.Sprintf("%s[SYSTEM]%s", cYellow, cReset)
	case "user":
		return fmt.Sprintf("%s[USER]%s  ", cGreen, cReset)
	case "assistant":
		return fmt.Sprintf("%s[ASSIST]%s", cCyan, cReset)
	case "tool":
		return fmt.Sprintf("%s[TOOL]%s  ", cDim, cReset)
	default:
		return fmt.Sprintf("[%s]", role)
	}
}

func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
