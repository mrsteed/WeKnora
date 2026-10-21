# 20261016 WeKnora Phase 1 执行报告

## 1. 执行摘要

本轮 Phase 1 / M1 以 Phase 0 已通过的执行 worktree 为基础继续推进：

1. 执行位置：`/home/hlsa/workspace/WeKnora-sync_v0.8.3_base`
2. 基线来源：upstream `v0.8.3`
3. 同步原则：
   - 上游已有文件只做最小补丁，不做整文件覆盖。
   - 上游 `v0.8.3` 中缺失的本地覆盖层目录，按业务需要新增。
   - 不把私有线装配层整块回灌到 `docker-compose.yml`、前端入口或全局 store。

本阶段完成了三类回迁：

1. 品牌层：恢复 `IntraMind` 品牌入口，包括标题、登录页可见文案、提示词自我介绍以及登录页/侧栏首屏资源。
2. 运维/部署覆盖层：恢复 MinIO 端口默认值、`DISABLE_REGISTRATION=true`、前端 dev 代理 `8081`、pnpm 前端构建链与本地生产脚本目录。
3. 导出资源层：恢复 `config/export` 资源，并为 `app` 增加只读挂载。

结论：M1 已完成，可作为进入后续 `M3`/`M2` 的平台覆盖层基线。

---

## 2. 变更明细

### 2.1 品牌层

本轮只迁移品牌表层，不改 0.8.3 页面结构：

1. `frontend/index.html`
   - `<title>` 从 `WeKnora` 改为 `IntraMind`
   - 首页 `<meta description>` 改为 `IntraMind` 品牌描述
2. `frontend/src/views/auth/Login.vue`
   - 登录页 logo 的 `alt` 改为 `IntraMind`
3. `frontend/src/i18n/locales/en-US.ts`
   - onboarding welcome/knowledge 文案改为 `IntraMind`
   - 登录注册入口文案 `New to WeKnora?` / `Create your account and start using WeKnora` 改为 `IntraMind`
4. `frontend/src/i18n/locales/zh-CN.ts`
   - 登录注册入口文案 `首次使用 WeKnora？` / `创建账户并开始使用 WeKnora` 改为 `IntraMind`
5. `frontend/src/i18n/locales/ko-KR.ts`
   - 登录注册入口文案改为 `IntraMind`
6. `frontend/src/i18n/locales/ru-RU.ts`
   - 登录注册入口文案改为 `IntraMind`
7. 提示词品牌行：
   - `config/prompt_templates/system_prompt.yaml`
   - `config/prompt_templates/agent_system_prompt.yaml`
   - `config/prompt_templates/fallback.yaml`
   - `config/prompt_templates/intent_prompts.yaml`
   上述文件中的 assistant 自我介绍统一改为 `IntraMind`，并明确其为 `developed by HLSA based on WeKnora`。
8. 资源替换：
   - `frontend/src/assets/img/weknora.png` 用历史品牌资源替换为 `IntraMind` logo
   - `frontend/src/assets/img/screenshot-1.svg` 用历史品牌提交版本替换

说明：

1. 品牌扫描后仍保留一批 `WeKnora` 文本，这是有意保留，不属于 M1 的“品牌入口”替换范围。
2. 主要保留项包括：协议/头名/本地存储 key、`WeKnoraCloud` 产品名、embed widget API 名、README 中的 upstream 项目说明与外链。

### 2.2 运维/部署覆盖层

本轮恢复的本地环境适配如下：

1. `docker-compose.yml`
   - `DISABLE_REGISTRATION` 默认值从 `false` 改为 `true`
   - `minio` 对外映射端口默认从 `9000/9001` 改为 `9009/9010`
   - `app` 新增 `./config/export:/app/config/export:ro` 挂载
2. `docker-compose.dev.yml`
   - `minio` 对外映射端口默认从 `9000/9001` 改为 `9009/9010`
3. `scripts/dev.sh`
   - 默认 profile 从仅 `langfuse` 调整为 `minio + langfuse`
   - 本地/远程开发模式的 `MINIO_ENDPOINT` 默认端口改为 `9009`
   - 启动成功后的 MinIO 提示文案同步改为 `9009/9010`
4. `scripts/quick-dev.sh`
   - MinIO Console 提示端口改为 `9010`
5. `frontend/vite.config.ts`
   - dev proxy 默认目标从 `http://localhost:8080` 改为 `http://localhost:8081`
6. `scripts/build_frontend_dist.sh`
   - 构建链从 `npm ci && npm run build` 切换为 `pnpm install --frozen-lockfile && pnpm run build`
   - 增加 `pnpm` 缺失时的明确报错
7. `frontend/package.json`
   - `tdesign-vue-next` 固定为 `1.20.3`
   - 显式补齐 `@vue/compiler-sfc`、`@vue/server-renderer`、`@intlify/core-base`
8. `frontend/pnpm-lock.yaml`
   - 由 `pnpm install` 生成并落盘，供后续 `build_frontend_dist.sh` 使用
9. 本地生产脚本覆盖层：
   - 新增目录 `my_docs/prod_scripts/`
   - 落入 `README.md`、`compose.sh`、`create_admin.sh`、`logs.sh`、`rebuild.sh`、`start.sh`、`status.sh`、`stop.sh` 及审查报告

### 2.3 导出资源层

本轮恢复的导出资源：

1. 新增目录 `config/export/`
2. 已确认落入：
   - `config/export/pdf.css`
   - `config/export/reference.docx`
   - `config/export/katex/*`

本阶段仅恢复资源与挂载，不恢复完整导出逻辑；导出逻辑继续留在后续 `M6`。

---

## 3. 验证记录

### 3.1 品牌层验证

已通过：

1. `cd frontend && npm run type-check`
2. `cd frontend && npm run build`

说明：

1. 首次 `type-check` 暴露的是前端构建链缺口，而非品牌改动本身。
2. 按本轮 M1 运维层补齐 `pnpm` 锁文件与显式依赖后，`type-check` 与 `build` 均通过。

### 3.2 运维/导出层验证

已通过：

1. `bash -n scripts/dev.sh scripts/quick-dev.sh scripts/build_frontend_dist.sh my_docs/prod_scripts/*.sh`
2. `docker compose config >/dev/null`
3. `docker compose -f docker-compose.dev.yml config >/dev/null`
4. `make check-env`
5. `./scripts/build_frontend_dist.sh`
6. `make dev-status`

### 3.3 未执行的破坏性验证

以下命令本轮未执行：

1. `make dev-start`
2. `make dev-stop`

原因：当前宿主机已存在一套正在运行的 `WeKnora-*-dev` 容器，且这组容器名由 compose 固定。
为避免误停或重置用户现有 dev 环境，本轮只做非破坏性验证。

---

## 4. 阶段结论

M1 的完成标准已满足：

1. 品牌元素已恢复到登录页/侧栏/提示词/首屏资源等入口层。
2. 本地 deploy/dev 覆盖脚本已恢复。
3. `config/export` 资源与挂载已恢复。
4. 本轮未引入权限模型、组织树、登录标识、知识库作用域等业务语义改动。

因此可继续进入下一阶段。