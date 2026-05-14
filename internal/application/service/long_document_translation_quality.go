package service

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	translationBatchPageMarkerRE     = regexp.MustCompile(`(?i)^\s*(?:第\s*\d+\s*页|page\s*\d+(?:\s*(?:of|/)\s*\d+)?)\s*$`)
	translationBatchHeaderFooterRE   = regexp.MustCompile(`(?i)(?:copyright|all rights reserved|confidential|内部资料|机密|版权所有|未经许可)`)
	translationBatchPromptLeakTagRE  = regexp.MustCompile(`(?i)</?(?:local_knowledge_context|document_(?:revision|continuation)_context|document_patch|target_section|target_parent|nearby_siblings|source_anchor_heading|source_section|destination_section(?:_heading)?|document_outline|document_tail|document_head|original_user_goal)\b`)
	translationBatchTableSeparatorRE = regexp.MustCompile(`^\|\s*:?-{3,}:?\s*(?:\|\s*:?-{3,}:?\s*)+\|?\s*$`)
	translationBatchTableAlignmentRE = regexp.MustCompile(`^:?-{3,}:?$`)
)

func applyLongDocumentTranslationBatchQualityGate(inputSnapshot string, output string) (string, error) {
	trimmedOutput := strings.TrimSpace(output)
	if trimmedOutput == "" {
		return "", fmt.Errorf("translation_batch_empty_output")
	}
	if normalized, _ := normalizeGeneratedMarkdown(trimmedOutput); strings.TrimSpace(normalized) != "" {
		trimmedOutput = strings.TrimSpace(normalized)
	}
	if err := validateLongDocumentTranslationBatchOutput(inputSnapshot, trimmedOutput); err != nil {
		return "", err
	}
	return trimmedOutput, nil
}

func validateLongDocumentTranslationBatchOutput(inputSnapshot string, output string) error {
	trimmedOutput := strings.TrimSpace(output)
	if trimmedOutput == "" {
		return fmt.Errorf("translation_batch_empty_output")
	}
	if strings.Count(trimmedOutput, "```")%2 != 0 {
		return fmt.Errorf("translation_batch_markdown_fence_unbalanced")
	}
	if hasRepeatedLongDocumentTranslationParagraph(trimmedOutput) {
		return fmt.Errorf("translation_batch_repeated_output")
	}
	if hasObviousLongDocumentTranslationSourceLeak(inputSnapshot, trimmedOutput) {
		return fmt.Errorf("translation_batch_source_leak")
	}
	if hasLongDocumentTranslationHeaderFooterNoise(trimmedOutput) {
		return fmt.Errorf("translation_batch_header_footer_noise")
	}
	if hasMalformedLongDocumentTranslationTable(trimmedOutput) {
		return fmt.Errorf("translation_batch_table_structure_invalid")
	}
	if translationBatchPromptLeakTagRE.MatchString(trimmedOutput) {
		return fmt.Errorf("translation_batch_prompt_leak")
	}
	return nil
}

func hasRepeatedLongDocumentTranslationParagraph(output string) bool {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	counts := map[string]int{}
	previous := ""
	for _, line := range lines {
		paragraph := strings.Join(strings.Fields(strings.TrimSpace(line)), " ")
		if len([]rune(paragraph)) < 40 || strings.HasPrefix(paragraph, "#") || strings.HasPrefix(paragraph, "|") {
			previous = ""
			continue
		}
		if paragraph == previous {
			return true
		}
		previous = paragraph
		counts[paragraph]++
		if counts[paragraph] >= 3 {
			return true
		}
	}
	return false
}

func hasObviousLongDocumentTranslationSourceLeak(inputSnapshot string, output string) bool {
	normalizedInput := normalizeLongDocumentTranslationLeakText(inputSnapshot)
	normalizedOutput := normalizeLongDocumentTranslationLeakText(output)
	if len([]rune(normalizedInput)) < 120 || len([]rune(normalizedOutput)) < 120 {
		return false
	}
	if normalizedInput == normalizedOutput {
		return true
	}
	inputRunes := []rune(normalizedInput)
	probeLen := min(240, len(inputRunes))
	probe := strings.TrimSpace(string(inputRunes[:probeLen]))
	return len([]rune(probe)) >= 120 && strings.Contains(normalizedOutput, probe)
}

func normalizeLongDocumentTranslationLeakText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func hasLongDocumentTranslationHeaderFooterNoise(output string) bool {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	counts := map[string]int{}
	for _, rawLine := range lines {
		line := strings.Join(strings.Fields(strings.TrimSpace(rawLine)), " ")
		if !isLikelyLongDocumentTranslationNoiseLine(line) {
			continue
		}
		counts[strings.ToLower(line)]++
		if counts[strings.ToLower(line)] >= 2 {
			return true
		}
	}
	return false
}

func isLikelyLongDocumentTranslationNoiseLine(line string) bool {
	if line == "" || len([]rune(line)) > 80 {
		return false
	}
	if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "|") {
		return false
	}
	return translationBatchPageMarkerRE.MatchString(line) || translationBatchHeaderFooterRE.MatchString(line)
}

func hasMalformedLongDocumentTranslationTable(output string) bool {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	for index := 0; index < len(lines); {
		if !isTranslationMarkdownTableLine(lines[index]) {
			index++
			continue
		}
		start := index
		for index < len(lines) && isTranslationMarkdownTableLine(lines[index]) {
			index++
		}
		if index-start < 2 {
			continue
		}
		if isMalformedTranslationMarkdownTable(lines[start:index]) {
			return true
		}
	}
	return false
}

func isTranslationMarkdownTableLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|")
}

func isMalformedTranslationMarkdownTable(lines []string) bool {
	separatorIndex := -1
	expectedColumns := 0
	for index, line := range lines {
		cells := splitTranslationMarkdownTableCells(line)
		if len(cells) < 2 {
			return true
		}
		if isTranslationMarkdownSeparatorRow(cells) {
			separatorIndex = index
			if expectedColumns == 0 {
				expectedColumns = len(cells)
			}
			continue
		}
		if expectedColumns == 0 {
			expectedColumns = len(cells)
		}
		if len(cells) != expectedColumns {
			return true
		}
	}
	return separatorIndex != 1
}

func splitTranslationMarkdownTableCells(line string) []string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")
	parts := strings.Split(trimmed, "|")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		result = append(result, strings.TrimSpace(part))
	}
	return result
}

func isTranslationMarkdownSeparatorRow(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		if !translationBatchTableAlignmentRE.MatchString(cell) {
			return false
		}
	}
	return true
}
