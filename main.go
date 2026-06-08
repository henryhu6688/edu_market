package main

import (
	"fmt"
	"log"

	"edu_market/config"
	"edu_market/database"
	"edu_market/router"
)

func main() {
	// 加载配置
	config.Load()

	// 初始化数据库
	database.Init()

	// 设置路由
	r := router.Setup()

	// 启动服务
	addr := fmt.Sprintf(":%d", config.App.Server.Port)
	log.Printf("edu_market 服务启动于 http://localhost%s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
