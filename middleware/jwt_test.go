package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"edu_market/config"
	"edu_market/dto/response"
	"edu_market/utils"

	"github.com/gin-gonic/gin"
)

// setupMiddlewareTest 初始化中间件测试环境
func setupMiddlewareTest() {
	gin.SetMode(gin.TestMode)
	if config.App == nil {
		config.App = &config.Config{}
	}
	config.App.JWT.Secret = "test-secret-key"
	config.App.JWT.AccessTTLMin = 30
}

// createTestRouter 创建带中间件的测试路由
func createTestRouter(middlewares ...gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	for _, m := range middlewares {
		r.Use(m)
	}
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	return r
}

// TestJWTAuthNoHeader 测试无 Authorization 头
func TestJWTAuthNoHeader(t *testing.T) {
	setupMiddlewareTest()
	r := createTestRouter(JWTAuth())

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("无Authorization头应返回401，实际: %d", w.Code)
	}

	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Message != "请先登录" {
		t.Errorf("错误信息应为'请先登录'，实际: %s", resp.Message)
	}
}

// TestJWTAuthBearerFormat 测试非 Bearer 格式 Token
func TestJWTAuthBearerFormat(t *testing.T) {
	setupMiddlewareTest()
	r := createTestRouter(JWTAuth())

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic abc123")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("非Bearer格式应返回401，实际: %d", w.Code)
	}

	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Message != "Token格式错误" {
		t.Errorf("错误信息应为'Token格式错误'，实际: %s", resp.Message)
	}
}

// TestJWTAuthInvalidToken 测试无效 Token
func TestJWTAuthInvalidToken(t *testing.T) {
	setupMiddlewareTest()
	r := createTestRouter(JWTAuth())

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("无效Token应返回401，实际: %d", w.Code)
	}
}

// TestJWTAuthValidToken 测试有效 Token
func TestJWTAuthValidToken(t *testing.T) {
	setupMiddlewareTest()
	r := createTestRouter(JWTAuth())

	token, err := utils.GenerateAccessToken(1, "testuser", "student")
	if err != nil {
		t.Fatalf("生成Token失败: %v", err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("有效Token应返回200，实际: %d", w.Code)
	}
}

// TestJWTAuthContextInjection 测试 Token 解析后上下文注入
func TestJWTAuthContextInjection(t *testing.T) {
	setupMiddlewareTest()
	r := gin.New()
	r.Use(JWTAuth())
	r.GET("/whoami", func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		username, _ := c.Get("username")
		role, _ := c.Get("role")
		c.JSON(http.StatusOK, gin.H{
			"user_id":  userID,
			"username": username,
			"role":     role,
		})
	})

	token, _ := utils.GenerateAccessToken(99, "admin_user", "admin")

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("请求失败: %d", w.Code)
	}

	var body map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["user_id"] != float64(99) {
		t.Errorf("user_id 应为 99，实际: %v", body["user_id"])
	}
	if body["username"] != "admin_user" {
		t.Errorf("username 应为 admin_user，实际: %v", body["username"])
	}
	if body["role"] != "admin" {
		t.Errorf("role 应为 admin，实际: %v", body["role"])
	}
}

// TestAdminOnlyAllowAdmin 测试管理员可通过
func TestAdminOnlyAllowAdmin(t *testing.T) {
	setupMiddlewareTest()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("role", "admin")
		c.Next()
	})
	r.Use(AdminOnly())
	r.GET("/admin/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/admin/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("admin角色应返回200，实际: %d", w.Code)
	}
}

// TestAdminOnlyRejectStudent 测试非管理员被拒
func TestAdminOnlyRejectStudent(t *testing.T) {
	setupMiddlewareTest()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("role", "student")
		c.Next()
	})
	r.Use(AdminOnly())
	r.GET("/admin/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/admin/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("student角色应返回403，实际: %d", w.Code)
	}

	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Message != "需要管理员权限" {
		t.Errorf("错误信息应为'需要管理员权限'，实际: %s", resp.Message)
	}
}

// TestAdminOnlyNoRole 测试无 role 上下文
func TestAdminOnlyNoRole(t *testing.T) {
	setupMiddlewareTest()
	r := createTestRouter(AdminOnly())

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("无role应返回403，实际: %d", w.Code)
	}
}
