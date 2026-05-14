package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

var (
	markdownQualityTightHeadingRE           = regexp.MustCompile(`^(\s*#{1,6})([^#\s].*)$`)
	markdownQualityTightNumberTitleRE       = regexp.MustCompile(`^(\s*#{3,6}\s+)(\d+(?:\.\d+)*)([^\s].*)$`)
	markdownQualityLooseNumberTitleRE       = regexp.MustCompile(`^(\s*#{3,6}\s+)(\d+(?:\s*\.\s*\d+)+)(\s*.*)$`)
	markdownQualityTightChapterTitleRE      = regexp.MustCompile(`^(\s*##\s+第\d+章)([^\s].*)$`)
	markdownQualityBulletSpacingRE          = regexp.MustCompile(`^(\s*[-+])(\S.*)$`)
	markdownQualitySingleIndentBulletRE     = regexp.MustCompile(`^ ([-*+].*)$`)
	markdownQualityBoldLabelTightRE         = regexp.MustCompile(`(\*\*[^*\n]+[：:]\*\*)([^\s\n])`)
	markdownQualityHeadingLineRE            = regexp.MustCompile(`^\s*(#{1,6})\s+(.+?)\s*$`)
	markdownQualitySubsectionNumberTitleRE  = regexp.MustCompile(`^(\d+(?:\.\d+)*)\s+(.+)$`)
	markdownQualityMalformedBulletRE        = regexp.MustCompile(`(?m)^\s*[-+][^\s]`)
	markdownQualityInternalPromptLeakTagRE  = regexp.MustCompile(`(?i)</?(?:local_knowledge_context|document_(?:revision|continuation)_context|document_patch|target_section|target_parent|nearby_siblings|source_anchor_heading|source_section|destination_section(?:_heading)?|document_outline|document_tail|document_head|original_user_goal)\b`)
	markdownQualityInternalPromptLeakLineRE = regexp.MustCompile(`(?i)^\s*(?:Current section(?: heading)?|Completed content so far|Allowed H3 subsections|Detected markdown issues|Markdown (?:fragment|content) to repair|snapshot_mode|document_edit_operation|document_merge_mode|target_heading|source_heading|resolved_target_heading|continuation_context_mode)\s*:?.*$`)
	markdownQualityInternalPromptLeakMetaRE = regexp.MustCompile(`(?i)^\s*[-*]\s*(?:knowledge_base_id|knowledge_id|chunk_id|source_query|snapshot_mode|document_edit_operation|document_merge_mode|target_heading|source_heading|resolved_target_heading|continuation_context_mode)\s*:`)
)

const markdownQualityMinimumBodyRunes = 8

type markdownQualityIssue struct {
	Code    string
	Message string
}

func normalizeGeneratedMarkdown(content string) (string, []string) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "", nil
	}
	if matches := markdownFenceRE.FindStringSubmatch(trimmed); len(matches) == 2 {
		trimmed = strings.TrimSpace(matches[1])
	}
	trimmed = strings.ReplaceAll(trimmed, "\r\n", "\n")
	trimmed = strings.ReplaceAll(trimmed, "\r", "\n")
	trimmed = strings.ReplaceAll(trimmed, "\f", "\n\n")

	headingNormalized := false
	lines := strings.Split(trimmed, "\n")
	normalizedLines := make([]string, 0, len(lines))
	inCodeFence := false
	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, " \t")
		trimmedLine := strings.TrimSpace(strings.TrimLeft(line, "\ufeff"))
		if strings.HasPrefix(trimmedLine, "```") {
			inCodeFence = !inCodeFence
			normalizedLines = append(normalizedLines, line)
			continue
		}
		if inCodeFence {
			normalizedLines = append(normalizedLines, line)
			continue
		}
		if markdownQualitySingleIndentBulletRE.MatchString(line) {
			line = "  " + strings.TrimLeft(line, " ")
		}
		if markdownQualityTightHeadingRE.MatchString(line) {
			line = markdownQualityTightHeadingRE.ReplaceAllString(line, `$1 $2`)
			headingNormalized = true
		}
		if markdownQualityTightNumberTitleRE.MatchString(line) {
			line = markdownQualityTightNumberTitleRE.ReplaceAllString(line, `$1$2 $3`)
			headingNormalized = true
		}
		if markdownQualityLooseNumberTitleRE.MatchString(line) {
			if normalizedLooseHeading := normalizeMarkdownLooseNumberHeading(line); normalizedLooseHeading != line {
				line = normalizedLooseHeading
				headingNormalized = true
			}
		}
		if markdownQualityTightChapterTitleRE.MatchString(line) {
			line = markdownQualityTightChapterTitleRE.ReplaceAllString(line, `$1 $2`)
			headingNormalized = true
		}
		if markdownQualityBulletSpacingRE.MatchString(line) {
			line = markdownQualityBulletSpacingRE.ReplaceAllString(line, `$1 $2`)
		}
		if normalizedStarBullet, changed := normalizeMarkdownStarBulletSpacing(line); changed {
			line = normalizedStarBullet
		}
		if markdownQualityBoldLabelTightRE.MatchString(line) {
			line = markdownQualityBoldLabelTightRE.ReplaceAllString(line, `$1 $2`)
		}
		normalizedLines = append(normalizedLines, line)
	}

	withHeadingSpacing := make([]string, 0, len(normalizedLines)+4)
	for index, line := range normalizedLines {
		trimmedLine := strings.TrimSpace(line)
		isHeading := markdownQualityHeadingLineRE.MatchString(trimmedLine)
		if isHeading && len(withHeadingSpacing) > 0 && strings.TrimSpace(withHeadingSpacing[len(withHeadingSpacing)-1]) != "" {
			withHeadingSpacing = append(withHeadingSpacing, "")
		}
		withHeadingSpacing = append(withHeadingSpacing, line)
		if isHeading && index+1 < len(normalizedLines) && strings.TrimSpace(normalizedLines[index+1]) != "" {
			withHeadingSpacing = append(withHeadingSpacing, "")
		}
	}

	compressed := make([]string, 0, len(withHeadingSpacing))
	previousBlank := false
	for _, line := range withHeadingSpacing {
		if strings.TrimSpace(line) == "" {
			if previousBlank {
				continue
			}
			compressed = append(compressed, "")
			previousBlank = true
			continue
		}
		compressed = append(compressed, line)
		previousBlank = false
	}

	normalized := strings.TrimSpace(strings.Join(compressed, "\n"))
	if normalized == "" {
		return "", nil
	}
	qualityIssues := []string{}
	if headingNormalized {
		qualityIssues = append(qualityIssues, types.ChatDocumentQualityIssueMarkdownHeadingNormalized)
	}
	return normalized, uniqueNonEmptyStrings(qualityIssues)
}

func normalizeGeneratedSectionMarkdown(content string, section dedicatedFullDocumentSection) (string, []string) {
	normalized, qualitySignals := normalizeGeneratedMarkdown(content)
	if strings.TrimSpace(normalized) == "" {
		return normalized, qualitySignals
	}

	section = normalizeDedicatedFullDocumentSectionForMarkdownQuality(section)
	plannedByNumber := make(map[string]string, len(section.Subsections))
	for _, subsection := range section.Subsections {
		number := normalizeMarkdownQualitySubsectionNumber(subsection.Number)
		title := strings.TrimSpace(subsection.Title)
		if number == "" || title == "" {
			continue
		}
		plannedByNumber[number] = title
	}

	changed := false
	droppedRepeatedHeading := false
	lines := strings.Split(normalized, "\n")
	repairedLines := make([]string, 0, len(lines)+4)
	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, " \t")
		trimmedLine := strings.TrimSpace(line)
		matches := markdownQualityHeadingLineRE.FindStringSubmatch(trimmedLine)
		if len(matches) != 3 {
			repairedLines = append(repairedLines, line)
			continue
		}

		level := len(matches[1])
		headingText := strings.TrimSpace(matches[2])
		if level <= 2 {
			if shouldDropRepeatedSectionHeading(headingText, section) {
				changed = true
				droppedRepeatedHeading = true
				continue
			}
			repairedLines = append(repairedLines, line)
			continue
		}

		number, title := parseMarkdownSubsectionNumberAndTitle(headingText)
		number = normalizeMarkdownQualitySubsectionNumber(number)
		plannedTitle := strings.TrimSpace(plannedByNumber[number])
		if number == "" || plannedTitle == "" {
			repairedLines = append(repairedLines, line)
			continue
		}

		trimmedTitle := strings.TrimSpace(title)
		if trimmedTitle == plannedTitle {
			repairedLines = append(repairedLines, fmt.Sprintf("### %s %s", number, plannedTitle))
			if line != repairedLines[len(repairedLines)-1] {
				changed = true
			}
			continue
		}
		if strings.HasPrefix(trimmedTitle, plannedTitle) {
			remainder := strings.TrimSpace(strings.TrimPrefix(trimmedTitle, plannedTitle))
			repairedLines = append(repairedLines, fmt.Sprintf("### %s %s", number, plannedTitle))
			if remainder != "" {
				repairedLines = append(repairedLines, "", remainder)
			}
			changed = true
			continue
		}
		repairedLines = append(repairedLines, line)
	}

	if !changed {
		return normalized, qualitySignals
	}
	repaired, repairedSignals := normalizeGeneratedMarkdown(strings.Join(repairedLines, "\n"))
	combinedSignals := append(append([]string{}, qualitySignals...), repairedSignals...)
	combinedSignals = append(combinedSignals, types.ChatDocumentQualityIssueMarkdownHeadingNormalized)
	if droppedRepeatedHeading {
		combinedSignals = append(combinedSignals, types.ChatDocumentQualityIssueMarkdownStructureInvalid)
	}
	return repaired, uniqueNonEmptyStrings(combinedSignals)
}

func validateGeneratedSectionMarkdown(content string, section dedicatedFullDocumentSection) []markdownQualityIssue {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return []markdownQualityIssue{{Code: types.ChatDocumentQualityIssueMarkdownStructureInvalid, Message: "section content is empty after markdown normalization"}}
	}
	issues := validateNormalizedMarkdownContent(trimmed)
	if markdownQualityBodyTooShort(trimmed) {
		issues = append(issues, markdownQualityIssue{Code: types.ChatDocumentQualityIssueMarkdownTooShort, Message: "section content is too short after markdown normalization"})
	}
	allowedByNumber := make(map[string]string, len(section.Subsections))
	allowedByTitle := make(map[string]string, len(section.Subsections))
	for _, subsection := range section.Subsections {
		number := strings.TrimSpace(subsection.Number)
		title := normalizeMarkdownQualityTitle(subsection.Title)
		if number == "" || title == "" {
			continue
		}
		allowedByNumber[number] = title
		allowedByTitle[title] = number
	}

	for _, rawLine := range strings.Split(trimmed, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || line == types.ChatDocumentCompletionMarker {
			continue
		}
		matches := markdownQualityHeadingLineRE.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}
		level := len(matches[1])
		headingText := strings.TrimSpace(matches[2])
		if level <= 2 {
			issues = append(issues, markdownQualityIssue{Code: types.ChatDocumentQualityIssueMarkdownStructureInvalid, Message: "section output must not repeat H1/H2 headings"})
			continue
		}
		if level > 4 {
			issues = append(issues, markdownQualityIssue{Code: types.ChatDocumentQualityIssueMarkdownStructureInvalid, Message: "section output must not contain H5 or deeper headings"})
			continue
		}
		if level == 4 {
			continue
		}
		number, title := parseMarkdownSubsectionNumberAndTitle(headingText)
		expectedPrefix := fmt.Sprintf("%d.", section.Number)
		if number == "" || !strings.HasPrefix(number, expectedPrefix) {
			issues = append(issues, markdownQualityIssue{Code: types.ChatDocumentQualityIssueMarkdownStructureInvalid, Message: fmt.Sprintf("subsection heading must use %s* numbering", expectedPrefix)})
			continue
		}
		if len(allowedByNumber) == 0 {
			continue
		}
		normalizedTitle := normalizeMarkdownQualityTitle(title)
		if expectedTitle, ok := allowedByNumber[number]; ok {
			if expectedTitle != normalizedTitle {
				issues = append(issues, markdownQualityIssue{Code: types.ChatDocumentQualityIssueMarkdownUnplannedSubsection, Message: fmt.Sprintf("subsection %s title does not match planned outline", number)})
			}
			continue
		}
		if plannedNumber, ok := allowedByTitle[normalizedTitle]; !ok || plannedNumber != number {
			issues = append(issues, markdownQualityIssue{Code: types.ChatDocumentQualityIssueMarkdownUnplannedSubsection, Message: fmt.Sprintf("subsection %s is not in the planned outline", number)})
		}
	}
	return dedupeMarkdownQualityIssues(issues)
}

func validateGeneratedDocumentMarkdown(content string) []markdownQualityIssue {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return []markdownQualityIssue{{Code: types.ChatDocumentQualityIssueMarkdownStructureInvalid, Message: "document content is empty after markdown normalization"}}
	}
	issues := validateNormalizedMarkdownContent(trimmed)
	if markdownQualityBodyTooShort(trimmed) {
		issues = append(issues, markdownQualityIssue{Code: types.ChatDocumentQualityIssueMarkdownTooShort, Message: "document content is too short after markdown normalization"})
	}
	for _, rawLine := range strings.Split(trimmed, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || line == types.ChatDocumentCompletionMarker {
			continue
		}
		matches := markdownQualityHeadingLineRE.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}
		if len(matches[1]) > 4 {
			issues = append(issues, markdownQualityIssue{Code: types.ChatDocumentQualityIssueMarkdownStructureInvalid, Message: "document output must not contain H5 or deeper headings"})
		}
	}
	return dedupeMarkdownQualityIssues(issues)
}

func validateNormalizedMarkdownContent(content string) []markdownQualityIssue {
	issues := make([]markdownQualityIssue, 0, 4)
	if containsMalformedMarkdownHeading(content) {
		issues = append(issues, markdownQualityIssue{Code: types.ChatDocumentQualityIssueMarkdownStructureInvalid, Message: "markdown heading must contain a space after # markers"})
	}
	if markdownQualityMalformedBulletRE.MatchString(content) || containsSingleIndentMarkdownBullet(content) || containsMalformedMarkdownStarBullet(content) {
		issues = append(issues, markdownQualityIssue{Code: types.ChatDocumentQualityIssueMarkdownStructureInvalid, Message: "markdown list items must use normalized indentation and spacing"})
	}
	if markdownQualityBoldLabelTightRE.MatchString(content) {
		issues = append(issues, markdownQualityIssue{Code: types.ChatDocumentQualityIssueMarkdownStructureInvalid, Message: "bold labels must be followed by a space before正文"})
	}
	if containsInternalPromptLeakage(content) {
		issues = append(issues, markdownQualityIssue{Code: types.ChatDocumentQualityIssueInternalPromptLeakage, Message: "output must not contain internal prompt labels, metadata, or context tags"})
	}
	return dedupeMarkdownQualityIssues(issues)
}

func applyGeneratedSectionMarkdownQualityGate(
	ctx context.Context,
	chatModel chat.Chat,
	agentConfig *types.AgentConfig,
	budget DocumentGenerationBudget,
	section dedicatedFullDocumentSection,
	content string,
) (string, []string, bool) {
	normalized, qualitySignals := normalizeGeneratedSectionMarkdown(content, section)
	issues := validateGeneratedSectionMarkdown(normalized, section)
	if len(issues) == 0 {
		return normalized, uniqueNonEmptyStrings(qualitySignals), true
	}
	repaired, repairErr := repairGeneratedMarkdown(ctx, chatModel, agentConfig, budget, buildSectionMarkdownRepairMessages(section, normalized, issues), budget.SectionMaxCompletionTokens)
	if repairErr != nil {
		return normalized, uniqueNonEmptyStrings(append(qualitySignals, markdownQualityIssueCodes(issues)...)), false
	}
	repairedNormalized, repairedSignals := normalizeGeneratedSectionMarkdown(repaired, section)
	issues = validateGeneratedSectionMarkdown(repairedNormalized, section)
	if len(issues) > 0 {
		combinedSignals := append(append([]string{}, qualitySignals...), repairedSignals...)
		combinedSignals = append(combinedSignals, markdownQualityIssueCodes(issues)...)
		return repairedNormalized, uniqueNonEmptyStrings(combinedSignals), false
	}
	return repairedNormalized, uniqueNonEmptyStrings(append(qualitySignals, repairedSignals...)), true
}

func applyGeneratedDocumentMarkdownQualityGate(
	ctx context.Context,
	chatModel chat.Chat,
	agentConfig *types.AgentConfig,
	budget DocumentGenerationBudget,
	content string,
) (string, []string, bool) {
	normalized, qualitySignals := normalizeGeneratedMarkdown(content)
	issues := validateGeneratedDocumentMarkdown(normalized)
	if len(issues) == 0 {
		return normalized, uniqueNonEmptyStrings(qualitySignals), true
	}
	repaired, repairErr := repairGeneratedMarkdown(ctx, chatModel, agentConfig, budget, buildDocumentMarkdownRepairMessages(normalized, issues), budget.ContinuationMaxCompletionTokens)
	if repairErr != nil {
		return normalized, uniqueNonEmptyStrings(append(qualitySignals, markdownQualityIssueCodes(issues)...)), false
	}
	repairedNormalized, repairedSignals := normalizeGeneratedMarkdown(repaired)
	issues = validateGeneratedDocumentMarkdown(repairedNormalized)
	if len(issues) > 0 {
		combinedSignals := append(append([]string{}, qualitySignals...), repairedSignals...)
		combinedSignals = append(combinedSignals, markdownQualityIssueCodes(issues)...)
		return repairedNormalized, uniqueNonEmptyStrings(combinedSignals), false
	}
	return repairedNormalized, uniqueNonEmptyStrings(append(qualitySignals, repairedSignals...)), true
}

func repairGeneratedMarkdown(
	ctx context.Context,
	chatModel chat.Chat,
	agentConfig *types.AgentConfig,
	budget DocumentGenerationBudget,
	messages []chat.Message,
	maxCompletionTokens int,
) (string, error) {
	if chatModel == nil {
		return "", fmt.Errorf("markdown repair chat model is nil")
	}
	repairCtx, cancel := withDocumentGenerationCallTimeout(ctx, agentConfig, budget)
	defer cancel()
	if maxCompletionTokens <= 0 || maxCompletionTokens > 2048 {
		maxCompletionTokens = 1024
	}
	thinking := false
	response, err := chatModel.Chat(repairCtx, messages, &chat.ChatOptions{
		Temperature:         0,
		MaxCompletionTokens: maxCompletionTokens,
		Thinking:            &thinking,
	})
	if err != nil {
		return "", err
	}
	if response == nil || strings.TrimSpace(response.Content) == "" {
		return "", fmt.Errorf("markdown repair returned empty content")
	}
	return response.Content, nil
}

func buildSectionMarkdownRepairMessages(section dedicatedFullDocumentSection, content string, issues []markdownQualityIssue) []chat.Message {
	var userContent strings.Builder
	userContent.WriteString("Current chapter heading:\n")
	userContent.WriteString(strings.TrimSpace(section.Heading))
	userContent.WriteString("\n\nAllowed H3 subsections:\n")
	if len(section.Subsections) == 0 {
		userContent.WriteString("- none\n")
	} else {
		for _, subsection := range section.Subsections {
			userContent.WriteString("- ")
			userContent.WriteString(strings.TrimSpace(subsection.Number))
			userContent.WriteString(" ")
			userContent.WriteString(strings.TrimSpace(subsection.Title))
			userContent.WriteString("\n")
		}
	}
	userContent.WriteString("\nDetected markdown issues:\n")
	for _, issue := range issues {
		userContent.WriteString("- ")
		userContent.WriteString(strings.TrimSpace(issue.Message))
		userContent.WriteString("\n")
	}
	userContent.WriteString("\nMarkdown fragment to repair:\n")
	userContent.WriteString(content)

	return []chat.Message{
		{
			Role:    "system",
			Content: "You repair markdown formatting only. Preserve all facts, ordering, terminology, and wording whenever possible. Return markdown only. Do not output markdown fences, explanations, or hidden reasoning. Keep the fragment inside the current chapter only. Do not output H1 or H2 headings. If H3 headings are present, they must use the planned numbering and titles exactly.",
		},
		{Role: "user", Content: userContent.String()},
	}
}

func buildDocumentMarkdownRepairMessages(content string, issues []markdownQualityIssue) []chat.Message {
	var userContent strings.Builder
	userContent.WriteString("Detected markdown issues:\n")
	for _, issue := range issues {
		userContent.WriteString("- ")
		userContent.WriteString(strings.TrimSpace(issue.Message))
		userContent.WriteString("\n")
	}
	userContent.WriteString("\nMarkdown content to repair:\n")
	userContent.WriteString(content)

	return []chat.Message{
		{
			Role:    "system",
			Content: "You repair markdown formatting only. Preserve all facts, ordering, terminology, and wording whenever possible. Return markdown only. Do not output markdown fences, explanations, or hidden reasoning.",
		},
		{Role: "user", Content: userContent.String()},
	}
}

func markdownQualityIssueCodes(issues []markdownQualityIssue) []string {
	codes := make([]string, 0, len(issues))
	for _, issue := range issues {
		if strings.TrimSpace(issue.Code) == "" {
			continue
		}
		codes = append(codes, strings.TrimSpace(issue.Code))
	}
	return uniqueNonEmptyStrings(codes)
}

func dedupeMarkdownQualityIssues(issues []markdownQualityIssue) []markdownQualityIssue {
	if len(issues) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(issues))
	result := make([]markdownQualityIssue, 0, len(issues))
	for _, issue := range issues {
		code := strings.TrimSpace(issue.Code)
		message := strings.TrimSpace(issue.Message)
		key := code + "\n" + message
		if code == "" && message == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, markdownQualityIssue{Code: code, Message: message})
	}
	return result
}

func parseMarkdownSubsectionNumberAndTitle(heading string) (string, string) {
	trimmed := strings.TrimSpace(heading)
	if trimmed == "" {
		return "", ""
	}
	if matches := markdownQualitySubsectionNumberTitleRE.FindStringSubmatch(trimmed); len(matches) == 3 {
		return strings.TrimSpace(matches[1]), strings.TrimSpace(matches[2])
	}
	return "", trimmed
}

func normalizeMarkdownLooseNumberHeading(line string) string {
	matches := markdownQualityLooseNumberTitleRE.FindStringSubmatch(line)
	if len(matches) != 4 {
		return line
	}
	prefix := matches[1]
	number := normalizeMarkdownQualitySubsectionNumber(matches[2])
	tail := strings.TrimSpace(matches[3])
	if number == "" {
		return line
	}
	if tail == "" {
		return prefix + number
	}
	return prefix + number + " " + tail
}

func normalizeMarkdownQualitySubsectionNumber(number string) string {
	replacer := strings.NewReplacer(" ", "", "\t", "")
	return strings.TrimSpace(replacer.Replace(number))
}

func normalizeDedicatedFullDocumentSectionForMarkdownQuality(section dedicatedFullDocumentSection) dedicatedFullDocumentSection {
	sectionNumber := section.Number
	if sectionNumber <= 0 {
		sectionNumber = 1
	}
	if normalized, ok := normalizeDedicatedFullDocumentSection(section, sectionNumber); ok {
		return normalized
	}
	return section
}

func shouldDropRepeatedSectionHeading(headingText string, section dedicatedFullDocumentSection) bool {
	normalizedHeading := normalizeMarkdownQualityTitle(headingText)
	if normalizedHeading == "" {
		return false
	}
	candidates := []string{
		section.Heading,
		section.Title,
		fmt.Sprintf("第%d章 %s", section.Number, strings.TrimSpace(section.Title)),
	}
	for _, candidate := range candidates {
		if normalizedHeading == normalizeMarkdownQualityTitle(candidate) {
			return true
		}
	}
	return false
}

func normalizeMarkdownQualityTitle(title string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(title)), " ")
}

func normalizeMarkdownStarBulletSpacing(line string) (string, bool) {
	trimmedLeft := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(trimmedLeft, "*") || strings.HasPrefix(trimmedLeft, "**") || len(trimmedLeft) < 2 {
		return line, false
	}
	if trimmedLeft[1] == ' ' || trimmedLeft[1] == '\t' {
		return line, false
	}
	indentLen := len(line) - len(trimmedLeft)
	return line[:indentLen] + "* " + trimmedLeft[1:], true
}

func containsMalformedMarkdownStarBullet(content string) bool {
	for _, rawLine := range strings.Split(content, "\n") {
		trimmedLeft := strings.TrimLeft(rawLine, " \t")
		if !strings.HasPrefix(trimmedLeft, "*") || strings.HasPrefix(trimmedLeft, "**") || len(trimmedLeft) < 2 {
			continue
		}
		if trimmedLeft[1] == ' ' || trimmedLeft[1] == '\t' {
			continue
		}
		return true
	}
	return false
}

func containsMalformedMarkdownHeading(content string) bool {
	for _, rawLine := range strings.Split(content, "\n") {
		trimmedLeft := strings.TrimLeft(rawLine, " \t")
		if !strings.HasPrefix(trimmedLeft, "#") {
			continue
		}
		hashCount := 0
		for hashCount < len(trimmedLeft) && trimmedLeft[hashCount] == '#' {
			hashCount++
		}
		if hashCount == 0 || hashCount > 6 || len(trimmedLeft) <= hashCount {
			continue
		}
		if trimmedLeft[hashCount] != ' ' && trimmedLeft[hashCount] != '\t' {
			return true
		}
	}
	return false
}

func containsSingleIndentMarkdownBullet(content string) bool {
	for _, rawLine := range strings.Split(content, "\n") {
		if strings.HasPrefix(rawLine, " - ") || strings.HasPrefix(rawLine, " * ") || strings.HasPrefix(rawLine, " + ") {
			return true
		}
	}
	return false
}

func containsInternalPromptLeakage(content string) bool {
	inCodeFence := false
	for _, rawLine := range strings.Split(content, "\n") {
		trimmedLine := strings.TrimSpace(strings.TrimLeft(rawLine, "\ufeff"))
		if strings.HasPrefix(trimmedLine, "```") {
			inCodeFence = !inCodeFence
			continue
		}
		if inCodeFence || trimmedLine == "" {
			continue
		}
		if markdownQualityInternalPromptLeakTagRE.MatchString(trimmedLine) ||
			markdownQualityInternalPromptLeakLineRE.MatchString(trimmedLine) ||
			markdownQualityInternalPromptLeakMetaRE.MatchString(trimmedLine) {
			return true
		}
	}
	return false
}

func markdownQualityBodyTooShort(content string) bool {
	if strings.TrimSpace(content) == "" {
		return true
	}
	var body strings.Builder
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || line == types.ChatDocumentCompletionMarker {
			continue
		}
		if markdownQualityHeadingLineRE.MatchString(line) {
			continue
		}
		line = strings.TrimLeft(line, "-+* ")
		body.WriteString(line)
	}
	return utf8.RuneCountInString(strings.TrimSpace(body.String())) < markdownQualityMinimumBodyRunes
}
