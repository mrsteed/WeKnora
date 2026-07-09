# Markdown → PDF / Word 转换方案（最佳效果）

---

## 整体思路

分两条链路分别处理 PDF 和 Word，不强求用同一个工具，因为两种格式的渲染逻辑差异较大。

---

## PDF 链路：Markdown → HTML → Chromium → PDF

**核心工具**：Go 内嵌 Goldmark 渲染 HTML，Chromium headless 打印 PDF。

**流程分四步：**

**第一步：Markdown 渲染为 HTML**
使用 Go 生态的 Goldmark 库在进程内完成渲染，支持 GFM 扩展（表格、任务列表、围栏代码块）、数学公式（KaTeX）、语法高亮（Chroma）。渲染结果是一段 HTML 片段，需要套进完整的 HTML 模板中。

**第二步：注入样式**
HTML 模板内嵌一套专为打印优化的 CSS，重点处理以下几项：
- 中文字体使用 Noto Serif CJK，代码字体使用 Noto Sans Mono，通过 `@font-face` 引用服务器本地字体文件，避免网络依赖
- 用 CSS `@media print` 控制分页行为，避免标题、代码块在页面底部被截断
- 页眉页脚通过 Chromium 的 `--header-template` 和 `--footer-template` 参数注入，支持页码、文档标题、生成时间
- 代码块保留背景色和行号，表格加边框线

**第三步：写入临时 HTML 文件**
将完整 HTML 写入服务器 `/tmp` 目录下的临时文件，文件名用 UUID 避免并发冲突。

**第四步：Chromium headless 打印**
Go 通过 `os/exec` 调用 `chromium-browser --headless --no-sandbox --print-to-pdf`，传入临时 HTML 路径和输出 PDF 路径。打印完成后删除临时 HTML 文件。

**关键配置项**：
- 纸张尺寸 A4，页边距 2cm
- 关闭背景图形裁剪，确保代码块背景色打印出来
- 使用 `--run-all-compositor-stages-before-draw` 等待 KaTeX 等 JS 渲染完成再截图

---

## Word 链路：Markdown → Pandoc → docx

**核心工具**：系统级 Pandoc，配合自定义 reference.docx 模板。

**流程分三步：**

**第一步：准备样式模板**
制作一个 `reference.docx`，在 Word 中手动调好各级标题样式、正文字体（中文宋体/微软雅黑，英文 Calibri）、代码块样式（Courier New，灰色背景）、页眉页脚。这个文件随服务器部署，Pandoc 生成的 docx 会继承所有样式定义。这一步是 Word 输出质量的关键，值得花时间打磨。

**第二步：Go 调用 Pandoc**
通过 `os/exec` 调用 Pandoc，传入 Markdown 文件路径、输出路径、`--reference-doc` 指向模板文件。Pandoc 本身对 GFM 支持完善，中文无需额外处理。

**第三步：后处理（可选）**
如果需要在 docx 里动态填充封面、元数据（文档标题、生成时间、作者），可以用 Go 的 `unioffice` 或 `go-docx` 库在 Pandoc 输出的基础上做二次写入，而不是从零构建整个文档结构。

---

## 并发与资源控制

Chromium 和 Pandoc 都是重进程，需要在 Go 层做并发限制：
- 用带缓冲的 channel 作为信号量，控制同时运行的转换进程数量（建议 Chromium 不超过 CPU 核数的一半）
- 设置 `exec.CommandContext` 超时，避免僵尸进程（建议 30 秒）
- 临时文件统一在 defer 里清理

---

## 部署依赖

Ubuntu 服务器需要预装：Chromium、Pandoc、texlive 字体包（Pandoc 的 LaTeX 引擎备用）、fonts-noto-cjk（中文字体）。这些通过 apt 安装后固定在服务器镜像中，Go 程序本身无额外依赖。

---

## 方案总结

| | PDF | Word |
|---|---|---|
| 核心工具 | Chromium headless | Pandoc + reference.docx |
| Go 集成方式 | os/exec 调用 | os/exec 调用 |
| 中文支持 | Noto CJK 字体 | 模板内置字体 |
| 样式控制 | CSS 完全可控 | 通过模板样式继承 |
| 效果瓶颈 | CSS 打印适配 | reference.docx 模板质量 |