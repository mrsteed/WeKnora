package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

func buildChatDocumentQuotedContext(ctx context.Context, artifact *types.ChatDocumentArtifact, query string, intent string, outputMode string, targetHeading string, mergeMode string) (string, error) {
	_ = ctx
	if artifact == nil || !artifact.CanUseAsBaseForIntent(intent) {
		return "", nil
	}

	outputMode = normalizeDocumentOutputMode(outputMode, intent)
	effectiveTargetHeading := resolveDocumentTargetHeading(query, targetHeading)
	effectiveMergeMode := normalizeChatDocumentMergeMode(mergeMode, intent, effectiveTargetHeading)

	contentRunes := []rune(strings.TrimSpace(artifact.ContentSnapshot))
	if len(contentRunes) == 0 {
		return "", nil
	}
	payload := string(contentRunes)
	truncated := false
	targetedPayload := false
	dualAnchorPayload := false
	matchedTargetHeading := ""
	editPlan := inferChatDocumentEditPlan(query, effectiveTargetHeading, effectiveMergeMode)
	if intent == types.ChatDocumentIntentRevise && outputMode == types.ChatDocumentOutputModeDelta {
		if focusedPayload, resolvedTargetHeading, ok := buildDualAnchorDocumentPayload(payload, editPlan); ok {
			payload = focusedPayload
			dualAnchorPayload = true
			targetedPayload = true
			matchedTargetHeading = resolvedTargetHeading
		} else if effectiveTargetHeading != "" {
			if focusedPayload, resolvedHeading, ok := buildTargetedDocumentPayload(payload, effectiveTargetHeading); ok {
				payload = focusedPayload
				targetedPayload = true
				matchedTargetHeading = resolvedHeading
			}
		}
	} else if effectiveTargetHeading != "" && intent == types.ChatDocumentIntentRevise {
		if focusedPayload, resolvedHeading, ok := buildTargetedDocumentPayload(payload, effectiveTargetHeading); ok {
			payload = focusedPayload
			targetedPayload = true
			matchedTargetHeading = resolvedHeading
		}
	}
	if !targetedPayload && len(contentRunes) > 30000 {
		truncated = true
		outline := strings.TrimSpace(extractMarkdownHeadingOutline(payload))
		tailSize := 16000
		if len(contentRunes) > types.ChatDocumentArtifactInlineContinuationMaxChars {
			tailSize = 24000
		}
		tail := strings.TrimSpace(string(contentRunes[chatDocumentMaxInt(0, len(contentRunes)-tailSize):]))
		if intent == types.ChatDocumentIntentRevise {
			head := strings.TrimSpace(string(contentRunes[:chatDocumentMinInt(len(contentRunes), 8000)]))
			payload = buildTruncatedDocumentPayload(outline, head, tail)
		} else {
			payload = buildTruncatedDocumentPayload(outline, "", tail)
		}
	}

	contextMode := artifact.ContinuationMode()
	metadata := fmt.Sprintf("- artifact_id: %s\n- revision_no: %d\n- completion_status: %s\n- operation: %s\n- snapshot_char_count: %d\n- continuation_context_mode: %s",
		artifact.ID, artifact.RevisionNo, artifact.CompletionStatus, artifact.Operation, len(contentRunes), contextMode)
	if effectiveTargetHeading != "" {
		metadata += fmt.Sprintf("\n- target_heading: %s", effectiveTargetHeading)
	}
	if editPlan.SourceHeading != "" {
		metadata += fmt.Sprintf("\n- source_heading: %s", editPlan.SourceHeading)
	}
	if editPlan.Operation != "" {
		metadata += fmt.Sprintf("\n- document_edit_operation: %s", editPlan.Operation)
	}
	if matchedTargetHeading != "" && matchedTargetHeading != effectiveTargetHeading {
		metadata += fmt.Sprintf("\n- resolved_target_heading: %s", matchedTargetHeading)
	}
	if effectiveMergeMode != "" {
		metadata += fmt.Sprintf("\n- document_merge_mode: %s", effectiveMergeMode)
	}
	if truncated {
		metadata += "\n- snapshot_mode: truncated"
	}
	if targetedPayload {
		metadata += "\n- snapshot_mode: targeted_section_context"
	}
	if dualAnchorPayload {
		metadata += "\n- snapshot_mode: dual_anchor_context"
	}
	goalBlock := buildChatDocumentGoalBlock(query) + buildChatDocumentTargetBlock(editPlan)

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
%s

上一份文档内容：
<document>
%s
</document>
</document_revision_context>`, metadata, goalBlock, payload), nil
	}

	if intent == types.ChatDocumentIntentRevise {
		return fmt.Sprintf(`<document_revision_context>
你正在修改同一会话中的上一份文档。

输出规则：
1. 优先输出 <document_patch> 包裹的结构化 patch，不要输出完整文档全文。
2. 结构化 patch 支持 <replace heading="## 标题">...</replace>、<append heading="## 标题">...</append>、<insert_after heading="## 标题">...</insert_after>。
3. replace 输出替换后的完整章节内容；append 输出要追加到目标章节末尾的 Markdown 片段；insert_after 输出要插入到目标章节后的 Markdown 片段。
4. 如果用户要求补充、扩写、细化某个章节或模块，优先输出 <append heading="目标章节标题">...</append>，把新增内容追加到目标章节内，不要把内容放到文档末尾。
5. 如果用户只要求修改单个章节且你无法稳定生成 patch，可退化为输出带标题的最终章节片段。
6. 不要输出 diff 标记，不要输出修改说明。
7. 不要重复未修改章节。
8. 输出内容必须能被系统合并回上一份文档，形成新的完整版本。

上一份文档元数据：
%s
%s

上一份文档内容：
<document>
%s
</document>
</document_revision_context>`, metadata, goalBlock, payload), nil
	}

	return fmt.Sprintf(`<document_continuation_context>
你正在继续生成同一会话中的上一份文档。

续写规则：
1. 不要从头重写上一份文档。
2. 从上一份文档末尾自然继续。
3. 保持标题层级、术语、编号、表格和 Markdown 风格一致。
4. 如果上一份文档末尾句子不完整，先补齐该句，再继续后续内容。
5. 默认只输出新增内容，不要重复上一份文档中已经完整输出的段落。
6. 如果你判断整篇文档已经完整输出，请在正文最后单独输出 %s。
7. 如果仍有剩余章节、表格、附录或总结未输出，不要输出该完成标记。
8. 如果上下文元数据中的 continuation_context_mode 为 outline_tail，说明完整文档过长；请依据目录和最近末尾窗口判断下一节，不要复述目录或旧正文。
9. 不要解释“我将继续”，直接输出文档正文。

上一份文档元数据：
%s
%s

上一份文档内容：
<document>
%s
</document>
</document_continuation_context>`, types.ChatDocumentCompletionMarker, metadata, goalBlock, payload), nil
}
