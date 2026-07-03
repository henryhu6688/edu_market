//go:build ignore

package main

import (
	"fmt"
	"edu_market/database"
	"edu_market/config"
	"edu_market/model"
)

func main() {
	config.App = &config.Config{
		Database: config.DatabaseConfig{
			Host: "127.0.0.1", Port: 3306, User: "root", Password: "123456",
			DBName: "edu_market", Charset: "utf8mb4",
		},
	}
	database.Init()

	fmt.Println("=== Categories ===")
	var cats []model.Category
	database.DB.Order("id ASC").Find(&cats)
	for _, c := range cats {
		fmt.Printf("#%d | %s | parent=%d\n", c.ID, c.Name, c.ParentID)
	}

	fmt.Println("\n=== Published Materials + Doc Count ===")
	var materials []model.Material
	database.DB.Where("status = ?", "published").Order("id ASC").Find(&materials)
	for _, m := range materials {
		var docCount int64
		database.DB.Model(&model.Document{}).Where("material_id = ?", m.ID).Count(&docCount)
		fmt.Printf("#%d | %-30s | ¥%6.2f | owner=%d | docs=%d\n", m.ID, m.Title, m.Price, m.UserID, docCount)
	}
}
