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
