//go:build ignore

package main

import (
	"fmt"
	"log"

	"edu_market/config"
	"edu_market/database"
	"edu_market/model"
	"edu_market/service/rag"

	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	// 加载配置
	config.App = &config.Config{
		Server: config.ServerConfig{Port: 8080, Mode: "debug"},
		AI:     config.AIConfig{Provider: "deepseek", APIURL: "https://api.deepseek.com/v1/chat/completions", Model: "deepseek-chat"},
		Agent:  config.AgentConfig{EmbeddingModel: "BAAI/bge-large-zh-v1.5", EmbeddingAPIURL: "https://api.siliconflow.cn/v1/embeddings", ChunkSize: 500, ChunkOverlap: 50},
	}
	// 读 API key from app.yml
	viper.SetConfigName("app")
	viper.SetConfigType("yml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("../config")
	viper.ReadInConfig()
	config.App.AI.APIKey = viper.GetString("ai.api_key")
	config.App.Agent.EmbeddingAPIKey = viper.GetString("agent.embedding_api_key")
	if m := viper.GetString("agent.embedding_model"); m != "" {
		config.App.Agent.EmbeddingModel = m
	}
	if u := viper.GetString("agent.embedding_api_url"); u != "" {
		config.App.Agent.EmbeddingAPIURL = u
	}
	if config.App.Agent.EmbeddingModel == "" {
		config.App.Agent.EmbeddingModel = "BAAI/bge-large-zh-v1.5"
	}
	if config.App.Agent.EmbeddingAPIURL == "" {
		config.App.Agent.EmbeddingAPIURL = "https://api.siliconflow.cn/v1/embeddings"
	}

	// 连数据库
	dsn := "root:123456@tcp(127.0.0.1:3306)/edu_market?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("DB: %v", err)
	}
	database.DB = db

	// Redis (可选)
	database.InitRedis()

	// 初始化 RAG
	rag.Init()
	ragSvc := rag.Get()
	if ragSvc == nil {
		log.Fatal("RAG 初始化失败")
	}

	// 遍历所有文档，按 material 分组索引
	var materials []model.Material
	database.DB.Preload("Documents").Find(&materials)

	for _, m := range materials {
		if len(m.Documents) == 0 {
			continue
		}
		// 拼合文档内容
		var fullText string
		for _, d := range m.Documents {
			fullText += d.Content + "\n\n"
		}
		fmt.Printf("Indexing material %d: %s (%d docs, %d chars)\n", m.ID, m.Title, len(m.Documents), len(fullText))
		if err := ragSvc.IndexCourse(m.ID, fullText); err != nil {
			fmt.Printf("  ERROR: %v\n", err)
		} else {
			fmt.Printf("  OK\n")
		}
	}

	fmt.Println("=== DONE ===")
}
