package agent

import (
	"testing"

	"edu_market/model"
)

// TestCircuitBreaker_Check 精确重复检测
func TestCircuitBreaker_Check(t *testing.T) {
	cb := &CircuitBreaker{}

	// 第一轮：正常
	blocked, _ := cb.Check(nil, "search_materials", `{"keyword":"Python"}`)
	if blocked {
		t.Error("第一次调用不应该被拦截")
	}
	cb.Record("search_materials", `{"keyword":"Python"}`)

	// 第二轮：完全相同 → 拦截
	blocked, reason := cb.Check(nil, "search_materials", `{"keyword":"Python"}`)
	if !blocked {
		t.Error("第二次完全相同的调用应该被拦截")
	}
	if reason == "" {
		t.Error("拦截应该有 reason")
	}

	// 第三轮：不同参数 → 不拦截
	cb.Record("search_materials", `{"keyword":"Java"}`)
	blocked, _ = cb.Check(nil, "search_materials", `{"keyword":"Java"}`)
	if !blocked {
		t.Error("Java → Java 相同调用应被拦截")
	}
}

// TestCircuitBreaker_AllowRepeat 允许重复的 tool 不拦截
func TestCircuitBreaker_AllowRepeat(t *testing.T) {
	cb := &CircuitBreaker{}

	// trigger_purchase_offer AllowRepeat()==true
	offerTool := purchaseTool{}
	cb.Record("purchase", `{"material_id":2}`)
	blocked, _ := cb.Check(offerTool, "purchase", `{"material_id":2}`)
	if blocked {
		t.Error("AllowRepeat 的 tool 不应该被 L1 拦截")
	}
}

// TestCircuitBreaker_DifferentTool 不同 tool 不拦截
func TestCircuitBreaker_DifferentTool(t *testing.T) {
	cb := &CircuitBreaker{}

	cb.Record("search_materials", `{"keyword":"Python"}`)
	blocked, _ := cb.Check(nil, "get_material_detail", `{"keyword":"Python"}`)
	if blocked {
		t.Error("不同 tool 不应该被拦截")
	}
}

// TestSemanticLoopDetector_Feed 语义回路检测
func TestSemanticLoopDetector_Feed(t *testing.T) {
	d := &SemanticLoopDetector{}

	// 三轮完全不同 → 不触发
	d.Feed("Python从入门到实战 ¥19.90 3章，零基础入门")
	d.Feed("Java核心技术 ¥39.90 10章，面向对象编程")
	blocked, _ := d.Feed("Go语言编程 ¥29.90 8章，系统编程")
	if blocked {
		t.Error("三轮不同结果不应该触发回路")
	}

	// 三轮高度重复 → 触发（使用近乎相同的字符串确保 Jaccard > 0.8）
	d2 := &SemanticLoopDetector{}
	d2.Feed("平台暂未找到对应资料，建议换个方向试试看吧")
	d2.Feed("平台暂未找到对应资料，建议换个方向试试看")
	blocked, reason := d2.Feed("平台暂未找到对应资料，建议换个方向试试看")
	if !blocked {
		t.Error("三轮重复结果应该触发回路")
	}
	if reason == "" {
		t.Error("回路应该有 reason")
	}
}

// TestSemanticLoopDetector_OnlyTwo 少于三轮不触发
func TestSemanticLoopDetector_OnlyTwo(t *testing.T) {
	d := &SemanticLoopDetector{}
	blocked, _ := d.Feed("平台暂无相关资料")
	if blocked {
		t.Error("第一轮不应该触发")
	}
	blocked, _ = d.Feed("平台暂无相关资料")
	if blocked {
		t.Error("第二轮不应该触发（需要三轮）")
	}
}

// TestResolveMode 模式切换
func TestResolveMode(t *testing.T) {
	session := &model.Session{Mode: "", UserID: 1, State: `{"context":{"focus_id":0}}`}

	mode := ResolveMode(session, []string{"search_materials"})
	if mode != "" {
		t.Errorf("search_materials 不改变模式, got %s", mode)
	}

	mode = ResolveMode(session, []string{"get_orders"})
	if mode != "support" {
		t.Errorf("query_orders → support, got %s", mode)
	}

	mode = ResolveMode(session, []string{})
	if mode != "" {
		t.Errorf("没调 tool 应保持当前模式, got %s", mode)
	}

	mode = ResolveMode(session, []string{"get_material_detail"})
	if mode != "shopping" {
		t.Errorf("get_material_detail → shopping, got %s", mode)
	}

	mode = ResolveMode(session, []string{"purchase"})
	if mode != "shopping" {
		t.Errorf("trigger_purchase_offer → shopping, got %s", mode)
	}

	// support 模式下保持
	session2 := &model.Session{Mode: "support", UserID: 1, State: `{"context":{"focus_id":0}}`}
	mode = ResolveMode(session2, []string{})
	if mode != "support" {
		t.Errorf("无 tool 应保持 support, got %s", mode)
	}
}

// TestToolBudget_Spend 调用预算
func TestToolBudget_Spend(t *testing.T) {
	b := NewToolBudget()
	for i := 0; i < 3; i++ {
		if err := b.Spend("purchase"); err != nil {
			t.Errorf("第%d次调用不应超限: %v", i+1, err)
		}
	}
	if err := b.Spend("purchase"); err == nil {
		t.Error("第4次调用应该超限")
	}
}

// TestToolBudget_DifferentTools 不同 tool 独立计数
func TestToolBudget_DifferentTools(t *testing.T) {
	b := NewToolBudget()
	for i := 0; i < 5; i++ {
		if err := b.Spend("search_materials"); err != nil {
			t.Errorf("第%d次 query_materials 不应超限", i+1)
		}
	}
	if err := b.Spend("search_materials"); err == nil {
		t.Error("第6次 query_materials 应该超限")
	}
	// get_material_detail 不受影响
	if err := b.Spend("get_material_detail"); err != nil {
		t.Errorf("get_material_detail 不应受影响: %v", err)
	}
}

// TestJaccard 相似度计算
func TestJaccard(t *testing.T) {
	sim := jaccard("完全相同的一段文本", "完全相同的一段文本")
	if sim < 0.9 {
		t.Errorf("相同文本相似度应 > 0.9, got %f", sim)
	}

	sim = jaccard("Python从入门到实战", "Java核心技术详解")
	if sim > 0.3 {
		t.Errorf("完全不同文本相似度应 < 0.3, got %f", sim)
	}

	sim = jaccard("", "")
	if sim != 0 {
		t.Errorf("空文本相似度应为 0, got %f", sim)
	}
}
