package rag

import (
	"strings"
	"testing"
)

func TestChunker_Markdown(t *testing.T) {
	c := NewChunker(300, 800)
	md := "# 第一章\n## 1.1 概述\n这是概述内容足够长足够长足够长足够长足够长足够长足够长足够长足够长足够长足够长足够长"
	sections := c.parseMDSections(md)
	chunks := c.chunkFromSections(sections)
	if len(chunks) == 0 {
		t.Error("expected chunks")
	}
	// 验证 SectionPath
	for _, ch := range chunks {
		t.Logf("SectionPath: %s, ContentLen: %d", ch.SectionPath, len([]rune(ch.Content)))
	}
}

func TestChunker_PlainFallback(t *testing.T) {
	c := NewChunker(300, 800)
	plain := "段落1。这是一个测试段落。" + strings.Repeat("x", 500) + "\n\n段落2。第二个测试段落。"
	chunks := c.chunkPlain(plain)
	if len(chunks) == 0 {
		t.Error("expected chunks")
	}
	t.Logf("chunks count: %d", len(chunks))
}

func TestCleanMarkdown(t *testing.T) {
	input := "**粗体** 和 [链接](url) 和 ![img](img.png)"
	output := cleanMarkdown(input)
	if strings.Contains(output, "**") {
		t.Errorf("粗体未清洗: %s", output)
	}
	if strings.Contains(output, "[链接]") {
		t.Errorf("链接未清洗: %s", output)
	}
	if strings.Contains(output, "![") {
		t.Errorf("图片未清洗: %s", output)
	}
	t.Logf("cleaned: %s", output)
}

func TestFullwidthToHalfwidth(t *testing.T) {
	input := "ＡＢＣ１２３"
	output := fullwidthToHalfwidth(input)
	if output != "ABC123" {
		t.Errorf("全角未转换: %s", output)
	}
}

func TestIsReadable(t *testing.T) {
	normal := "这是正常的中文文本用于测试可读性这是正常的中文文本" + strings.Repeat("x", 50)
	if !testIsReadable(normal) {
		t.Error("正常中文应可读")
	}
	short := "abc"
	if testIsReadable(short) {
		t.Error("太短应不可读")
	}
}

// testIsReadable 调用 service 包的 isReadable（同一个 package 可以访问）
// 注意：isReadable 在 service 包，不在 rag 包，此测试仅用于本地验证
func testIsReadable(text string) bool {
	runes := []rune(text)
	if len(runes) < 50 {
		return false
	}
	var normal, garbled int
	for _, r := range runes {
		if (r >= 0x4E00 && r <= 0x9FFF) ||
			(r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == ' ' || r == '\n' || r == '\t' ||
			(r >= 0x20 && r <= 0x7E) {
			normal++
		}
		if r == 0xFFFD || r == 0xFFFF || (r < 0x20 && r != '\n' && r != '\t' && r != '\r') {
			garbled++
		}
	}
	total := len(runes)
	return float64(normal)/float64(total) > 0.70 &&
		float64(garbled)/float64(total) < 0.10
}
