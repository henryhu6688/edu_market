package service

import (
	"testing"
)

func TestClassifyIntent_Purchase_Keyword(t *testing.T) {
	tests := []string{"我想买", "我要买", "帮我下单", "想买", "买哪个"}
	for _, msg := range tests {
		if got := ClassifyIntent(msg); got != IntentPurchase {
			t.Errorf("ClassifyIntent(%q) = %s, want %s", msg, got, IntentPurchase)
		}
	}
}

func TestClassifyIntent_AfterSale_Keyword(t *testing.T) {
	tests := []string{"我要退款", "我的订单", "支付失败", "退货", "申请退款"}
	for _, msg := range tests {
		if got := ClassifyIntent(msg); got != IntentAfterSale {
			t.Errorf("ClassifyIntent(%q) = %s, want %s", msg, got, IntentAfterSale)
		}
	}
}

func TestClassifyIntent_Consult_Keyword(t *testing.T) {
	tests := []string{"有没有入门资料", "推荐一下", "学什么好", "想学", "哪个好"}
	for _, msg := range tests {
		if got := ClassifyIntent(msg); got != IntentConsult {
			t.Errorf("ClassifyIntent(%q) = %s, want %s", msg, got, IntentConsult)
		}
	}
}

func TestClassifyIntent_Chat_Keyword(t *testing.T) {
	// 这些是闲聊，不命中任何关键词 → 返回空，由 Agent 自己处理
	tests := []string{"你好", "在吗", "讲个笑话", "谢谢"}
	for _, msg := range tests {
		if got := ClassifyIntent(msg); got != "" {
			t.Errorf("ClassifyIntent(%q) = %s, want \"\" (let Agent handle)", msg, got)
		}
	}
}

func TestClassifyIntent_Unmatched(t *testing.T) {
	// 关键词不命中时返回空，由 Agent 自己处理
	tests := []string{"Python 和 Java 的区别", "周末去哪玩", "钱付了没到账"}
	for _, msg := range tests {
		got := ClassifyIntent(msg)
		if got != "" {
			t.Errorf("ClassifyIntent(%q) = %s, want \"\" (no keyword match, let Agent handle)", msg, got)
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
