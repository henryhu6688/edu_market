package service

import (
	"archive/zip"
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

// Parse 解析上传文件，返回 Markdown 格式内容
func (p *DocumentParser) Parse(filename string, reader io.Reader) (string, error) {
	ext := strings.ToLower(filename)
	if dot := strings.LastIndex(ext, "."); dot >= 0 {
		ext = ext[dot:]
	}
	if !p.isAllowed(ext) {
		return "", fmt.Errorf("不支持: %s（支持 .txt .md .docx .pdf .pptx）", ext)
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
		return "", fmt.Errorf("暂不支持: %s", ext)
	}
	if err != nil {
		return "", err
	}
	if text == "" {
		return "", fmt.Errorf("文件内容为空")
	}
	return textToMarkdown(text), nil
}

func (p *DocumentParser) isAllowed(ext string) bool {
	for _, f := range p.formats {
		if f == ext {
			return true
		}
	}
	return false
}

// textToMarkdown 纯文本转 Markdown
func textToMarkdown(text string) string {
	paragraphs := strings.Split(strings.TrimSpace(text), "\n\n")
	var result strings.Builder
	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		lines := strings.Split(para, "\n")
		for i, line := range lines {
			result.WriteString(line)
			if i < len(lines)-1 {
				result.WriteString("  \n")
			}
		}
		result.WriteString("\n\n")
	}
	return strings.TrimSpace(result.String())
}

// parseDOCX 解析 DOCX 文件
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

	return strings.TrimSpace(doc.Editable().GetContent()), nil
}

// parsePDF 解析 PDF 文件
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
		return "", fmt.Errorf("PDF 提取失败: %w", err)
	}
	if text == "" {
		return "", fmt.Errorf("PDF 无文字（可能是扫描图片）")
	}
	return text, nil
}

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

// parsePPTX 解析 PPTX 文件
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
			result.WriteString("\n\n---\n\n")
		}
	}
	return strings.TrimSpace(result.String()), nil
}

// extractTextFromDocxXML 从 DOCX XML 提取 <w:t> 标签文字
func extractTextFromDocxXML(xml string) string {
	var result strings.Builder
	var current strings.Builder
	inTag := false
	inWT := false

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
			if strings.HasPrefix(tag, "w:t ") || tag == "w:t" {
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

// extractTextFromPPTXXML 从 PPTX slide XML 提取 <a:t> 标签文字
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

// extractTextFromMarkdown 从 Markdown 提取纯文本（用于 RAG）
func extractTextFromMarkdown(md string) string {
	text := md
	// 去图片 ![alt](url)
	for {
		start := strings.Index(text, "![")
		if start < 0 {
			break
		}
		end := strings.Index(text[start:], "](")
		if end < 0 {
			break
		}
		close := strings.Index(text[start+end+2:], ")")
		if close < 0 {
			break
		}
		text = text[:start] + text[start+end+2+close+1:]
	}
	// 去链接 [text](url) 保留 text
	for {
		start := strings.Index(text, "[")
		if start < 0 {
			break
		}
		end := strings.Index(text[start:], "](")
		if end < 0 {
			break
		}
		close := strings.Index(text[start+end+2:], ")")
		if close < 0 {
			break
		}
		linkText := text[start+1 : start+end]
		text = text[:start] + linkText + text[start+end+2+close+1:]
	}
	// 去格式标记
	for _, ch := range []string{"**", "__", "~~", "`", "#", "*", ">"} {
		text = strings.ReplaceAll(text, ch, "")
	}
	return strings.TrimSpace(text)
}
