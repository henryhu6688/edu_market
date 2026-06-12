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

func TestClassifyIntent_Purchase_LLM(t *testing.T) {
	tests := []string{"帮我下单买这个", "我要这个资料买了", "现在下单", "我想买这个"}
	for _, msg := range tests {
		got := ClassifyIntent(msg)
		if got != IntentPurchase {
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

func TestClassifyIntent_AfterSale_LLM(t *testing.T) {
	tests := []string{"我怎么退掉这个", "订单怎么查", "钱付了没到账"}
	for _, msg := range tests {
		got := ClassifyIntent(msg)
		if got != IntentAfterSale {
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

func TestClassifyIntent_Consult_LLM(t *testing.T) {
	// 这些走 LLM 快判
	tests := []string{"Python 和 Java 有什么区别", "数据分析适合我吗", "有没有深度学习相关的"}
	for _, msg := range tests {
		got := ClassifyIntent(msg)
		if got != IntentConsult {
			t.Errorf("ClassifyIntent(%q) = %s, want %s", msg, got, IntentConsult)
		}
	}
}

func TestClassifyIntent_Chat_Keyword(t *testing.T) {
	tests := []string{"你好", "在吗", "讲个笑话", "谢谢"}
	for _, msg := range tests {
		if got := ClassifyIntent(msg); got != IntentChat {
			t.Errorf("ClassifyIntent(%q) = %s, want %s", msg, got, IntentChat)
		}
	}
}

func TestClassifyIntent_Chat_LLM(t *testing.T) {
	// LLM 应该能识别这些是闲聊
	tests := []string{"周末去哪玩", "你叫什么名字", "天气不错"}
	for _, msg := range tests {
		got := ClassifyIntent(msg)
		if got != IntentChat {
			t.Errorf("ClassifyIntent(%q) = %s, want %s", msg, got, IntentChat)
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
