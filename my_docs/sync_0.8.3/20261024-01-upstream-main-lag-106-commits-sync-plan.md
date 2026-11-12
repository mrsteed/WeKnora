# 20261024 上游 main 落后 106 提交同步计划（第三轮）

## 1. 文档目标

本文档针对 `WeKnora-sync_v0.8.3_base` 仓库当前分支 `sync_v0.8.3_base`，量化其相对上游
`origin/main` 的落后/领先情况，并给出一份**可直接执行的同步计划**。

背景：本仓库已完成两轮工作——

1. 第一轮：把私有基线从 `v0.7.2` 提升到 `v0.8.3`（见目录内 `20261016-01` ~ `20261016-12`）。
2. 第二轮：在 `v0.8.3` 基线上沉淀本地业务改动（i18n、docreader、wiki、my_docs 等）。

本轮（第三轮）要解决的问题：上游 `main` 在 `v0.8.3`（2026-10-14）之后又推进了 106 个提交，
当前仓库需要决策并落地如何把这些上游演进同步进来，同时不丢失已沉淀的本地业务改动。

> 本文档**只做“盘点 + 计划”**，不实际执行合并/回迁。真正执行时以本文为总控，配合各阶段
> checklist 落地。

---

## 2. 结论速览（TL;DR）

| 指标 | 数值 | 说明 |
| --- | --- | --- |
| 当前分支 | `sync_v0.8.3_base` | HEAD `33aee4e1f`（2026-10-24） |
| 上游同步目标 | `origin/main` | HEAD `b445d3bd13`（2026-10-21） |
| 分叉点（merge-base） | `3e22822c1d` | `v0.8.3` finalize（2026-10-14） |
| **落后上游** | **106 个提交** | `git rev-list --count HEAD..origin/main` |
| **本地领先** | **8 个提交 / 428 个文件** | 本地业务沉淀 |
| **双方都改的冲突候选文件** | **64 个** | merge/rebase 时的冲突热点 |
| 上游是否发新版本 | **否** | 无 tag、无 v0.8.4，仅 v0.8.3 后的持续迭代 |
| 上游 106 提交构成 | 57 fix / 30 feat / 其余 chore、docs、test | 修复为主，含部分新功能 |
| `my_docs` 是否受影响 | 否 | 42 个已被跟踪、纯本地文档，与上游无交集，不会冲突 |

**一句话判断**：这是“在 v0.8.3 基线之上，把上游 v0.8.3 之后的 106 个增量提交合并进来”的
任务；由于本地已在 428 个文件上做了业务沉淀、且与上游 64 个文件重叠，合并有**确定的冲突与
回归风险**，不宜一把梭，应按“先隔离本地、分域合并、逐域验证”的节奏推进。

---

## 3. 同步现状盘点（数据依据）

### 3.1 三方的位置关系

```text
 v0.8.3 (分叉点 3e22822, 2026-10-14)
   |
   |  上游继续演进 106 个提交（fix 57 / feat 30 / 其余）
   v
 origin/main = b445d3bd13 (2026-10-21, Merge 'Tencent:main' into main)

 v0.8.3 (同一个分叉点)
   |
   |  本地业务沉淀 8 个提交 / 428 个文件
   v
 sync_v0.8.3_base (HEAD) = 33aee4e1f (2026-10-24)
```

两条线在 `v0.8.3` 处分叉后各自前进，本次同步即是把“上游线”并进“本地线”。

### 3.2 本地领先的 8 个提交（必须完整保留）

```text
33aee4e1f docs: 新增 20261023-01 图片解析与对话使用链路分析
3b218940a feat(frontend): 上传进度新增 正在解析/入库中/上传失败/已删除 状态文案(6 语言)
fb6798882 fix(wiki): 索引页占位内容按产品主语言中文呈现
f34e7b54b chore(frontend): 移除跳转到 weknora.weixin.qq.com 的文档外链
98f577242 feat(frontend): 文档预览支持一键下载，补充工具箱技能调研文档
aebda2dae fix(frontend): 修复会话行菜单被侧栏拖拽遮挡，下线成员/组织管理中的添加类入口
3eb9ed6ed docs(my_docs): 补充 v0.8.3 同步过程文档与历史方案报告
955e17ce5 chore(v0.8.3): 固化 v0.8.3 同步基线，修复全量测试
```

涉及 428 个文件。其中 `my_docs`、`docs`、`artifacts/bsk-gw` 等是**纯本地**目录/文件，与上游
无交集，合并时不会产生冲突、也应原样保留。

### 3.3 上游 106 个提交的改动面

按一级目录统计（文件数）：

| 目录 | 文件数 | 说明 |
| --- | --- | --- |
| internal | 765 | 后端业务主体（handler/service/router/middleware/types/...) |
| frontend | 282 | 前端 Vue + i18n + api + views |
| migrations | 58 | 数据库迁移脚本（**高风险**，注意与本地 schema 兼容） |
| website-docs | 40 | 网站文档 |
| docreader | 16 | 文档解析（本地也改过，需注意） |
| docs | 10 | 项目文档 |
| client | 10 | 客户端 |
| cli | 5 | 命令行 |
| third_party / helm / config | 3/3/3 | 打包/部署/配置 |
| 其它根文件 | 各 1 | go.mod、go.sum、docker-compose、Makefile、README 等 |
| **合计** | **1208** | |

### 3.4 冲突热点：双方都改的 64 个文件

以下文件在上游 106 提交与本地 8 提交里**都被修改**，是合并时最可能产生冲突、最需要人工判断
的地方。按域归类：

**配置 / 部署（4）**
- `config/config.yaml`
- `config/prompt_templates/agent_system_prompt.yaml`
- `docker-compose.yml`
- `docker/Dockerfile.app`

**Go 依赖（2）**
- `go.mod`
- `go.sum`

**前端（27）**
- `frontend/vite.config.ts`
- `frontend/src/router/index.ts`
- `frontend/src/stores/menu.ts`
- `frontend/src/config/settingsAccess.ts`
- `frontend/src/config/settingsRoute.test.ts`
- i18n 六语言：`frontend/src/i18n/locales/{zh-CN,zh-TW,en-US,ja-JP,ko-KR,ru-RU}.ts`
- `frontend/src/api/agent/index.ts`
- `frontend/src/api/knowledge-base/index.ts`
- `frontend/src/components/{doc-content,document-preview,manual-knowledge-editor,menu,ModelEditorDialog}.vue`
- `frontend/src/views/platform/index.vue`
- `frontend/src/views/agent/{AgentEditorModal,AgentList}.vue`
- `frontend/src/views/chat/components/botmsg.vue`
- `frontend/src/views/knowledge/components/{DocumentListView,FAQEntryManager,KbFolderTree}.vue`

**后端 internal（31）**
- `internal/container/container.go`（**装配核心，重点**）
- `internal/router/{rbac,routes_auth_tenant,routes_knowledge}.go`（**装配核心，重点**）
- `internal/handler/{auth,datasource,embed_channel,im,knowledgebase,mcp_endpoint,system,weknoracloud,wiki_page}.go`
- `internal/handler/session/{sandbox_desktop_ws,stream}.go`
- `internal/im/service.go`
- `internal/middleware/{embed_auth,embed_auth_test,mcp_endpoint_auth,mcp_endpoint_auth_test}.go`
- `internal/types/{context_clone,context_helpers,custom_agent,knowledgebase,user}.go`
- `internal/application/service/{session_knowledge_qa,session_qa_helpers}.go`
- `internal/application/service/*_test.go`（datasource/mcp/model_delete/tenant_sandbox_config）
- `internal/database/migration_sqlite_versioned_schema_test.go`

---

## 4. 关键决策点：同步到什么深度？

必须先明确意图，否则后面的做法完全不同。有三种可选口径：

### 口径 A：全量合并（吸收全部 106 提交）
- **含义**：`merge origin/main`，把上游 v0.8.3 之后的所有 fix + feat 全部纳入，仓库实质升
  级到“v0.8.3 + 106 增量”的形态。
- **适用**：希望持续跟进上游、愿意处理全部新功能带来的行为变化与回归面。
- **代价**：30 个 feat 会引入新行为（assistant / agent / sandbox / automation 等），验证面
  最大；64 个冲突文件全部要处理；migrations 58 个文件要逐个核对是否与本地 schema 冲突。

### 口径 B：只吸收 fix（停留在 v0.8.3 基线，挑 bug 修复）
- **含义**：从 106 提交里按 `fix(` 前缀挑选 57 个修复，`cherry-pick` 进本分支，**不引入新
  功能**。
- **适用**：本仓库定位是“稳定的 v0.8.3 定制版”，不想被动吃进上游新特性，只要更稳定。
- **代价**：57 个 cherry-pick 里若有依赖某 feat 的 fix，会 cherry-pick 失败或引入半套逻辑，
  需要逐个甄别；冲突文件仍要处理（只是范围可能更小）。

### 口径 C：按需求挑选（最细粒度）
- **含义**：只挑与本仓库实际业务相关的提交（例如与 docreader、wiki、knowledge、i18n 相关的
  fix），其余不碰。
- **适用**：上游功能与本定制版无关、只想“补洞”的场景。
- **代价**：甄别成本最高，且要人工判断每个提交是否安全独立。

> **推荐**：默认走**口径 A（全量 merge）**，原因——
> 1. `sync_v0.8.3_base` 分支名含 “base”，语义就是“跟随上游主线做基线”，长期停在 v0.8.3 只吃
>    fix 会与上游越拉越远，后续更难合并。
> 2. merge 相对 57 次 cherry-pick 更整体、冲突只解析一次，历史也更干净。
> 3. 上游 fix 占一半以上，本就是演进主体，一并吸收更合理。
>
> 若本仓库的诉求是“绝对稳定、不接受任何新特性”，则改用口径 B。执行前请与需求方确认口径，
> 本计划后续阶段以**口径 A** 为主线编排，并在需要处标注口径 B 的差异点。

---

## 5. 总体策略：先隔离本地，再分域合并

核心原则（沿用第二轮同步硬约束的精神，并针对本轮调整）：

1. **文件级真相源仍是 upstream。** 判断某个上游已存在文件“应该长什么样”，看
   `git show origin/main:<path>`；本地只在“业务语义”层面做最小保留。
2. **先给本地改动上保险。** 合并前先把当前 `sync_v0.8.3_base` 打一个 tag 或备份分支，
   万一合并失败可一键回退。
3. **my_docs / docs / artifacts 等纯本地目录不动。** 它们与上游无交集，合并天然保留。
4. **重点人工文件**：`container.go`、`router/*.go`、`config.yaml`、`go.mod`、六语言 i18n、
   `docker-compose.yml`。这些是装配/配置核心，冲突解决要逐个 hunk 判断，禁止整文件覆盖。
5. **migrations 单独一道工序**：58 个迁移文件要核对是否与本地已落库的 schema 版本冲突，
   不能在合并完成后才发现 DB 迁移冲突。

---

## 6. 分阶段执行计划（以口径 A 为主线）

> 每个阶段结束都产出一个可回退的提交点，并做该阶段的验证，再进入下一阶段。所有命令默认在
> `WeKnora-sync_v0.8.3_base` 仓库根执行（`cd "$HOME/workspace/WeKnora-sync_v0.8.3_base"`）。

### Phase 0：保护与快照（必须最先做）
1. 确认工作树干净：`git status`（当前已 clean）。
2. 备份当前头：`git branch sync_v0.8.3_base_backup_$(date +%Ymd) HEAD`
   或 `git tag backup/v0.8.3+local 33aee4e1f`。
3. 再次确认上游最新：`git fetch origin main` 并核对 `origin/main` 仍为 `b445d3bd13`。
   若上游又前进了，刷新第 2 节的数字（重跑 3.1 的统计命令）。
4. 产出：一个可随时 `git reset --hard` 回去的干净备份点。

### Phase 1：试合并（不落地，只看冲突面）
1. 用 `--no-commit --no-ff` 试合并：
   `git merge --no-commit --no-ff origin/main`
2. `git status` + `git diff --name-only --diff-filter=U` 拿到**真实冲突文件清单**
   （注意：真实冲突数 ≤ 64，多数“双方都改”的文件 git 能自动合并，只有真正重叠 hunk 才冲突）。
3. 把真实冲突文件清单存档到本目录，作为后续逐文件处理的依据。
4. **本阶段不提交**，看完冲突面后 `git merge --abort` 回到干净状态。
5. 产出：一份“真实冲突文件清单 + 每个文件冲突块数”的执行底稿。

### Phase 2：正式合并 + 冲突逐域解决
1. `git merge origin/main` 触发真合并。
2. 按域分批解冲突（顺序建议：先装配核心，后业务，最后前端）：
   - **批 1（装配/依赖）**：`go.mod`、`go.sum`、`container.go`、`router/*.go` 三件
     装配核心 + `config.yaml`、`docker-compose.yml`、`Dockerfile.app`。
     原则：以上游 `origin/main` 为底，仅最小化保留本地业务差异，禁止整文件覆盖。
   - **批 2（后端业务/handler/middleware/types）**：`internal/handler/*`、`im/service.go`、
     `middleware/*`、`types/*`、`application/service/*`。逐文件看冲突，业务语义优先本地、
     基础设施跟上游。
   - **批 3（前端）**：`vite.config.ts`、`router/index.ts`、`stores/menu.ts`、六语言 i18n、
     `api/*`、`components/*`、`views/*`。i18n 六语言注意“双方都加了 key”的情形，逐 key 合并。
   - **批 4（测试文件）**：`*_test.go` 冲突通常“两边都保留 + 去重”。
3. 每解完一批：`git add <该批文件>`，然后 `git build`/`go build ./...` 冒烟一次（见 Phase 3）。
4. 全部解完后 `git diff --check` 确认无语法级冲突残留。
5. 产出：一个合并后的待验证提交点（此时**先不 push**）。

### Phase 3：编译与迁移校验
1. 后端编译：`go build ./...`（含 `cli`、`docreader` 相关 Go 部分按仓库实际）。
2. 前端安装与构建：按仓库既有方式 `pnpm install && pnpm build`（见 `20261029-02-生产模式编译
   启动与运维命令方案.md`）。
3. **数据库迁移专项**：重点核对 `migrations/` 下新增/变更脚本与本地已落库版本是否冲突，
   必要时本地库做一次干净迁移演练。
4. 冒烟：本地起服务，跑核心链路（登录、知识库问答、agent、wiki）确认可用。
5. 产出：编译/构建/迁移/冒烟四项全绿的验证结论。

### Phase 4：全量回归与收尾
1. 跑仓库既有测试集（`go test ./...` 或仓库约定命令）+ 前端单测。
2. 对照第 2 节 8 个本地提交，**逐项确认本地业务改动仍在**（i18n 文案、docreader 修复、
   wiki 中文占位、一键下载、菜单遮挡修复、my_docs 全在）。
3. 复核 64 个冲突文件相对 `origin/main` 的 diff，确认没有因解冲突丢掉上游无关能力。
4. 更新 `CHANGELOG.md` / 在 `my_docs/sync_0.8.3/` 新增本阶段的**执行报告**（沿用既有编号风格）。
5. 完成后由用户决定是否 `git push`（BeadForge/WeKnora 仓库均遵循“不主动 commit/push”，
   合并提交可先保留本地，push 由用户拍板）。
6. 产出：同步执行报告 + 可上线的分支状态。

---

## 7. 命令速查

```bash
# 定位
cd "$HOME/workspace/WeKnora-sync_v0.8.3_base"
git fetch origin main
MB=$(git merge-base HEAD origin/main)

# 统计 落后/领先
git rev-list --left-right --count HEAD...origin/main   # 输出: 本地领先数 落后数

# 上游 106 提交明细（按类型筛）
git log --oneline $MB..origin/main
git log --oneline $MB..origin/main | grep -cE "^[0-9a-f]+ fix\("
git log --oneline $MB..origin/main | grep -cE "^[0-9a-f]+ feat\("

# 本地 8 提交明细
git log --oneline origin/main..HEAD

# 双方都改的文件（冲突候选）
git diff --name-only $MB origin/main | sort > /tmp/up.txt
git diff --name-only $MB HEAD        | sort > /tmp/lo.txt
comm -12 /tmp/lo.txt /tmp/up.txt

# 备份
git branch sync_v0.8.3_base_backup_$(date +%Ymd) HEAD

# 试合并（只看冲突，不落地）
git merge --no-commit --no-ff origin/main
git diff --name-only --diff-filter=U
git merge --abort

# 真合并
git merge origin/main
# ... 逐域解冲突 ...
git add -A && git diff --check
```


---

## 8. 风险清单与回滚

### 8.1 主要风险

| 风险 | 等级 | 应对 |
| --- | --- | --- |
| 装配核心（container/router）解冲突引入装配错配，服务起不来 | 高 | 以上游为底最小化本地保留；每批 `go build` 冒烟；起不来即回退该批 |
| migrations 58 文件与本地已落库 schema 冲突 | 高 | Phase 3 单独做迁移演练；不通过不上线 |
| 六语言 i18n 双方都加 key，漏合导致某语言缺 key / 构建报错 | 中 | 按 key 逐一合并；前端 build + i18n 单测兜底 |
| 30 个 feat 引入新行为，触发本地已固化流程回归 | 中 | Phase 4 全量回归 + 对照 8 个本地提交逐项确认；若不接受新特性改走口径 B |
| go.mod/go.sum 版本漂移导致依赖不一致 | 中 | 以后上游为底 `go mod tidy` 核对 |
| 误把整文件上游内容直接 checkout，丢掉本地业务 | 中 | 硬约束 #4：装配/配置文件禁止整文件覆盖 |
| 合到一半被打断，现场不干净 | 低 | 每批单独 `git add` + 提交点；`git merge --abort` 可随时回 Phase 1 之前的备份 |

### 8.2 回滚

- 合并前：`git reset --hard sync_v0.8.3_base_backup_<date>` 即可回到本次计划开始前的状态。
- 合并中：`git merge --abort` 回到合并前。
- 合并后未 push：`git reset --hard <备份点>`；已 push：用备份点打 tag 回滚发布。

---

## 9. 本轮硬约束（高于一切临时经验）

1. **upstream `origin/main` 是文件级真相源。** 上游已存在的文件冲突，先看
   `git show origin/main:<path>` 确定上游目标形态，本地只在业务语义层做最小保留。
2. **装配/基础设施文件禁止整文件覆盖**，重点：`internal/container/container.go`、
   `internal/router/router.go`、`internal/router/routes_*.go`、`frontend/src/App.vue`、
   `config/config.yaml`、`docker-compose.yml`、`go.mod`。
3. **本地 8 个提交的业务语义必须完整保留**：六语言上传状态文案、wiki 中文占位、文档一键
   下载、会话行菜单遮挡修复、下线成员/组织“添加类入口”、`my_docs` 与 `docs` 全部文档。
4. **只允许在 Phase 4 全部验证通过后结束**；任何一项（编译/构建/迁移/回归）不绿不得收工。
5. **不主动 commit/push。** 合并产生的提交与 push 时机由用户拍板；本计划默认合并提交留在
   本地，验证通过后交用户决定。
6. **执行前确认口径（A/B/C）。** 若口径与本文默认（A 全量合并）不同，先修订第 6 节再执行。

---

## 10. 关联文档

- 第一轮同步（v0.7.2 → v0.8.3）总计划与执行报告：本目录 `20261016-01` ~ `20261016-12`。
- 生产模式编译/启动与运维命令：`my_docs/20260929-02-生产模式编译启动与运维命令方案.md`。
- 开发模式运行方式：`my_docs/20260929-01-开发模式运行方式与项目结构梳理.md`。
- 本轮执行后应新增：`my_docs/sync_0.8.3/20261024-0x-...-execution-report.md`。

---

## 11. 待用户确认项（执行前必须拍板）

1. **同步口径**：A 全量合并（推荐）/ B 只挑 fix / C 按需挑选。
2. **是否接受 30 个 feat 的新行为**（决定 A 还是 B）。
3. **合并后测试范围**：是否要跑完整 `go test ./...` + 前端全量单测，还是核心链路冒烟即可。
4. **push 时机**：合并提交是否随验证通过即 push 到 `origin/sync_v0.8.3_base`。
