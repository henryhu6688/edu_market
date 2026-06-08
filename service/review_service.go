package service

import (
	"errors"

	"edu_market/database"
	"edu_market/model"
)

// ReviewService 评论服务
type ReviewService struct{}

// Create 创建评论
func (s *ReviewService) Create(userID, courseID uint, rating int, content string) (*model.Review, error) {
	// 检查课程是否存在
	var course model.Course
	if err := database.DB.First(&course, courseID).Error; err != nil {
		return nil, errors.New("课程不存在")
	}

	// 检查是否已购买
	var order model.Order
	if err := database.DB.Where("user_id = ? AND course_id = ? AND status = ?",
		userID, courseID, "paid").First(&order).Error; err != nil {
		return nil, errors.New("请先购买课程后再评论")
	}

	review := &model.Review{
		UserID:   userID,
		CourseID: courseID,
		Rating:   rating,
		Content:  content,
	}

	if err := database.DB.Create(review).Error; err != nil {
		return nil, errors.New("评论失败")
	}

	return review, nil
}

// ListByCourse 课程评论列表
func (s *ReviewService) ListByCourse(courseID uint, page, pageSize int) ([]model.Review, int64, error) {
	var reviews []model.Review
	var total int64

	page, pageSize = GetPagination(page, pageSize)

	database.DB.Model(&model.Review{}).Where("course_id = ?", courseID).Count(&total)
	if err := database.DB.Where("course_id = ?", courseID).
		Preload("User").Offset((page-1)*pageSize).Limit(pageSize).
		Order("created_at DESC").Find(&reviews).Error; err != nil {
		return nil, 0, err
	}

	return reviews, total, nil
}
