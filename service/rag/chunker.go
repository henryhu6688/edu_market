package rag

import (
	"regexp"
	"strings"
)

// Chunk 切片单元
type Chunk struct {
	Content     string
	SectionPath string
}

// ParsedSection 解析后的文档章节
type ParsedSection struct {
	Title    string
	Level    int
	Content  string
	Children []*ParsedSection
}

// Chunker 结构切片器
type Chunker struct {
	minSize int
	maxSize int
}

// NewChunker 创建切片器
func NewChunker(minSize, maxSize int) *Chunker {
	return &Chunker{minSize: minSize, maxSize: maxSize}
}

// parseMDSections 从 Markdown # 标题提取章节树。
func (c *Chunker) parseMDSections(text string) []*ParsedSection {
	lines := strings.Split(text, "\n")
	re := regexp.MustCompile(`^(#{1,6})\s+(.+)$`)

	var sections []*ParsedSection
	var stack []*ParsedSection

	for _, line := range lines {
		m := re.FindStringSubmatch(line)
		if m == nil {
			if len(stack) > 0 {
				stack[len(stack)-1].Content += line + "\n"
			}
			continue
		}
		level := len(m[1])
		title := strings.TrimSpace(m[2])
		sec := &ParsedSection{Level: level, Title: title}

		for len(stack) > 0 && stack[len(stack)-1].Level >= level {
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 {
			stack[len(stack)-1].Children = append(stack[len(stack)-1].Children, sec)
		} else {
			sections = append(sections, sec)
		}
		stack = append(stack, sec)
	}
	return sections
}

// chunkFromSections 从章节树提取叶节点切片。
func (c *Chunker) chunkFromSections(sections []*ParsedSection) []Chunk {
	var chunks []Chunk
	c.collectLeafChunks(sections, "", &chunks)
	return c.mergeShortChunks(chunks)
}

func (c *Chunker) collectLeafChunks(sections []*ParsedSection, parentPath string, chunks *[]Chunk) {
	for _, s := range sections {
		path := s.Title
		if parentPath != "" {
			path = parentPath + " > " + s.Title
		}

		if len(s.Children) == 0 {
			content := c.cleanTitleMarkers(s.Content)
			if len([]rune(content)) > 0 {
				if len([]rune(content)) > c.maxSize {
					for _, sub := range c.splitByParagraphs(content, path) {
						*chunks = append(*chunks, sub)
					}
				} else {
					*chunks = append(*chunks, Chunk{Content: content, SectionPath: path})
				}
			}
		} else {
			c.collectLeafChunks(s.Children, path, chunks)
		}
	}
}

// cleanTitleMarkers 切片后移除 # 标记。
func (c *Chunker) cleanTitleMarkers(text string) string {
	re := regexp.MustCompile(`(?m)^#{1,6}\s+`)
	return strings.TrimSpace(re.ReplaceAllString(text, ""))
}

// mergeShortChunks 将 < minSize 的短 chunk 合并到上一个。
func (c *Chunker) mergeShortChunks(chunks []Chunk) []Chunk {
	if len(chunks) <= 1 {
		return chunks
	}
	var result []Chunk
	prev := chunks[0]
	for i := 1; i < len(chunks); i++ {
		if len([]rune(prev.Content)) < c.minSize {
			prev.Content += "\n\n" + chunks[i].Content
			if chunks[i].SectionPath != "" {
				prev.SectionPath = chunks[i].SectionPath
			}
		} else {
			result = append(result, prev)
			prev = chunks[i]
		}
	}
	result = append(result, prev)
	return result
}

// splitByParagraphs 超长 chunk 按段落边界再切。
func (c *Chunker) splitByParagraphs(text string, path string) []Chunk {
	paragraphs := strings.Split(text, "\n\n")
	var chunks []Chunk
	var buf string
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if len([]rune(buf))+len([]rune(p)) < c.maxSize {
			buf += p + "\n\n"
		} else {
			if buf != "" {
				chunks = append(chunks, Chunk{Content: strings.TrimSpace(buf), SectionPath: path})
			}
			buf = p + "\n\n"
		}
	}
	if buf != "" {
		chunks = append(chunks, Chunk{Content: strings.TrimSpace(buf), SectionPath: path})
	}
	return chunks
}

// chunkPlain 纯文本切片（PDF/TXT 无结构时用）。
func (c *Chunker) chunkPlain(text string) []Chunk {
	return c.splitByParagraphs(text, "")
}
