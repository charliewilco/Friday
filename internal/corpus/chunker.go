package corpus

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charliewilco/friday/internal/config"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type Chunk struct {
	HeadingPath string
	Content     string
	TokenCount  int
}

type section struct {
	Heading      string
	HeadingPath  string
	Level        int
	Text         string
	TokenCount   int
	SourceOffset int
}

func ChunkMarkdown(path string, input []byte, cfg config.ContentConfig) ([]Chunk, string, error) {
	fm, err := ParseFrontmatter(input)
	if err != nil {
		return nil, "", err
	}

	title := FrontmatterTitle(fm.Values)
	source := fm.Body
	if len(source) == 0 {
		source = input
	}

	md := goldmark.New()
	doc := md.Parser().Parse(text.NewReader(source))
	headings := collectHeadings(doc, source)
	if title == "" && len(headings) > 0 {
		title = headings[0].Text
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}

	sections := collectSections(headings, source, title)
	sections = mergeSmallSections(sections, cfg.MinChunkTokens)

	frontmatterContext := ""
	if cfg.IncludeFrontmatter {
		frontmatterContext = BuildFrontmatterContext(fm.Values)
	}

	var chunks []Chunk
	for _, sec := range sections {
		for _, split := range splitSection(sec, cfg.MaxChunkTokens) {
			content := strings.TrimSpace(split.Text)
			if content == "" {
				continue
			}
			final := buildChunkContent(frontmatterContext, split.HeadingPath, content)
			chunks = append(chunks, Chunk{
				HeadingPath: split.HeadingPath,
				Content:     final,
				TokenCount:  estimateTokens(final),
			})
		}
	}

	if len(chunks) == 0 {
		content := buildChunkContent(frontmatterContext, title, strings.TrimSpace(string(source)))
		chunks = append(chunks, Chunk{
			HeadingPath: title,
			Content:     content,
			TokenCount:  estimateTokens(content),
		})
	}

	return chunks, title, nil
}

type headingInfo struct {
	Level         int
	Text          string
	HeadingStart  int
	ContentStart  int
	HeadingPath   string
	AncestorTexts []string
}

func collectHeadings(doc ast.Node, source []byte) []headingInfo {
	var headings []headingInfo
	var stack []headingInfo

	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		heading, ok := node.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}

		for len(stack) > 0 && stack[len(stack)-1].Level >= heading.Level {
			stack = stack[:len(stack)-1]
		}

		textValue := extractNodeText(source, heading)
		pathParts := make([]string, 0, len(stack)+1)
		for _, item := range stack {
			pathParts = append(pathParts, item.Text)
		}
		pathParts = append(pathParts, textValue)

		info := headingInfo{
			Level:         heading.Level,
			Text:          textValue,
			HeadingStart:  nodeStart(heading),
			ContentStart:  nodeEnd(heading),
			HeadingPath:   strings.Join(pathParts, " > "),
			AncestorTexts: pathParts,
		}
		headings = append(headings, info)
		stack = append(stack, info)
		return ast.WalkContinue, nil
	})

	return headings
}

func collectSections(headings []headingInfo, source []byte, fallbackTitle string) []section {
	if len(headings) == 0 {
		textValue := strings.TrimSpace(string(source))
		return []section{{
			Heading:     fallbackTitle,
			HeadingPath: fallbackTitle,
			Level:       1,
			Text:        textValue,
			TokenCount:  estimateTokens(textValue),
		}}
	}

	sections := make([]section, 0, len(headings))
	for idx, heading := range headings {
		end := len(source)
		for nextIdx := idx + 1; nextIdx < len(headings); nextIdx++ {
			if headings[nextIdx].Level <= heading.Level {
				end = headings[nextIdx].HeadingStart
				break
			}
		}

		textValue := strings.TrimSpace(string(source[heading.ContentStart:end]))
		if textValue == "" {
			continue
		}

		sections = append(sections, section{
			Heading:      heading.Text,
			HeadingPath:  heading.HeadingPath,
			Level:        heading.Level,
			Text:         textValue,
			TokenCount:   estimateTokens(textValue),
			SourceOffset: heading.ContentStart,
		})
	}

	return sections
}

func mergeSmallSections(sections []section, minTokens int) []section {
	if minTokens <= 0 || len(sections) < 2 {
		return sections
	}

	merged := make([]section, 0, len(sections))
	i := 0
	for i < len(sections) {
		current := sections[i]
		if current.TokenCount >= minTokens {
			merged = append(merged, current)
			i++
			continue
		}

		if i+1 < len(sections) && sections[i+1].Level == current.Level {
			next := sections[i+1]
			current.Text = strings.TrimSpace(current.Text + "\n\n" + next.Text)
			current.TokenCount = estimateTokens(current.Text)
			merged = append(merged, current)
			i += 2
			continue
		}

		if len(merged) > 0 && merged[len(merged)-1].Level == current.Level {
			merged[len(merged)-1].Text = strings.TrimSpace(merged[len(merged)-1].Text + "\n\n" + current.Text)
			merged[len(merged)-1].TokenCount = estimateTokens(merged[len(merged)-1].Text)
			i++
			continue
		}

		merged = append(merged, current)
		i++
	}

	return merged
}

func splitSection(sec section, maxTokens int) []section {
	if maxTokens <= 0 || sec.TokenCount <= maxTokens {
		return []section{sec}
	}

	paragraphs := splitParagraphs(sec.Text)
	if len(paragraphs) <= 1 {
		return []section{sec}
	}

	var chunks []section
	var current bytes.Buffer
	for _, paragraph := range paragraphs {
		candidate := strings.TrimSpace(current.String())
		if candidate != "" {
			candidate += "\n\n" + paragraph
		} else {
			candidate = paragraph
		}

		if estimateTokens(candidate) > maxTokens && current.Len() > 0 {
			textValue := strings.TrimSpace(current.String())
			chunks = append(chunks, section{
				Heading:     sec.Heading,
				HeadingPath: sec.HeadingPath,
				Level:       sec.Level,
				Text:        textValue,
				TokenCount:  estimateTokens(textValue),
			})
			current.Reset()
			current.WriteString(paragraph)
			continue
		}

		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(paragraph)
	}

	if current.Len() > 0 {
		textValue := strings.TrimSpace(current.String())
		chunks = append(chunks, section{
			Heading:     sec.Heading,
			HeadingPath: sec.HeadingPath,
			Level:       sec.Level,
			Text:        textValue,
			TokenCount:  estimateTokens(textValue),
		})
	}

	if len(chunks) == 0 {
		return []section{sec}
	}
	return chunks
}

func splitParagraphs(textValue string) []string {
	raw := strings.Split(textValue, "\n\n")
	parts := make([]string, 0, len(raw))
	for _, part := range raw {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func buildChunkContent(frontmatterContext, headingPath, sectionText string) string {
	parts := make([]string, 0, 3)
	if strings.TrimSpace(frontmatterContext) != "" {
		parts = append(parts, strings.TrimSpace(frontmatterContext))
	}
	if strings.TrimSpace(headingPath) != "" {
		parts = append(parts, strings.TrimSpace(headingPath))
	}
	if strings.TrimSpace(sectionText) != "" {
		parts = append(parts, strings.TrimSpace(sectionText))
	}
	return strings.Join(parts, "\n\n")
}

func estimateTokens(textValue string) int {
	if textValue == "" {
		return 0
	}
	return max(1, len(textValue)/4)
}

func extractNodeText(source []byte, node ast.Node) string {
	var builder strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch value := child.(type) {
		case *ast.Text:
			builder.Write(value.Segment.Value(source))
		case *ast.CodeSpan:
			builder.WriteString(string(value.Text(source)))
		default:
			builder.WriteString(extractNodeText(source, child))
		}
	}
	return strings.TrimSpace(builder.String())
}

func nodeStart(node ast.Node) int {
	lines := node.Lines()
	if lines == nil || lines.Len() == 0 {
		return 0
	}
	return lines.At(0).Start
}

func nodeEnd(node ast.Node) int {
	lines := node.Lines()
	if lines == nil || lines.Len() == 0 {
		return 0
	}
	last := lines.At(lines.Len() - 1)
	return last.Stop
}

func DebugSections(path string, input []byte, cfg config.ContentConfig) error {
	chunks, _, err := ChunkMarkdown(path, input, cfg)
	if err != nil {
		return err
	}
	for idx, chunk := range chunks {
		fmt.Printf("%d: %s (%d)\n%s\n\n", idx, chunk.HeadingPath, chunk.TokenCount, chunk.Content)
	}
	return nil
}
