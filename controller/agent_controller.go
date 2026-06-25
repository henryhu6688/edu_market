package controller

import (
	"encoding/json"
	"fmt"

	"edu_market/dto/request"
	"edu_market/service/agent"
	"edu_market/utils"
	"log/slog"

	"github.com/gin-gonic/gin"
)

// AgentController Agent 控制器
type AgentController struct {
	svc *agent.AgentService
}

// NewAgentController 创建 AgentController
func NewAgentController(svc *agent.AgentService) *AgentController {
	return &AgentController{svc: svc}
}

// Chat 发起/继续 Agent 对话（SSE 流式响应）
func (ctr *AgentController) Chat(c *gin.Context) {
	var req request.AgentChatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	userID := c.GetUint("user_id")
	requestID, _ := c.Get("request_id")
	rid := fmt.Sprint(requestID)
	slog.Info("Agent 对话请求", "request_id", rid, "user_id", userID, "question", req.Question[:min(len(req.Question), 50)])

	// 设置 SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")

	// 创建 SSE 回调（写入 gin.Context 的 ResponseWriter）
	sseHandler := func(event string, data string) error {
		_, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, data)
		if err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	}

	// 获取 RAG 检索函数
	var searchFunc agent.SearchFunc
	rag := agent.GetRAG()
	if rag != nil {
		searchFunc = func(courseID uint, query string, topK int) (string, error) {
			results, err := rag.Search(courseID, query, topK)
			if err != nil {
				return "", err
			}
			if len(results) == 0 {
				return "", nil
			}
			// 拼接结果
			var parts []string
			for _, r := range results {
				parts = append(parts, r.Content)
			}
			bytes, _ := json.Marshal(parts)
			return string(bytes), nil
		}
	}

	session, err := ctr.svc.Chat(userID, req.SessionID, req.Question, searchFunc, sseHandler, rid)
	if err != nil {
		// 引擎内部已尝试发 error 事件，这里补一个兜底
		fmt.Fprintf(c.Writer, "event: error\ndata: {\"message\":\"%s\"}\n\n", err.Error())
		c.Writer.Flush()
		return
	}

	// 如果前端没传 session_id，通过 done 事件告知
	if req.SessionID == nil && session != nil {
		doneData, _ := json.Marshal(map[string]interface{}{
			"session_id": session.ID,
			"agent_type": session.AgentType,
		})
		fmt.Fprintf(c.Writer, "event: done\ndata: %s\n\n", string(doneData))
		c.Writer.Flush()
	}
}

// GetSessions 获取会话列表
func (ctr *AgentController) GetSessions(c *gin.Context) {
	var req request.AgentSessionsReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	userID := c.GetUint("user_id")
	sessions, total, err := ctr.svc.GetSessions(userID, req.Page, req.PageSize)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	req.Page, req.PageSize = agent.GetPagination(req.Page, req.PageSize)
	utils.PageSuccess(c, sessions, total, req.Page, req.PageSize)
}

// DeleteSession 删除/关闭会话
func (ctr *AgentController) DeleteSession(c *gin.Context) {
	sessionID, err := parseUintParam(c, "id")
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	userID := c.GetUint("user_id")
	if err := ctr.svc.DeleteSession(userID, sessionID); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, nil)
}

// GetMessages 获取会话消息历史
func (ctr *AgentController) GetMessages(c *gin.Context) {
	var req request.AgentMessagesReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	sessionID, err := parseUintParam(c, "id")
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	userID := c.GetUint("user_id")
	messages, total, err := ctr.svc.GetMessages(sessionID, userID, req.Page, req.PageSize)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	req.Page, req.PageSize = agent.GetPagination(req.Page, req.PageSize)
	utils.PageSuccess(c, messages, total, req.Page, req.PageSize)
}

// parseUintParam 解析路由中的 uint 参数
func parseUintParam(c *gin.Context, name string) (uint, error) {
	id := c.Param(name)
	if id == "" {
		return 0, fmt.Errorf("缺少参数 %s", name)
	}
	var result uint
	if _, err := fmt.Sscanf(id, "%d", &result); err != nil {
		return 0, fmt.Errorf("参数 %s 格式错误", name)
	}
	return result, nil
}
