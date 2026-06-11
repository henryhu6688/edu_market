package service

import (
	"errors"

	"edu_market/database"
	"edu_market/model"
)

// DocumentService 在线文档服务
type DocumentService struct{}

// Create 创建文档（仅资料发布者）
func (s *DocumentService) Create(doc *model.Document, userID uint) error {
	var material model.Material
	if err := database.DB.First(&material, doc.MaterialID).Error; err != nil {
		return errors.New("资料不存在")
	}
	if material.UserID != userID {
		return errors.New("只有资料发布者可以添加文档")
	}
	return database.DB.Create(doc).Error
}

// GetByID 获取文档内容（含购买权限校验）
func (s *DocumentService) GetByID(docID, userID uint) (*model.Document, error) {
	var doc model.Document
	if err := database.DB.First(&doc, docID).Error; err != nil {
		return nil, errors.New("文档不存在")
	}

	// 试读文档直接放行
	if doc.IsFreePreview {
		return &doc, nil
	}

	var material model.Material
	database.DB.First(&material, doc.MaterialID)

	// 发布者可看
	if material.UserID == userID {
		return &doc, nil
	}

	// 已购买可看（order.course_id 存 material_id）
	var count int64
	database.DB.Model(&model.Order{}).
		Where("user_id = ? AND course_id = ? AND status = ?", userID, material.ID, "paid").
		Count(&count)
	if count > 0 {
		return &doc, nil
	}

	return nil, errors.New("请先购买该资料")
}

// GetTree 获取文档目录树（不含 content）
func (s *DocumentService) GetTree(materialID uint) ([]model.Document, error) {
	var docs []model.Document
	if err := database.DB.Where("material_id = ?", materialID).
		Order("sort_order ASC, id ASC").Find(&docs).Error; err != nil {
		return nil, err
	}
	return docs, nil
}

// Update 更新文档（仅发布者，自动保存触发 RAG）
func (s *DocumentService) Update(docID, userID uint, updates map[string]interface{}) error {
	var doc model.Document
	if err := database.DB.First(&doc, docID).Error; err != nil {
		return errors.New("文档不存在")
	}
	var material model.Material
	database.DB.First(&material, doc.MaterialID)
	if material.UserID != userID {
		return errors.New("只有资料发布者可以编辑文档")
	}

	if err := database.DB.Model(&doc).Updates(updates).Error; err != nil {
		return err
	}

	// content 更新后异步触发 RAG 重新切片
	if _, ok := updates["content"]; ok {
		go reindexDocument(&doc)
	}
	return nil
}

// Delete 删除文档（仅发布者）
func (s *DocumentService) Delete(docID, userID uint) error {
	var doc model.Document
	if err := database.DB.First(&doc, docID).Error; err != nil {
		return errors.New("文档不存在")
	}
	var material model.Material
	database.DB.First(&material, doc.MaterialID)
	if material.UserID != userID {
		return errors.New("只有资料发布者可以删除文档")
	}
	return database.DB.Delete(&doc).Error
}

// reindexDocument 从 Tiptap JSON 提取纯文本 → 切片 → 向量入库
func reindexDocument(doc *model.Document) {
	text := extractTextFromTiptapJSON(doc.Content)
	database.DB.Where("course_id = ?", doc.MaterialID).Delete(&model.DocumentChunk{})
	if rag := GetRAG(); rag != nil {
		rag.IndexCourse(doc.MaterialID, text)
	}
}
