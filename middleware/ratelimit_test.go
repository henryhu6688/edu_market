package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"edu_market/database"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestRateLimit_PassWhenRedisDown(t *testing.T) {
	r := gin.New()
	r.Use(RateLimit())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRateLimit_BlockExceeded(t *testing.T) {
	if database.RDB == nil {
		t.Skip("Redis not available")
	}

	r := gin.New()
	r.Use(RateLimit())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	for i := 0; i < 31; i++ {
		req, _ := http.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if i >= 30 && w.Code != 429 {
			t.Errorf("request 31 expected 429, got %d", w.Code)
		}
	}
}
