package service

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"edu_market/config"
)

// DocumentParser 文件解析器
type DocumentParser struct {
	formats []string
}

// NewDocumentParser 创建解析器
func NewDocumentParser() *DocumentParser {
	return &DocumentParser{formats: config.App.Document.AllowedFormats}
}

// Parse 解析上传文件，返回 Tiptap JSON
func (p *DocumentParser) Parse(filename string, reader io.Reader) (string, error) {
	ext := strings.ToLower(filename)
	if dot := strings.LastIndex(ext, "."); dot >= 0 {
		ext = ext[dot:]
	}
	if !p.isAllowed(ext) {
		return "", fmt.Errorf("不支持的文件格式: %s", ext)
	}

	var text string
	var err error
	switch ext {
	case ".txt", ".md":
		bytes, e := io.ReadAll(reader)
		err = e
		text = string(bytes)
	default:
		return "", fmt.Errorf("格式 %s 的解析待实现（需引入第三方库）", ext)
	}
	if err != nil {
		return "", err
	}
	if text == "" {
		return "", fmt.Errorf("文件内容为空")
	}
	return textToTiptapJSON(text), nil
}

func (p *DocumentParser) isAllowed(ext string) bool {
	for _, f := range p.formats {
		if f == ext {
			return true
		}
	}
	return false
}

// textToTiptapJSON 纯文本转 Tiptap JSON（段落 = 双换行分隔）
func textToTiptapJSON(text string) string {
	paragraphs := strings.Split(strings.TrimSpace(text), "\n\n")
	var content []map[string]interface{}
	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		lines := strings.Split(para, "\n")
		var textNodes []map[string]interface{}
		for i, line := range lines {
			if line == "" {
				continue
			}
			textNodes = append(textNodes, map[string]interface{}{
				"type": "text", "text": line,
			})
			if i < len(lines)-1 {
				textNodes = append(textNodes, map[string]interface{}{
					"type": "hardBreak",
				})
			}
		}
		content = append(content, map[string]interface{}{
			"type":    "paragraph",
			"content": textNodes,
		})
	}
	doc := map[string]interface{}{"type": "doc", "content": content}
	b, _ := json.Marshal(doc)
	return string(b)
}

// extractTextFromTiptapJSON 从 Tiptap JSON 提取纯文本（用于 RAG）
func extractTextFromTiptapJSON(jsonStr string) string {
	var doc struct {
		Content []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &doc); err != nil {
		return jsonStr // fallback: 原样返回
	}
	var parts []string
	for _, node := range doc.Content {
		for _, child := range node.Content {
			if child.Text != "" {
				parts = append(parts, child.Text)
			}
		}
	}
	return strings.Join(parts, "\n")
}
