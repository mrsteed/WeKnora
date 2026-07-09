# Markdown 导出 PDF / Word 实现一致性审查报告

日期：2026-07-08

## 1. 审查范围

本次审查以 `my_docs/pdf导出.md` 为目标方案，核对以下实际实现：

- `internal/utils/export/export.go`
- `internal/utils/export/markdown.go`
- `internal/utils/export/policy.go`
- `internal/handler/export.go`
- `docker/Dockerfile.app`
- `docker-compose.yml`
- `frontend/src/utils/exportUtils.ts`

同时在当前工作区主机上做了最小验证：

- `command -v pandoc` 无输出
- `command -v chromium` 无输出
- `go test ./internal/utils/export` 可通过编译

结论先行：当前实现与方案文档“方向一致、细节不全”，已经具备基础导出能力，但还没有达到文档中定义的“效果最好”目标。尤其是 Word 模板、公式渲染、页眉页脚、并发治理和安全收敛仍有明显缺口。

---

## 2. 总体结论

### 2.1 一致的部分

1. 已有统一后端导出接口，支持 `markdown / pdf / docx / xlsx`。
2. PDF 目前确实采用“Markdown 先转 HTML，再交给 Chromium 输出 PDF”的主链路。
3. Word 目前确实采用 Pandoc 输出 DOCX。
4. 后端已提供能力探测与超时控制。
5. 容器镜像 `docker/Dockerfile.app` 已预装 `chromium`、`pandoc` 和中文字体，容器部署具备基础运行条件。

### 2.2 不一致或未完成的部分

1. PDF 渲染未实现数学公式支持，和文档中要求的 KaTeX 不一致。
2. PDF 未实现页眉、页脚、页码、文档标题、生成时间注入。
3. PDF 未实现“等待 KaTeX / JS 渲染完成后再输出”。
4. Word 未接入 `reference.docx` 模板，当前输出质量受 Pandoc 默认样式限制。
5. 未实现 Chromium / Pandoc 转换并发限流。
6. Docker 镜像未补齐文档提到的 TeX 备用链路。
7. 当前宿主机没有 `pandoc` / `chromium`，若直接在宿主机运行服务，则 PDF 与 DOCX 能力会被判定不可用。
8. 导出路径缺少自动化测试，效果回归主要依赖人工验证。

### 2.3 综合判断

如果标准只是“能导出”，当前实现基本成立。

如果标准是 `my_docs/pdf导出.md` 中定义的“Markdown 导出 PDF 和 Word 格式效果最好”，当前实现不能算一致，建议按本报告第 6 节分阶段修正。

---

## 3. 逐项对照审查

| 目标项 | 文档要求 | 当前实现 | 一致性 | 影响 |
|---|---|---|---|---|
| PDF 主链路 | Markdown → HTML → Chromium → PDF | 已实现 | 部分一致 | 主方向正确 |
| Markdown 渲染 | Goldmark + GFM + 数学公式 + 语法高亮 | 仅 Goldmark + GFM + highlighting | 部分一致 | 公式缺失，代码展示一般 |
| PDF HTML 载入方式 | 写临时 HTML 文件后打印 | 直接使用 data URL 导航 | 可接受偏差 | 相对资源、调试、模板扩展受限 |
| 打印样式 | 打印优化 CSS、本地字体、分页控制 | 已有基础 CSS，无本地字体绑定、分页规则有限 | 部分一致 | 中文和长文档细节不稳定 |
| 页眉页脚 | 标题 / 页码 / 生成时间 | 未实现 | 不一致 | 正式文档观感不足 |
| KaTeX 渲染等待 | 等待 JS 渲染完成 | 未实现 | 不一致 | 公式未来接入后会有空白风险 |
| Word 主链路 | Pandoc + reference.docx | 仅 Pandoc | 不一致 | DOCX 样式质量不达标 |
| Word 后处理 | 元数据 / 封面可选后处理 | 未实现 | 可接受缺失 | 不是阻断项 |
| 并发控制 | Chromium / Pandoc 需限流 | 未实现 | 不一致 | 并发高时 CPU / 内存抖动 |
| 超时控制 | 建议 30 秒级 timeout | 已实现 | 一致 | 基础健壮性具备 |
| 部署依赖 | Chromium / Pandoc / TeX / CJK 字体 | 容器有 Chromium / Pandoc / 字体，无 TeX；宿主机缺失 | 部分一致 | 宿主机不可直接用，备用链路不完整 |
| 测试保障 | 至少要有回归验证 | 未见专门测试 | 不一致 | 后续改动易回退 |

---

## 4. 关键发现

### P0-1. 当前宿主机缺少核心依赖，直接运行服务时 PDF / DOCX 导出不可用

证据：

1. 当前主机执行 `command -v pandoc`、`command -v chromium` 均无输出。
2. `internal/utils/export/policy.go` 会在缺少依赖时将 PDF / DOCX 标记为 unavailable。
3. `internal/utils/export/export.go` 会在找不到 Chromium 或 Pandoc 时直接返回错误。

影响：

1. 如果服务运行在宿主机而不是 `docker-compose.yml` 的 app 容器内，导出能力会直接失效。
2. 前端菜单虽然有能力探测，但最终仍取决于后端运行环境。

结论：这不是代码逻辑错误，但属于部署一致性缺口，必须先补齐依赖。

### P0-2. PDF 导出链路存在安全收敛不足：允许原始 HTML，且 Chromium 以 `--no-sandbox` 运行

证据：

1. `internal/utils/export/markdown.go` 使用 `html.WithUnsafe()`，会保留 Markdown 中的原始 HTML。
2. `internal/utils/export/export.go` 通过 Chromium headless 渲染，并显式启用 `--no-sandbox`。
3. 导出接口接收的是客户端直接提交的 Markdown 文本，而不是仅服务端受控模板。

影响：

1. 导出内容中若含脚本、远程资源或恶意 HTML，Chromium 可能在导出阶段执行这些内容。
2. 这会把“文档导出”变成一个潜在的浏览器执行面。

建议：

1. 默认关闭原始 HTML 透传，或在进入 Chromium 前做白名单 sanitization。
2. 如必须保留数学公式和少量内联 HTML，改为“受控标签白名单 + 本地静态资源”。
3. 评估是否能在容器内去掉 `--no-sandbox`，至少不要在宿主机裸跑时沿用该参数。

### P1-1. PDF 渲染没有实现数学公式支持，不满足目标方案

证据：

1. `my_docs/pdf导出.md` 明确要求数学公式使用 KaTeX。
2. `internal/utils/export/markdown.go` 仅启用了 `extension.GFM` 和 code highlighting，没有数学扩展，也没有注入 KaTeX JS/CSS。
3. 当前 PDF 模板中不存在 KaTeX 资源和渲染完成标记。

影响：

1. 含 `$...$` 或 `$$...$$` 的 Markdown 无法得到高质量公式排版。
2. 对技术文档、算法报告、方案文档的 PDF 效果影响明显。

### P1-2. PDF 缺少页眉页脚、页码和生成时间，正式文档观感不足

证据：

1. 方案文档要求通过 Chromium header / footer template 注入这些信息。
2. `internal/utils/export/export.go` 当前 `page.PrintToPDF()` 只设置了背景和边距，没有启用 `DisplayHeaderFooter`，也没有模板。

影响：

1. 打印版缺少页码和文档上下文。
2. 长文档归档、对外流转、留痕管理体验较弱。

### P1-3. Word 导出未接入 `reference.docx`，输出质量不会达到“最好”

证据：

1. 方案文档将 `reference.docx` 视为 Word 输出质量的关键。
2. 仓库中没有 `reference.docx` 文件。
3. `internal/utils/export/export.go` 调用 Pandoc 时没有传 `--reference-doc`。

影响：

1. 标题、正文、中英文字体、代码块、表格、页眉页脚都只能依赖 Pandoc 默认样式。
2. 生成的 DOCX 在企业交付或正式材料场景下可读性和品牌一致性不足。

### P1-4. 没有并发限流，导出高峰时容易拖垮实例

证据：

1. 方案文档明确要求 Chromium 和 Pandoc 做并发控制。
2. `internal/utils/export/export.go` 未见 semaphore、队列或并发上限控制。
3. 当前仅依赖请求超时，无法控制瞬时高并发资源竞争。

影响：

1. 多个 PDF 并发导出会集中消耗 CPU、内存和共享字体缓存。
2. Pandoc 与 Chromium 都是重进程，容易造成尾延迟和 OOM 风险。

### P2-1. Docker 镜像只满足基础导出，不满足方案中的备用依赖要求

证据：

1. `docker/Dockerfile.app` 已安装 `chromium`、`pandoc`、`fonts-noto-cjk` 等。
2. 但未安装 `texlive-xetex`、`texlive-lang-chinese` 等文档中提到的 TeX 备用链路。

影响：

1. 若后续 Pandoc 模板、复杂公式或应急转换链路需要 TeX，本镜像仍需补包。
2. 当前虽然不阻塞最基础 docx 输出，但与目标文档不完全一致。

### P2-2. 现有实现选择 `chromedp + data URL`，方向可用，但扩展性弱于临时 HTML 文件方案

证据：

1. 方案文档建议写临时 HTML 再调用 Chromium 打印。
2. 当前实现将完整 HTML 转成 base64 data URL，再由 Chromium 导航。

影响：

1. 如果后续要引入本地 KaTeX 静态资源、字体文件、页脚模板调试、截图排查，data URL 方式可维护性较差。
2. 这不是必须立即修复的问题，但若要追求“最佳效果”，建议切到临时 HTML 文件方案。

### P2-3. 缺少导出自动化测试，后续修正后容易回退

证据：

1. 未检索到 `internal/utils/export` 相关专门测试文件。
2. 当前只验证到 `go test ./internal/utils/export` 能编译。

影响：

1. 页眉、分页、公式、模板样式等改动几乎没有回归保护。
2. 未来升级 Chromium、Pandoc 或 CSS 时风险较高。

---

## 5. 关于“当前代码是否与文档一致”的明确判断

### 5.1 可以认定为一致的部分

1. 采用 Goldmark 做 Markdown 渲染。
2. 采用 Chromium 生成 PDF。
3. 采用 Pandoc 生成 DOCX。
4. 通过后端统一接口完成导出。

### 5.2 只能认定为“部分一致”的部分

1. PDF 样式已做，但仍是基础打印样式，不是方案文档描述的成品级打印模板。
2. 部署镜像具备基础依赖，但不是完整依赖闭环。
3. 超时已实现，但并发治理未实现。

### 5.3 不能认定为一致的部分

1. KaTeX 公式支持。
2. PDF 页眉页脚和页码。
3. Word 的 `reference.docx` 样式模板。
4. 受控的本地字体引用和更稳定的资源组织。
5. 并发限流。

最终判断：当前实现是“可用版”，不是“最佳效果版”。

---

## 6. 可行的修正计划

建议按四个阶段推进，避免一次性重构过大。

### 阶段 1：先补齐运行依赖与模板资产

目标：保证宿主机、容器、开发环境都能稳定跑通导出。

建议动作：

1. 宿主机安装依赖：执行 `my_docs/install_markdown_export_deps.sh`。
2. Docker 镜像补充：在 `docker/Dockerfile.app` 增加 `texlive-xetex`、`texlive-lang-chinese`、`texlive-fonts-recommended`。
3. 新增目录 `config/export/`，放置：
   - `reference.docx`
   - `pdf.css`
   - 可选的本地 KaTeX 资源目录
4. 新增环境变量：
   - `WEKNORA_EXPORT_REFERENCE_DOCX`
   - `WEKNORA_EXPORT_PDF_STYLE_PATH`
   - `WEKNORA_EXPORT_PDF_CONCURRENCY`
   - `WEKNORA_EXPORT_DOCX_CONCURRENCY`

预期结果：

1. 本机和容器都能稳定探测到 PDF / DOCX 能力。
2. 后续样式和模板不再硬编码在 Go 字符串里。

### 阶段 2：提升 PDF 效果并收紧安全边界

目标：让 PDF 达到正式交付质量，同时降低 Chromium 渲染风险。

建议动作：

1. 将当前 data URL 方案改成“临时 HTML 文件 + 临时资源目录”。
2. 将打印 CSS 外置到 `config/export/pdf.css`，避免长字符串维护困难。
3. 增加数学公式支持：
   - 方案 A：Goldmark 增加数学扩展，输出受控 math 节点，再由本地 KaTeX 资源渲染。
   - 方案 B：直接在 HTML 模板中注入本地 KaTeX 静态资源，并在渲染后写入 `window.__WEKNORA_EXPORT_READY__ = true`。
4. `page.PrintToPDF()` 启用 header / footer：
   - 标题
   - 页码
   - 生成时间
5. 增加分页规则：
   - 标题后避免立刻分页
   - 代码块、表格、图片尽量避免跨页切断
6. 处理安全：
   - 默认关闭任意原始 HTML
   - 或在进入 Chromium 前使用白名单清洗 HTML
   - 评估容器内移除 `--no-sandbox`

预期结果：

1. 中文、代码块、表格、公式、页码、页脚都可控。
2. PDF 输出接近文档定义的目标形态。

### 阶段 3：提升 DOCX 效果

目标：让 Word 输出具备正式样式，而不是 Pandoc 默认文档。

建议动作：

1. 制作 `reference.docx`，建议统一以下样式：
   - 正文中文：宋体或微软雅黑
   - 正文英文：Calibri
   - 一级到三级标题：字号、颜色、段前段后统一
   - 代码块：等宽字体、灰底、边框
   - 表格：表头底色、边框线、分页重复表头
   - 页眉页脚：标题、页码
2. Pandoc 调用增加：
   - `--reference-doc <path>`
   - `--metadata title=<title>`
   - `--metadata date=<date>`
3. 若要补封面、审批信息或文档属性，再考虑在 Pandoc 输出后做轻量后处理。

预期结果：

1. DOCX 输出观感稳定。
2. Word 样式从“默认生成”升级到“可交付文档”。

### 阶段 4：并发治理与验证体系

目标：把功能从“能用”提升到“可运维、可回归”。

建议动作：

1. 为 PDF / DOCX 增加独立信号量限流。
2. 并发上限默认值建议：
   - PDF：`max(1, CPU 核数 / 2)`
   - DOCX：`max(1, CPU 核数 / 2)` 或固定 2
3. 增加测试：
   - HTML 结构快照测试
   - 能力探测测试
   - `reference.docx` 路径存在性测试
   - 集成测试：在装有 Chromium / Pandoc 的环境跑一组标准样例
4. 增加人工验收样例：
   - 中文标题与长段落
   - 多页代码块
   - 宽表格
   - 图片
   - 行内公式与块级公式

预期结果：

1. 高并发下更稳。
2. 后续升级更容易发现回退。

---

## 7. 缺失依赖安装方案

### 7.1 当前宿主机缺失项

当前工作区主机缺少至少以下命令：

- `pandoc`
- `chromium`
- `xelatex`

### 7.2 已输出的安装脚本

已新增脚本：`my_docs/install_markdown_export_deps.sh`

作用：

1. 在 Ubuntu / Debian 上安装 Markdown 导出 PDF / Word 所需基础依赖。
2. 安装完成后输出各命令版本，便于核验。

安装范围：

- `chromium`
- `pandoc`
- `fonts-noto-cjk`
- `fonts-noto-cjk-extra`
- `fonts-liberation`
- `texlive-xetex`
- `texlive-lang-chinese`
- `texlive-fonts-recommended`

说明：这些包里，前 2 个是当前代码直接依赖，后面的字体和 TeX 包用于中文效果和备用链路。

### 7.3 容器镜像建议补包

建议将 `docker/Dockerfile.app` 中的安装列表补充为至少包含：

- `texlive-xetex`
- `texlive-lang-chinese`
- `texlive-fonts-recommended`

这样可保证容器环境与方案文档一致，不需要再依赖宿主机状态。

---

## 8. 建议执行顺序

如果你要尽快把效果提上去，建议按下面顺序做：

1. 先补宿主机和容器依赖。
2. 再补 `reference.docx` 和 `pdf.css` 这两个资产。
3. 然后修 PDF：页眉页脚、公式、分页、安全。
4. 再修 DOCX：模板、元数据。
5. 最后加限流和测试。

原因：

1. 依赖不齐，后面所有调样式都没法稳定验证。
2. `reference.docx` 和 `pdf.css` 是效果提升的收益最大项。
3. 并发与测试应在功能基本定型后一起补，避免反复改。

---

## 9. 最终结论

当前代码实现与 `my_docs/pdf导出.md` 不是完全一致，而是“主链路一致、最佳效果相关能力缺失较多”。

更准确地说：

1. PDF 和 DOCX 已经“能导出”。
2. 但 PDF 还没有达到公式、页脚、分页、正式排版都完善的程度。
3. DOCX 还没有达到模板化、品牌化、正式交付文档的程度。
4. 若目标是“效果最好”，下一步应优先补 `reference.docx`、PDF 打印模板、公式支持和并发治理。

本报告建议作为后续修正实施的基准版本。