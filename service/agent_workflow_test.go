package service

import (
	"testing"
)

func TestClassifyIntent_Purchase(t *testing.T) {
	tests := []string{"我想买", "帮我下单", "多少钱", "怎么收费"}
	for _, msg := range tests {
		if got := ClassifyIntent(msg); got != IntentPurchase {
			t.Errorf("ClassifyIntent(%q) = %s, want %s", msg, got, IntentPurchase)
		}
	}
}

func TestClassifyIntent_AfterSale(t *testing.T) {
	tests := []string{"我要退款", "我的订单呢", "支付失败了", "怎么退货"}
	for _, msg := range tests {
		if got := ClassifyIntent(msg); got != IntentAfterSale {
			t.Errorf("ClassifyIntent(%q) = %s, want %s", msg, got, IntentAfterSale)
		}
	}
}

func TestClassifyIntent_Consult(t *testing.T) {
	tests := []string{"有没有入门资料", "推荐一下", "讲什么内容", "适合我吗", "学什么好"}
	for _, msg := range tests {
		if got := ClassifyIntent(msg); got != IntentConsult {
			t.Errorf("ClassifyIntent(%q) = %s, want %s", msg, got, IntentConsult)
		}
	}
}

func TestClassifyIntent_Chat(t *testing.T) {
	tests := []string{"你好", "在吗", "讲个笑话", "谢谢"}
	for _, msg := range tests {
		if got := ClassifyIntent(msg); got != IntentChat {
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
