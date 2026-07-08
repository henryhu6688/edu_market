package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"edu_market/config"
	"edu_market/database"
	"edu_market/service/rag"

	"github.com/spf13/viper"
	"gorm.io/gorm/logger"
)

func main() {
	step := flag.String("step", "", "只执行某一步: gen|expand|eval (留空=全流程)")
	queryCount := flag.Int("queries", 100, "生成测试查询数量")
	topK := flag.Int("topk", 5, "评测 TopK 值")
	queriesFile := flag.String("queries-file", "data/eval/rag_queries.json", "标注数据文件路径")
	reportDir := flag.String("report-dir", "data/eval/reports/rag", "报告输出目录")
	flag.Parse()

	// 加载配置
	loadRAGEvalConfig()

	// 初始化数据库（静默模式）
	database.InitLogLevel = logger.Warn
	database.Init()

	// Redis 可选
	database.InitRedis()

	// RAG 初始化
	rag.Init()
	ragSvc := rag.Get()
	if ragSvc == nil {
		fmt.Fprintln(os.Stderr, "RAG 服务初始化失败（Qdrant 未启动？）")
		os.Exit(1)
	}
	fmt.Println("RAG 服务就绪")

	switch *step {
	case "gen":
		runGen(*queryCount, *queriesFile)
	case "expand":
		runExpand(*queriesFile, ragSvc)
	case "eval":
		runEvalOnly(*queriesFile, *topK, ragSvc, *reportDir)
	default:
		runFull(*queryCount, *queriesFile, *topK, ragSvc, *reportDir)
	}
}

func loadRAGEvalConfig() {
	config.App = &config.Config{
		Server:   config.ServerConfig{Port: 8080, Mode: "test"},
		Database: config.DatabaseConfig{Host: "127.0.0.1", Port: 3306, User: "root", Password: readYAML("database.password"), DBName: readYAML("database.dbname"), Charset: "utf8mb4", MaxIdleConns: 5, MaxOpenConns: 10},
		Redis:    config.RedisConfig{Addr: "127.0.0.1:6379", Password: "", DB: 0},
		AI: config.AIConfig{
			Provider: "deepseek",
			APIKey:   readYAML("ai.api_key"),
			APIURL:   readYAML("ai.api_url"),
			Model:    readYAML("ai.model"),
		},
		Agent: config.AgentConfig{
			MaxToolRounds:   10,
			ContextMaxMsg:   20,
			ChunkSize:       500,
			ChunkOverlap:    50,
			EmbeddingModel:  "BAAI/bge-m3",
			EmbeddingAPIURL: "https://api.siliconflow.cn/v1/embeddings",
			EmbeddingAPIKey: readYAML("agent.embedding_api_key"),
		},
		RAG: config.RAGConfig{
			QdrantURL:        "http://localhost:6335",
			QdrantCollection: "chunks",
			HybridSearch:     true,
			Rerank:           true,
			RerankTopK:       5,
			CleanerEnabled:   true,
			StructuralChunk:  true,
			ChunkSize:        500,
			ChunkMin:         300,
			ChunkMax:         800,
			BM25Enabled:      true,
			CacheEnabled:     true,
			CacheTTL:         3600,
			CacheSimThreshold: 0.85,
			CacheMaxEntries:  10,
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
		config.App.Database.Password = readYAML("database.password")
	}
}

func readYAML(key string) string {
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

// runFull 全流程。
func runFull(count int, file string, topK int, ragSvc *rag.RAGService, reportDir string) {
	ts := time.Now().Format("20060102_150405")
	reportRunDir := reportDir + "/" + ts

	// Step 1
	fmt.Println("=== Step 1: 生成测试问题 ===")
	queries, err := generateQueries(count)
	if err != nil {
		fmt.Fprintf(os.Stderr, "生成问题失败: %v\n", err)
		os.Exit(1)
	}
	saveQueries(queries, file)
	fmt.Printf("生成 %d 条问题 → %s\n", len(queries), file)

	// Step 2
	fmt.Println("\n=== Step 2: 扩展 ground truth ===")
	queries, err = expandGroundTruth(queries, ragSvc, file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "扩展 ground truth 失败: %v\n", err)
	}
	saveQueries(queries, file)
	incomplete := 0
	for _, q := range queries {
		if q.GTIncomplete {
			incomplete++
		}
	}
	fmt.Printf("扩展完成，%d/%d 条已补全\n", len(queries)-incomplete, len(queries))

	// Step 3
	fmt.Println("\n=== Step 3: 分层评测 ===")
	configs := defaultEvalConfigs()
	groups, err := runEval(queries, configs, topK, ragSvc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "评测失败: %v\n", err)
		os.Exit(1)
	}

	// 报告
	report := generateReport(groups, topK)
	path, err := saveReport(report, reportRunDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "保存报告失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\n=== 评测完成 ===\n")
	fmt.Printf("报告: %s\n", path)
}

func runGen(count int, file string) {
	queries, err := generateQueries(count)
	if err != nil {
		fmt.Fprintf(os.Stderr, "生成问题失败: %v\n", err)
		os.Exit(1)
	}
	saveQueries(queries, file)
	fmt.Printf("生成 %d 条问题 → %s\n", len(queries), file)
}

func runExpand(file string, ragSvc *rag.RAGService) {
	queries, err := loadQueries(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载标注数据失败: %v\n", err)
		os.Exit(1)
	}
	queries, err = expandGroundTruth(queries, ragSvc, file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "扩展失败: %v\n", err)
	}
	saveQueries(queries, file)
	fmt.Printf("扩展完成，保存至 %s\n", file)
}

func runEvalOnly(file string, topK int, ragSvc *rag.RAGService, reportDir string) {
	queries, err := loadQueries(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载标注数据失败: %v\n", err)
		os.Exit(1)
	}
	configs := defaultEvalConfigs()
	groups, err := runEval(queries, configs, topK, ragSvc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "评测失败: %v\n", err)
		os.Exit(1)
	}
	report := generateReport(groups, topK)
	ts := time.Now().Format("20060102_150405")
	path, _ := saveReport(report, reportDir+"/"+ts)
	fmt.Printf("评测完成，报告: %s\n", path)
}

// saveQueries 保存标注数据到 JSON 文件。
func saveQueries(queries []RAGQuery, path string) {
	b, _ := json.MarshalIndent(queries, "", "  ")
	os.MkdirAll("data/eval", 0755)
	os.WriteFile(path, b, 0644)
}

// loadQueries 从 JSON 文件读取标注数据。
func loadQueries(path string) ([]RAGQuery, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var queries []RAGQuery
	if err := json.Unmarshal(b, &queries); err != nil {
		return nil, err
	}
	return queries, nil
}
