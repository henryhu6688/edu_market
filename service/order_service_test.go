package service

import (
	"testing"

	"edu_market/database"
	"edu_market/model"
)

// cleanTestOrders 清理测试订单
func cleanTestOrders(t *testing.T, orderNos ...string) {
	for _, no := range orderNos {
		database.DB.Where("order_no = ?", no).Delete(&model.Order{})
	}
}

// TestOrderCreate 测试创建订单
func TestOrderCreate(t *testing.T) {
	// 先确保有一个课程
	course := createTestCourse(t, "测试订单课程")
	defer cleanTestCourse(t, course)

	svc := &OrderService{}
	order, err := svc.Create(1, course.ID)
	if err != nil {
		t.Fatalf("创建订单失败: %v", err)
	}
	defer cleanTestOrders(t, order.OrderNo)

	if order.OrderNo == "" {
		t.Error("订单号不应为空")
	}
	if order.Status != "pending" {
		t.Errorf("订单状态应为 pending，实际: %s", order.Status)
	}
	if order.Amount != course.Price {
		t.Errorf("订单金额应为 %.2f，实际: %.2f", course.Price, order.Amount)
	}
}

// TestOrderCreateCourseNotFound 测试课程不存在
func TestOrderCreateCourseNotFound(t *testing.T) {
	svc := &OrderService{}
	_, err := svc.Create(1, 99999)
	if err == nil {
		t.Error("课程不存在应该返回错误")
	}
}

// TestOrderListByUser 测试用户订单列表
func TestOrderListByUser(t *testing.T) {
	course := createTestCourse(t, "测试用户订单列表")
	defer cleanTestCourse(t, course)

	svc := &OrderService{}
	order, _ := svc.Create(1, course.ID)
	defer cleanTestOrders(t, order.OrderNo)

	orders, total, err := svc.ListByUser(1, 1, 10, "")
	if err != nil {
		t.Fatalf("获取订单列表失败: %v", err)
	}
	if total < 1 {
		t.Error("应有至少1条订单")
	}
	if len(orders) < 1 {
		t.Error("订单列表不应为空")
	}
}

// TestOrderPay 测试支付订单
func TestOrderPay(t *testing.T) {
	course := createTestCourse(t, "测试支付课程")
	defer cleanTestCourse(t, course)

	svc := &OrderService{}
	order, _ := svc.Create(1, course.ID)
	defer cleanTestOrders(t, order.OrderNo)

	err := svc.Pay(order.OrderNo, 1)
	if err != nil {
		t.Fatalf("支付失败: %v", err)
	}

	// 验证状态变更
	var updated model.Order
	database.DB.Where("order_no = ?", order.OrderNo).First(&updated)
	if updated.Status != "paid" {
		t.Errorf("支付后状态应为 paid，实际: %s", updated.Status)
	}
	if updated.PaidAt == nil {
		t.Error("支付时间不应为空")
	}
}

// TestOrderPayNotFound 测试支付不存在订单
func TestOrderPayNotFound(t *testing.T) {
	svc := &OrderService{}
	err := svc.Pay("ORD_NOT_EXIST", 1)
	if err == nil {
		t.Error("支付不存在的订单应该返回错误")
	}
}

// TestOrderPayAlreadyPaid 测试重复支付
func TestOrderPayAlreadyPaid(t *testing.T) {
	course := createTestCourse(t, "测试重复支付")
	defer cleanTestCourse(t, course)

	svc := &OrderService{}
	order, _ := svc.Create(1, course.ID)
	defer cleanTestOrders(t, order.OrderNo)

	svc.Pay(order.OrderNo, 1)
	err := svc.Pay(order.OrderNo, 1)
	if err == nil {
		t.Error("重复支付应该返回错误")
	}
}

// TestOrderListByStatus 测试按状态筛选订单
func TestOrderListByStatus(t *testing.T) {
	course := createTestCourse(t, "测试筛选订单")
	defer cleanTestCourse(t, course)

	svc := &OrderService{}
	order, _ := svc.Create(1, course.ID)
	defer cleanTestOrders(t, order.OrderNo)

	// 查 pending 状态
	_, total, err := svc.ListByUser(1, 1, 10, "pending")
	if err != nil {
		t.Fatalf("获取pending订单失败: %v", err)
	}
	if total < 1 {
		t.Error("应有pending订单")
	}

	// 查 paid 状态（还未支付，应该为空）
	_, total, _ = svc.ListByUser(1, 1, 10, "paid")
	if total > 0 {
		t.Log("有 paid 订单（可能是之前测试残留）")
	}
}
