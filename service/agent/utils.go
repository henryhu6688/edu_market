package agent

import (
	"math"
	"strings"
)

// CleanTransferMarkers 清除回答中的切换标记，返回干净的文本
func CleanTransferMarkers(answer string) string {
	answer = strings.ReplaceAll(answer, "[TRANSFER:qa]", "")
	answer = strings.ReplaceAll(answer, "[TRANSFER:course_recommend]", "")
	answer = strings.ReplaceAll(answer, "[TRANSFER:customer_service]", "")
	// 清理多余空格
	answer = strings.Join(strings.Fields(answer), " ")
	return strings.TrimSpace(answer)
}

// TruncateRunes 截取字符串前 n 个字符（Unicode 安全）
func TruncateRunes(s string, maxLen int) string {
	// 去掉切换标记再截取
	s = CleanTransferMarkers(s)
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// GetPagination 分页参数处理
func GetPagination(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	pageSize = int(math.Min(float64(pageSize), 100))
	return page, pageSize
}
