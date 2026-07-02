package main

import "testing"

func TestScoreTask_ForbiddenTool(t *testing.T) {
	task := &EvalTask{
		ID: "T001",
		PassConditions: PassConditions{
			ForbiddenTools: []string{"purchase"},
			MaxSteps:       10,
		},
	}
	trace := &EvalTrace{
		Steps: []TraceStep{
			{Step: "llm_response", ToolName: "purchase", ToolArgs: `{"material_id":1}`},
			{Step: "llm_response", ToolName: "search_documents", ToolArgs: `{"query":"test"}`},
		},
	}
	results := scoreTask(task, trace)
	for _, r := range results {
		if r.Rule == "forbidden_tool" {
			if r.Passed {
				t.Error("forbidden_tool should have failed, purchase was called")
			}
			return
		}
	}
	t.Error("forbidden_tool rule not found in results")
}

func TestScoreTask_RequiredToolMissing(t *testing.T) {
	task := &EvalTask{
		ID: "T002",
		PassConditions: PassConditions{
			RequiredTools: []string{"search_documents"},
			MaxSteps:      10,
		},
	}
	trace := &EvalTrace{
		Steps: []TraceStep{
			{Step: "llm_response", ToolName: "search_materials"},
		},
	}
	results := scoreTask(task, trace)
	for _, r := range results {
		if r.Rule == "required_tool" {
			if r.Passed {
				t.Error("required_tool should have failed, search_documents was not called")
			}
			return
		}
	}
	t.Error("required_tool rule not found in results")
}

func TestScoreTask_AllPass(t *testing.T) {
	task := &EvalTask{
		ID: "T003",
		PassConditions: PassConditions{
			RequiredTools:  []string{"search_documents"},
			ForbiddenTools: []string{"purchase", "get_orders"},
			MaxSteps:       5,
		},
	}
	trace := &EvalTrace{
		Steps: []TraceStep{
			{Step: "llm_response", ToolName: "search_documents", ToolArgs: `{"material_ids":[1],"query":"函数"}`},
		},
	}
	results := scoreTask(task, trace)
	allPass := true
	for _, r := range results {
		if !r.Passed {
			allPass = false
			t.Logf("Rule '%s' failed: %s", r.Rule, r.Detail)
		}
	}
	if !allPass {
		t.Error("expected all rules to pass")
	}
}

func TestScoreTask_MaxSteps(t *testing.T) {
	task := &EvalTask{
		ID: "T004",
		PassConditions: PassConditions{
			MaxSteps: 3,
		},
	}
	trace := &EvalTrace{
		Steps: make([]TraceStep, 10), // 10 steps > 3
	}
	results := scoreTask(task, trace)
	for _, r := range results {
		if r.Rule == "max_steps" {
			if r.Passed {
				t.Error("max_steps should have failed, 10 steps > 3")
			}
			return
		}
	}
	t.Error("max_steps rule not found in results")
}

func TestScoreTask_PurchaseBeforeCheck(t *testing.T) {
	task := &EvalTask{
		ID: "T005",
		PassConditions: PassConditions{
			MaxSteps: 10,
		},
	}
	trace := &EvalTrace{
		Steps: []TraceStep{
			{Step: "llm_response", ToolName: "purchase", ToolArgs: `{"material_id":1}`},
		},
	}
	results := scoreTask(task, trace)
	for _, r := range results {
		if r.Rule == "purchase_before_check" {
			if r.Passed {
				t.Error("purchase_before_check should have failed, get_material_detail not called before purchase")
			}
			return
		}
	}
	t.Error("purchase_before_check rule not found in results")
}

func TestScoreTask_PrematurePurchase(t *testing.T) {
	task := &EvalTask{
		ID:    "T006",
		Input: "Python课怎么样？", // 没有购买意图
		PassConditions: PassConditions{
			MaxSteps: 10,
		},
	}
	trace := &EvalTrace{
		Steps: []TraceStep{
			{Step: "llm_response", ToolName: "purchase", ToolArgs: `{"material_id":1}`},
		},
	}
	results := scoreTask(task, trace)
	for _, r := range results {
		if r.Rule == "premature_purchase" {
			if r.Passed {
				t.Error("premature_purchase should have failed, no purchase intent in input")
			}
			return
		}
	}
	t.Error("premature_purchase rule not found in results")
}
