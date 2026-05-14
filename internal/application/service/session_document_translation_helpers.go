package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
)

const longDocumentTranslationOutputFormatMarkdown = "markdown"

var markdownFenceRE = regexp.MustCompile("(?s)^```(?:markdown|md)?\\s*(.*?)\\s*```$")
var translationMarkdownFenceRE = regexp.MustCompile("(?s)^```(?:markdown|md)?\\s*(.*?)\\s*```$")

type longDocumentBatchPlan struct {
	start int
	end   int
	input string
}

type longDocumentBatchAccumulator struct {
	parts []string
	chars int
	count int
}

type longDocumentTranslationPromptBatch struct {
	ChunkStartSeq int
	ChunkEndSeq   int
	InputSnapshot string
}

func planLongDocumentTranslationBatches(cfg *config.Config, chunks []*types.Chunk) []longDocumentBatchPlan {
	if len(chunks) == 0 {
		return nil
	}
	batchChunkSize := 8
	batchMaxChars := 24000
	if cfg != nil && cfg.LongDocument != nil {
		if cfg.LongDocument.BatchChunkSize > 0 {
			batchChunkSize = cfg.LongDocument.BatchChunkSize
		}
		if cfg.LongDocument.BatchMaxChars > 0 {
			batchMaxChars = cfg.LongDocument.BatchMaxChars
		}
	}
	batchChunkSize = max(1, batchChunkSize)
	batchMaxChars = max(1, batchMaxChars)

	plans := make([]longDocumentBatchPlan, 0)
	var (
		current longDocumentBatchAccumulator
		start   int
		end     int
		prevEnd int
	)
	prevEnd = -1
	flush := func() {
		if current.count == 0 {
			return
		}
		plans = append(plans, longDocumentBatchPlan{start: start, end: end, input: strings.Join(current.parts, "")})
		current = longDocumentBatchAccumulator{}
		start = 0
		end = 0
	}
	for _, chunk := range chunks {
		content := uniqueLongDocumentTranslationChunkContent(prevEnd, chunk)
		if content == "" {
			if chunk != nil && chunk.EndAt > prevEnd {
				prevEnd = chunk.EndAt
			}
			continue
		}
		if current.count == 0 {
			start = chunk.ChunkIndex
		}
		contentLen := len([]rune(content))
		if current.count > 0 && (current.count+1 > batchChunkSize || current.chars+contentLen > batchMaxChars) {
			flush()
			start = chunk.ChunkIndex
		}
		current.parts = append(current.parts, content)
		current.chars += contentLen
		current.count++
		end = chunk.ChunkIndex
		if chunk.EndAt > prevEnd {
			prevEnd = chunk.EndAt
		}
		if current.chars >= batchMaxChars {
			flush()
		}
	}
	flush()
	return plans
}

func buildLongDocumentSnapshotHash(chunks []*types.Chunk) string {
	hash := sha256.New()
	for _, chunk := range chunks {
		_, _ = hash.Write([]byte(fmt.Sprintf("%s:%d:%s\n", chunk.ID, chunk.ChunkIndex, chunk.Content)))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func sanitizeGeneratedMarkdown(content string) string {
	trimmed := strings.TrimSpace(content)
	if matches := translationMarkdownFenceRE.FindStringSubmatch(trimmed); len(matches) == 2 {
		trimmed = strings.TrimSpace(matches[1])
	}
	trimmed = strings.ReplaceAll(trimmed, "\r\n", "\n")
	trimmed = strings.ReplaceAll(trimmed, "\f", "\n\n")
	return strings.TrimSpace(trimmed)
}

func uniqueLongDocumentTranslationChunkContent(previousEnd int, chunk *types.Chunk) string {
	if chunk == nil || strings.TrimSpace(chunk.Content) == "" {
		return ""
	}
	contentRunes := []rune(chunk.Content)
	if previousEnd < 0 || chunk.StartAt >= previousEnd {
		return string(contentRunes)
	}
	if chunk.EndAt <= previousEnd {
		return ""
	}
	suffixLen := chunk.EndAt - previousEnd
	offset := len(contentRunes) - suffixLen
	if offset < 0 {
		offset = 0
	}
	if offset > len(contentRunes) {
		offset = len(contentRunes)
	}
	return string(contentRunes[offset:])
}

func buildLongDocumentTranslationBatchPrompt(ctx context.Context, outputFormat string, batch longDocumentTranslationPromptBatch, options types.ChatDocumentTranslationOptions) (string, error) {
	tpl := `你是一个专业文档翻译助手。请将给定文档片段翻译为 {{.TargetLanguage}}，并严格输出 Markdown 正文。

要求：
1. 保留原始标题层级、列表、表格和引用结构。
2. 不要补充与原文无关的解释，不要添加前言或结语。
3. 如果原文已经是 Markdown，请继续输出合法 Markdown，不要人为增加多余空行或重排段落层级。
4. 如果原文来自 PDF/OCR，遇到仅因视觉换行产生的断行时，请合并为自然段；只有在标题、列表项、表格行、引用块等结构边界处才保留换行。
5. 明显属于页眉、页脚、页码、分页符、重复刊名、孤立控制符或重复段落的解析噪声，不要带入结果。
6. 无法确定的专有名词保留原文，并在必要时直接音译。
7. 只输出翻译后的正文，不要包裹代码块围栏。

任务类型：{{.TaskKind}}
目标格式：{{.OutputFormat}}
片段范围：{{.ChunkStart}}-{{.ChunkEnd}}

原文片段：
{{.Input}}
`
	data := map[string]string{
		"TargetLanguage": firstNonEmptyString(strings.TrimSpace(options.TargetLanguage), types.LanguageNameFromContext(ctx), "Chinese (Simplified)"),
		"TaskKind":       types.ChatDocumentTaskKindTranslation,
		"OutputFormat":   firstNonEmptyString(strings.TrimSpace(outputFormat), longDocumentTranslationOutputFormatMarkdown),
		"ChunkStart":     fmt.Sprintf("%d", batch.ChunkStartSeq),
		"ChunkEnd":       fmt.Sprintf("%d", batch.ChunkEndSeq),
		"Input":          batch.InputSnapshot,
	}
	tmpl, err := template.New("long_document_translation").Parse(tpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
