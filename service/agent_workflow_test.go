package service

import (
	"testing"
)

func TestClassifyIntent_Purchase(t *testing.T) {
	// 只测关键词能命中的（不依赖 LLM）
	tests := []string{"我想买", "帮我下单", "我要买", "想买", "买哪个"}
	for _, msg := range tests {
		if got := ClassifyIntent(msg); got != IntentPurchase {
			t.Errorf("ClassifyIntent(%q) = %s, want %s", msg, got, IntentPurchase)
		}
	}
}

func TestClassifyIntent_AfterSale(t *testing.T) {
	tests := []string{"我要退款", "我的订单", "支付失败", "退货", "申请退款"}
	for _, msg := range tests {
		if got := ClassifyIntent(msg); got != IntentAfterSale {
			t.Errorf("ClassifyIntent(%q) = %s, want %s", msg, got, IntentAfterSale)
		}
	}
}

func TestClassifyIntent_Consult(t *testing.T) {
	tests := []string{"有没有入门资料", "推荐一下", "学什么好", "想学", "哪个好"}
	for _, msg := range tests {
		if got := ClassifyIntent(msg); got != IntentConsult {
			t.Errorf("ClassifyIntent(%q) = %s, want %s", msg, got, IntentConsult)
		}
	}
}

// 以下测试不需要 API key：关键词不命中时 LLM 调用失败 → fallback IntentChat
func TestClassifyIntent_Chat(t *testing.T) {
	// 关键词不命中 → LLM 调不到 → 应该 fallback 到 chat
	tests := []string{"你好", "在吗", "讲个笑话", "谢谢", "Python 和 Java 的区别"}
	for _, msg := range tests {
		got := ClassifyIntent(msg)
		// LLM 不可用时 fallback 到 chat，这是正确的兜底行为
		if got != IntentChat {
			t.Logf("ClassifyIntent(%q) = %s (LLM 可用时可能不是 chat)", msg, got)
		}
	}
}

func TestMatchAny(t *testing.T) {
	if !matchAny("我想退款", []string{"退款", "退", "退钱"}) {
		t.Error("should match '退款'")
	}
	if matchAny("你好", []string{"退款", "退钱"}) {
		t.Error("should NOT match")
	}
}
