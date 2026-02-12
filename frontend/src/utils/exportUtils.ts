import { marked } from 'marked';
import * as XLSX from 'xlsx';
import { postBlob, get } from './request';

/**
 * 生成带时间戳的文件名
 */
export const generateFilename = (prefix?: string): string => {
  const now = new Date();
  const pad = (n: number) => String(n).padStart(2, '0');
  const timestamp = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}_${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`;
  return `${prefix || '对话导出'}_${timestamp}`;
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

/**
 * 将 Markdown 转换为带样式的 HTML 字符串（前端版本，用于前端降级）
 */
export const markdownToStyledHTML = (markdown: string): string => {
  const rawHTML = marked.parse(markdown) as string;
  return `
    <html>
    <head>
      <meta charset="utf-8">
      <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, "PingFang SC", "Microsoft YaHei", sans-serif;
               line-height: 1.6; padding: 20px; max-width: 800px; margin: 0 auto; color: #333; }
        h1, h2, h3, h4, h5, h6 { margin-top: 1.2em; margin-bottom: 0.6em; }
        code { background: #f5f5f5; padding: 2px 6px; border-radius: 3px; font-size: 0.9em; }
        pre { background: #f5f5f5; padding: 16px; border-radius: 6px; overflow-x: auto; }
        pre code { background: none; padding: 0; }
        blockquote { border-left: 4px solid #ddd; margin: 1em 0; padding: 0.5em 1em; color: #666; }
        table { border-collapse: collapse; width: 100%; margin: 1em 0; }
        th, td { border: 1px solid #ddd; padding: 8px 12px; text-align: left; }
        th { background: #f5f5f5; font-weight: 600; }
        img { max-width: 100%; }
        ul, ol { padding-left: 2em; }
        a { color: #0066cc; }
      </style>
    </head>
    <body>${rawHTML}</body>
    </html>
  `;
};

/**
 * 导出为 Markdown 文件 (.md)
 */
export const exportAsMarkdown = (content: string, filename: string): void => {
  const blob = new Blob([content], { type: 'text/markdown;charset=utf-8' });
  triggerDownload(blob, `${filename}.md`);
};

// ============================================================
// 后端导出能力缓存
// ============================================================
let _capabilities: { pdf: boolean; docx: boolean } | null = null;

/**
 * 查询后端导出能力（是否安装 wkhtmltopdf / pandoc）
 * 结果会被缓存，只请求一次
 */
export const getExportCapabilities = async (): Promise<{ pdf: boolean; docx: boolean }> => {
  if (_capabilities) return _capabilities;
  try {
    const res = await get('/api/v1/export/capabilities');
    const data = (res as any).data?.data || (res as any).data;
    _capabilities = {
      pdf: data?.pdf?.available === true,
      docx: data?.docx?.available === true,
    };
  } catch {
    _capabilities = { pdf: false, docx: false };
  }
  return _capabilities;
};

/**
 * 通过后端 API 导出文档（PDF / DOCX）
 * 后端使用 wkhtmltopdf（PDF）或 pandoc（DOCX）生成高质量文档
 */
const exportViaBackend = async (
  content: string,
  format: 'pdf' | 'docx',
  filename: string,
): Promise<void> => {
  const res = await postBlob('/api/v1/export/document', {
    content,
    format,
    filename_prefix: filename,
  });

  // axios interceptor 的 success handler 已返回 response.data，
  // 所以 res 直接就是 Blob（responseType: "blob" 时）。
  // 如果 res 不是 Blob（例如 interceptor 返回了完整 response），则取 .data。
  const rawData = res instanceof Blob ? res : (res as any).data || res;

  // 检查是否实际拿到了有效的二进制数据
  if (!(rawData instanceof Blob) || rawData.size === 0) {
    throw new Error('Backend returned empty or invalid response');
  }

  // 如果后端返回了 JSON 错误（Content-Type 为 application/json），则解析并抛出
  if (rawData.type && rawData.type.includes('application/json')) {
    const text = await rawData.text();
    let errMsg = 'Backend export failed';
    try {
      const json = JSON.parse(text);
      errMsg = json?.error?.message || json?.message || errMsg;
    } catch { /* ignore parse error */ }
    throw new Error(errMsg);
  }

  const blob = new Blob([rawData], {
    type: format === 'pdf'
      ? 'application/pdf'
      : 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  });

  const ext = format === 'pdf' ? 'pdf' : 'docx';
  triggerDownload(blob, `${filename}.${ext}`);
};

/**
 * 前端降级：使用 html2pdf.js 生成 PDF（当后端 wkhtmltopdf 不可用时）
 */
const exportAsPDFFallback = async (content: string, filename: string): Promise<void> => {
  const html2pdf = (await import('html2pdf.js')).default;
  const styledHTML = markdownToStyledHTML(content);

  const container = document.createElement('div');
  container.innerHTML = styledHTML;
  container.style.position = 'absolute';
  container.style.left = '-9999px';
  container.style.width = '800px';
  document.body.appendChild(container);

  try {
    await html2pdf()
      .set({
        margin: [15, 15, 15, 15],
        filename: `${filename}.pdf`,
        image: { type: 'jpeg', quality: 0.95 },
        html2canvas: {
          scale: 2,
          useCORS: true,
          logging: false,
        },
        jsPDF: {
          unit: 'mm',
          format: 'a4',
          orientation: 'portrait',
        },
        pagebreak: { mode: ['avoid-all', 'css', 'legacy'] },
      } as any)
      .from(container)
      .save();
  } finally {
    document.body.removeChild(container);
  }
};

/**
 * 前端降级：使用 html-docx-js-typescript 生成 DOCX（当后端 pandoc 不可用时）
 */
const exportAsWordFallback = async (content: string, filename: string): Promise<void> => {
  const { asBlob } = await import('html-docx-js-typescript');
  const styledHTML = markdownToStyledHTML(content);
  const result = asBlob(styledHTML);
  // asBlob may return a Blob or an ArrayBuffer depending on the version;
  // ensure we always pass a proper Blob to triggerDownload.
  const docxBlob = result instanceof Blob
    ? result
    : new Blob([result], {
        type: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
      });
  triggerDownload(docxBlob, `${filename}.docx`);
};

/**
 * 导出为 PDF 文件
 * 优先使用后端 wkhtmltopdf（高质量），失败时降级到前端 html2pdf.js
 */
export const exportAsPDF = async (content: string, filename: string): Promise<void> => {
  try {
    await exportViaBackend(content, 'pdf', filename);
  } catch (err) {
    console.warn('[Export] Backend PDF export failed, falling back to frontend html2pdf.js:', err);
    await exportAsPDFFallback(content, filename);
  }
};

/**
 * 导出为 Word DOCX 文件
 * 优先使用后端 pandoc（高质量），失败时降级到前端 html-docx-js-typescript
 */
export const exportAsWord = async (content: string, filename: string): Promise<void> => {
  try {
    await exportViaBackend(content, 'docx', filename);
  } catch (err) {
    console.warn('[Export] Backend DOCX export failed, falling back to frontend html-docx-js-typescript:', err);
    await exportAsWordFallback(content, filename);
  }
};

/**
 * 从 Markdown 文本中提取表格数据
 */
const extractTablesFromMarkdown = (content: string): string[][][] => {
  const tables: string[][][] = [];
  const lines = content.split('\n');
  let currentTable: string[][] = [];
  let inTable = false;

  for (const line of lines) {
    const trimmed = line.trim();
    if (trimmed.startsWith('|') && trimmed.endsWith('|')) {
      // 跳过分隔行 (|---|---|)
      const inner = trimmed.slice(1, -1);
      if (/^[\s\-:|]+$/.test(inner)) {
        continue;
      }
      const cells = inner.split('|').map(cell => cell.trim());
      currentTable.push(cells);
      inTable = true;
    } else {
      if (inTable && currentTable.length > 0) {
        tables.push(currentTable);
        currentTable = [];
      }
      inTable = false;
    }
  }
  if (currentTable.length > 0) {
    tables.push(currentTable);
  }
  return tables;
};

/**
 * 导出为 XLSX 文件
 */
export const exportAsXLSX = (content: string, filename: string): void => {
  const workbook = XLSX.utils.book_new();
  const tables = extractTablesFromMarkdown(content);

  if (tables.length > 0) {
    tables.forEach((table, index) => {
      const worksheet = XLSX.utils.aoa_to_sheet(table);
      const colWidths = table[0]?.map((_, colIdx) => {
        const maxLen = Math.max(
          ...table.map(row => (row[colIdx] || '').length)
        );
        return { wch: Math.min(Math.max(maxLen + 2, 10), 50) };
      });
      if (colWidths) worksheet['!cols'] = colWidths;

      const sheetName = tables.length === 1 ? 'Sheet1' : `表格${index + 1}`;
      XLSX.utils.book_append_sheet(workbook, worksheet, sheetName);
    });
  } else {
    const rows = content.split('\n').map(line => [line]);
    const worksheet = XLSX.utils.aoa_to_sheet([['内容'], ...rows]);
    worksheet['!cols'] = [{ wch: 80 }];
    XLSX.utils.book_append_sheet(workbook, worksheet, 'Sheet1');
  }

  XLSX.writeFile(workbook, `${filename}.xlsx`);
};
