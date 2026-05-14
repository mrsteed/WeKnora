import test from 'node:test';
import assert from 'node:assert/strict';

import {
  getDocumentCompletionStatusDetail,
  isDocumentCompletionContinuing,
  normalizeDocumentQualityIssues,
} from '../utils/documentCompletion.js';

test('normalizes unique document quality issues', () => {
  assert.deepEqual(
    normalizeDocumentQualityIssues(['markdown_structure_invalid', 'markdown_structure_invalid', 'markdown_too_short']),
    ['markdown_structure_invalid', 'markdown_too_short'],
  );
});

test('describes section batch limit as auto-continue ready', () => {
  assert.equal(
    getDocumentCompletionStatusDetail({
      completion_status: 'partial',
      document_generation_status: 'continuing',
      finish_reason: 'section_batch_limit',
    }),
    '当前轮已达到分批生成上限，系统可继续自动续写剩余章节。',
  );
});

test('treats continuation pending as a continuing partial state', () => {
  assert.equal(
    isDocumentCompletionContinuing({
      completion_status: 'partial',
      document_generation_status: 'continuing',
      finish_reason: 'continuation_pending',
    }),
    true,
  );
  assert.equal(
    getDocumentCompletionStatusDetail({
      completion_status: 'partial',
      document_generation_status: 'continuing',
      finish_reason: 'continuation_pending',
    }),
    '当前轮内容已生成，系统可继续自动续写剩余章节。',
  );
});

test('describes length-limited continuing partial states clearly', () => {
  assert.equal(
    getDocumentCompletionStatusDetail({
      completion_status: 'partial',
      document_generation_status: 'continuing',
      finish_reason: 'length',
    }),
    '当前轮输出达到长度上限，系统可继续自动续写剩余章节。',
  );
});

test('describes outline mismatch using quality issues when available', () => {
  assert.equal(
    getDocumentCompletionStatusDetail({
      completion_status: 'partial',
      failure_reason: 'outline_or_section_incomplete',
      quality_issues: ['markdown_structure_invalid', 'markdown_unplanned_subsection'],
    }),
    '文档结构校验未通过：小节标题或层级与规划大纲不一致，出现未规划的小节标题。',
  );
});

test('describes blocked local knowledge misses clearly', () => {
  assert.equal(
    getDocumentCompletionStatusDetail({
      completion_status: 'partial',
      failure_reason: 'local_knowledge_not_found',
    }),
    '本地知识不足：当前章节缺少可用证据，建议补充资料后继续生成。',
  );
});

test('describes exhausted llm timeout retries clearly', () => {
  assert.equal(
    getDocumentCompletionStatusDetail({
      completion_status: 'partial',
      finish_reason: 'llm_timeout_retry_exhausted',
      failure_reason: 'llm_timeout',
      document_generation_status: 'continuing',
    }),
    '模型响应连续两轮超时：系统已停止自动续写，请人工继续或稍后重试。',
  );
});

test('describes needs review from quality issues', () => {
  assert.equal(
    getDocumentCompletionStatusDetail({
      completion_status: 'partial',
      document_generation_status: 'needs_review',
      quality_issues: ['markdown_too_short'],
    }),
    '文档正文已生成，但存在质量告警：部分章节内容明显偏短。',
  );
});

test('describes completed document status clearly', () => {
  assert.equal(
    getDocumentCompletionStatusDetail({
      completion_status: 'completed',
      document_generation_status: 'completed',
    }),
    '完整文档已生成，可直接预览或下载。',
  );
});

test('describes blocked document status using quality issues', () => {
  assert.equal(
    getDocumentCompletionStatusDetail({
      completion_status: 'partial',
      document_generation_status: 'blocked',
      quality_issues: ['internal_prompt_leakage'],
    }),
    '当前文档生成已被阻断：正文混入了内部提示词或上下文标签。',
  );
});
