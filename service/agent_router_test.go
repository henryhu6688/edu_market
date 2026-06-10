package service

import (
	"testing"

	"edu_market/model"
)

func TestRouteIntent_CustomerService(t *testing.T) {
	tests := []string{
		"我要退款",
		"我的订单在哪里",
		"支付失败了怎么办",
		"怎么买课程",
	}

	for _, msg := range tests {
		agentType, needLLM := RouteIntent(msg)
		if needLLM {
			t.Errorf("消息 %q 应该直接匹配，却需要 LLM", msg)
		}
		if agentType != model.AgentCustomerService {
			t.Errorf("消息 %q 应路由到客服，实际 %s", msg, agentType)
		}
	}
}

func TestRouteIntent_CourseRecommend(t *testing.T) {
	tests := []string{
		"有什么推荐的课程",
		"零基础学什么好",
		"想学编程有没有入门课程",
	}

	for _, msg := range tests {
		agentType, needLLM := RouteIntent(msg)
		if needLLM {
			t.Errorf("消息 %q 应该直接匹配，却需要 LLM", msg)
		}
		if agentType != model.AgentCourseRecommend {
			t.Errorf("消息 %q 应路由到推荐，实际 %s", msg, agentType)
		}
	}
}

func TestRouteIntent_QA(t *testing.T) {
	tests := []string{
		"第三章的公式怎么推导",
		"这个讲义里的定理怎么证明",
		"课件上这里为什么这样写",
	}

	for _, msg := range tests {
		agentType, needLLM := RouteIntent(msg)
		if needLLM {
			t.Errorf("消息 %q 应该直接匹配，却需要 LLM", msg)
		}
		if agentType != model.AgentQA {
			t.Errorf("消息 %q 应路由到答疑，实际 %s", msg, agentType)
		}
	}
}

func TestRouteIntent_Unclear(t *testing.T) {
	tests := []string{
		"你好",
		"在吗",
		"帮我看看",
	}

	for _, msg := range tests {
		_, needLLM := RouteIntent(msg)
		if !needLLM {
			t.Errorf("消息 %q 应该需要 LLM 判断", msg)
		}
	}
}

func TestDetectTransfer(t *testing.T) {
	tests := []struct {
		answer string
		should bool
		target string
	}{
		{"这是你的订单信息 [TRANSFER:qa]", true, model.AgentQA},
		{"推荐这门课程 [TRANSFER:course_recommend]", true, model.AgentCourseRecommend},
		{"帮你查一下 [TRANSFER:customer_service]", true, model.AgentCustomerService},
		{"这是正常的回答内容", false, ""},
	}

	for _, tc := range tests {
		should, target := DetectTransfer(tc.answer)
		if should != tc.should || target != tc.target {
			t.Errorf("DetectTransfer(%q) = (%v, %s), want (%v, %s)",
				tc.answer, should, target, tc.should, tc.target)
		}
	}
}

func TestCleanTransferMarkers(t *testing.T) {
	answer := "推荐这门课 [TRANSFER:qa] 详细内容如下"
	cleaned := CleanTransferMarkers(answer)
	if cleaned != "推荐这门课 详细内容如下" {
		t.Errorf("CleanTransferMarkers 结果: %q, want: %q", cleaned, "推荐这门课 详细内容如下")
	}
}

func TestGetAgentPrompt(t *testing.T) {
	tests := []struct {
		agentType string
		wantContain string
	}{
		{model.AgentCustomerService, "智能客服"},
		{model.AgentCourseRecommend, "学习顾问"},
		{model.AgentQA, "课程助教"},
		{"unknown", "智能客服"}, // default fallback
	}

	for _, tc := range tests {
		prompt := GetAgentPrompt(tc.agentType)
		if prompt == "" {
			t.Errorf("GetAgentPrompt(%q) 返回空", tc.agentType)
		}
	}
}
