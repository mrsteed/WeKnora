package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

var (
	chatDocumentContinueIntentRE    = regexp.MustCompile(`(?i)(继续生成|接着写|续写|从上次中断处继续|补全剩余|继续输出|继续补齐|继续补充|接着补齐|接着补充|补齐剩余|补充剩余|继续完善|继续扩写)`)
	chatDocumentReviseIntentRE      = regexp.MustCompile(`(?i)(修改上一版|基于上一个文档修改|把上一份改成|调整上一版|完善上一版)`)
	chatDocumentRegenerateIntentRE  = regexp.MustCompile(`(?i)(重新生成|从头生成|重写一版|不要基于上一版)`)
	chatDocumentScopedTargetRE      = regexp.MustCompile(`(?i)(章节|小节|段落|标题|模块|部分|第[0-9一二三四五六七八九十百零]+(?:章|节|部分)|[0-9]+(?:\.[0-9]+)+|智慧运行|智慧安防|数据湖|算力平台|应急中心|AR眼镜)`)
	chatDocumentTailContinueRE      = regexp.MustCompile(`(?i)(剩余内容|剩余章节|后续章节|余下章节|从上次中断|文档末尾|继续剩余|当前文档为基准|自动续写)`)
	chatDocumentQuotedTargetRE      = regexp.MustCompile(`["“'‘]([^"”'’\n]{1,40})["”'’](?:章节|小节|模块|部分)?`)
	chatDocumentScopedPhraseRE      = regexp.MustCompile(`(?:在|对|把|将|就)?\s*(?:第[0-9一二三四五六七八九十百零]+(?:章|节|部分)|[0-9]+(?:\.[0-9]+)+|[\p{Han}A-Za-z0-9_-]{2,40})(?:章节|小节|模块|部分)`)
	chatDocumentTargetLeadTrimRE    = regexp.MustCompile(`^(?:(?:请|帮我|麻烦|再)\s*)*(?:(?:继续|接着|续写|补充|扩写|完善|细化|补齐|补全|修改|调整|重写|生成)\s*)+`)
	chatDocumentHeadingMarkerTrimRE = regexp.MustCompile(`^#{1,6}\s*`)
	chatDocumentHeadingNumberTrimRE = regexp.MustCompile(`^(?:(?:[0-9]+(?:\.[0-9]+)*)|(?:第[0-9一二三四五六七八九十百零]+(?:章|节|部分)?)|(?:[一二三四五六七八九十百零]+))[、.．\s-]*`)
	chatDocumentHeadingRE           = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+)$`)
	chatDocumentCodeFenceRE         = regexp.MustCompile("(?m)^```")
	chatDocumentListRE              = regexp.MustCompile(`(?m)^\s*(?:[-*+]\s+|\d+\.\s+)`)
	chatDocumentTableRE             = regexp.MustCompile(`(?m)^\|.+\|\s*$`)
	chatDocumentPatchEnvelopeRE     = regexp.MustCompile(`(?s)^\s*<document_patch>\s*(.*?)\s*</document_patch>\s*$`)
	chatDocumentPatchOperationRE    = regexp.MustCompile(`(?s)<(replace|append|insert_after)\s+heading=(?:"([^"]+)"|'([^']+)')\s*>(.*?)</(replace|append|insert_after)>`)
	chatDocumentQueryHintRE         = regexp.MustCompile(`(?i)(方案|文档|报告|markdown|技术方案|设计方案|plan|report|document)`)
	chatDocumentDuplicatePhraseRE   = regexp.MustCompile(`(?i)^(我已修改|下面是修改建议|已根据你的要求修改)`)
	chatDocumentRevisionLeadRE      = regexp.MustCompile(`(?i)^(我已修改|下面是修改|以下是修改|已根据你的要求修改|根据你的要求|我已经根据|已按要求)`)
	chatDocumentMoveIntentRE        = regexp.MustCompile(`(?i)(合并到|并入|移动到|移到|放到|放入|追加到|补充到|归并到|整合到|纳入)`)
	chatDocumentDestinationLeadRE   = regexp.MustCompile(`(?i)(?:合并到|并入|移动到|移到|放到|放入|追加到|补充到|归并到|整合到|纳入)\s*([^，。；\n]+)`)
	chatDocumentSourceTailRE        = regexp.MustCompile(`(?i)(?:把|将)\s*([^，。；\n]+?)\s*(?:后续的内容|之后的内容|后面的内容|后续内容|之后内容|后面内容|章节内容|小节内容)`)
	chatDocumentSourceMoveRE        = regexp.MustCompile(`(?i)(?:把|将)\s*([^，。；\n]+?)\s*(?:合并到|并入|移动到|移到|放到|放入|追加到|补充到|归并到|整合到|纳入)`)
	chatDocumentResetLeadRE         = regexp.MustCompile(`(?m)^\s*(?:#{1,6}\s*)?(?:一[、.．]|1[、.．]|一、|1\.\s*)`)
	chatDocumentLateSectionRE       = regexp.MustCompile(`(?m)^\s*(?:#{1,6}\s*)?(?:[3-9]\.|[1-9][0-9]+\.|[三四五六七八九十][、.．])`)
	chatDocumentTerminalHeadingRE   = regexp.MustCompile(`(?i)(实施方能力保障|保障措施|结束语|总结|附录|结论|交付保障|实施保障)`)
	chatDocumentCompletionNoticeRE  = regexp.MustCompile(`(?i)^\s*(?:文档|全文|整篇文档|本篇文档)?(?:已完成|已经完成|已全部输出|已完整输出|无需继续|无须继续|没有新增内容|无新增内容)(?:[。.!！\s]*)$`)
)

type chatDocumentEditPlan struct {
	Operation          string
	SourceHeading      string
	DestinationHeading string
	MergeMode          string
}

type chatDocumentArtifactService struct {
	repo            interfaces.ChatDocumentArtifactRepository
	evidenceRefRepo interfaces.ChatDocumentEvidenceRefRepository
}

func NewChatDocumentArtifactService(repo interfaces.ChatDocumentArtifactRepository, evidenceRefRepo interfaces.ChatDocumentEvidenceRefRepository) interfaces.ChatDocumentArtifactService {
	return &chatDocumentArtifactService{repo: repo, evidenceRefRepo: evidenceRefRepo}
}

func (s *chatDocumentArtifactService) DetectIntent(ctx context.Context, sessionID string, query string, hint string) (*types.DocumentIntentResult, error) {
	_ = ctx
	_ = sessionID
	trimmedQuery := strings.TrimSpace(query)
	trimmedHint := strings.TrimSpace(hint)

	intent := types.ChatDocumentIntentNormal
	switch {
	case chatDocumentRegenerateIntentRE.MatchString(trimmedQuery):
		intent = types.ChatDocumentIntentRegenerate
	case chatDocumentReviseIntentRE.MatchString(trimmedQuery):
		intent = types.ChatDocumentIntentRevise
	case shouldTreatContinueAsScopedRevision(trimmedQuery):
		intent = types.ChatDocumentIntentRevise
	case chatDocumentContinueIntentRE.MatchString(trimmedQuery):
		intent = types.ChatDocumentIntentContinue
	case trimmedHint == types.ChatDocumentIntentContinue,
		trimmedHint == types.ChatDocumentIntentRevise,
		trimmedHint == types.ChatDocumentIntentRegenerate:
		intent = trimmedHint
	}

	return &types.DocumentIntentResult{
		Intent:        intent,
		Operation:     chatDocumentOperationForIntent(intent),
		TargetHeading: resolveDocumentTargetHeading(trimmedQuery, ""),
		MergeMode:     normalizeChatDocumentMergeMode("", intent, resolveDocumentTargetHeading(trimmedQuery, "")),
	}, nil
}

func shouldTreatContinueAsScopedRevision(query string) bool {
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return false
	}
	return chatDocumentContinueIntentRE.MatchString(trimmedQuery) &&
		chatDocumentScopedTargetRE.MatchString(trimmedQuery) &&
		!chatDocumentTailContinueRE.MatchString(trimmedQuery)
}

func (s *chatDocumentArtifactService) GetLatestArtifact(ctx context.Context, sessionID string) (*types.ChatDocumentArtifact, error) {
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("tenant context is required")
	}
	artifact, err := s.repo.GetLatestArtifactBySession(ctx, tenantID, sessionID)
	if err != nil {
		return nil, err
	}
	if err := s.attachEvidenceRefs(ctx, artifact); err != nil {
		return nil, err
	}
	return hydrateChatDocumentArtifactDerivedFields(artifact), nil
}

func (s *chatDocumentArtifactService) GetArtifact(ctx context.Context, artifactID string) (*types.ChatDocumentArtifact, error) {
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("tenant context is required")
	}
	artifact, err := s.repo.GetArtifactByID(ctx, tenantID, artifactID)
	if err != nil {
		return nil, err
	}
	if err := s.attachEvidenceRefs(ctx, artifact); err != nil {
		return nil, err
	}
	return hydrateChatDocumentArtifactDerivedFields(artifact), nil
}

func (s *chatDocumentArtifactService) GetArtifactBySourceMessageID(ctx context.Context, sourceMessageID string) (*types.ChatDocumentArtifact, error) {
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("tenant context is required")
	}
	artifact, err := s.repo.GetArtifactBySourceMessageID(ctx, tenantID, sourceMessageID)
	if err != nil {
		return nil, err
	}
	if err := s.attachEvidenceRefs(ctx, artifact); err != nil {
		return nil, err
	}
	return hydrateChatDocumentArtifactDerivedFields(artifact), nil
}

func (s *chatDocumentArtifactService) BuildQuotedContext(ctx context.Context, artifact *types.ChatDocumentArtifact, query string, intent string, outputMode string, targetHeading string, mergeMode string) (string, error) {
	return buildChatDocumentQuotedContext(ctx, artifact, query, intent, outputMode, targetHeading, mergeMode)
}

func buildChatDocumentGoalBlock(query string) string {
	goal := strings.TrimSpace(query)
	if goal == "" {
		return ""
	}
	return fmt.Sprintf(`
原始/本轮用户目标：
<original_user_goal>
%s
</original_user_goal>`, goal)
}

func buildChatDocumentTargetBlock(plan chatDocumentEditPlan) string {
	plan.SourceHeading = strings.TrimSpace(plan.SourceHeading)
	plan.DestinationHeading = strings.TrimSpace(plan.DestinationHeading)
	plan.MergeMode = strings.TrimSpace(plan.MergeMode)
	plan.Operation = strings.TrimSpace(plan.Operation)
	if plan.SourceHeading == "" && plan.DestinationHeading == "" && plan.MergeMode == "" && plan.Operation == "" {
		return ""
	}
	parts := make([]string, 0, 4)
	if plan.SourceHeading != "" {
		parts = append(parts, fmt.Sprintf("- source_heading: %s", plan.SourceHeading))
	}
	if plan.DestinationHeading != "" {
		parts = append(parts, fmt.Sprintf("- target_heading: %s", plan.DestinationHeading))
	}
	if plan.MergeMode != "" {
		parts = append(parts, fmt.Sprintf("- merge_mode: %s", plan.MergeMode))
	}
	if plan.Operation != "" {
		parts = append(parts, fmt.Sprintf("- operation: %s", plan.Operation))
	}
	return "\n\n结构化编辑目标：\n<document_edit_target>\n" + strings.Join(parts, "\n") + "\n</document_edit_target>"
}

func resolveDocumentTargetHeading(query string, targetHeading string) string {
	if trimmed := strings.TrimSpace(targetHeading); trimmed != "" {
		return trimmed
	}
	if destination := inferDocumentDestinationHeading(query); destination != "" {
		return destination
	}
	return inferSectionTargetFromQuery(query)
}

func normalizeChatDocumentMergeMode(mergeMode string, intent string, targetHeading string) string {
	trimmedMergeMode := strings.TrimSpace(mergeMode)
	if trimmedMergeMode != "" {
		return trimmedMergeMode
	}
	if strings.TrimSpace(targetHeading) == "" {
		return ""
	}
	switch intent {
	case types.ChatDocumentIntentContinue, types.ChatDocumentIntentRevise:
		return types.ChatDocumentMergeModeAppendToSection
	default:
		return ""
	}
}

func buildTargetedDocumentPayload(content string, targetHeading string) (string, string, bool) {
	start, end, matchedHeading, found, ambiguous := findMarkdownSectionRangeBySelector(content, targetHeading)
	if !found || ambiguous {
		return "", "", false
	}
	outline := strings.TrimSpace(extractMarkdownHeadingOutline(content))
	targetSection := strings.TrimSpace(content[start:end])
	targetParent, previousSibling, nextSibling := extractTargetSectionWindow(content, start)
	sections := make([]string, 0, 5)
	if outline != "" {
		sections = append(sections, "<document_outline>\n"+outline+"\n</document_outline>")
	}
	if targetParent != "" {
		sections = append(sections, "<target_parent>\n"+targetParent+"\n</target_parent>")
	}
	sections = append(sections,
		"<target_section_heading>\n"+matchedHeading+"\n</target_section_heading>",
		"<target_section>\n"+targetSection+"\n</target_section>",
	)
	nearby := make([]string, 0, 2)
	if previousSibling != "" {
		nearby = append(nearby, "<previous_sibling>\n"+previousSibling+"\n</previous_sibling>")
	}
	if nextSibling != "" {
		nearby = append(nearby, "<next_sibling>\n"+nextSibling+"\n</next_sibling>")
	}
	if len(nearby) > 0 {
		sections = append(sections, "<nearby_siblings>\n"+strings.Join(nearby, "\n\n")+"\n</nearby_siblings>")
	}
	return strings.Join(sections, "\n\n"), matchedHeading, true
}

func inferChatDocumentEditPlan(query string, targetHeading string, mergeMode string) chatDocumentEditPlan {
	plan := chatDocumentEditPlan{
		DestinationHeading: strings.TrimSpace(targetHeading),
		MergeMode:          strings.TrimSpace(mergeMode),
	}
	if plan.DestinationHeading == "" {
		plan.DestinationHeading = inferDocumentDestinationHeading(query)
	}
	if plan.DestinationHeading == "" {
		plan.DestinationHeading = inferSectionTargetFromQuery(query)
	}
	if plan.MergeMode == "" {
		plan.MergeMode = normalizeChatDocumentMergeMode("", types.ChatDocumentIntentRevise, plan.DestinationHeading)
	}
	if !chatDocumentMoveIntentRE.MatchString(query) {
		return plan
	}
	plan.SourceHeading = inferDocumentSourceHeading(query, plan.DestinationHeading)
	if plan.SourceHeading != "" && plan.DestinationHeading != "" && !sameDocumentHeading(plan.SourceHeading, plan.DestinationHeading) {
		plan.Operation = "move_after_heading_to_section"
	}
	return plan
}

func inferDocumentDestinationHeading(query string) string {
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return ""
	}
	if matches := chatDocumentDestinationLeadRE.FindStringSubmatch(trimmedQuery); len(matches) == 2 {
		return cleanDocumentHeadingSelector(matches[1])
	}
	return ""
}

func inferDocumentSourceHeading(query string, destinationHeading string) string {
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return ""
	}
	for _, re := range []*regexp.Regexp{chatDocumentSourceTailRE, chatDocumentSourceMoveRE} {
		if matches := re.FindStringSubmatch(trimmedQuery); len(matches) == 2 {
			candidate := cleanDocumentHeadingSelector(matches[1])
			if candidate != "" && !sameDocumentHeading(candidate, destinationHeading) {
				return candidate
			}
		}
	}
	return ""
}

func cleanDocumentHeadingSelector(selector string) string {
	trimmed := strings.TrimSpace(selector)
	trimmed = strings.Trim(trimmed, "\"'“”‘’")
	trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "里面"))
	trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "里"))
	trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "中"))
	trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "内"))
	trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "正文"))
	trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "这一节"))
	trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "该节"))
	trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "该章节"))
	trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "这个章节"))
	trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "部分"))
	trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "模块"))
	trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "小节"))
	trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "章节"))
	trimmed = strings.TrimSpace(strings.Trim(trimmed, "，。；:："))
	return trimmed
}

func sameDocumentHeading(left string, right string) bool {
	leftNorm := normalizeHeadingForMatch(left)
	rightNorm := normalizeHeadingForMatch(right)
	return leftNorm != "" && leftNorm == rightNorm
}

func buildDualAnchorDocumentPayload(content string, plan chatDocumentEditPlan) (string, string, bool) {
	if strings.TrimSpace(plan.SourceHeading) == "" || strings.TrimSpace(plan.DestinationHeading) == "" {
		return "", "", false
	}
	sourceStart, sourceEnd, resolvedSourceHeading, sourceFound, sourceAmbiguous := findMarkdownSectionRangeBySelector(content, plan.SourceHeading)
	if !sourceFound || sourceAmbiguous {
		return "", "", false
	}
	destinationStart, destinationEnd, resolvedDestinationHeading, destinationFound, destinationAmbiguous := findMarkdownSectionRangeBySelector(content, plan.DestinationHeading)
	if !destinationFound || destinationAmbiguous {
		return "", "", false
	}
	outline := strings.TrimSpace(extractMarkdownHeadingOutline(content))
	sourceSection := strings.TrimSpace(content[sourceStart:sourceEnd])
	destinationSection := strings.TrimSpace(content[destinationStart:destinationEnd])
	sourceParent, sourcePrev, sourceNext := extractTargetSectionWindow(content, sourceStart)
	destinationParent, destinationPrev, destinationNext := extractTargetSectionWindow(content, destinationStart)
	sections := make([]string, 0, 8)
	if outline != "" {
		sections = append(sections, "<document_outline>\n"+outline+"\n</document_outline>")
	}
	sections = append(sections,
		"<source_anchor_heading>\n"+resolvedSourceHeading+"\n</source_anchor_heading>",
		"<source_section>\n"+sourceSection+"\n</source_section>",
		"<destination_section_heading>\n"+resolvedDestinationHeading+"\n</destination_section_heading>",
		"<destination_section>\n"+destinationSection+"\n</destination_section>",
	)
	if sourceParent != "" {
		sections = append(sections, "<source_parent>\n"+sourceParent+"\n</source_parent>")
	}
	if destinationParent != "" && !sameDocumentHeading(sourceParent, destinationParent) {
		sections = append(sections, "<destination_parent>\n"+destinationParent+"\n</destination_parent>")
	}
	neighborParts := make([]string, 0, 4)
	if sourcePrev != "" {
		neighborParts = append(neighborParts, "<source_previous_sibling>\n"+sourcePrev+"\n</source_previous_sibling>")
	}
	if sourceNext != "" {
		neighborParts = append(neighborParts, "<source_next_sibling>\n"+sourceNext+"\n</source_next_sibling>")
	}
	if destinationPrev != "" {
		neighborParts = append(neighborParts, "<destination_previous_sibling>\n"+destinationPrev+"\n</destination_previous_sibling>")
	}
	if destinationNext != "" {
		neighborParts = append(neighborParts, "<destination_next_sibling>\n"+destinationNext+"\n</destination_next_sibling>")
	}
	if len(neighborParts) > 0 {
		sections = append(sections, "<nearby_siblings>\n"+strings.Join(neighborParts, "\n\n")+"\n</nearby_siblings>")
	}
	return strings.Join(sections, "\n\n"), resolvedDestinationHeading, true
}

func extractTargetSectionWindow(content string, targetStart int) (string, string, string) {
	matches := chatDocumentHeadingRE.FindAllStringSubmatchIndex(content, -1)
	targetIndex := -1
	for idx, match := range matches {
		if len(match) < 6 {
			continue
		}
		if match[0] == targetStart {
			targetIndex = idx
			break
		}
	}
	if targetIndex < 0 {
		return "", "", ""
	}
	targetLevel := len(content[matches[targetIndex][2]:matches[targetIndex][3]])
	parent := ""
	for idx := targetIndex - 1; idx >= 0; idx-- {
		if len(matches[idx]) < 6 {
			continue
		}
		level := len(content[matches[idx][2]:matches[idx][3]])
		if level < targetLevel {
			parent = strings.TrimSpace(content[matches[idx][0]:matches[idx][1]])
			break
		}
	}
	previousSibling := ""
	for idx := targetIndex - 1; idx >= 0; idx-- {
		if len(matches[idx]) < 6 {
			continue
		}
		level := len(content[matches[idx][2]:matches[idx][3]])
		if level == targetLevel {
			start, end := markdownSectionRangeFromMatches(content, matches, idx)
			previousSibling = truncateRunes(strings.TrimSpace(content[start:end]), 1200)
			break
		}
		if level < targetLevel {
			break
		}
	}
	nextSibling := ""
	for idx := targetIndex + 1; idx < len(matches); idx++ {
		if len(matches[idx]) < 6 {
			continue
		}
		level := len(content[matches[idx][2]:matches[idx][3]])
		if level == targetLevel {
			start, end := markdownSectionRangeFromMatches(content, matches, idx)
			nextSibling = truncateRunes(strings.TrimSpace(content[start:end]), 1200)
			break
		}
		if level < targetLevel {
			break
		}
	}
	return parent, previousSibling, nextSibling
}

func markdownSectionRangeFromMatches(content string, matches [][]int, idx int) (int, int) {
	start := matches[idx][0]
	currentLevel := len(content[matches[idx][2]:matches[idx][3]])
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
	return start, end
}

func (s *chatDocumentArtifactService) RegisterFromAssistantMessage(ctx context.Context, message *types.Message, options types.RegisterChatDocumentArtifactOptions) (*types.ChatDocumentArtifact, error) {
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("tenant context is required")
	}
	if message == nil || message.Role != "assistant" {
		return nil, nil
	}

	completionStatus := message.CompletionStatusOrLegacy()
	if completionStatus != types.MessageCompletionStatusCompleted && completionStatus != types.MessageCompletionStatusPartial {
		return nil, nil
	}

	if existing, err := s.repo.GetArtifactBySourceMessageID(ctx, tenantID, message.ID); err != nil {
		return nil, err
	} else if existing != nil {
		if err := s.attachEvidenceRefs(ctx, existing); err != nil {
			return nil, err
		}
		return existing, nil
	}

	content := strings.TrimSpace(message.Content)
	documentGenerationStatus := types.NormalizeOptionalChatDocumentGenerationStatus(options.DocumentGenerationStatus)
	if cleaned, completed := types.StripChatDocumentCompletionMarker(content); completed {
		content = cleaned
		message.Content = cleaned
		documentGenerationStatus = types.ChatDocumentGenerationStatusCompleted
	}
	generationDecision := inferChatDocumentGenerationDecision(options.BaseArtifact, content, documentGenerationStatus, options)
	documentGenerationStatus = generationDecision.Status
	if documentGenerationStatus == "" {
		documentGenerationStatus = defaultChatDocumentGenerationStatus(options)
	}
	options.DocumentGenerationStatus = documentGenerationStatus
	if !shouldRegisterChatDocumentArtifact(content, options) {
		return nil, nil
	}

	snapshotResult := buildChatDocumentSnapshotResult(content, options)
	snapshot := snapshotResult.Snapshot
	preparation := prepareChatDocumentArtifactSnapshot(snapshot, options)
	if !preparation.ShouldCreate {
		return nil, nil
	}
	snapshot = preparation.Snapshot
	artifactMarkdownQuality := evaluateArtifactMarkdownQuality(snapshot, completionStatus, documentGenerationStatus, options)
	if artifactMarkdownQuality.DocumentGenerationStatus != "" {
		documentGenerationStatus = artifactMarkdownQuality.DocumentGenerationStatus
		options.DocumentGenerationStatus = documentGenerationStatus
	}
	quality := evaluateRevisionArtifactQuality(snapshot, options)
	if !quality.ShouldCreate {
		return nil, nil
	}
	quality.QualityIssues = uniqueStrings(append(quality.QualityIssues, options.QualityIssues...))
	quality.QualityIssues = uniqueStrings(append(quality.QualityIssues, generationDecision.QualityIssues...))
	quality.QualityIssues = uniqueStrings(append(quality.QualityIssues, snapshotResult.QualityIssues...))
	quality.QualityIssues = uniqueStrings(append(quality.QualityIssues, preparation.QualityIssues...))
	quality.QualityIssues = uniqueStrings(append(quality.QualityIssues, artifactMarkdownQuality.QualityIssues...))

	userID, _ := types.UserIDFromContext(ctx)
	artifactStatus := artifactStatusFromCompletionStatus(completionStatus)
	if chatDocumentContainsString(quality.QualityIssues, types.ChatDocumentQualityIssueSnapshotTruncated) ||
		chatDocumentContainsString(quality.QualityIssues, types.ChatDocumentQualityIssueInlineContextTooLarge) ||
		chatDocumentContainsString(quality.QualityIssues, types.ChatDocumentQualityIssueDeltaMergeUncertain) {
		artifactStatus = types.ChatDocumentArtifactStatusPartial
	}
	if quality.Status != "" {
		artifactStatus = quality.Status
	}
	if artifactMarkdownQuality.Status != "" {
		artifactStatus = artifactMarkdownQuality.Status
	}
	if documentGenerationStatus == types.ChatDocumentGenerationStatusNeedsReview || documentGenerationStatus == types.ChatDocumentGenerationStatusBlocked {
		artifactStatus = types.ChatDocumentArtifactStatusPartial
	}
	artifact := &types.ChatDocumentArtifact{
		TenantID:                 tenantID,
		SessionID:                message.SessionID,
		SourceMessageID:          message.ID,
		SourceRequestID:          message.RequestID,
		ParentArtifactID:         "",
		RevisionNo:               1,
		Title:                    resolveChatDocumentArtifactTitle(snapshot, options),
		ArtifactKind:             detectChatDocumentArtifactKind(snapshot),
		ContentType:              contentTypeForChatDocument(snapshot),
		ContentSnapshot:          snapshot,
		ContentChecksum:          checksumText(snapshot),
		Status:                   artifactStatus,
		CompletionStatus:         completionStatus,
		DocumentGenerationStatus: documentGenerationStatus,
		DocumentTaskKind:         normalizeOptionalDocumentTaskKind(options.DocumentTaskKind),
		SourceTitle:              strings.TrimSpace(options.SourceTitle),
		TargetLanguage:           strings.TrimSpace(options.TargetLanguage),
		OutputFormat:             normalizeOptionalTranslationOutputFormat(options.TranslationOutputFormat),
		Operation:                normalizeChatDocumentOperation(options.Operation),
		CreatedBy:                userID,
	}

	if base := options.BaseArtifact; base != nil && artifact.Operation != types.ChatDocumentOperationCreate && artifact.Operation != types.ChatDocumentOperationRegenerate {
		artifact.ParentArtifactID = base.ID
		artifact.RevisionNo = base.RevisionNo + 1
	}
	hydrateChatDocumentArtifactDerivedFields(artifact, quality.QualityIssues...)

	if err := s.repo.CreateArtifact(ctx, artifact); err != nil {
		return nil, err
	}
	if err := s.persistArtifactEvidenceRefs(ctx, artifact, message, options); err != nil {
		return nil, err
	}
	return artifact, nil
}

type chatDocumentGenerationDecision struct {
	Status        string
	QualityIssues []string
}

func inferChatDocumentGenerationDecision(base *types.ChatDocumentArtifact, delta string, explicitStatus string, options types.RegisterChatDocumentArtifactOptions) chatDocumentGenerationDecision {
	status := types.NormalizeOptionalChatDocumentGenerationStatus(explicitStatus)
	if status == types.ChatDocumentGenerationStatusCompleted {
		return chatDocumentGenerationDecision{Status: status}
	}
	if normalizeChatDocumentOperation(options.Operation) != types.ChatDocumentOperationContinue || base == nil {
		return chatDocumentGenerationDecision{Status: status}
	}
	baseSnapshot := strings.TrimSpace(base.ContentSnapshot)
	delta = strings.TrimSpace(delta)
	if baseSnapshot == "" || delta == "" {
		return chatDocumentGenerationDecision{Status: status}
	}
	if chatDocumentIsCompletionNotice(delta) {
		return chatDocumentGenerationDecision{
			Status:        types.ChatDocumentGenerationStatusCompleted,
			QualityIssues: []string{types.ChatDocumentQualityIssueTerminalSectionTail},
		}
	}
	issues := make([]string, 0, 2)
	if chatDocumentContinuationRepeatsHead(baseSnapshot, delta) {
		issues = append(issues, types.ChatDocumentQualityIssueDuplicateDocumentHead)
	}
	if chatDocumentContinuationSectionResets(baseSnapshot, delta) {
		issues = append(issues, types.ChatDocumentQualityIssueSectionNumberReset)
	}
	if chatDocumentContinuationLowNovelty(baseSnapshot, delta) {
		issues = append(issues, types.ChatDocumentQualityIssueLowNoveltyDelta)
	}
	if len(issues) == 0 && chatDocumentContinuationTerminalTail(baseSnapshot, delta) {
		return chatDocumentGenerationDecision{
			Status:        types.ChatDocumentGenerationStatusCompleted,
			QualityIssues: []string{types.ChatDocumentQualityIssueTerminalSectionTail},
		}
	}
	if len(issues) > 0 {
		return chatDocumentGenerationDecision{
			Status:        types.ChatDocumentGenerationStatusNeedsReview,
			QualityIssues: uniqueStrings(issues),
		}
	}
	return chatDocumentGenerationDecision{Status: status}
}

func chatDocumentContinuationRepeatsHead(base string, delta string) bool {
	baseHeadings := chatDocumentHeadingTitles(base, 4)
	if len(baseHeadings) == 0 {
		return false
	}
	deltaHeadings := chatDocumentHeadingTitles(delta, 2)
	if len(deltaHeadings) == 0 {
		return false
	}
	firstDeltaHeading := normalizeChatDocumentHeadingTitle(deltaHeadings[0])
	if firstDeltaHeading == "" {
		return false
	}
	for _, heading := range baseHeadings {
		if firstDeltaHeading == normalizeChatDocumentHeadingTitle(heading) {
			return true
		}
	}
	return false
}

func chatDocumentIsCompletionNotice(content string) bool {
	return chatDocumentCompletionNoticeRE.MatchString(strings.TrimSpace(content))
}

func chatDocumentContinuationSectionResets(base string, delta string) bool {
	if !chatDocumentLateSectionRE.MatchString(base) || !chatDocumentResetLeadRE.MatchString(delta) {
		return false
	}
	return true
}

func chatDocumentContinuationLowNovelty(base string, delta string) bool {
	if runeLen(delta) < 120 {
		return false
	}
	baseTail := chatDocumentTailRunes(base, 12000)
	deltaHead := truncateRunes(delta, 4000)
	return chatDocumentTokenContainment(baseTail, deltaHead) >= 0.82
}

func chatDocumentContinuationTerminalTail(base string, delta string) bool {
	baseTail := chatDocumentTailRunes(base, 6000)
	if !chatDocumentTerminalHeadingRE.MatchString(baseTail) {
		return false
	}
	if strings.TrimSpace(delta) == "" {
		return true
	}
	if chatDocumentHeadingRE.MatchString(delta) {
		return false
	}
	return runeLen(delta) <= 300
}

func chatDocumentHeadingTitles(content string, limit int) []string {
	matches := chatDocumentHeadingRE.FindAllStringSubmatch(content, -1)
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	titles := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) == 3 {
			titles = append(titles, strings.TrimSpace(match[2]))
		}
	}
	return titles
}

func normalizeChatDocumentHeadingTitle(title string) string {
	title = strings.TrimSpace(title)
	title = strings.Trim(title, "# 　\t")
	title = regexp.MustCompile(`\s+`).ReplaceAllString(title, "")
	return strings.ToLower(title)
}

func chatDocumentTailRunes(content string, limit int) string {
	runes := []rune(strings.TrimSpace(content))
	if limit <= 0 || len(runes) <= limit {
		return string(runes)
	}
	return string(runes[len(runes)-limit:])
}

func chatDocumentTokenContainment(base string, delta string) float64 {
	baseTokens := chatDocumentNoveltyTokens(base)
	deltaTokens := chatDocumentNoveltyTokens(delta)
	if len(deltaTokens) == 0 || len(baseTokens) == 0 {
		return 0
	}
	baseSet := make(map[string]struct{}, len(baseTokens))
	for _, token := range baseTokens {
		baseSet[token] = struct{}{}
	}
	contained := 0
	for _, token := range deltaTokens {
		if _, ok := baseSet[token]; ok {
			contained++
		}
	}
	return float64(contained) / float64(len(deltaTokens))
}

func chatDocumentNoveltyTokens(content string) []string {
	content = strings.ToLower(strings.TrimSpace(content))
	if content == "" {
		return nil
	}
	fields := strings.FieldsFunc(content, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && !(r >= '\u4e00' && r <= '\u9fff')
	})
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		runes := []rune(field)
		if len(runes) <= 12 {
			tokens = append(tokens, field)
			continue
		}
		for start := 0; start < len(runes); start += 8 {
			end := chatDocumentMinInt(start+12, len(runes))
			tokens = append(tokens, string(runes[start:end]))
			if end == len(runes) {
				break
			}
		}
	}
	return tokens
}

func (s *chatDocumentArtifactService) ListBySession(ctx context.Context, sessionID string, limit int) ([]*types.ChatDocumentArtifact, error) {
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("tenant context is required")
	}
	artifacts, err := s.repo.ListArtifactsBySession(ctx, tenantID, sessionID, limit)
	if err != nil {
		return nil, err
	}
	if err := s.attachEvidenceRefs(ctx, artifacts...); err != nil {
		return nil, err
	}
	for _, artifact := range artifacts {
		hydrateChatDocumentArtifactDerivedFields(artifact)
		trimChatDocumentArtifactForList(artifact)
	}
	return artifacts, nil
}

func trimChatDocumentArtifactForList(artifact *types.ChatDocumentArtifact) {
	if artifact == nil {
		return
	}
	artifact.ContentSnapshot = ""
}

func (s *chatDocumentArtifactService) persistArtifactEvidenceRefs(ctx context.Context, artifact *types.ChatDocumentArtifact, message *types.Message, options types.RegisterChatDocumentArtifactOptions) error {
	if artifact == nil {
		return nil
	}
	refs := types.NormalizeChatDocumentEvidenceRefs(options.EvidenceRefs)
	if len(refs) == 0 {
		artifact.EvidenceRefs = nil
		artifact.EvidenceSummary = nil
		return nil
	}
	records := make([]*types.ChatDocumentEvidenceRef, 0, len(refs))
	for _, ref := range refs {
		record := ref
		record.TenantID = artifact.TenantID
		record.RunID = strings.TrimSpace(options.GenerationRunID)
		record.ArtifactID = artifact.ID
		if strings.TrimSpace(record.MessageID) == "" && message != nil {
			record.MessageID = message.ID
		}
		records = append(records, &record)
	}
	if s.evidenceRefRepo != nil {
		if err := s.evidenceRefRepo.CreateEvidenceRefs(ctx, records); err != nil {
			return err
		}
	}
	artifact.EvidenceRefs = make([]types.ChatDocumentEvidenceRef, 0, len(records))
	for _, record := range records {
		if record == nil {
			continue
		}
		artifact.EvidenceRefs = append(artifact.EvidenceRefs, *record)
	}
	artifact.EvidenceSummary = buildChatDocumentEvidenceSummary(artifact.EvidenceRefs)
	return nil
}

func (s *chatDocumentArtifactService) attachEvidenceRefs(ctx context.Context, artifacts ...*types.ChatDocumentArtifact) error {
	if len(artifacts) == 0 {
		return nil
	}
	artifactIDs := make([]string, 0, len(artifacts))
	artifactByID := make(map[string]*types.ChatDocumentArtifact, len(artifacts))
	for _, artifact := range artifacts {
		if artifact == nil || strings.TrimSpace(artifact.ID) == "" {
			continue
		}
		artifact.EvidenceRefs = nil
		artifactIDs = append(artifactIDs, artifact.ID)
		artifactByID[artifact.ID] = artifact
	}
	if len(artifactIDs) == 0 || s.evidenceRefRepo == nil {
		return nil
	}
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return fmt.Errorf("tenant context is required")
	}
	refs, err := s.evidenceRefRepo.ListEvidenceRefsByArtifactIDs(ctx, tenantID, artifactIDs)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if ref == nil {
			continue
		}
		artifact := artifactByID[strings.TrimSpace(ref.ArtifactID)]
		if artifact == nil {
			continue
		}
		artifact.EvidenceRefs = append(artifact.EvidenceRefs, *ref)
	}
	return nil
}

func (s *chatDocumentArtifactService) ListRevisions(ctx context.Context, artifactID string) ([]*types.ChatDocumentArtifact, error) {
	artifact, err := s.GetArtifact(ctx, artifactID)
	if err != nil || artifact == nil {
		return nil, err
	}
	artifacts, err := s.ListBySession(ctx, artifact.SessionID, 200)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]*types.ChatDocumentArtifact, len(artifacts))
	for _, item := range artifacts {
		allowed[item.ID] = item
	}
	chainRootID := artifact.ID
	for current := artifact; current != nil && current.ParentArtifactID != ""; current = allowed[current.ParentArtifactID] {
		chainRootID = current.ParentArtifactID
	}

	chain := make([]*types.ChatDocumentArtifact, 0)
	for _, item := range artifacts {
		if belongsToArtifactChain(item, chainRootID, allowed) {
			chain = append(chain, item)
		}
	}
	sort.Slice(chain, func(i, j int) bool {
		if chain[i].RevisionNo == chain[j].RevisionNo {
			return chain[i].CreatedAt.Before(chain[j].CreatedAt)
		}
		return chain[i].RevisionNo < chain[j].RevisionNo
	})
	return chain, nil
}

func belongsToArtifactChain(artifact *types.ChatDocumentArtifact, rootID string, all map[string]*types.ChatDocumentArtifact) bool {
	for current := artifact; current != nil; current = all[current.ParentArtifactID] {
		if current.ID == rootID {
			return true
		}
		if current.ParentArtifactID == "" {
			return false
		}
	}
	return false
}

func resolveChatDocumentArtifactTitle(snapshot string, options types.RegisterChatDocumentArtifactOptions) string {
	if normalizeOptionalDocumentTaskKind(options.DocumentTaskKind) == types.ChatDocumentTaskKindTranslation {
		return buildTranslationArtifactTitle(options.SourceTitle, options.TargetLanguage)
	}
	title := strings.TrimSpace(extractChatDocumentTitle(snapshot))
	if title != "" {
		return title
	}
	return title
}

func buildTranslationArtifactTitle(sourceTitle string, targetLanguage string) string {
	trimmedSourceTitle := strings.TrimSpace(sourceTitle)
	trimmedTargetLanguage := strings.TrimSpace(targetLanguage)
	switch {
	case trimmedSourceTitle != "" && trimmedTargetLanguage != "":
		return fmt.Sprintf("%s（%s译文）", trimmedSourceTitle, trimmedTargetLanguage)
	case trimmedSourceTitle != "":
		return trimmedSourceTitle + "（译文）"
	case trimmedTargetLanguage != "":
		return trimmedTargetLanguage + "译文"
	default:
		return "全文译文"
	}
}

func normalizeOptionalDocumentTaskKind(taskKind string) string {
	switch strings.TrimSpace(taskKind) {
	case types.ChatDocumentTaskKindTranslation:
		return types.ChatDocumentTaskKindTranslation
	case types.ChatDocumentTaskKindWriting:
		return types.ChatDocumentTaskKindWriting
	default:
		return ""
	}
}

func normalizeOptionalTranslationOutputFormat(outputFormat string) string {
	trimmed := strings.TrimSpace(outputFormat)
	if trimmed == "" {
		return ""
	}
	return trimmed
}

func shouldRegisterChatDocumentArtifact(content string, options types.RegisterChatDocumentArtifactOptions) bool {
	trimmed := strings.TrimSpace(content)
	operation := normalizeChatDocumentOperation(options.Operation)
	hasAuthoritativeSignal := hasAuthoritativeChatDocumentArtifactSignal(options)
	if trimmed == "" {
		return options.BaseArtifact != nil &&
			types.NormalizeChatDocumentGenerationStatus(options.DocumentGenerationStatus) == types.ChatDocumentGenerationStatusCompleted &&
			(operation == types.ChatDocumentOperationContinue || operation == types.ChatDocumentOperationRevise)
	}
	if !hasAuthoritativeSignal {
		if runeLen(trimmed) < 2000 {
			return false
		}
		headingCount := len(chatDocumentHeadingRE.FindAllStringSubmatch(trimmed, -1))
		if headingCount >= 2 || (headingCount >= 1 && (chatDocumentListRE.MatchString(trimmed) || chatDocumentTableRE.MatchString(trimmed))) {
			return chatDocumentQueryHintRE.MatchString(options.UserQuery)
		}
		return chatDocumentQueryHintRE.MatchString(options.UserQuery) && runeLen(trimmed) >= 3000
	}
	if options.BaseArtifact != nil && (operation == types.ChatDocumentOperationContinue || operation == types.ChatDocumentOperationRevise) {
		return true
	}
	if options.NeedArtifact {
		if runeLen(trimmed) >= 120 {
			return true
		}
		headingCount := len(chatDocumentHeadingRE.FindAllStringSubmatch(trimmed, -1))
		return headingCount > 0 || chatDocumentListRE.MatchString(trimmed) || chatDocumentTableRE.MatchString(trimmed)
	}
	if operation == types.ChatDocumentOperationRevise && options.BaseArtifact != nil {
		return true
	}
	if runeLen(trimmed) < 2000 {
		return false
	}
	headingCount := len(chatDocumentHeadingRE.FindAllStringSubmatch(trimmed, -1))
	if headingCount >= 2 {
		return true
	}
	if headingCount >= 1 && (chatDocumentListRE.MatchString(trimmed) || chatDocumentTableRE.MatchString(trimmed)) {
		return true
	}
	return chatDocumentQueryHintRE.MatchString(options.UserQuery) && runeLen(trimmed) >= 3000
}

func hasAuthoritativeChatDocumentArtifactSignal(options types.RegisterChatDocumentArtifactOptions) bool {
	if options.NeedArtifact || options.UseLongDocument {
		return true
	}
	if strings.TrimSpace(options.OutputMode) == types.ChatDocumentOutputModeFull {
		return true
	}
	if strings.TrimSpace(options.GenerationRunID) != "" {
		return true
	}
	operation := normalizeChatDocumentOperation(options.Operation)
	return options.BaseArtifact != nil &&
		(operation == types.ChatDocumentOperationContinue || operation == types.ChatDocumentOperationRevise || operation == types.ChatDocumentOperationRegenerate)
}

func defaultChatDocumentGenerationStatus(options types.RegisterChatDocumentArtifactOptions) string {
	if options.UseLongDocument || strings.TrimSpace(options.OutputMode) == types.ChatDocumentOutputModeFull || strings.TrimSpace(options.GenerationRunID) != "" {
		return types.ChatDocumentGenerationStatusContinuing
	}
	return ""
}

type chatDocumentSnapshotBuildResult struct {
	Snapshot      string
	QualityIssues []string
}

func buildChatDocumentSnapshot(content string, options types.RegisterChatDocumentArtifactOptions) string {
	return buildChatDocumentSnapshotResult(content, options).Snapshot
}

func buildChatDocumentSnapshotResult(content string, options types.RegisterChatDocumentArtifactOptions) chatDocumentSnapshotBuildResult {
	trimmed := strings.TrimSpace(content)
	base := options.BaseArtifact
	operation := normalizeChatDocumentOperation(options.Operation)
	outputMode := normalizeDocumentOutputMode(options.OutputMode, options.Intent)
	if base == nil || strings.TrimSpace(base.ContentSnapshot) == "" {
		return chatDocumentSnapshotBuildResult{Snapshot: trimmed}
	}
	if (trimmed == "" || chatDocumentIsCompletionNotice(trimmed)) && types.NormalizeChatDocumentGenerationStatus(options.DocumentGenerationStatus) == types.ChatDocumentGenerationStatusCompleted {
		return chatDocumentSnapshotBuildResult{Snapshot: strings.TrimSpace(base.ContentSnapshot)}
	}
	if outputMode == types.ChatDocumentOutputModeFull {
		return chatDocumentSnapshotBuildResult{Snapshot: trimmed}
	}
	targetHeading := resolveDocumentTargetHeading(options.UserQuery, options.TargetHeading)
	mergeMode := normalizeChatDocumentMergeMode(options.MergeMode, options.Intent, targetHeading)
	if operation == types.ChatDocumentOperationContinue {
		if targetHeading != "" && mergeMode == types.ChatDocumentMergeModeAppendToSection {
			merged := mergeChatDocumentRevisionDelta(base.ContentSnapshot, buildChatDocumentAppendPatch(targetHeading, trimmed))
			if chatDocumentContainsString(merged.QualityIssues, types.ChatDocumentQualityIssueDeltaMergeUncertain) {
				merged.QualityIssues = uniqueStrings(append(merged.QualityIssues, types.ChatDocumentQualityIssueTargetSectionUncertain))
			}
			return chatDocumentSnapshotBuildResult{Snapshot: merged.Snapshot, QualityIssues: merged.QualityIssues}
		}
		return chatDocumentSnapshotBuildResult{Snapshot: mergeChatDocumentContinuation(base.ContentSnapshot, trimmed)}
	}
	if operation == types.ChatDocumentOperationRevise {
		merged := mergeChatDocumentRevisionDelta(base.ContentSnapshot, trimmed)
		return chatDocumentSnapshotBuildResult{Snapshot: merged.Snapshot, QualityIssues: merged.QualityIssues}
	}
	return chatDocumentSnapshotBuildResult{Snapshot: trimmed}
}

type revisionArtifactQualityResult struct {
	Status        string
	ShouldCreate  bool
	QualityIssues []string
}

func evaluateRevisionArtifactQuality(snapshot string, options types.RegisterChatDocumentArtifactOptions) revisionArtifactQualityResult {
	if normalizeChatDocumentOperation(options.Operation) != types.ChatDocumentOperationRevise {
		return revisionArtifactQualityResult{ShouldCreate: true}
	}
	trimmedSnapshot := strings.TrimSpace(snapshot)
	if chatDocumentDuplicatePhraseRE.MatchString(trimmedSnapshot) && runeLen(trimmedSnapshot) <= 120 {
		return revisionArtifactQualityResult{ShouldCreate: false}
	}
	base := options.BaseArtifact
	if base == nil {
		return revisionArtifactQualityResult{ShouldCreate: true}
	}
	baseLen := runeLen(base.ContentSnapshot)
	if baseLen == 0 {
		return revisionArtifactQualityResult{ShouldCreate: true}
	}

	status := ""
	issues := make([]string, 0, 2)
	snapshotLen := runeLen(snapshot)
	if snapshotLen < int(float64(baseLen)*0.4) && !strings.Contains(options.UserQuery, "精简") && !strings.Contains(options.UserQuery, "摘要") && !strings.Contains(options.UserQuery, "压缩") {
		status = types.ChatDocumentArtifactStatusPartial
		issues = append(issues, types.ChatDocumentQualityIssueRevisionTooShort)
	}
	if chatDocumentHeadingRE.MatchString(base.ContentSnapshot) && !chatDocumentHeadingRE.MatchString(snapshot) {
		status = types.ChatDocumentArtifactStatusPartial
		issues = append(issues, types.ChatDocumentQualityIssueRevisionMissingHeading)
	}
	return revisionArtifactQualityResult{Status: status, ShouldCreate: true, QualityIssues: issues}
}

type artifactSnapshotPreparationResult struct {
	Snapshot      string
	ShouldCreate  bool
	QualityIssues []string
}

type artifactMarkdownQualityResult struct {
	Status                   string
	DocumentGenerationStatus string
	QualityIssues            []string
}

type markdownHeadingNormalizationResult struct {
	Content string
	Changed bool
}

func normalizeGeneratedMarkdownHeadingLine(line string) (string, bool, bool) {
	leading := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	trimmed := strings.TrimSpace(strings.TrimLeft(line, "\ufeff"))
	if trimmed == "" {
		return line, false, false
	}
	markerCount := 0
	for markerCount < len(trimmed) && markerCount < 6 && trimmed[markerCount] == '#' {
		markerCount++
	}
	if markerCount == 0 || markerCount >= len(trimmed) || trimmed[markerCount] == '#' {
		return line, false, false
	}
	rest := strings.TrimSpace(trimmed[markerCount:])
	if rest == "" {
		return line, false, false
	}
	normalized := leading + strings.Repeat("#", markerCount) + " " + rest
	return normalized, true, normalized != line
}

func normalizeGeneratedMarkdownHeadings(content string) markdownHeadingNormalizationResult {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	if strings.TrimSpace(normalized) == "" {
		return markdownHeadingNormalizationResult{}
	}
	lines := strings.Split(normalized, "\n")
	result := make([]string, 0, len(lines)+8)
	inCodeFence := false
	changed := false
	for index, rawLine := range lines {
		line := strings.TrimLeft(rawLine, "\ufeff")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCodeFence = !inCodeFence
			result = append(result, line)
			continue
		}
		if !inCodeFence {
			if headingLine, ok, lineChanged := normalizeGeneratedMarkdownHeadingLine(line); ok {
				if lineChanged {
					changed = true
				}
				if len(result) > 0 && strings.TrimSpace(result[len(result)-1]) != "" {
					result = append(result, "")
					changed = true
				}
				result = append(result, headingLine)
				nextHeading := false
				nextTrimmed := ""
				if index+1 < len(lines) {
					nextTrimmed = strings.TrimSpace(strings.TrimLeft(lines[index+1], "\ufeff"))
					_, nextHeading, _ = normalizeGeneratedMarkdownHeadingLine(lines[index+1])
				}
				if nextTrimmed != "" && !nextHeading {
					result = append(result, "")
					changed = true
				}
				continue
			}
		}
		result = append(result, line)
	}
	content = strings.TrimSpace(strings.Join(result, "\n"))
	if content != strings.TrimSpace(normalized) {
		changed = true
	}
	return markdownHeadingNormalizationResult{Content: content, Changed: changed}
}

func prepareChatDocumentArtifactSnapshot(snapshot string, options types.RegisterChatDocumentArtifactOptions) artifactSnapshotPreparationResult {
	trimmed := strings.TrimSpace(snapshot)
	trimmed, _ = types.StripChatDocumentCompletionMarker(trimmed)
	issues := make([]string, 0, 3)
	if normalizeChatDocumentOperation(options.Operation) == types.ChatDocumentOperationRevise {
		var trimmedLead bool
		trimmed, trimmedLead = trimRevisionPreamble(trimmed)
		if trimmedLead {
			issues = append(issues, types.ChatDocumentQualityIssueRevisionPreambleTrimmed)
		}
	}
	normalizedMarkdown, normalizationIssues := normalizeGeneratedMarkdown(trimmed)
	if strings.TrimSpace(normalizedMarkdown) != "" {
		trimmed = normalizedMarkdown
	}
	issues = append(issues, normalizationIssues...)
	var closedFence bool
	trimmed, closedFence = closeUnclosedChatDocumentCodeFence(trimmed)
	if closedFence {
		issues = append(issues, types.ChatDocumentQualityIssueUnclosedCodeFence)
	}
	if runeLen(trimmed) > types.ChatDocumentArtifactSnapshotMaxChars {
		trimmed = strings.TrimSpace(truncateRunes(trimmed, types.ChatDocumentArtifactSnapshotMaxChars))
		trimmed, closedFence = closeUnclosedChatDocumentCodeFence(trimmed)
		if closedFence {
			issues = append(issues, types.ChatDocumentQualityIssueUnclosedCodeFence)
		}
		issues = append(issues,
			types.ChatDocumentQualityIssueSnapshotTruncated,
			types.ChatDocumentQualityIssueInlineContextTooLarge,
		)
	}
	return artifactSnapshotPreparationResult{
		Snapshot:      strings.TrimSpace(trimmed),
		ShouldCreate:  strings.TrimSpace(trimmed) != "",
		QualityIssues: uniqueStrings(issues),
	}
}

func evaluateArtifactMarkdownQuality(snapshot string, completionStatus string, documentGenerationStatus string, options types.RegisterChatDocumentArtifactOptions) artifactMarkdownQualityResult {
	if !shouldApplyArtifactMarkdownQualityGate(options, documentGenerationStatus) {
		return artifactMarkdownQualityResult{DocumentGenerationStatus: types.NormalizeOptionalChatDocumentGenerationStatus(documentGenerationStatus)}
	}
	operation := normalizeChatDocumentOperation(options.Operation)
	if options.BaseArtifact != nil &&
		(operation == types.ChatDocumentOperationContinue || operation == types.ChatDocumentOperationRevise) &&
		strings.TrimSpace(snapshot) == strings.TrimSpace(options.BaseArtifact.ContentSnapshot) {
		return artifactMarkdownQualityResult{DocumentGenerationStatus: types.NormalizeOptionalChatDocumentGenerationStatus(documentGenerationStatus)}
	}
	issues := validateGeneratedDocumentMarkdown(strings.TrimSpace(snapshot))
	if len(issues) == 0 {
		return artifactMarkdownQualityResult{DocumentGenerationStatus: types.NormalizeOptionalChatDocumentGenerationStatus(documentGenerationStatus)}
	}
	nextDocumentGenerationStatus := types.NormalizeOptionalChatDocumentGenerationStatus(documentGenerationStatus)
	if completionStatus == types.MessageCompletionStatusCompleted && nextDocumentGenerationStatus != types.ChatDocumentGenerationStatusBlocked {
		nextDocumentGenerationStatus = types.ChatDocumentGenerationStatusNeedsReview
	}
	return artifactMarkdownQualityResult{
		Status:                   types.ChatDocumentArtifactStatusPartial,
		DocumentGenerationStatus: nextDocumentGenerationStatus,
		QualityIssues:            markdownQualityIssueCodes(issues),
	}
}

func shouldApplyArtifactMarkdownQualityGate(options types.RegisterChatDocumentArtifactOptions, documentGenerationStatus string) bool {
	if strings.TrimSpace(options.OutputMode) == types.ChatDocumentOutputModeFull {
		return true
	}
	if options.UseLongDocument {
		return true
	}
	return strings.TrimSpace(documentGenerationStatus) != ""
}

func mergeChatDocumentContinuation(base string, delta string) string {
	baseTrimmed := strings.TrimSpace(base)
	deltaTrimmed := strings.TrimSpace(delta)
	if baseTrimmed == "" {
		return deltaTrimmed
	}
	if deltaTrimmed == "" {
		return baseTrimmed
	}
	overlap := longestChatDocumentOverlap(baseTrimmed, deltaTrimmed, 1024)
	if overlap > 0 {
		return baseTrimmed + deltaTrimmed[overlap:]
	}
	return baseTrimmed + "\n\n" + deltaTrimmed
}

func hydrateChatDocumentArtifactDerivedFields(artifact *types.ChatDocumentArtifact, extraIssues ...string) *types.ChatDocumentArtifact {
	if artifact == nil {
		return nil
	}
	snapshot := strings.TrimSpace(artifact.ContentSnapshot)
	structure := analyzeChatDocumentStructure(snapshot)
	artifact.EvidenceSummary = buildChatDocumentEvidenceSummary(artifact.EvidenceRefs)
	issues := append([]string{}, artifact.QualityIssues...)
	issues = append(issues, extraIssues...)
	if runeLen(snapshot) > types.ChatDocumentArtifactInlineContinuationMaxChars {
		issues = append(issues, types.ChatDocumentQualityIssueInlineContextTooLarge)
	}
	if structure.HasUnclosedCodeFence {
		issues = append(issues, types.ChatDocumentQualityIssueUnclosedCodeFence)
	}
	if artifact.Operation == types.ChatDocumentOperationRevise && artifact.Status == types.ChatDocumentArtifactStatusPartial && structure.HeadingCount == 0 {
		issues = append(issues, types.ChatDocumentQualityIssueRevisionMissingHeading)
	}
	artifact.DocumentGenerationStatus = types.NormalizeOptionalChatDocumentGenerationStatus(artifact.DocumentGenerationStatus)
	artifact.StructureInfo = structure
	artifact.SnapshotCharCount = runeLen(snapshot)
	artifact.QualityIssues = uniqueStrings(issues)
	artifact.QualityIssueDetails = types.ChatDocumentQualityIssueDetails(artifact.QualityIssues)
	artifact.CanContinueDocument = artifact.CanContinue()
	artifact.CanInlineContinue = artifact.CanInlineContinueWithFullSnapshot()
	artifact.CanAutoContinueDocument = artifact.CanAutoContinue()
	artifact.CanManualContinueDocument = artifact.CanManualContinue()
	artifact.CanManualReviseDocument = artifact.CanManualRevise()
	artifact.CanUseAsBaseDocument = artifact.CanUseAsBase()
	artifact.CanViewDocument = artifact.CanView()
	artifact.CanIndexDocument = artifact.CanIndex()
	artifact.ContinuationContextMode = artifact.ContinuationMode()
	artifact.UserHint = chatDocumentUserHintForIssues(artifact.QualityIssues, artifact.Operation)
	return artifact
}

func buildChatDocumentEvidenceSummary(refs []types.ChatDocumentEvidenceRef) *types.ChatDocumentEvidenceSummary {
	if len(refs) == 0 {
		return nil
	}
	type sourceAggregate struct {
		knowledgeBaseID string
		knowledgeID     string
		sourceTitle     string
		chunkIDs        map[string]struct{}
		chunkCount      int
		maxScore        float64
	}

	summary := &types.ChatDocumentEvidenceSummary{}
	knowledgeBaseIDs := make(map[string]struct{}, len(refs))
	knowledgeIDs := make(map[string]struct{}, len(refs))
	chunkIDs := make(map[string]struct{}, len(refs))
	sourceAggregates := make(map[string]*sourceAggregate, len(refs))

	for _, ref := range refs {
		summary.RefCount++
		if ref.KnowledgeBaseID != "" {
			knowledgeBaseIDs[ref.KnowledgeBaseID] = struct{}{}
		}
		if ref.KnowledgeID != "" {
			knowledgeIDs[ref.KnowledgeID] = struct{}{}
		}
		if ref.ChunkID != "" {
			chunkIDs[ref.ChunkID] = struct{}{}
		}

		sourceKey := strings.Join([]string{ref.KnowledgeBaseID, ref.KnowledgeID, ref.SourceTitle}, "|")
		aggregate := sourceAggregates[sourceKey]
		if aggregate == nil {
			aggregate = &sourceAggregate{
				knowledgeBaseID: ref.KnowledgeBaseID,
				knowledgeID:     ref.KnowledgeID,
				sourceTitle:     ref.SourceTitle,
				chunkIDs:        make(map[string]struct{}),
			}
			sourceAggregates[sourceKey] = aggregate
		}
		if ref.ChunkID == "" {
			aggregate.chunkCount++
		} else if _, exists := aggregate.chunkIDs[ref.ChunkID]; !exists {
			aggregate.chunkIDs[ref.ChunkID] = struct{}{}
			aggregate.chunkCount++
		}
		if ref.Score > aggregate.maxScore {
			aggregate.maxScore = ref.Score
		}
	}

	summary.KnowledgeBaseCount = len(knowledgeBaseIDs)
	summary.KnowledgeCount = len(knowledgeIDs)
	summary.ChunkCount = len(chunkIDs)
	if summary.ChunkCount == 0 {
		summary.ChunkCount = summary.RefCount
	}

	summary.Sources = make([]types.ChatDocumentEvidenceSourceSummary, 0, len(sourceAggregates))
	for _, aggregate := range sourceAggregates {
		summary.Sources = append(summary.Sources, types.ChatDocumentEvidenceSourceSummary{
			KnowledgeBaseID: aggregate.knowledgeBaseID,
			KnowledgeID:     aggregate.knowledgeID,
			SourceTitle:     aggregate.sourceTitle,
			ChunkCount:      aggregate.chunkCount,
			MaxScore:        aggregate.maxScore,
		})
	}
	sort.Slice(summary.Sources, func(i, j int) bool {
		left := summary.Sources[i]
		right := summary.Sources[j]
		if left.ChunkCount != right.ChunkCount {
			return left.ChunkCount > right.ChunkCount
		}
		if left.MaxScore != right.MaxScore {
			return left.MaxScore > right.MaxScore
		}
		if left.SourceTitle != right.SourceTitle {
			return left.SourceTitle < right.SourceTitle
		}
		return left.KnowledgeID < right.KnowledgeID
	})
	if len(summary.Sources) > 8 {
		summary.Sources = append([]types.ChatDocumentEvidenceSourceSummary(nil), summary.Sources[:8]...)
	}
	return summary
}

func analyzeChatDocumentStructure(content string) *types.ChatDocumentStructureInfo {
	matches := chatDocumentHeadingRE.FindAllStringSubmatch(content, -1)
	headingTitles := make([]string, 0, chatDocumentMinInt(len(matches), 12))
	for _, match := range matches {
		if len(headingTitles) >= 12 {
			break
		}
		headingTitles = append(headingTitles, strings.TrimSpace(match[2]))
	}
	codeFenceCount := len(chatDocumentCodeFenceRE.FindAllString(content, -1))
	return &types.ChatDocumentStructureInfo{
		HeadingCount:         len(matches),
		HeadingTitles:        headingTitles,
		HasList:              chatDocumentListRE.MatchString(content),
		HasTable:             chatDocumentTableRE.MatchString(content),
		CodeFenceCount:       codeFenceCount,
		HasUnclosedCodeFence: codeFenceCount%2 != 0,
	}
}

func trimRevisionPreamble(snapshot string) (string, bool) {
	firstHeading := chatDocumentHeadingRE.FindStringIndex(snapshot)
	if firstHeading != nil {
		preamble := strings.TrimSpace(snapshot[:firstHeading[0]])
		if preamble != "" && runeLen(preamble) <= 200 && chatDocumentRevisionLeadRE.MatchString(preamble) {
			return strings.TrimSpace(snapshot[firstHeading[0]:]), true
		}
		return snapshot, false
	}
	parts := strings.SplitN(snapshot, "\n\n", 2)
	if len(parts) == 2 {
		firstParagraph := strings.TrimSpace(parts[0])
		if runeLen(firstParagraph) <= 120 && chatDocumentRevisionLeadRE.MatchString(firstParagraph) {
			return strings.TrimSpace(parts[1]), true
		}
	}
	return snapshot, false
}

func closeUnclosedChatDocumentCodeFence(snapshot string) (string, bool) {
	structure := analyzeChatDocumentStructure(snapshot)
	if !structure.HasUnclosedCodeFence {
		return snapshot, false
	}
	return strings.TrimSpace(snapshot) + "\n```", true
}

func chatDocumentUserHintForIssues(issues []string, operation string) string {
	if chatDocumentContainsString(issues, types.ChatDocumentQualityIssueDuplicateDocumentHead) {
		return "检测到本轮续写重新输出了文档开头，系统已暂停自动续写。请检查完整文档后再继续。"
	}
	if chatDocumentContainsString(issues, types.ChatDocumentQualityIssueSectionNumberReset) {
		return "检测到本轮续写出现章节编号回退，系统已暂停自动续写。请检查完整文档后再继续。"
	}
	if chatDocumentContainsString(issues, types.ChatDocumentQualityIssueLowNoveltyDelta) {
		return "检测到本轮续写与已有内容高度重复，系统已暂停自动续写。请检查完整文档后再继续。"
	}
	if chatDocumentContainsString(issues, types.ChatDocumentQualityIssueTerminalSectionTail) {
		return "检测到文档已到收尾章节，系统已停止自动续写。"
	}
	if chatDocumentContainsString(issues, types.ChatDocumentQualityIssueSnapshotTruncated) {
		return "当前版本过长，系统只保留了截断快照。建议先缩小修改范围，再继续生成。"
	}
	if chatDocumentContainsString(issues, types.ChatDocumentQualityIssueInlineContextTooLarge) {
		if operation == types.ChatDocumentOperationRevise {
			return "当前文档较长，系统会使用目录、开头和末尾窗口辅助修改；建议指定章节或段落范围。"
		}
		return "当前文档较长，系统会使用目录和末尾窗口继续自动续写，避免把整篇文档一次性塞入上下文。"
	}
	if chatDocumentContainsString(issues, types.ChatDocumentQualityIssueTargetSectionUncertain) {
		return "目标章节未能唯一定位，系统已按保守策略合并到文档末尾。建议明确章节标题或编号后重试。"
	}
	if operation == types.ChatDocumentOperationRevise && chatDocumentContainsString(issues, types.ChatDocumentQualityIssueDeltaMergeUncertain) {
		return "本次修改有部分片段无法精确定位，系统已按保守策略合并到文档末尾。建议检查完整文档后继续微调。"
	}
	if operation == types.ChatDocumentOperationRevise && chatDocumentContainsString(issues, types.ChatDocumentQualityIssueRevisionTooShort) {
		return "本次修改结果明显短于上一版，建议继续补齐缺失章节后再作为新基线。"
	}
	if operation == types.ChatDocumentOperationRevise && chatDocumentContainsString(issues, types.ChatDocumentQualityIssueRevisionMissingHeading) {
		return "本次修改结果缺少原有标题结构，建议继续补齐章节层级后再作为新基线。"
	}
	return ""
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		unique = append(unique, trimmed)
	}
	if len(unique) == 0 {
		return nil
	}
	return unique
}

func chatDocumentContainsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func longestChatDocumentOverlap(base string, delta string, maxSize int) int {
	maxOverlap := chatDocumentMinInt(chatDocumentMinInt(len(base), len(delta)), maxSize)
	for size := maxOverlap; size > 0; size-- {
		if strings.HasSuffix(base, delta[:size]) {
			return size
		}
	}
	return 0
}

func detectChatDocumentArtifactKind(content string) string {
	if chatDocumentHeadingRE.MatchString(content) || chatDocumentTableRE.MatchString(content) || strings.Contains(content, "```") {
		return types.ChatDocumentArtifactKindMarkdown
	}
	return types.ChatDocumentArtifactKindText
}

func contentTypeForChatDocument(content string) string {
	if detectChatDocumentArtifactKind(content) == types.ChatDocumentArtifactKindMarkdown {
		return "text/markdown"
	}
	return "text/plain"
}

func extractChatDocumentTitle(content string) string {
	if matches := chatDocumentHeadingRE.FindStringSubmatch(content); len(matches) == 3 {
		return truncateRunes(strings.TrimSpace(matches[2]), 255)
	}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if line != "" {
			return truncateRunes(line, 255)
		}
	}
	return "未命名文档"
}

func artifactStatusFromCompletionStatus(completionStatus string) string {
	if completionStatus == types.MessageCompletionStatusPartial {
		return types.ChatDocumentArtifactStatusPartial
	}
	if completionStatus == types.MessageCompletionStatusFailed {
		return types.ChatDocumentArtifactStatusFailed
	}
	return types.ChatDocumentArtifactStatusAvailable
}

func checksumText(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func extractMarkdownHeadingOutline(content string) string {
	matches := chatDocumentHeadingRE.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return ""
	}
	lines := make([]string, 0, len(matches))
	for _, match := range matches {
		lines = append(lines, strings.Repeat("#", len(match[1]))+" "+strings.TrimSpace(match[2]))
	}
	return strings.Join(lines, "\n")
}

func buildTruncatedDocumentPayload(outline string, head string, tail string) string {
	sections := make([]string, 0, 3)
	if outline != "" {
		sections = append(sections, "<document_outline>\n"+outline+"\n</document_outline>")
	}
	if head != "" {
		sections = append(sections, "<document_head>\n"+head+"\n</document_head>")
	}
	if tail != "" {
		sections = append(sections, "<document_tail>\n"+tail+"\n</document_tail>")
	}
	return strings.Join(sections, "\n\n")
}

func firstMarkdownHeading(content string) (string, int, bool) {
	matches := chatDocumentHeadingRE.FindStringSubmatch(content)
	if len(matches) != 3 {
		return "", 0, false
	}
	return strings.TrimSpace(matches[2]), len(matches[1]), true
}

func looksLikeFullDocumentRevision(content string, firstHeadingLevel int) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	if firstHeadingLevel == 1 {
		return true
	}
	return len(chatDocumentHeadingRE.FindAllStringSubmatch(trimmed, -1)) >= 2
}

func findMarkdownSectionRangeBySelector(content string, selector string) (int, int, string, bool, bool) {
	trimmedSelector := strings.TrimSpace(selector)
	if trimmedSelector == "" {
		return 0, 0, "", false, false
	}
	type sectionCandidate struct {
		start          int
		end            int
		matchedHeading string
	}

	selectorHeading := trimmedSelector
	selectorLevel := 0
	if heading, level, ok := parseMarkdownHeadingSelector(trimmedSelector); ok {
		selectorHeading = heading
		selectorLevel = level
	}
	selectorNorm := normalizeHeadingForMatch(selectorHeading)
	bestScore := 0
	candidates := make([]sectionCandidate, 0, 2)

	matches := chatDocumentHeadingRE.FindAllStringSubmatchIndex(content, -1)
	for idx, match := range matches {
		if len(match) < 6 {
			continue
		}
		currentLevel := len(content[match[2]:match[3]])
		currentHeading := strings.TrimSpace(content[match[4]:match[5]])
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

		score := 0
		switch {
		case selectorLevel > 0 && currentLevel == selectorLevel && currentHeading == selectorHeading:
			score = 4
		default:
			currentNorm := normalizeHeadingForMatch(currentHeading)
			switch {
			case currentHeading == selectorHeading:
				score = 2
			case selectorNorm != "" && currentNorm == selectorNorm:
				score = 2
			case selectorNorm != "" && len([]rune(selectorNorm)) >= 4 && (strings.Contains(currentNorm, selectorNorm) || strings.Contains(selectorNorm, currentNorm)):
				score = 1
			}
		}
		if score == 0 {
			continue
		}
		if score > bestScore {
			bestScore = score
			candidates = candidates[:0]
		}
		if score == bestScore {
			candidates = append(candidates, sectionCandidate{
				start:          match[0],
				end:            end,
				matchedHeading: strings.Repeat("#", currentLevel) + " " + currentHeading,
			})
		}
	}
	if len(candidates) == 0 {
		return 0, 0, "", false, false
	}
	if len(candidates) > 1 {
		return 0, 0, "", false, true
	}
	selected := candidates[0]
	return selected.start, selected.end, selected.matchedHeading, true, false
}

func normalizeHeadingForMatch(title string) string {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return ""
	}
	trimmed = chatDocumentHeadingMarkerTrimRE.ReplaceAllString(trimmed, "")
	trimmed = chatDocumentHeadingNumberTrimRE.ReplaceAllString(trimmed, "")
	trimmed = strings.TrimSpace(trimmed)
	trimmed = regexp.MustCompile(`\s+`).ReplaceAllString(trimmed, "")
	return strings.ToLower(trimmed)
}

func inferSectionTargetFromQuery(query string) string {
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return ""
	}
	if matches := chatDocumentQuotedTargetRE.FindStringSubmatch(trimmedQuery); len(matches) == 2 {
		return strings.TrimSpace(matches[1])
	}
	if match := chatDocumentScopedPhraseRE.FindString(trimmedQuery); match != "" {
		cleaned := strings.TrimSpace(match)
		for _, prefix := range []string{"在", "对", "把", "将", "就"} {
			cleaned = strings.TrimPrefix(cleaned, prefix)
		}
		cleaned = strings.TrimSpace(cleaned)
		for _, suffix := range []string{"章节", "小节", "模块", "部分"} {
			cleaned = strings.TrimSuffix(cleaned, suffix)
		}
		cleaned = chatDocumentTargetLeadTrimRE.ReplaceAllString(cleaned, "")
		return strings.TrimSpace(cleaned)
	}
	for _, keyword := range []string{"智慧运行", "智慧安防", "数据湖", "算力平台", "智能安全监控应急中心", "应急中心", "AR眼镜"} {
		if strings.Contains(trimmedQuery, keyword) {
			return keyword
		}
	}
	return ""
}

func chatDocumentOperationForIntent(intent string) string {
	switch intent {
	case types.ChatDocumentIntentContinue:
		return types.ChatDocumentOperationContinue
	case types.ChatDocumentIntentRevise:
		return types.ChatDocumentOperationRevise
	case types.ChatDocumentIntentRegenerate:
		return types.ChatDocumentOperationRegenerate
	default:
		return types.ChatDocumentOperationCreate
	}
}

func normalizeChatDocumentOperation(operation string) string {
	switch strings.TrimSpace(operation) {
	case types.ChatDocumentOperationContinue:
		return types.ChatDocumentOperationContinue
	case types.ChatDocumentOperationRevise:
		return types.ChatDocumentOperationRevise
	case types.ChatDocumentOperationRegenerate:
		return types.ChatDocumentOperationRegenerate
	default:
		return types.ChatDocumentOperationCreate
	}
}

func normalizeDocumentOutputMode(outputMode string, intent string) string {
	switch strings.TrimSpace(outputMode) {
	case types.ChatDocumentOutputModeFull:
		return types.ChatDocumentOutputModeFull
	case types.ChatDocumentOutputModeDelta:
		return types.ChatDocumentOutputModeDelta
	}

	switch intent {
	case types.ChatDocumentIntentContinue, types.ChatDocumentIntentRevise:
		return types.ChatDocumentOutputModeDelta
	case types.ChatDocumentIntentRegenerate:
		return types.ChatDocumentOutputModeFull
	default:
		return types.ChatDocumentOutputModeFull
	}
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func runeLen(value string) int {
	return len([]rune(value))
}

func chatDocumentMinInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func chatDocumentMaxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
