package service

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
		return "", fmt.Errorf("不支持: %s（支持 .txt .md .docx .pdf）", ext)
	}

	var text string
	var err error
	switch ext {
	case ".txt", ".md":
		bytes, e := io.ReadAll(reader)
		err = e
		text = string(bytes)
	case ".docx":
		text, err = parseDocxReader(reader)
		if err == nil {
			text = cleanDOCX(text)
		}
	case ".pdf":
		text, err = parsePDFReader(reader)
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

// parseDocxReader 解析 DOCX 文件
func parseDocxReader(reader io.Reader) (string, error) {
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

// parsePDFReader 解析 PDF 文件（含 OCR 降级）
func parsePDFReader(reader io.Reader) (string, error) {
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
		return "", err
	}

	// 可读性检查
	if isReadable(text) {
		return cleanPDF(text), nil
	}

	// OCR 降级
	ocrText, err := tesseractOCR(tmp.Name())
	if err != nil {
		return "", fmt.Errorf("PDF OCR 失败: %w", err)
	}
	if !isReadable(ocrText) {
		return "", fmt.Errorf("PDF 质量过低，OCR 后仍无法识别，建议上传文字版 PDF")
	}
	return cleanPDF(ocrText), nil
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

// ============ OCR ============

// tesseractOCR 对 PDF 逐页做 OCR 识别。
func tesseractOCR(filePath string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "ocr_*")
	if err != nil {
		return "", fmt.Errorf("创建 OCR 临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command("pdftoppm", "-png", "-r", "300", filePath,
		filepath.Join(tmpDir, "page"))
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdftoppm 失败: %w", err)
	}

	files, _ := filepath.Glob(filepath.Join(tmpDir, "page-*.png"))
	var result strings.Builder
	for _, f := range files {
		cmd := exec.Command("tesseract", f, "stdout", "-l", "chi_sim+eng")
		out, _ := cmd.Output()
		result.Write(out)
		result.WriteString("\n")
	}
	return result.String(), nil
}

// ============ 可读性检查 ============

// isReadable 检查文本是否可读（非扫描件/乱码）。
// 正常字符率 > 70% 且乱码率 < 10% 视为可读。
func isReadable(text string) bool {
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

// ============ PDF 清洗 ============

// cleanPDF PDF 格式专用清洗。
func cleanPDF(text string) string {
	text = removeRepeatingLines(text) // 页眉页脚 + 水印
	text = removePageNumbers(text)    // 页码
	text = mergeHardLineBreaks(text)  // 硬换行合并
	text = removeTOClines(text)       // 目录残留
	return text
}

// ============ DOCX 清洗 ============

// cleanDOCX DOCX 格式专用清洗。
func cleanDOCX(text string) string {
	re := regexp.MustCompile(`[│├─┼┤┬┴└┘┌┐╭╮╰╯]+`)
	text = re.ReplaceAllString(text, "")
	text = removeRepeatingLines(text)
	text = removePageNumbers(text)
	return text
}

// ============ 共用辅助函数 ============

// removeRepeatingLines 删除出现 ≥3 次的同文行（页眉页脚/水印）。
func removeRepeatingLines(text string) string {
	lines := strings.Split(text, "\n")
	count := make(map[string]int)
	for _, l := range lines {
		count[strings.TrimSpace(l)]++
	}
	var cleaned []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if len([]rune(t)) < 50 && count[t] >= 3 {
			continue
		}
		// 水印：同一短文本（<20字）出现 ≥5 次
		if len([]rune(t)) < 20 && count[t] >= 5 {
			continue
		}
		cleaned = append(cleaned, l)
	}
	return strings.Join(cleaned, "\n")
}

// removePageNumbers 删除纯数字短行（页码）。
func removePageNumbers(text string) string {
	lines := strings.Split(text, "\n")
	re := regexp.MustCompile(`^[\s-]*\d{1,5}[\s-]*$`)
	var cleaned []string
	for _, l := range lines {
		if !re.MatchString(strings.TrimSpace(l)) {
			cleaned = append(cleaned, l)
		}
	}
	return strings.Join(cleaned, "\n")
}

// mergeHardLineBreaks 合并 PDF 宽度截断造成的硬换行。
func mergeHardLineBreaks(text string) string {
	lines := strings.Split(text, "\n")
	var merged []string
	for i := 0; i < len(lines); i++ {
		if i+1 < len(lines) && isMidSentenceBreak(lines[i], lines[i+1]) {
			if len(merged) > 0 {
				merged[len(merged)-1] += lines[i+1]
			}
			i++
		} else {
			merged = append(merged, lines[i])
		}
	}
	return strings.Join(merged, "\n")
}

func isMidSentenceBreak(cur, next string) bool {
	if len(cur) == 0 || len(next) == 0 {
		return false
	}
	last := []rune(cur)[len([]rune(cur))-1]
	// 中文非句号结尾 或 小写字母结尾 → 句子中间被截断
	return (last >= 0x4E00 && last <= 0x9FFF && last != '。' && last != '；' && last != '！') ||
		(last >= 'a' && last <= 'z')
}

// removeTOClines 删除含连续 ... 的目录行。
func removeTOClines(text string) string {
	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, l := range lines {
		if !strings.Contains(l, ".....") && !strings.Contains(l, "……") {
			cleaned = append(cleaned, l)
		}
	}
	return strings.Join(cleaned, "\n")
}

// ============ Markdown 提取（RAG 用）============

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
