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
	chatDocumentContinueIntentRE   = regexp.MustCompile(`(?i)(继续生成|接着写|续写|从上次中断处继续|补全剩余|继续输出|继续补齐|继续补充|接着补齐|接着补充|补齐剩余|补充剩余|继续完善|继续扩写)`)
	chatDocumentReviseIntentRE     = regexp.MustCompile(`(?i)(修改上一版|基于上一个文档修改|把上一份改成|调整上一版|完善上一版)`)
	chatDocumentRegenerateIntentRE = regexp.MustCompile(`(?i)(重新生成|从头生成|重写一版|不要基于上一版)`)
	chatDocumentHeadingRE          = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+)$`)
	chatDocumentCodeFenceRE        = regexp.MustCompile("(?m)^```")
	chatDocumentListRE             = regexp.MustCompile(`(?m)^\s*(?:[-*+]\s+|\d+\.\s+)`)
	chatDocumentTableRE            = regexp.MustCompile(`(?m)^\|.+\|\s*$`)
	chatDocumentPatchEnvelopeRE    = regexp.MustCompile(`(?s)^\s*<document_patch>\s*(.*?)\s*</document_patch>\s*$`)
	chatDocumentPatchOperationRE   = regexp.MustCompile(`(?s)<(replace|append|insert_after)\s+heading=(?:"([^"]+)"|'([^']+)')\s*>(.*?)</(replace|append|insert_after)>`)
	chatDocumentQueryHintRE        = regexp.MustCompile(`(?i)(方案|文档|报告|markdown|技术方案|设计方案|plan|report|document)`)
	chatDocumentDuplicatePhraseRE  = regexp.MustCompile(`(?i)^(我已修改|下面是修改建议|已根据你的要求修改)`)
	chatDocumentRevisionLeadRE     = regexp.MustCompile(`(?i)^(我已修改|下面是修改|以下是修改|已根据你的要求修改|根据你的要求|我已经根据|已按要求)`)
)

type chatDocumentArtifactService struct {
	repo interfaces.ChatDocumentArtifactRepository
}

func NewChatDocumentArtifactService(repo interfaces.ChatDocumentArtifactRepository) interfaces.ChatDocumentArtifactService {
	return &chatDocumentArtifactService{repo: repo}
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
	case chatDocumentContinueIntentRE.MatchString(trimmedQuery):
		intent = types.ChatDocumentIntentContinue
	case trimmedHint == types.ChatDocumentIntentContinue,
		trimmedHint == types.ChatDocumentIntentRevise,
		trimmedHint == types.ChatDocumentIntentRegenerate:
		intent = trimmedHint
	}

	return &types.DocumentIntentResult{
		Intent:    intent,
		Operation: chatDocumentOperationForIntent(intent),
	}, nil
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
	return hydrateChatDocumentArtifactDerivedFields(artifact), nil
}

func (s *chatDocumentArtifactService) BuildQuotedContext(ctx context.Context, artifact *types.ChatDocumentArtifact, query string, intent string, outputMode string) (string, error) {
	_ = ctx
	_ = query
	if artifact == nil || !artifact.CanContinue() {
		return "", nil
	}

	outputMode = normalizeDocumentOutputMode(outputMode, intent)

	contentRunes := []rune(strings.TrimSpace(artifact.ContentSnapshot))
	if len(contentRunes) == 0 {
		return "", nil
	}
	if len(contentRunes) > types.ChatDocumentArtifactInlineContinuationMaxChars {
		return "", fmt.Errorf("artifact content is too large for inline continuation")
	}

	payload := string(contentRunes)
	truncated := false
	if len(contentRunes) > 30000 {
		truncated = true
		outline := strings.TrimSpace(extractMarkdownHeadingOutline(payload))
		tail := strings.TrimSpace(string(contentRunes[chatDocumentMaxInt(0, len(contentRunes)-16000):]))
		if intent == types.ChatDocumentIntentRevise {
			head := strings.TrimSpace(string(contentRunes[:chatDocumentMinInt(len(contentRunes), 8000)]))
			payload = buildTruncatedDocumentPayload(outline, head, tail)
		} else {
			payload = buildTruncatedDocumentPayload(outline, "", tail)
		}
	}

	metadata := fmt.Sprintf("- artifact_id: %s\n- revision_no: %d\n- completion_status: %s\n- operation: %s",
		artifact.ID, artifact.RevisionNo, artifact.CompletionStatus, artifact.Operation)
	if truncated {
		metadata += "\n- snapshot_mode: truncated"
	}

	if intent == types.ChatDocumentIntentRevise && outputMode == types.ChatDocumentOutputModeFull {
		return fmt.Sprintf(`<document_revision_context>
你正在修改同一会话中的上一份文档。

修改规则：
1. 以上一份文档为基线进行修改。
2. 不要丢失用户没有要求删除的章节和内容。
3. 按用户本轮要求调整结构、补充细节或修正文案。
4. 输出修改后的完整 Markdown 文档。
5. 不要输出 diff 标记，不要输出修改说明，除非用户明确要求。

上一份文档元数据：
%s

上一份文档内容：
<document>
%s
</document>
</document_revision_context>`, metadata, payload), nil
	}

	if intent == types.ChatDocumentIntentRevise {
		return fmt.Sprintf(`<document_revision_context>
你正在修改同一会话中的上一份文档。

输出规则：
1. 优先输出 <document_patch> 包裹的结构化 patch，不要输出完整文档全文。
2. 结构化 patch 支持 <replace heading="## 标题">...</replace>、<append heading="## 标题">...</append>、<insert_after heading="## 标题">...</insert_after>。
3. replace 输出替换后的完整章节内容；append 输出要追加到目标章节末尾的 Markdown 片段；insert_after 输出要插入到目标章节后的 Markdown 片段。
4. 如果用户只要求修改单个章节且你无法稳定生成 patch，可退化为输出带标题的最终章节片段。
5. 不要输出 diff 标记，不要输出修改说明。
6. 不要重复未修改章节。
7. 输出内容必须能被系统合并回上一份文档，形成新的完整版本。

上一份文档元数据：
%s

上一份文档内容：
<document>
%s
</document>
</document_revision_context>`, metadata, payload), nil
	}

	return fmt.Sprintf(`<document_continuation_context>
你正在继续生成同一会话中的上一份文档。

续写规则：
1. 不要从头重写上一份文档。
2. 从上一份文档末尾自然继续。
3. 保持标题层级、术语、编号、表格和 Markdown 风格一致。
4. 如果上一份文档末尾句子不完整，先补齐该句，再继续后续内容。
5. 默认只输出新增内容，不要重复上一份文档中已经完整输出的段落。
6. 不要解释“我将继续”，直接输出文档正文。

上一份文档元数据：
%s

上一份文档内容：
<document>
%s
</document>
</document_continuation_context>`, metadata, payload), nil
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
		return existing, nil
	}

	content := strings.TrimSpace(message.Content)
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
	quality := evaluateRevisionArtifactQuality(snapshot, options)
	if !quality.ShouldCreate {
		return nil, nil
	}
	quality.QualityIssues = uniqueStrings(append(quality.QualityIssues, snapshotResult.QualityIssues...))
	quality.QualityIssues = uniqueStrings(append(quality.QualityIssues, preparation.QualityIssues...))

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
	artifact := &types.ChatDocumentArtifact{
		TenantID:         tenantID,
		SessionID:        message.SessionID,
		SourceMessageID:  message.ID,
		SourceRequestID:  message.RequestID,
		ParentArtifactID: "",
		RevisionNo:       1,
		Title:            extractChatDocumentTitle(snapshot),
		ArtifactKind:     detectChatDocumentArtifactKind(snapshot),
		ContentType:      contentTypeForChatDocument(snapshot),
		ContentSnapshot:  snapshot,
		ContentChecksum:  checksumText(snapshot),
		Status:           artifactStatus,
		CompletionStatus: completionStatus,
		Operation:        normalizeChatDocumentOperation(options.Operation),
		CreatedBy:        userID,
	}

	if base := options.BaseArtifact; base != nil && artifact.Operation != types.ChatDocumentOperationCreate && artifact.Operation != types.ChatDocumentOperationRegenerate {
		artifact.ParentArtifactID = base.ID
		artifact.RevisionNo = base.RevisionNo + 1
	}
	hydrateChatDocumentArtifactDerivedFields(artifact, quality.QualityIssues...)

	if err := s.repo.CreateArtifact(ctx, artifact); err != nil {
		return nil, err
	}
	return artifact, nil
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
	for _, artifact := range artifacts {
		hydrateChatDocumentArtifactDerivedFields(artifact)
	}
	return artifacts, nil
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

func shouldRegisterChatDocumentArtifact(content string, options types.RegisterChatDocumentArtifactOptions) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	operation := normalizeChatDocumentOperation(options.Operation)
	if options.BaseArtifact != nil && (operation == types.ChatDocumentOperationContinue || operation == types.ChatDocumentOperationRevise) {
		return true
	}
	if runeLen(trimmed) < 2000 {
		return false
	}
	if operation == types.ChatDocumentOperationRevise && options.BaseArtifact != nil {
		return true
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
	if outputMode == types.ChatDocumentOutputModeFull {
		return chatDocumentSnapshotBuildResult{Snapshot: trimmed}
	}
	if operation == types.ChatDocumentOperationContinue {
		return chatDocumentSnapshotBuildResult{Snapshot: mergeChatDocumentContinuation(base.ContentSnapshot, trimmed)}
	}
	if operation == types.ChatDocumentOperationRevise {
		merged := mergeChatDocumentRevisionDelta(base.ContentSnapshot, trimmed)
		return chatDocumentSnapshotBuildResult{Snapshot: merged.Snapshot, QualityIssues: merged.QualityIssues}
	}
	return chatDocumentSnapshotBuildResult{Snapshot: trimmed}
}

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

func prepareChatDocumentArtifactSnapshot(snapshot string, options types.RegisterChatDocumentArtifactOptions) artifactSnapshotPreparationResult {
	trimmed := strings.TrimSpace(snapshot)
	issues := make([]string, 0, 3)
	if normalizeChatDocumentOperation(options.Operation) == types.ChatDocumentOperationRevise {
		var trimmedLead bool
		trimmed, trimmedLead = trimRevisionPreamble(trimmed)
		if trimmedLead {
			issues = append(issues, types.ChatDocumentQualityIssueRevisionPreambleTrimmed)
		}
	}
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
	artifact.StructureInfo = structure
	artifact.SnapshotCharCount = runeLen(snapshot)
	artifact.QualityIssues = uniqueStrings(issues)
	artifact.CanInlineContinue = artifact.CanContinue()
	artifact.UserHint = chatDocumentUserHintForIssues(artifact.QualityIssues, artifact.Operation)
	return artifact
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
	if chatDocumentContainsString(issues, types.ChatDocumentQualityIssueInlineContextTooLarge) {
		return "当前文档过长，无法直接基于整篇继续生成。请改为指定章节修改，或先让模型生成精简版后再继续。"
	}
	if chatDocumentContainsString(issues, types.ChatDocumentQualityIssueSnapshotTruncated) {
		return "当前版本过长，系统只保留了截断快照。建议先缩小修改范围，再继续生成。"
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
	if chatDocumentContainsString(issues, types.ChatDocumentQualityIssueUnclosedCodeFence) {
		return "检测到末尾代码块未闭合，系统已自动补全代码围栏。"
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

func findMarkdownSectionRange(content string, heading string, level int) (int, int, bool) {
	matches := chatDocumentHeadingRE.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return 0, 0, false
	}

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
	body := strings.TrimSpace(matches[1])
	if body == "" {
		return nil, true, false
	}

	opMatches := chatDocumentPatchOperationRE.FindAllStringSubmatchIndex(body, -1)
	if len(opMatches) == 0 {
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
		start, end, matchedHeading, found := findMarkdownSectionRangeBySelector(current, operation.Heading)
		switch operation.Action {
		case "replace":
			if !found {
				current = appendChatDocumentFragment(current, operation.Content)
				issues = append(issues, types.ChatDocumentQualityIssueDeltaMergeUncertain)
				continue
			}
			replacement := ensurePatchReplaceHeading(operation.Content, operation.Heading, matchedHeading)
			current = replaceChatDocumentRange(current, start, end, replacement)
		case "append":
			if !found {
				current = appendChatDocumentFragment(current, operation.Content)
				issues = append(issues, types.ChatDocumentQualityIssueDeltaMergeUncertain)
				continue
			}
			current = replaceChatDocumentRange(current, start, end, appendChatDocumentFragment(current[start:end], operation.Content))
		case "insert_after":
			if !found {
				current = appendChatDocumentFragment(current, operation.Content)
				issues = append(issues, types.ChatDocumentQualityIssueDeltaMergeUncertain)
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

func findMarkdownSectionRangeBySelector(content string, selector string) (int, int, string, bool) {
	trimmedSelector := strings.TrimSpace(selector)
	if trimmedSelector == "" {
		return 0, 0, "", false
	}
	if heading, level, ok := parseMarkdownHeadingSelector(trimmedSelector); ok {
		start, end, found := findMarkdownSectionRange(content, heading, level)
		if !found {
			return 0, 0, "", false
		}
		return start, end, strings.Repeat("#", level) + " " + heading, true
	}

	matches := chatDocumentHeadingRE.FindAllStringSubmatchIndex(content, -1)
	for idx, match := range matches {
		if len(match) < 6 {
			continue
		}
		currentLevel := len(content[match[2]:match[3]])
		currentHeading := strings.TrimSpace(content[match[4]:match[5]])
		if currentHeading != trimmedSelector {
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
		return match[0], end, strings.Repeat("#", currentLevel) + " " + currentHeading, true
	}
	return 0, 0, "", false
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
