package utils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"edu_market/dto/response"

	"github.com/gin-gonic/gin"
)

// setupGinContext 创建测试用的 Gin Context
func setupGinContext(w *httptest.ResponseRecorder) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	return c
}

// TestSuccess 测试成功响应
func TestSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	c := setupGinContext(w)

	Success(c, gin.H{"name": "test"})

	if w.Code != http.StatusOK {
		t.Errorf("状态码应为 200，实际: %d", w.Code)
	}

	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 200 {
		t.Errorf("code 应为 200，实际: %d", resp.Code)
	}
	if resp.Message != "success" {
		t.Errorf("message 应为 success，实际: %s", resp.Message)
	}
}

// TestCreated 测试创建成功响应
func TestCreated(t *testing.T) {
	w := httptest.NewRecorder()
	c := setupGinContext(w)

	Created(c, gin.H{"id": 1})

	if w.Code != http.StatusCreated {
		t.Errorf("状态码应为 201，实际: %d", w.Code)
	}

	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 201 {
		t.Errorf("code 应为 201，实际: %d", resp.Code)
	}
	if resp.Message != "created" {
		t.Errorf("message 应为 created，实际: %s", resp.Message)
	}
}

// TestError 测试错误响应
func TestError(t *testing.T) {
	w := httptest.NewRecorder()
	c := setupGinContext(w)

	Error(c, http.StatusBadGateway, "网关错误")

	if w.Code != http.StatusBadGateway {
		t.Errorf("状态码应为 502，实际: %d", w.Code)
	}

	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != http.StatusBadGateway {
		t.Errorf("code 应为 502，实际: %d", resp.Code)
	}
	if resp.Message != "网关错误" {
		t.Errorf("message 应为 网关错误，实际: %s", resp.Message)
	}
	if resp.Data != nil {
		t.Error("错误响应 data 应为 nil")
	}
}

// TestBadRequest 测试400响应
func TestBadRequest(t *testing.T) {
	w := httptest.NewRecorder()
	c := setupGinContext(w)

	BadRequest(c, "参数错误")

	if w.Code != http.StatusBadRequest {
		t.Errorf("状态码应为 400，实际: %d", w.Code)
	}
}

// TestUnauthorized 测试401响应
func TestUnauthorized(t *testing.T) {
	w := httptest.NewRecorder()
	c := setupGinContext(w)

	Unauthorized(c, "请登录")

	if w.Code != http.StatusUnauthorized {
		t.Errorf("状态码应为 401，实际: %d", w.Code)
	}
}

// TestForbidden 测试403响应
func TestForbidden(t *testing.T) {
	w := httptest.NewRecorder()
	c := setupGinContext(w)

	Forbidden(c, "无权限")

	if w.Code != http.StatusForbidden {
		t.Errorf("状态码应为 403，实际: %d", w.Code)
	}
}

// TestNotFound 测试404响应
func TestNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	c := setupGinContext(w)

	NotFound(c, "资源不存在")

	if w.Code != http.StatusNotFound {
		t.Errorf("状态码应为 404，实际: %d", w.Code)
	}
}

// TestInternalError 测试500响应
func TestInternalError(t *testing.T) {
	w := httptest.NewRecorder()
	c := setupGinContext(w)

	InternalError(c, "服务器错误")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("状态码应为 500，实际: %d", w.Code)
	}
}

// TestPageSuccess 测试分页成功响应
func TestPageSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	c := setupGinContext(w)

	list := []string{"a", "b", "c"}
	PageSuccess(c, list, 100, 1, 10)

	if w.Code != http.StatusOK {
		t.Errorf("状态码应为 200，实际: %d", w.Code)
	}

	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 200 {
		t.Errorf("code 应为 200，实际: %d", resp.Code)
	}

	// 验证分页数据
	dataJSON, _ := json.Marshal(resp.Data)
	var pageData response.PageData
	json.Unmarshal(dataJSON, &pageData)
	if pageData.Total != 100 {
		t.Errorf("total 应为 100，实际: %d", pageData.Total)
	}
	if pageData.Page != 1 {
		t.Errorf("page 应为 1，实际: %d", pageData.Page)
	}
	if pageData.PageSize != 10 {
		t.Errorf("pageSize 应为 10，实际: %d", pageData.PageSize)
	}
}

// TestSuccessNilData 测试成功响应 data 为 nil
func TestSuccessNilData(t *testing.T) {
	w := httptest.NewRecorder()
	c := setupGinContext(w)

	Success(c, nil)

	if w.Code != http.StatusOK {
		t.Errorf("状态码应为 200，实际: %d", w.Code)
	}
}
