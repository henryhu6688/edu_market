package service

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"edu_market/config"

	"github.com/nguyenthenguyen/docx"
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
	case ".docx":
		text, err = parseDocx(reader)
	case ".pdf":
		text, err = parsePDF(reader)
	case ".pptx":
		text, err = parsePPTX(reader)
	default:
		return "", fmt.Errorf("格式 %s 暂不支持（支持: .txt .md .docx .pdf .pptx）", ext)
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

// parsePDF 解析 PDF 文件（通过系统 pdftotext 工具提取文字）
func parsePDF(reader io.Reader) (string, error) {
	tmp, err := os.CreateTemp("", "upload-*.pdf")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, reader); err != nil {
		return "", fmt.Errorf("写入临时文件失败: %w", err)
	}
	tmp.Close()

	text, err := parsePDFWithPdfToText(tmp.Name())
	if err != nil {
		return "", fmt.Errorf("PDF 文字提取失败: %w", err)
	}
	if text == "" {
		return "", fmt.Errorf("PDF 文字提取为空，请确认文件是文字版 PDF（非扫描图片）")
	}
	return text, nil
}

// parsePDFWithPdfToText 调用系统 pdftotext 命令提取文本
func parsePDFWithPdfToText(path string) (string, error) {
	outFile := path + ".txt"
	defer os.Remove(outFile)

	cmd := exec.Command("pdftotext", "-layout", "-enc", "UTF-8", path, outFile)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdftotext 执行失败: %w", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// parsePPTX 解析 PPTX 文件（ZIP 内 slide XML 提取文本）
func parsePPTX(reader io.Reader) (string, error) {
	tmp, err := os.CreateTemp("", "upload-*.pptx")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, reader); err != nil {
		return "", fmt.Errorf("写入临时文件失败: %w", err)
	}
	tmp.Close()

	z, err := zip.OpenReader(tmp.Name())
	if err != nil {
		return "", fmt.Errorf("解析 PPTX 失败: %w", err)
	}
	defer z.Close()

	var slides []string
	for _, f := range z.File {
		// 只处理 ppt/slides/slide*.xml
		if !strings.HasPrefix(f.Name, "ppt/slides/slide") || !strings.HasSuffix(f.Name, ".xml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		slides = append(slides, string(data))
	}

	var result strings.Builder
	for i, slideXML := range slides {
		result.WriteString(extractTextFromPPTXXML(slideXML))
		if i < len(slides)-1 {
			result.WriteString("\n\n---\n\n") // 幻灯片分隔
		}
	}
	return strings.TrimSpace(result.String()), nil
}

// extractTextFromPPTXXML 从 PPTX slide XML 中提取 <a:t> 标签内的文字
func extractTextFromPPTXXML(xml string) string {
	var result strings.Builder
	var current strings.Builder
	inTag := false
	inAT := false

	for i := 0; i < len(xml); i++ {
		ch := xml[i]
		if ch == '<' {
			inTag = true
			current.Reset()
			continue
		}
		if ch == '>' {
			inTag = false
			tag := current.String()
			if strings.HasPrefix(tag, "a:t ") || tag == "a:t" {
				inAT = true
			} else if tag == "/a:t" {
				inAT = false
			} else if tag == "/a:p" {
				result.WriteString("\n")
			}
			current.Reset()
			continue
		}
		if !inTag && inAT {
			result.WriteByte(ch)
		} else if inTag {
			current.WriteByte(ch)
		}
	}
	return strings.TrimSpace(result.String())
}

// parseDocx 解析 DOCX 文件
func parseDocx(reader io.Reader) (string, error) {
	tmp, err := os.CreateTemp("", "upload-*.docx")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, reader); err != nil {
		return "", fmt.Errorf("写入临时文件失败: %w", err)
	}
	tmp.Close()

	doc, err := docx.ReadDocxFile(tmp.Name())
	if err != nil {
		return "", fmt.Errorf("解析 DOCX 失败: %w", err)
	}
	defer doc.Close()

	xmlContent := doc.Editable().GetContent()
	return extractTextFromDocxXML(xmlContent), nil
}

// extractTextFromDocxXML 从 DOCX 的 document.xml 中提取纯文本
func extractTextFromDocxXML(xml string) string {
	var result strings.Builder
	var current strings.Builder
	inTag := false
	inWT := false // 是否在 <w:t>...</w:t> 内

	for i := 0; i < len(xml); i++ {
		ch := xml[i]
		if ch == '<' {
			inTag = true
			current.Reset()
			continue
		}
		if ch == '>' {
			inTag = false
			tag := current.String()
			// 检查标签名
			if len(tag) >= 4 && tag[:4] == "w:t " || tag == "w:t" || (len(tag) > 4 && tag[:5] == "w:t ") {
				inWT = true
			} else if tag == "/w:t" {
				inWT = false
				result.WriteString(" ")
			} else if strings.HasPrefix(tag, "/w:p") || tag == "/w:p" {
				result.WriteString("\n")
			}
			current.Reset()
			continue
		}
		if !inTag && inWT {
			result.WriteByte(ch)
		} else if inTag {
			current.WriteByte(ch)
		}
	}
	return strings.TrimSpace(result.String())
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
