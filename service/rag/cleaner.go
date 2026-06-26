package rag

import (
	"regexp"
	"strings"
)

// cleanMarkdown Markdown 层通用清洗。保留 # 标题标记给切片器。
func cleanMarkdown(text string) string {
	// 1. 控制字符
	for _, c := range []rune{0x00, 0xFFFD, 0x0C} {
		text = strings.ReplaceAll(text, string(c), "")
	}

	// 2. 图片占位符 ![...](...) → 删除
	text = regexp.MustCompile(`!\[.*?\]\(.*?\)`).ReplaceAllString(text, "")

	// 3. 链接 [text](url) → 保留 text
	text = regexp.MustCompile(`\[(.+?)\]\(.+?\)`).ReplaceAllString(text, "$1")

	// 4. 加粗 **text** → 保留 text
	text = regexp.MustCompile(`\*\*(.+?)\*\*`).ReplaceAllString(text, "$1")

	// 5. 删除线 ~~text~~ → 保留 text
	text = regexp.MustCompile(`~~(.+?)~~`).ReplaceAllString(text, "$1")

	// 6. 空行合并
	text = regexp.MustCompile(`\n{3,}`).ReplaceAllString(text, "\n\n")

	// 7. 全角英文/数字 → 半角
	text = fullwidthToHalfwidth(text)

	return strings.TrimSpace(text)
}

func fullwidthToHalfwidth(s string) string {
	var buf strings.Builder
	for _, r := range s {
		switch {
		case r >= 'Ａ' && r <= 'Ｚ':
			buf.WriteRune(r - 'Ａ' + 'A')
		case r >= 'ａ' && r <= 'ｚ':
			buf.WriteRune(r - 'ａ' + 'a')
		case r >= '０' && r <= '９':
			buf.WriteRune(r - '０' + '0')
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
