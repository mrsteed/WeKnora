package service

import (
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

type chatDocumentRevisionMergeResult struct {
	Snapshot      string
	QualityIssues []string
}

type chatDocumentPatchOperation struct {
	Action  string
	Heading string
	Content string
}

func mergeChatDocumentRevisionDelta(base string, delta string) chatDocumentRevisionMergeResult {
	baseTrimmed := strings.TrimSpace(base)
	deltaTrimmed := strings.TrimSpace(delta)
	if baseTrimmed == "" {
		return chatDocumentRevisionMergeResult{Snapshot: deltaTrimmed}
	}
	if deltaTrimmed == "" {
		return chatDocumentRevisionMergeResult{Snapshot: baseTrimmed}
	}

	patchPayload, patchExtractionIssues, patchExtracted := extractEmbeddedChatDocumentPatch(deltaTrimmed)
	if patchExtracted {
		deltaTrimmed = patchPayload
	}

	patchOps, patchDetected, patchValid := parseChatDocumentPatch(deltaTrimmed)
	if patchDetected {
		result := applyChatDocumentStructuredPatch(baseTrimmed, deltaTrimmed, patchOps, patchValid)
		result.QualityIssues = uniqueStrings(append(result.QualityIssues, patchExtractionIssues...))
		return result
	}

	deltaHeading, deltaLevel, ok := firstMarkdownHeading(deltaTrimmed)
	if !ok {
		return chatDocumentRevisionMergeResult{
			Snapshot:      appendChatDocumentFragment(baseTrimmed, deltaTrimmed),
			QualityIssues: []string{types.ChatDocumentQualityIssueDeltaMergeUncertain},
		}
	}

	start, end, found := findMarkdownSectionRange(baseTrimmed, deltaHeading, deltaLevel)
	if !found {
		if looksLikeFullDocumentRevision(deltaTrimmed, deltaLevel) {
			return chatDocumentRevisionMergeResult{Snapshot: deltaTrimmed}
		}
		return chatDocumentRevisionMergeResult{
			Snapshot:      appendChatDocumentFragment(baseTrimmed, deltaTrimmed),
			QualityIssues: []string{types.ChatDocumentQualityIssueDeltaMergeUncertain},
		}
	}

	return chatDocumentRevisionMergeResult{Snapshot: replaceChatDocumentRange(baseTrimmed, start, end, deltaTrimmed)}
}

func findMarkdownSectionRange(content string, heading string, level int) (int, int, bool) {
	matches := chatDocumentHeadingRE.FindAllStringSubmatchIndex(content, -1)
	for idx, match := range matches {
		if len(match) < 6 {
			continue
		}
		currentLevel := len(content[match[2]:match[3]])
		currentHeading := strings.TrimSpace(content[match[4]:match[5]])
		if currentLevel != level || currentHeading != heading {
			continue
		}
		end := len(content)
		for nextIdx := idx + 1; nextIdx < len(matches); nextIdx++ {
			next := matches[nextIdx]
			if len(next) < 6 {
				continue
			}
			nextLevel := len(content[next[2]:next[3]])
			if nextLevel <= currentLevel {
				end = next[0]
				break
			}
		}
		return match[0], end, true
	}
	return 0, 0, false
}

func parseChatDocumentPatch(content string) ([]chatDocumentPatchOperation, bool, bool) {
	matches := chatDocumentPatchEnvelopeRE.FindStringSubmatch(content)
	if len(matches) != 2 {
		return nil, false, false
	}
	body := matches[1]
	if strings.TrimSpace(body) == "" {
		return nil, true, true
	}

	opMatches := chatDocumentPatchOperationRE.FindAllStringSubmatchIndex(body, -1)
	if len(opMatches) == 0 {
		if strings.TrimSpace(body) == "" {
			return nil, true, true
		}
		return nil, true, false
	}

	operations := make([]chatDocumentPatchOperation, 0, len(opMatches))
	valid := true
	lastEnd := 0
	for _, match := range opMatches {
		if len(match) < 12 {
			valid = false
			continue
		}
		if strings.TrimSpace(body[lastEnd:match[0]]) != "" {
			valid = false
		}
		if strings.TrimSpace(body[match[2]:match[3]]) != strings.TrimSpace(body[match[10]:match[11]]) {
			valid = false
		}
		heading := ""
		if match[4] >= 0 && match[5] >= 0 {
			heading = body[match[4]:match[5]]
		} else if match[6] >= 0 && match[7] >= 0 {
			heading = body[match[6]:match[7]]
		}
		operations = append(operations, chatDocumentPatchOperation{
			Action:  strings.TrimSpace(body[match[2]:match[3]]),
			Heading: strings.TrimSpace(heading),
			Content: strings.TrimSpace(body[match[8]:match[9]]),
		})
		lastEnd = match[1]
	}
	if strings.TrimSpace(body[lastEnd:]) != "" {
		valid = false
	}
	return operations, true, valid
}

func extractEmbeddedChatDocumentPatch(content string) (string, []string, bool) {
	trimmed := strings.TrimSpace(content)
	start := strings.Index(trimmed, "<document_patch>")
	if start < 0 {
		return trimmed, nil, false
	}
	end := strings.LastIndex(trimmed, "</document_patch>")
	if end < 0 || end < start {
		return trimmed, nil, false
	}
	end += len("</document_patch>")
	prefix := strings.TrimSpace(trimmed[:start])
	suffix := strings.TrimSpace(trimmed[end:])
	if suffix != "" {
		return trimmed, nil, false
	}
	if prefix == "" {
		return strings.TrimSpace(trimmed[start:end]), nil, true
	}
	if runeLen(prefix) <= 200 && chatDocumentRevisionLeadRE.MatchString(prefix) {
		return strings.TrimSpace(trimmed[start:end]), []string{types.ChatDocumentQualityIssueRevisionPreambleTrimmed}, true
	}
	return trimmed, nil, false
}

func applyChatDocumentStructuredPatch(base string, rawDelta string, operations []chatDocumentPatchOperation, valid bool) chatDocumentRevisionMergeResult {
	issues := make([]string, 0, 1)
	current := strings.TrimSpace(base)
	if !valid {
		issues = append(issues, types.ChatDocumentQualityIssueDeltaMergeUncertain)
	}
	if len(operations) == 0 {
		return chatDocumentRevisionMergeResult{
			Snapshot:      appendChatDocumentFragment(current, extractChatDocumentPatchFallbackContent(rawDelta)),
			QualityIssues: uniqueStrings(issues),
		}
	}

	for _, operation := range operations {
		if strings.TrimSpace(operation.Content) == "" {
			issues = append(issues, types.ChatDocumentQualityIssueDeltaMergeUncertain)
			continue
		}
		start, end, matchedHeading, found, ambiguous := findMarkdownSectionRangeBySelector(current, operation.Heading)
		switch operation.Action {
		case "replace":
			if !found {
				current = appendChatDocumentFragment(current, operation.Content)
				issues = append(issues, types.ChatDocumentQualityIssueDeltaMergeUncertain)
				if ambiguous {
					issues = append(issues, types.ChatDocumentQualityIssueTargetSectionUncertain)
				}
				continue
			}
			replacement := ensurePatchReplaceHeading(operation.Content, operation.Heading, matchedHeading)
			current = replaceChatDocumentRange(current, start, end, replacement)
		case "append":
			if !found {
				current = appendChatDocumentFragment(current, operation.Content)
				issues = append(issues, types.ChatDocumentQualityIssueDeltaMergeUncertain)
				if ambiguous {
					issues = append(issues, types.ChatDocumentQualityIssueTargetSectionUncertain)
				}
				continue
			}
			current = replaceChatDocumentRange(current, start, end, appendChatDocumentFragment(current[start:end], operation.Content))
		case "insert_after":
			if !found {
				current = appendChatDocumentFragment(current, operation.Content)
				issues = append(issues, types.ChatDocumentQualityIssueDeltaMergeUncertain)
				if ambiguous {
					issues = append(issues, types.ChatDocumentQualityIssueTargetSectionUncertain)
				}
				continue
			}
			current = joinChatDocumentSegments(current[:end], operation.Content, current[end:])
		default:
			current = appendChatDocumentFragment(current, operation.Content)
			issues = append(issues, types.ChatDocumentQualityIssueDeltaMergeUncertain)
		}
	}

	return chatDocumentRevisionMergeResult{
		Snapshot:      strings.TrimSpace(current),
		QualityIssues: uniqueStrings(issues),
	}
}

func extractChatDocumentPatchFallbackContent(content string) string {
	matches := chatDocumentPatchEnvelopeRE.FindStringSubmatch(content)
	if len(matches) != 2 {
		return strings.TrimSpace(content)
	}
	body := strings.TrimSpace(matches[1])
	if body == "" {
		return ""
	}
	opMatches := chatDocumentPatchOperationRE.FindAllStringSubmatch(body, -1)
	if len(opMatches) == 0 {
		return body
	}
	fragments := make([]string, 0, len(opMatches))
	for _, match := range opMatches {
		if len(match) < 5 {
			continue
		}
		fragments = append(fragments, strings.TrimSpace(match[4]))
	}
	return joinChatDocumentSegments(fragments...)
}

func buildChatDocumentAppendPatch(targetHeading string, content string) string {
	targetHeading = strings.TrimSpace(targetHeading)
	content = strings.TrimSpace(content)
	if targetHeading == "" {
		return content
	}
	return fmt.Sprintf("<document_patch>\n<append heading=%q>\n%s\n</append>\n</document_patch>", targetHeading, content)
}

func parseMarkdownHeadingSelector(selector string) (string, int, bool) {
	matches := chatDocumentHeadingRE.FindStringSubmatch(selector)
	if len(matches) != 3 {
		return "", 0, false
	}
	return strings.TrimSpace(matches[2]), len(matches[1]), true
}

func ensurePatchReplaceHeading(content string, selector string, matchedHeading string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return trimmed
	}
	if _, _, ok := firstMarkdownHeading(trimmed); ok {
		return trimmed
	}
	headingLine := strings.TrimSpace(matchedHeading)
	if headingLine == "" {
		headingLine = strings.TrimSpace(selector)
	}
	if headingLine == "" {
		return trimmed
	}
	return joinChatDocumentSegments(headingLine, trimmed)
}

func replaceChatDocumentRange(content string, start int, end int, replacement string) string {
	return joinChatDocumentSegments(content[:start], replacement, content[end:])
}

func appendChatDocumentFragment(base string, fragment string) string {
	return joinChatDocumentSegments(base, fragment)
}

func joinChatDocumentSegments(parts ...string) string {
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		segments = append(segments, trimmed)
	}
	return strings.Join(segments, "\n\n")
}
