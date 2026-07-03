//go:build ignore

package main

import (
	"fmt"
	"edu_market/database"
	"edu_market/config"
	"edu_market/model"
	"time"
)

func main() {
	config.App = &config.Config{
		Database: config.DatabaseConfig{
			Host: "127.0.0.1", Port: 3306, User: "root", Password: "123456",
			DBName: "edu_market", Charset: "utf8mb4",
		},
	}
	database.Init()

	evalUserID := uint(10)

	// 1. 给 eval_test_user 创建购买记录
	orders := []struct{ mid uint; price float64; title string }{
		{2, 19.90, "Python 从入门到实战"},
		{7, 29.90, "Go 语言教程"},
	}
	for _, o := range orders {
		var count int64
		database.DB.Model(&model.Order{}).Where("user_id = ? AND course_id = ?", evalUserID, o.mid).Count(&count)
		if count > 0 {
			fmt.Printf("跳过已存在的订单: material #%d\n", o.mid)
			continue
		}
		now := time.Now()
		order := model.Order{
			OrderNo:  fmt.Sprintf("EVAL-ORDER-%d-%d", evalUserID, o.mid),
			UserID:   evalUserID,
			CourseID: o.mid,
			Amount:   o.price,
			Status:   "paid",
			PaidAt:   &now,
		}
		database.DB.Create(&order)
		fmt.Printf("创建购买: user=%d material=#%d(%s) ¥%.2f\n", evalUserID, o.mid, o.title, o.price)
	}

	// 2. 为 eval_test_user 创建一个发布资料（用于测试发布者场景）
	var publishedCount int64
	database.DB.Model(&model.Material{}).Where("user_id = ? AND status = ?", evalUserID, "published").Count(&publishedCount)
	if publishedCount == 0 {
		m := model.Material{
			Title:       "Eval 测试 — Python 入门",
			Description: "评估测试专用资料，Python 基础入门内容",
			Price:       9.90,
			CategoryID:  1, // 编程开发
			UserID:      evalUserID,
			Status:      "published",
		}
		database.DB.Create(&m)
		fmt.Printf("创建发布资料: #%d \"%s\"\n", m.ID, m.Title)
	} else {
		fmt.Printf("eval_test_user 已有 %d 份发布资料\n", publishedCount)
		var mat model.Material
		database.DB.Where("user_id = ? AND status = ?", evalUserID, "published").First(&mat)
		fmt.Printf("  -> #%d \"%s\"\n", mat.ID, mat.Title)
	}

	// 3. 列出最终状态
	fmt.Println("\n=== eval_test_user 最终状态 ===")
	fmt.Printf("用户 ID=%d\n", evalUserID)
	var userOrders []model.Order
	database.DB.Where("user_id = ?", evalUserID).Order("id ASC").Find(&userOrders)
	fmt.Printf("购买订单 %d 个:\n", len(userOrders))
	for _, o := range userOrders {
		fmt.Printf("  - order #%d: material=%d status=%s\n", o.ID, o.CourseID, o.Status)
	}
	var mats []model.Material
	database.DB.Where("user_id = ?", evalUserID).Order("id ASC").Find(&mats)
	fmt.Printf("发布资料 %d 个:\n", len(mats))
	for _, m := range mats {
		fmt.Printf("  - material #%d: \"%s\" status=%s\n", m.ID, m.Title, m.Status)
	}
}
