package service

import (
	"testing"

	"edu_market/database"
	"edu_market/model"
)

// cleanTestReviews 清理测试评论
func cleanTestReviews(t *testing.T, ids ...uint) {
	for _, id := range ids {
		database.DB.Delete(&model.Review{}, id)
	}
}

// TestReviewCreateNoPurchase 测试未购买无法评论
func TestReviewCreateNoPurchase(t *testing.T) {
	course := createTestCourse(t, "测试评论-未购买")
	defer cleanTestCourse(t, course)

	svc := &ReviewService{}
	_, err := svc.Create(course.UserID, course.ID, 5, "好评")
	if err == nil {
		t.Error("未购买课程应该不能评论")
	}
	if err != nil && err.Error() != "请先购买课程后再评论" {
		t.Errorf("错误信息不匹配: %s", err.Error())
	}
}

// TestReviewCreateCourseNotFound 测试课程不存在
func TestReviewCreateCourseNotFound(t *testing.T) {
	svc := &ReviewService{}
	_, err := svc.Create(1, 99999, 5, "好评")
	if err == nil {
		t.Error("课程不存在应该返回错误")
	}
}

// TestReviewListByCourse 测试课程评论列表
func TestReviewListByCourse(t *testing.T) {
	svc := &ReviewService{}
	reviews, total, err := svc.ListByCourse(1, 1, 10)
	if err != nil {
		t.Fatalf("获取评论列表失败: %v", err)
	}
	t.Logf("课程1的评论数: %d, 当前页: %d", total, len(reviews))
}

// TestReviewListEmpty 测试无评论课程
func TestReviewListEmpty(t *testing.T) {
	svc := &ReviewService{}
	reviews, _, err := svc.ListByCourse(99999, 1, 10)
	if err != nil {
		t.Fatalf("获取评论列表失败: %v", err)
	}
	if len(reviews) != 0 {
		t.Errorf("不存在的课程评论应为空，实际: %d", len(reviews))
	}
}

// TestReviewCreateWithPurchase 测试购买后评论（需要完整购买流程）
func TestReviewCreateWithPurchase(t *testing.T) {
	course := createTestCourse(t, "测试评论-已购买")
	defer cleanTestCourse(t, course)

	// 创建订单并支付
	orderSvc := &OrderService{}
	order, err := orderSvc.Create(course.UserID, course.ID)
	if err != nil {
		t.Fatalf("创建订单失败: %v", err)
	}
	defer database.DB.Where("order_no = ?", order.OrderNo).Delete(&model.Order{})

	if err := orderSvc.Pay(order.OrderNo, course.UserID); err != nil {
		t.Fatalf("支付失败: %v", err)
	}

	// 评论
	svc := &ReviewService{}
	review, err := svc.Create(course.UserID, course.ID, 4, "还不错")
	if err != nil {
		t.Fatalf("创建评论失败: %v", err)
	}
	defer cleanTestReviews(t, review.ID)

	if review.Rating != 4 {
		t.Errorf("评分应为 4，实际: %d", review.Rating)
	}
	if review.Content != "还不错" {
		t.Errorf("内容应为'还不错'，实际: %s", review.Content)
	}
}
