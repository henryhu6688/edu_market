package agent

import (
	"strings"
	"testing"
)

// TestHardFieldCorrector_Correct 硬字段修正
func TestHardFieldCorrector_Correct(t *testing.T) {
	c := &HardFieldCorrector{}
	facts := []FactItem{
		{Content: "《Python 从入门到实战》¥19.90 3章", Source: "get_material_detail(2)"},
	}

	answer := "推荐《Python入门》，只要 ¥9.90，很划算"
	corrected := c.Correct(answer, facts, "test")

	if !strings.Contains(corrected, "《Python 从入门到实战》") {
		t.Errorf("资料名应被修正为事实值, got: %s", corrected)
	}
	if !strings.Contains(corrected, "¥19.90") {
		t.Errorf("价格应被修正为事实值, got: %s", corrected)
	}
}

// TestHardFieldCorrector_NoCorrection 正确内容不误修
func TestHardFieldCorrector_NoCorrection(t *testing.T) {
	c := &HardFieldCorrector{}
	facts := []FactItem{
		{Content: "《Python 从入门到实战》¥19.90", Source: "get_material_detail(2)"},
	}

	answer := "推荐《Python 从入门到实战》，只要 ¥19.90"
	corrected := c.Correct(answer, facts, "test")

	if corrected != answer {
		t.Errorf("正确内容不应被修改, got: %s", corrected)
	}
}

// TestHardFieldCorrector_EmptyFacts 无 facts 时不修改
func TestHardFieldCorrector_EmptyFacts(t *testing.T) {
	c := &HardFieldCorrector{}
	answer := "推荐《Python入门》，只要 ¥9.90"
	corrected := c.Correct(answer, nil, "test")

	if corrected != answer {
		t.Errorf("无 facts 时不应修改, got: %s", corrected)
	}
}

// TestHardFieldCorrector_NoPrice 无价格时不影响
func TestHardFieldCorrector_NoPrice(t *testing.T) {
	c := &HardFieldCorrector{}
	facts := []FactItem{
		{Content: "用户水平：零基础", Source: "user:said"},
	}

	answer := "推荐《Python入门》，只要 ¥9.90"
	corrected := c.Correct(answer, facts, "test")

	// facts 中没有价格/资料名，不应修改
	if corrected != answer {
		t.Errorf("无价格/资料名事实时不应修改, got: %s", corrected)
	}
}

// TestExtractHardFields 提取硬字段
func TestExtractHardFields(t *testing.T) {
	facts := []FactItem{
		{Content: "《Python 从入门到实战》¥19.90 3章", Source: "get_material_detail(2)"},
		{Content: "《Java核心技术》¥39.90 10章", Source: "get_material_detail(5)"},
	}

	result := extractHardFields(facts)

	// 应该提取第一个 price 和 title（map 只存一个 key）
	if result["price"] == "" && result["title"] == "" {
		t.Error("应该提取到价格和资料名")
	}
}

// TestExtractHardFieldsFromText 从文本提取
func TestExtractHardFieldsFromText(t *testing.T) {
	text := "推荐《Python入门》，只要 ¥9.90，很划算"
	fields := extractHardFieldsFromText(text)

	if len(fields) == 0 {
		t.Error("应该提取到硬字段")
	}

	hasPrice := false
	hasTitle := false
	for _, f := range fields {
		if f.field == "price" && f.value == "¥9.90" {
			hasPrice = true
		}
		if f.field == "title" && f.value == "《Python入门》" {
			hasTitle = true
		}
	}
	if !hasPrice {
		t.Error("应该提取到价格 ¥9.90")
	}
	if !hasTitle {
		t.Error("应该提取到资料名《Python入门》")
	}
}
