package service

import (
	"errors"
	"fmt"
	"time"

	"edu-market/database"
	"edu-market/model"

	"gorm.io/gorm"
)

// OrderService 订单服务
type OrderService struct{}

// Create 创建订单
func (s *OrderService) Create(userID, courseID uint) (*model.Order, error) {
	// 查询课程
	var course model.Course
	if err := database.DB.First(&course, courseID).Error; err != nil {
		return nil, errors.New("课程不存在")
	}

	// 检查是否已购买
	var existOrder model.Order
	if err := database.DB.Where("user_id = ? AND course_id = ? AND status = ?",
		userID, courseID, "paid").First(&existOrder).Error; err == nil {
		return nil, errors.New("已购买过该课程")
	}

	// 生成订单号
	orderNo := fmt.Sprintf("ORD%d%d", time.Now().UnixMilli(), userID)

	order := &model.Order{
		OrderNo:  orderNo,
		UserID:   userID,
		CourseID: courseID,
		Amount:   course.Price,
		Status:   "pending",
	}

	if err := database.DB.Create(order).Error; err != nil {
		return nil, errors.New("创建订单失败")
	}

	return order, nil
}

// ListByUser 用户订单列表
func (s *OrderService) ListByUser(userID uint, page, pageSize int, status string) ([]model.Order, int64, error) {
	var orders []model.Order
	var total int64

	page, pageSize = GetPagination(page, pageSize)

	query := database.DB.Model(&model.Order{}).Where("user_id = ?", userID)
	if status != "" {
		query = query.Where("status = ?", status)
	}

	query.Count(&total)
	if err := query.Preload("Course").Offset((page - 1) * pageSize).Limit(pageSize).
		Order("created_at DESC").Find(&orders).Error; err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}

// Pay 模拟支付
func (s *OrderService) Pay(orderNo string, userID uint) error {
	var order model.Order
	if err := database.DB.Where("order_no = ? AND user_id = ?", orderNo, userID).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("订单不存在")
		}
		return err
	}

	if order.Status != "pending" {
		return errors.New("订单状态不正确")
	}

	now := time.Now()
	return database.DB.Model(&order).Updates(map[string]interface{}{
		"status":  "paid",
		"paid_at": &now,
	}).Error
}
