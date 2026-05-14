const QUALITY_ISSUE_MESSAGES = {
  markdown_structure_invalid: '小节标题或层级与规划大纲不一致',
  markdown_unplanned_subsection: '出现未规划的小节标题',
  markdown_too_short: '部分章节内容明显偏短',
  markdown_heading_normalized: '部分标题格式已自动修复',
  internal_prompt_leakage: '正文混入了内部提示词或上下文标签',
  unclosed_code_fence: '存在未闭合的代码块，系统已尝试自动补全',
  inline_context_too_large: '文档过长，当前版本使用了受限上下文继续生成',
  section_number_reset: '章节编号存在回退或跳号',
  duplicate_document_head: '文档标题存在重复',
};

const FAILURE_REASON_MESSAGES = {
  local_knowledge_not_found: '本地知识不足：当前章节缺少可用证据，建议补充资料后继续生成。',
  section_generation_timeout: '章节生成超时：系统已保留当前已生成内容，可继续续写。',
  section_generation_error: '章节生成中断：系统已保留当前已生成内容，可继续续写。',
  llm_timeout_retry_exhausted: '模型响应连续两轮超时：系统已停止自动续写，请人工继续或稍后重试。',
  auto_continue_round_limit: '已达到自动续写轮次上限，请人工继续或重新发起任务。',
  outline_parse_failed: '大纲结构解析失败：本轮未能生成可用章节结构。',
  empty_document_edit_completion: '当前轮未生成可用文档内容，请重试。',
};

const CONTINUING_FINISH_REASONS = new Set(['', 'stop', 'length', 'section_batch_limit', 'continuation_pending']);

function trimText(value) {
  return typeof value === 'string' ? value.trim() : '';
}

export function normalizeDocumentQualityIssues(rawIssues) {
  if (!rawIssues) {
    return [];
  }

  const issues = [];
  const append = (value) => {
    const normalized = trimText(value);
    if (normalized && !issues.includes(normalized)) {
      issues.push(normalized);
    }
  };

  if (Array.isArray(rawIssues)) {
    rawIssues.forEach(append);
    return issues;
  }

  append(rawIssues);
  return issues;
}

function describeQualityIssues(qualityIssues) {
  const messages = normalizeDocumentQualityIssues(qualityIssues)
    .map((issue) => QUALITY_ISSUE_MESSAGES[issue] || '')
    .filter(Boolean);

  if (messages.length === 0) {
    return '';
  }
  return messages.join('，');
}

function describeGenerationStatus(documentGenerationStatus, detail = '') {
  switch (documentGenerationStatus) {
    case 'continuing':
      return detail || '当前轮内容已生成，系统可继续自动续写剩余章节。';
    case 'completed':
      return detail || '完整文档已生成，可直接预览或下载。';
    case 'needs_review':
      return detail || '文档正文已生成，但存在质量告警，建议人工复核后再继续。';
    case 'blocked':
      return detail || '当前文档生成已被阻断，请补充资料或调整要求后重试。';
    default:
      return detail;
  }
}

export function isDocumentCompletionContinuing(payload = {}) {
  const documentGenerationStatus = trimText(
    payload.document_generation_status ||
      payload.documentGenerationStatus ||
      payload.chat_document_artifact?.document_generation_status,
  );
  const failureReason = trimText(payload.failure_reason || payload.failureReason);
  const finishReason = trimText(payload.finish_reason || payload.finishReason);
  const autoContinueNext = payload.auto_continue_next ?? payload.autoContinueNext;

  if (documentGenerationStatus !== 'continuing') {
    return false;
  }
  if (autoContinueNext === true) {
    return true;
  }
  if (failureReason) {
    return false;
  }
  return CONTINUING_FINISH_REASONS.has(finishReason);
}

export function getDocumentCompletionStatusDetail(payload = {}) {
  const completionStatus = trimText(payload.completion_status || payload.completionStatus);
  const finishReason = trimText(payload.finish_reason || payload.finishReason);
  const failureReason = trimText(payload.failure_reason || payload.failureReason);
  const documentGenerationStatus = trimText(
    payload.document_generation_status ||
      payload.documentGenerationStatus ||
      payload.chat_document_artifact?.document_generation_status,
  );
  const qualityIssues = normalizeDocumentQualityIssues(
    payload.quality_issues || payload.qualityIssues || payload.chat_document_artifact?.quality_issues,
  );

  if (completionStatus === 'partial' && isDocumentCompletionContinuing(payload)) {
    if (finishReason === 'section_batch_limit') {
      return '当前轮已达到分批生成上限，系统可继续自动续写剩余章节。';
    }
    if (finishReason === 'length') {
      return '当前轮输出达到长度上限，系统可继续自动续写剩余章节。';
    }
    return '当前轮内容已生成，系统可继续自动续写剩余章节。';
  }

  if (failureReason === 'outline_or_section_incomplete' || finishReason === 'outline_or_section_incomplete') {
    const detail = describeQualityIssues(qualityIssues);
    if (detail) {
      return `文档结构校验未通过：${detail}。`;
    }
    return '文档结构校验未通过：章节或小节与规划大纲不一致。';
  }

  if (FAILURE_REASON_MESSAGES[failureReason]) {
    return FAILURE_REASON_MESSAGES[failureReason];
  }
  if (FAILURE_REASON_MESSAGES[finishReason]) {
    return FAILURE_REASON_MESSAGES[finishReason];
  }

  if (documentGenerationStatus === 'needs_review') {
    const detail = describeQualityIssues(qualityIssues);
    if (detail) {
      return describeGenerationStatus(documentGenerationStatus, `文档正文已生成，但存在质量告警：${detail}。`);
    }
    return describeGenerationStatus(documentGenerationStatus, '文档正文已生成，但存在结构或格式告警，建议人工复核后再继续。');
  }

  if (documentGenerationStatus === 'blocked') {
    const detail = describeQualityIssues(qualityIssues);
    if (detail) {
      return describeGenerationStatus(documentGenerationStatus, `当前文档生成已被阻断：${detail}。`);
    }
    return describeGenerationStatus(documentGenerationStatus);
  }

  if (documentGenerationStatus === 'completed' && completionStatus === 'completed') {
    const detail = describeQualityIssues(qualityIssues);
    if (detail) {
      return describeGenerationStatus('needs_review', `文档正文已生成，但仍有质量告警：${detail}。`);
    }
    return describeGenerationStatus(documentGenerationStatus);
  }

  if ((completionStatus === 'partial' || completionStatus === 'failed') && qualityIssues.length > 0) {
    const detail = describeQualityIssues(qualityIssues);
    if (detail) {
      return `文档已生成，但存在质量告警：${detail}。`;
    }
  }

  return '';
}
