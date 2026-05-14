import { postBlob, get } from './request';

export type ExportFormat = 'markdown' | 'pdf' | 'docx' | 'xlsx';

export interface ExportCapability {
  available: boolean;
  engine: string;
  reason?: string;
  maxContentBytes: number;
  timeoutSeconds: number;
}

export interface ExportCapabilities {
  markdown: ExportCapability;
  pdf: ExportCapability;
  docx: ExportCapability;
  xlsx: ExportCapability;
}

export interface ChatDocumentEvidenceRef {
  query?: string;
  section_heading?: string;
  knowledge_base_id?: string;
  knowledge_id?: string;
  chunk_id?: string;
  source_title?: string;
  score?: number;
  content_checksum?: string;
}

export interface ChatDocumentEvidenceSourceSummary {
  knowledge_base_id?: string;
  knowledge_id?: string;
  source_title?: string;
  chunk_count?: number;
  max_score?: number;
}

export interface ChatDocumentEvidenceSummary {
  ref_count?: number;
  knowledge_base_count?: number;
  knowledge_count?: number;
  chunk_count?: number;
  sources?: ChatDocumentEvidenceSourceSummary[];
}

export interface ChatDocumentArtifactExportPayload {
  evidence_refs?: ChatDocumentEvidenceRef[];
  evidence_summary?: ChatDocumentEvidenceSummary | null;
}

/**
 * 生成导出文件名前缀。
 *
 * 后端接口会统一追加时间戳并补齐扩展名，这里只负责提供可读的业务前缀，
 * 避免前后端重复追加时间戳导致文件名出现双时间戳。
 */
export const generateFilename = (prefix?: string): string => {
  return prefix?.trim() || '对话导出';
};

/**
 * 触发浏览器下载
 */
export const triggerDownload = (blob: Blob, filename: string): void => {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
};

// ============================================================
// 后端导出能力缓存
// ============================================================
let _capabilities: ExportCapabilities | null = null;

const EXPORT_EXTENSION_MAP: Record<ExportFormat, string> = {
  markdown: 'md',
  pdf: 'pdf',
  docx: 'docx',
  xlsx: 'xlsx',
};

const EVIDENCE_APPENDIX_HEADING = '## 本地知识库引用附录';

/**
 * 查询后端导出能力。
 * 当能力探测失败时保留默认可用状态，避免探测异常直接屏蔽导出入口。
 */
export const getExportCapabilities = async (): Promise<ExportCapabilities> => {
  if (_capabilities) return _capabilities;
  try {
    const res = await get('/api/v1/export/capabilities');
    const data = (res as any).data?.data || (res as any).data;
    _capabilities = {
      markdown: normalizeCapability(data?.markdown, true, 'builtin', 2 * 1024 * 1024, 5),
      pdf: normalizeCapability(data?.pdf, false, 'chromium', 1024 * 1024, 45),
      docx: normalizeCapability(data?.docx, false, 'pandoc', 1024 * 1024, 30),
      xlsx: normalizeCapability(data?.xlsx, true, 'excelize', 1024 * 1024, 10),
    };
  } catch {
    _capabilities = {
      markdown: normalizeCapability(undefined, true, 'builtin', 2 * 1024 * 1024, 5),
      pdf: normalizeCapability(undefined, true, 'chromium', 1024 * 1024, 45),
      docx: normalizeCapability(undefined, true, 'pandoc', 1024 * 1024, 30),
      xlsx: normalizeCapability(undefined, true, 'excelize', 1024 * 1024, 10),
    };
  }
  return _capabilities;
};

const normalizeCapability = (
  raw: any,
  fallbackAvailable: boolean,
  fallbackEngine: string,
  fallbackMaxContentBytes: number,
  fallbackTimeoutSeconds: number,
): ExportCapability => ({
  available: raw?.available ?? fallbackAvailable,
  engine: raw?.engine || raw?.tool || fallbackEngine,
  reason: raw?.reason,
  maxContentBytes: raw?.max_content_bytes ?? fallbackMaxContentBytes,
  timeoutSeconds: raw?.timeout_seconds ?? fallbackTimeoutSeconds,
});

/**
 * 通过统一后端接口导出文档。
 */
const exportViaBackend = async (
  content: string,
  format: ExportFormat,
  filename: string,
): Promise<void> => {
  let res: any;
  try {
    res = await postBlob('/api/v1/export/document', {
      content,
      format,
      filename_prefix: filename,
    }, { rawResponse: true });
  } catch (error) {
    throw normalizeExportError(error);
  }

  const rawData = res instanceof Blob ? res : res.data || (res as any);

  if (!(rawData instanceof Blob) || rawData.size === 0) {
    throw new Error('Backend returned empty or invalid response');
  }

  if (rawData.type && rawData.type.includes('application/json')) {
    const text = await rawData.text();
    let parsed: any;
    try {
      parsed = JSON.parse(text);
    } catch {
      throw new Error('Backend export failed');
    }
    throw normalizeExportError(parsed);
  }

  const blob = rawData instanceof Blob ? rawData : new Blob([rawData]);
  const downloadFilename =
    extractFilenameFromDisposition(res instanceof Blob ? undefined : res.headers?.['content-disposition']) ||
    buildFallbackFilename(filename, format);
  triggerDownload(blob, downloadFilename);
};

/**
 * 导出为 Markdown 文件。
 */
export const exportAsMarkdown = async (content: string, filename: string): Promise<void> => {
  await exportViaBackend(content, 'markdown', filename);
};

/**
 * 导出为 PDF 文件。
 */
export const exportAsPDF = async (content: string, filename: string): Promise<void> => {
  await exportViaBackend(content, 'pdf', filename);
};

/**
 * 导出为 Word DOCX 文件。
 */
export const exportAsWord = async (content: string, filename: string): Promise<void> => {
  await exportViaBackend(content, 'docx', filename);
};

/**
 * 导出为 XLSX 文件。
 */
export const exportAsXLSX = async (content: string, filename: string): Promise<void> => {
  await exportViaBackend(content, 'xlsx', filename);
};

export const appendChatDocumentEvidenceAppendix = (
  content: string,
  artifact?: ChatDocumentArtifactExportPayload | null,
): string => {
  const normalizedContent = typeof content === 'string' ? content.trim() : '';
  if (!normalizedContent) {
    return '';
  }
  if (normalizedContent.includes(EVIDENCE_APPENDIX_HEADING)) {
    return normalizedContent;
  }

  const evidenceRefs = normalizeEvidenceRefs(artifact?.evidence_refs);
  const evidenceSummary = normalizeEvidenceSummary(artifact?.evidence_summary);
  if (evidenceRefs.length === 0 && !evidenceSummary) {
    return normalizedContent;
  }

  const appendixSections = [EVIDENCE_APPENDIX_HEADING];
  if (evidenceSummary) {
    appendixSections.push(buildEvidenceSummaryBlock(evidenceSummary));
  }
  if (evidenceRefs.length > 0) {
    appendixSections.push(buildEvidenceDetailsBlock(evidenceRefs));
  }

  return `${normalizedContent}\n\n${appendixSections.filter(Boolean).join('\n\n')}`.trim();
};

const normalizeExportError = (error: any): Error => {
  const message = error?.error?.message || error?.message || 'Backend export failed';
  const requestId = error?.error?.details?.request_id || error?.details?.request_id;
  if (!requestId) {
    return new Error(message);
  }
  return new Error(`${message} (Request ID: ${requestId})`);
};

const extractFilenameFromDisposition = (contentDisposition?: string): string | null => {
  if (!contentDisposition) {
    return null;
  }

  const utf8Match = contentDisposition.match(/filename\*=UTF-8''([^;]+)/i);
  if (utf8Match?.[1]) {
    try {
      return decodeURIComponent(utf8Match[1]);
    } catch {
      return utf8Match[1];
    }
  }

  const plainMatch = contentDisposition.match(/filename="?([^";]+)"?/i);
  return plainMatch?.[1] || null;
};

const buildFallbackFilename = (prefix: string, format: ExportFormat): string => {
  const now = new Date();
  const pad = (value: number) => String(value).padStart(2, '0');
  const timestamp = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}_${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`;
  return `${prefix}_${timestamp}.${EXPORT_EXTENSION_MAP[format]}`;
};

const normalizeEvidenceRefs = (refs?: ChatDocumentEvidenceRef[] | null): ChatDocumentEvidenceRef[] => {
  if (!Array.isArray(refs) || refs.length === 0) {
    return [];
  }
  const uniqueRefs = new Map<string, ChatDocumentEvidenceRef>();
  refs.forEach((ref) => {
    if (!ref || typeof ref !== 'object') {
      return;
    }
    const normalized: ChatDocumentEvidenceRef = {
      query: normalizeEvidenceString(ref.query),
      section_heading: normalizeEvidenceString(ref.section_heading),
      knowledge_base_id: normalizeEvidenceString(ref.knowledge_base_id),
      knowledge_id: normalizeEvidenceString(ref.knowledge_id),
      chunk_id: normalizeEvidenceString(ref.chunk_id),
      source_title: normalizeEvidenceString(ref.source_title),
      score: typeof ref.score === 'number' ? ref.score : undefined,
      content_checksum: normalizeEvidenceString(ref.content_checksum),
    };
    if (!normalized.knowledge_base_id && !normalized.knowledge_id && !normalized.chunk_id && !normalized.query) {
      return;
    }
    const key = [
      normalized.section_heading,
      normalized.query,
      normalized.knowledge_base_id,
      normalized.knowledge_id,
      normalized.chunk_id,
      normalized.content_checksum,
    ].join('|');
    if (!uniqueRefs.has(key)) {
      uniqueRefs.set(key, normalized);
    }
  });
  return Array.from(uniqueRefs.values());
};

const normalizeEvidenceSummary = (summary?: ChatDocumentEvidenceSummary | null): ChatDocumentEvidenceSummary | null => {
  if (!summary || typeof summary !== 'object') {
    return null;
  }
  return {
    ref_count: normalizeCount(summary.ref_count),
    knowledge_base_count: normalizeCount(summary.knowledge_base_count),
    knowledge_count: normalizeCount(summary.knowledge_count),
    chunk_count: normalizeCount(summary.chunk_count),
    sources: Array.isArray(summary.sources)
      ? summary.sources
          .map((source) => ({
            knowledge_base_id: normalizeEvidenceString(source?.knowledge_base_id),
            knowledge_id: normalizeEvidenceString(source?.knowledge_id),
            source_title: normalizeEvidenceString(source?.source_title),
            chunk_count: normalizeCount(source?.chunk_count),
            max_score: typeof source?.max_score === 'number' ? source.max_score : undefined,
          }))
          .filter((source) => source.source_title || source.knowledge_id || source.knowledge_base_id)
      : [],
  };
};

const buildEvidenceSummaryBlock = (summary: ChatDocumentEvidenceSummary): string => {
  const lines = ['摘要：'];
  const refCount = normalizeCount(summary.ref_count);
  const knowledgeBaseCount = normalizeCount(summary.knowledge_base_count);
  const knowledgeCount = normalizeCount(summary.knowledge_count);
  const chunkCount = normalizeCount(summary.chunk_count);
  lines.push(`- 引用条目数：${refCount}`);
  lines.push(`- 涉及知识库数：${knowledgeBaseCount}`);
  lines.push(`- 涉及文档数：${knowledgeCount}`);
  lines.push(`- 涉及 Chunk 数：${chunkCount}`);
  if (summary.sources && summary.sources.length > 0) {
    lines.push('- 主要来源：');
    summary.sources.slice(0, 5).forEach((source, index) => {
      const label = source.source_title || source.knowledge_id || source.knowledge_base_id || `来源 ${index + 1}`;
      const chunkCountLabel = normalizeCount(source.chunk_count);
      const scoreLabel = typeof source.max_score === 'number' ? `，最高分 ${source.max_score.toFixed(3)}` : '';
      lines.push(`  ${index + 1}. ${label}（Chunk ${chunkCountLabel}${scoreLabel}）`);
    });
  }
  return lines.join('\n');
};

const buildEvidenceDetailsBlock = (refs: ChatDocumentEvidenceRef[]): string => {
  const lines = ['明细：'];
  refs.forEach((ref, index) => {
    const sourceTitle = ref.source_title || ref.knowledge_id || ref.chunk_id || `引用 ${index + 1}`;
    lines.push(`${index + 1}. 来源：${sourceTitle}`);
    if (ref.section_heading) {
      lines.push(`   章节：${ref.section_heading}`);
    }
    if (ref.query) {
      lines.push(`   检索问题：${ref.query}`);
    }
    if (ref.knowledge_base_id) {
      lines.push(`   知识库 ID：${ref.knowledge_base_id}`);
    }
    if (ref.knowledge_id) {
      lines.push(`   文档 ID：${ref.knowledge_id}`);
    }
    if (ref.chunk_id) {
      lines.push(`   Chunk ID：${ref.chunk_id}`);
    }
    if (typeof ref.score === 'number') {
      lines.push(`   相关度：${ref.score.toFixed(3)}`);
    }
  });
  return lines.join('\n');
};

const normalizeEvidenceString = (value: unknown): string => {
  return typeof value === 'string' ? value.trim() : '';
};

const normalizeCount = (value: unknown): number => {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
};
