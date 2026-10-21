# 20261016 WeKnora 0.7.2 -> 0.8.3 总执行计划

## 1. 文档目标

本文档基于 `my_docs/sync_0.8.3/` 目录下现有五份同步文档，整理出一份**可直接执行的总计划**，用于统一指导后续从 `v0.7.2` 同步到 `v0.8.3` 的完整实施过程。

这份文档的职责不是替代原始文档，而是把它们收敛成一条连续执行路线，并且在每个阶段明确：

1. 当前阶段要做什么。
2. 为什么这个阶段必须先做或后做。
3. 这一阶段应以哪份文档为主参考。
4. 这一阶段还需要搭配哪些补充文档一起使用。
5. 阶段完成后产出什么，再进入下一阶段。

---

## 2. 源文档清单

本总计划最初基于以下五份规划文档整理，后续又叠加了执行报告作为实际依赖修正依据：

1. [my_docs/sync_0.8.3/20261016-01-sync-plan-0.7.2-to-0.8.3.md](my_docs/sync_0.8.3/20261016-01-sync-plan-0.7.2-to-0.8.3.md)
用途：升级背景、版本差异、总路线、风险判断、阶段划分。

2. [my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md](my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md)
用途：模块拆分、模块依赖、同步手法、M0-M7 模块边界。

3. [my_docs/sync_0.8.3/20261016-03-sync-M0-M2-checklist.md](my_docs/sync_0.8.3/20261016-03-sync-M0-M2-checklist.md)
用途：M0-M2 分支名、建议命令、验证命令、回并顺序。

4. [my_docs/sync_0.8.3/20261016-04-sync-M3-M4-checklist.md](my_docs/sync_0.8.3/20261016-04-sync-M3-M4-checklist.md)
用途：M3-M4 分支名、建议命令、验证命令、回并顺序。

5. [my_docs/sync_0.8.3/20261016-05-sync-M5-M7-checklist.md](my_docs/sync_0.8.3/20261016-05-sync-M5-M7-checklist.md)
用途：M5-M7 分支名、建议命令、验证命令、试运行与回滚演练。

补充执行报告：

6. [my_docs/sync_0.8.3/20261016-07-phase0-execution-report.md](my_docs/sync_0.8.3/20261016-07-phase0-execution-report.md)
用途：记录 Phase 0 的 compose 阻塞点与 Lite fallback 结论。

7. [my_docs/sync_0.8.3/20261016-08-phase1-execution-report.md](my_docs/sync_0.8.3/20261016-08-phase1-execution-report.md)
用途：记录 Phase 1 已恢复的品牌、运维脚本、导出资源与前端构建修复。

8. [my_docs/sync_0.8.3/20261016-09-phase2-execution-report.md](my_docs/sync_0.8.3/20261016-09-phase2-execution-report.md)
用途：记录 Phase 2 的实际依赖判断，确认 M2-B 需等待 org-tree substrate。

9. [my_docs/sync_0.8.3/20261016-10-phase3-substrate-execution-report.md](my_docs/sync_0.8.3/20261016-10-phase3-substrate-execution-report.md)
用途：记录转入 M3 后，org-tree substrate 的实际恢复范围、兼容修复与当前后端阻塞点。

说明：

1. `07`-`10` 号文件属于**第一次尝试的历史执行报告**。
2. 它们只用于复盘依赖关系、阻塞点和踩坑记录。
3. 它们**不是**文件级回迁来源；与上游 `v0.8.3` 文件内容冲突时，一律以上游 `v0.8.3` 为准。

---

## 3. 使用方式

建议把这份文档当成**总控台文档**使用：

1. 先读本文件，确认全局阶段顺序和依赖关系。
2. 真正执行到某个阶段时，再打开对应 checklist 文档，按分支与命令逐项落地。
3. 若某一阶段出现问题，先回到该阶段的主参考文档定位，不要直接在后续阶段临时补救。

一句话原则：

1. 本文负责“总路线”。
2. 对应 checklist 负责“怎么做”。

### 3.1 第二轮同步硬约束（2026-10-16 修订）

以下约束在再次同步时高于所有旧执行报告、临时经验和私有分支现状：

1. **文件级真相源只有 upstream `v0.8.3`。** 判断某个已存在文件“应该长什么样”时，先看 `git show v0.8.3:<path>`。
2. **`0.7.2` / 私有分支代码只作为“业务差异来源”，不是整文件覆盖源。** 只允许按 hunk 吸收明确的本地业务语义。
3. **禁止对上游已存在的装配/基础设施文件做整文件 checkout、整段覆盖或目录级复制。** 重点包括：
	- `internal/container/container.go`
	- `internal/router/router.go`
	- `internal/router/routes_*.go`
	- `internal/container/*.go`
	- `frontend/src/App.vue`
	- 全局 store / 全局入口文件
4. **如果上游 `v0.8.3` 已有同类能力，但当前工作树缺失或签名漂移，先恢复到 `v0.8.3` 形态，再最小化补本地差异。** 不允许平行新增“兼容文件”替代上游文件组织。
5. **只有在上游 `v0.8.3` 真正不存在某个本地业务文件/能力时，才允许新增文件。**
6. 每个模块开始前，先把目标文件分三类：
	- 上游装配文件：只做最小补丁。
	- 上游业务文件：以 `v0.8.3` 为底按 hunk 吸收本地语义。
	- 上游缺失文件：确认确属本地业务所需后再新增。
7. 每批修改完成后，必须复核该批文件相对 `v0.8.3` 的 diff，确认没有因为整文件覆盖丢掉上游无关能力。
8. 若某文件出现的偏移无法直接解释为 `0.7.2` 本地业务语义差异，则先恢复或重建为 `v0.8.3` 版本，再重做最小化回迁。

---

## 4. 总体执行原则

本次同步必须坚持以下原则：

1. 不把当前私有线直接 merge/rebase 到上游 `v0.8.3`。
2. 以 `v0.8.3` 作为新底座，再按模块回迁本地能力。
3. 先环境与基线，后入口与权限，再知识体验，最后 Agent/MCP/Sandbox、导出解析与试运行。
4. 上游已有成熟实现的能力优先复用，本地只补业务语义差异。
5. 每阶段结束前，必须产出可验证的状态，不允许跨阶段带着“半成品”前进。
6. 当本节与 `3.1 第二轮同步硬约束` 或任何旧执行报告冲突时，以 `3.1` 为准。

这些原则主要来自：

1. [my_docs/sync_0.8.3/20261016-01-sync-plan-0.7.2-to-0.8.3.md](my_docs/sync_0.8.3/20261016-01-sync-plan-0.7.2-to-0.8.3.md) 中的总路线与风险判断。
2. [my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md](my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md) 中的模块依赖与同步手法。

---

## 5. 总体阶段总览

| 阶段 | 模块 | 核心目标 | 主参考文档 | 补充参考文档 | 进入条件 | 退出条件 |
|---|---|---|---|---|---|---|
| Phase 0 | M0 | 落地上游 0.8.3 干净基线 | 03 | 01, 02 | 当前同步准备完毕 | 0.8.3 可迁移、可启动、可健康检查 |
| Phase 1 | M1 | 恢复品牌、部署、运维、导出资源等低冲突覆盖层 | 03 | 02 | M0 通过 | 品牌与运维链路恢复，不引入业务语义改动 |
| Phase 2 | M3 | 先带回 org-tree substrate，再恢复作用域、可见性、组织树与 IM 归属主线 | 04 | 01, 02, 09, 10 | M1 通过 | org-tree substrate 可编译可验证；权限主线稳定，无跨账号/跨创建者越权 |
| Phase 3 | M2 | 在 M3 完成后回填登录、空间初始化、创建门禁与 orgTree cache 卫生 | 03 | 01, 02, 09, 10 | M3 通过 | 登录入口稳定；WorkspaceSetupWizard/root org reuse 可用；创建门禁与 orgTree cache 卫生闭环 |
| Phase 4 | M4 | 恢复知识库目录、下载、来源定位、上传人等体验能力 | 04 | 01, 02 | M2 通过 | 知识库体验能力恢复且服从 M3 权限模型，并与 M2 初始化语义一致 |
| Phase 5 | M5 | 稳定 Agent、Model Catalog、MCP、Sandbox 策略 | 05 | 02 | M4 通过 | Agent/MCP/Sandbox 以 0.8.3 骨架稳定运行 |
| Phase 6 | M6 | 恢复导出链路、验证 anydoc/MinerU/docreader 解析链路 | 05 | 02 | M5 通过 | 导出与解析链路稳定且与知识引用一致 |
| Phase 7 | M7 | 全量回归、试运行、备份、切换、回滚演练 | 05 | 01, 02 | M6 通过 | 得到可上线/可回滚的最终版本 |

文档编号说明：

1. `01` = 总计划。
2. `02` = 模块化策略。
3. `03` = M0-M2 checklist。
4. `04` = M3-M4 checklist。
5. `05` = M5-M7 checklist。
6. `09` = Phase 2 执行报告（已确认 M2-B 对 org-tree substrate 的依赖）。

### 5.1 实际依赖修正（2026-10-16）

第一次执行后已确认：

1. `M2` 作为一个完整模块，并不能稳定地先于 `M3` 执行。
2. `M2-B`（bootstrap / WorkspaceSetupWizard / 根组织复用）并不是单独的入口层补丁。
3. 它依赖一整套当前上游 `v0.8.3` 基线中不存在的 org-tree substrate，包括：
	- `internal/handler/org_tree.go`
	- `internal/application/service/org_tree.go`
	- `frontend/src/api/org-tree/`
	- `frontend/src/stores/orgTree.ts`
	- `frontend/src/components/WorkspaceSetupWizard.vue`
4. `M2-C` 中依赖 orgTree store 的 cache 卫生也同样受这一底座约束。

因此第二轮同步的正式执行顺序修正为：

1. `M0 -> M1` 保持不变。
2. **先执行 `M3`，带回 org-tree substrate 与权限主线。**
3. 待 `M3` 稳定后，再统一执行 `M2`，完成登录、初始化向导、root org reuse、创建门禁与 orgTree cache 卫生。
4. 然后进入 `M4 -> M5 -> M6 -> M7`。

说明：

1. 第一次执行中验证过 `M2-A` 和 `M2-C` 有一部分可以独立落地。
2. 但为了防止第二轮再次因为模块边界判断失真而破坏 `v0.8.3` 基线，本总计划不再把这些“可独立子块”作为正式顺序的一部分。
3. 第二轮总控顺序按模块主依赖关系收敛为：`M0 -> M1 -> M3 -> M2 -> M4 -> M5 -> M6 -> M7`。

---

## 6. Phase 0：M0 上游 0.8.3 干净基线

### 6.1 本阶段目标

先证明纯上游 `v0.8.3` 在当前环境中可迁移、可启动、可通过最小健康检查。

### 6.2 为什么必须先做

如果 M0 不成立，后续所有本地回迁都会失去基准，届时无法判断问题来自：

1. 上游 0.8.3 本身。
2. 本地环境配置。
3. 私有改动回迁。

### 6.3 主参考文档

主参考：

1. [my_docs/sync_0.8.3/20261016-03-sync-M0-M2-checklist.md](my_docs/sync_0.8.3/20261016-03-sync-M0-M2-checklist.md)

补充参考：

1. [my_docs/sync_0.8.3/20261016-01-sync-plan-0.7.2-to-0.8.3.md](my_docs/sync_0.8.3/20261016-01-sync-plan-0.7.2-to-0.8.3.md)
2. [my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md](my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md)

### 6.4 本阶段执行重点

1. 建立 `upstream` 只读 remote。
2. 从 upstream `v0.8.3` **直接创建新的独立执行 worktree** `sync_v0.8.3_base`，旧 worktree / 旧阶段分支只删除不复用。
3. 准备 `.env`，保留现网 `SYSTEM_AES_KEY`，补齐 `SYSTEM_SIGNING_KEY` 与 `JWT_SECRET`。
4. 按上游路径启动 `docker compose`。
5. 跑 `make migrate-up` 与最小健康检查。

### 6.5 本阶段产出

1. 可启动的 `sync_v0.8.3_base`。
2. 环境变量差异表。
3. 迁移执行记录。

### 6.6 进入下一阶段的条件

1. `health`、`system/capabilities`、`system/info` 可访问。
2. 迁移无阻塞性失败。
3. 上游基线问题与本地环境问题已经初步分离。

---

## 7. Phase 1：M1 品牌、部署、运维、导出资源

### 7.1 本阶段目标

在不引入业务语义冲突的前提下，恢复私有品牌、运维脚本、部署覆盖层和导出资源挂载。

### 7.2 为什么排在 M0 后、M3 前

1. M0 先保证上游基线成立。
2. M1 再恢复环境与展示层，减少后续阶段排错噪音。
3. 第二轮正式顺序中，M3 先于 M2 执行，因此 M1 结束后直接进入 M3 的 substrate 与权限主线恢复。

### 7.3 主参考文档

主参考：

1. [my_docs/sync_0.8.3/20261016-03-sync-M0-M2-checklist.md](my_docs/sync_0.8.3/20261016-03-sync-M0-M2-checklist.md)

补充参考：

1. [my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md](my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md)

### 7.4 本阶段执行重点

1. 子分支 A：品牌层，仅迁移 logo、标题、登录页视觉资源和品牌文案。
2. 子分支 B：部署/运维脚本层，恢复 dev/prod 覆盖脚本、前端代理、MinIO 端口等适配。
3. 子分支 C：导出资源层，只恢复 `config/export` 资源与挂载，不恢复完整导出逻辑。

### 7.5 本阶段产出

1. `sync_v0.8.3_stage1_platform`。
2. 可用的品牌与脚本覆盖层。
3. 可供 M6 使用的导出资源基础。

### 7.6 进入下一阶段的条件

1. 环境与脚本稳定。
2. 未引入权限和业务语义回归。
3. 后续 M3/M2 阶段都不再依赖旧分支脚本临时兜底。

---

## 8. Phase 2：M3 组织树、RBAC、可见性、IM 归属主线

### 8.1 本阶段目标

恢复本地私有线最核心的业务差异：先带回 org-tree substrate，再继续同租户作用域解析、KB/Agent 可见性、组织树管理与 IM 渠道创建者隔离。

### 8.2 为什么排在 M1 后、M2 前

1. 第一次执行已经证明：完整的 `M2` 模块依赖 `M3` 底座，不能稳定前置。
2. `M3` 是权限主线与组织树底座。
3. `M2` 中的初始化向导、root org reuse、orgTree cache 卫生都要建立在这个底座之上。

### 8.3 主参考文档

主参考：

1. [my_docs/sync_0.8.3/20261016-04-sync-M3-M4-checklist.md](my_docs/sync_0.8.3/20261016-04-sync-M3-M4-checklist.md)

补充参考：

1. [my_docs/sync_0.8.3/20261016-01-sync-plan-0.7.2-to-0.8.3.md](my_docs/sync_0.8.3/20261016-01-sync-plan-0.7.2-to-0.8.3.md)
2. [my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md](my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md)
3. [my_docs/sync_0.8.3/20261016-09-phase2-execution-report.md](my_docs/sync_0.8.3/20261016-09-phase2-execution-report.md)
4. [my_docs/sync_0.8.3/20261016-10-phase3-substrate-execution-report.md](my_docs/sync_0.8.3/20261016-10-phase3-substrate-execution-report.md)

### 8.4 本阶段执行重点

0. **先行批次：org-tree substrate**。
1. 子分支 A：same-tenant scope core。
2. 子分支 B：KB/智能体可见性。
3. 子分支 C：组织树与成员管理。
4. 子分支 D：IM 渠道归属与隔离。

### 8.5 本阶段产出

1. `sync_v0.8.3_stage3_scope`。
2. org-tree substrate 已恢复为可编译、可验证的底座。
3. 收口的同租户作用域授权层。
4. 角色、组织、资源、创建者边界稳定的权限模型。

### 8.6 进入下一阶段的条件

1. org-tree substrate 已恢复到“可编译、可做窄测试、前端可 type-check”的状态。
2. KB/Agent 可见范围与页面表现一致。
3. 组织树多角色视图一致。
4. IM 渠道按创建者隔离。
5. 到这一步才允许开始执行完整的 `M2`。

---

## 9. Phase 3：M2 登录、空间初始化、创建门禁

### 9.1 本阶段目标

在 `M3` 已完成的前提下，恢复本地账号登录习惯、空间初始化路径和错误入口门禁，并完成依赖 org-tree 的剩余闭环。

### 9.2 为什么排在 M3 后、M4 前

1. 第一次执行已经证明：初始化向导、root org reuse 和 orgTree cache 卫生依赖 `M3` 的 substrate 与权限主线。
2. 如果先做 `M2` 再做 `M3`，会再次诱发对上游装配文件的错误覆盖。
3. `M4` 的目录、下载和来源定位又依赖 `M2` 的初始化/入口语义与 `M3` 的权限模型，因此 `M2` 应位于 `M3` 与 `M4` 之间。

### 9.3 主参考文档

主参考：

1. [my_docs/sync_0.8.3/20261016-03-sync-M0-M2-checklist.md](my_docs/sync_0.8.3/20261016-03-sync-M0-M2-checklist.md)

补充参考：

1. [my_docs/sync_0.8.3/20261016-01-sync-plan-0.7.2-to-0.8.3.md](my_docs/sync_0.8.3/20261016-01-sync-plan-0.7.2-to-0.8.3.md)
2. [my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md](my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md)
3. [my_docs/sync_0.8.3/20261016-09-phase2-execution-report.md](my_docs/sync_0.8.3/20261016-09-phase2-execution-report.md)
4. [my_docs/sync_0.8.3/20261016-10-phase3-substrate-execution-report.md](my_docs/sync_0.8.3/20261016-10-phase3-substrate-execution-report.md)

### 9.4 本阶段执行重点

1. 子分支 A：登录 identifier 化，恢复“用户名或邮箱登录”。
2. 子分支 B：bootstrap 与初始化向导，支持冲突恢复、根组织复用、结构化 409 错误。
3. 子分支 C：创建门禁与跨账号 orgTree cache 卫生。

### 9.4.1 第二轮执行说明

1. 第一次执行中，`M2-A` 与 `M2-C` 的一部分曾被证明可独立落地。
2. 但为了保证第二轮严格服从依赖关系并减少对 `v0.8.3` 装配文件的误伤，第二轮总计划不再要求把这些内容前置到 `M3` 之前。
3. 第二轮中，`M2` 统一在 `M3` 稳定后执行。

### 9.5 本阶段产出

1. `sync_v0.8.3_stage2_auth`。
2. 可稳定使用的登录入口。
3. 可重试、可恢复的初始化向导。
4. root org reuse、创建门禁与 orgTree cache 卫生完成闭环。

### 9.6 进入下一阶段的条件

1. 用户名/邮箱双路登录可用。
2. WorkspaceSetupWizard、root org reuse 与结构化 409 语义可用。
3. 无模型与无权限入口被统一拦截。
4. orgTree cache 卫生已接到 `M3` 底座上。

---

## 10. Phase 4：M4 知识库目录、下载、来源定位、上传人

### 10.1 本阶段目标

恢复知识库体验能力，但严格建立在 M3 权限主线之上。

### 10.2 为什么排在 M2 后

目录、下载、移动目标、来源定位看起来像“知识库功能”，实质上都需要回答两个问题：

1. 用户是否看得见这个知识条目/目录/来源。
2. 用户是否有权移动、下载、重命名、回根目录。

这些既依赖 M3 的权限模型，也依赖 M2 中初始化与入口层语义已经闭环。

### 10.3 主参考文档

主参考：

1. [my_docs/sync_0.8.3/20261016-04-sync-M3-M4-checklist.md](my_docs/sync_0.8.3/20261016-04-sync-M3-M4-checklist.md)

补充参考：

1. [my_docs/sync_0.8.3/20261016-01-sync-plan-0.7.2-to-0.8.3.md](my_docs/sync_0.8.3/20261016-01-sync-plan-0.7.2-to-0.8.3.md)
2. [my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md](my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md)

### 10.4 本阶段执行重点

1. 子分支 A：批量下载，优先复用上游 0.8.2 主路径。
2. 子分支 B：引用来源定位，优先复用上游 0.8.3 source locator。
3. 子分支 C：目录树与空目录。
4. 子分支 D：移动目标范围与执行权限统一。
5. 子分支 E：文档创建者归属与上传人展示。

### 10.5 本阶段产出

1. `sync_v0.8.3_stage4_kb`。
2. 稳定的知识库目录与下载体验。
3. 不越权的来源定位与移动目标规则。

### 10.6 进入下一阶段的条件

1. 批量下载可用。
2. 来源定位不再依赖旧根目录卡片查找逻辑。
3. 空目录、回根目录、目录重命名语义稳定。
4. 上传人展示与归属链一致。

---

## 11. Phase 5：M5 Agent、Model Catalog、MCP、Sandbox

### 11.1 本阶段目标

把 Agent、工具范围、MCP、sandbox 和 model catalog 都稳定接到 0.8.3 的主骨架上。

### 11.2 为什么排在 M4 后

1. Agent 的知识范围要复用 M3 的可见性。
2. sandbox、MCP、tool scope 又会影响聊天、引用、附件、导出与解析路径。
3. 所以必须在知识和权限基础稳定后再做。

### 11.3 主参考文档

主参考：

1. [my_docs/sync_0.8.3/20261016-05-sync-M5-M7-checklist.md](my_docs/sync_0.8.3/20261016-05-sync-M5-M7-checklist.md)

补充参考：

1. [my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md](my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md)
2. [my_docs/sync_0.8.3/20261016-04-sync-M3-M4-checklist.md](my_docs/sync_0.8.3/20261016-04-sync-M3-M4-checklist.md)

### 11.4 本阶段执行重点

1. 子分支 A：Agent readiness 与模型入口稳定性。
2. 子分支 B：知识范围与默认工具语义。
3. 子分支 C：built-in MCP server 与 OAuth / tool catalog。
4. 子分支 D：sandbox 配置、绑定与运行时策略。

### 11.5 本阶段产出

1. `sync_v0.8.3_stage5_agent`。
2. 统一的 Agent 创建门禁与工具范围控制。
3. 基于 0.8.3 主骨架的 MCP 和 sandbox 运行路径。

### 11.6 进入下一阶段的条件

1. Agent 创建入口稳定。
2. 工具不会越权调用不可见知识。
3. MCP OAuth 和 tool catalog 可用。
4. sandbox 配置、绑定、检查链路稳定。

---

## 12. Phase 6：M6 导出链路、anydoc、MinerU、DocReader

### 12.1 本阶段目标

恢复导出能力，并验证/收口 anydoc 与 DocReader 解析链路，确保解析结果与知识引用、预览、下载、导出一致。

### 12.2 为什么排在 M5 后

1. 导出会依赖消息、附件、图片、引用与资源路径。
2. 解析会影响知识引用、预览、下载、聊天答案来源。
3. 这些在 M5 前还可能受 Agent/MCP/sandbox/附件策略变化影响。

### 12.3 主参考文档

主参考：

1. [my_docs/sync_0.8.3/20261016-05-sync-M5-M7-checklist.md](my_docs/sync_0.8.3/20261016-05-sync-M5-M7-checklist.md)

补充参考：

1. [my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md](my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md)
2. [my_docs/sync_0.8.3/20261016-03-sync-M0-M2-checklist.md](my_docs/sync_0.8.3/20261016-03-sync-M0-M2-checklist.md)

### 12.4 本阶段执行重点

1. 子分支 A：导出链路。
2. 子分支 B：anydoc 解析链路。
3. 子分支 C：MinerU、HTML 表格与 DocReader 解析链路。

### 12.5 本阶段产出

1. `sync_v0.8.3_stage6_export_parse`。
2. 可用的 Markdown/PDF/Word/XLSX 导出链路。
3. 已验证的 anydoc / MinerU / docreader 解析结果。

### 12.6 进入下一阶段的条件

1. 四种导出格式可用。
2. anydoc build tag 与静态库路径验证完成。
3. DocReader 解析结果不退化。
4. 导出、预览、下载、聊天引用结果一致。

---

## 13. Phase 7：M7 全量回归、试运行、备份、切换、回滚演练

### 13.1 本阶段目标

把代码从“能开发”推进到“能上线”，并形成可执行的试运行和回滚路径。

### 13.2 为什么这是最后一个阶段

M7 不负责“边试运行边补功能”。它只负责：

1. 全量自动化验证。
2. 综合手工 smoke。
3. 备份与回滚路径验证。
4. 试运行与上线前决策。

### 13.3 主参考文档

主参考：

1. [my_docs/sync_0.8.3/20261016-05-sync-M5-M7-checklist.md](my_docs/sync_0.8.3/20261016-05-sync-M5-M7-checklist.md)

补充参考：

1. [my_docs/sync_0.8.3/20261016-01-sync-plan-0.7.2-to-0.8.3.md](my_docs/sync_0.8.3/20261016-01-sync-plan-0.7.2-to-0.8.3.md)
2. [my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md](my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md)

### 13.4 本阶段执行重点

1. `sync_v0.8.3_rc1`：自动化回归与综合 smoke。
2. `sync_v0.8.3_trial`：试运行、备份、切换、回滚演练。

### 13.5 本阶段产出

1. `rc1-回归报告`。
2. `trial-试运行报告`。
3. 可执行的备份与回滚路径。

### 13.6 结束条件

1. 自动化回归通过或已明确分类例外。
2. 综合 smoke 通过。
3. 备份可恢复。
4. 回滚演练可执行。
5. 才能判断是否进入正式上线窗口。

---

## 14. 推荐的实际执行顺序

建议严格按以下顺序推进：

1. Phase 0：M0。
2. Phase 1：M1。
3. Phase 2：先做 `M3 substrate`，再做 `M3-A -> M3-B -> M3-C -> M3-D`。
4. Phase 3：在 `M3` 完成后，统一做 `M2-A -> M2-B -> M2-C`。
5. Phase 4：`M4-A -> M4-B -> M4-C -> M4-D -> M4-E`。
6. Phase 5：`M5-A -> M5-B -> M5-C -> M5-D`。
7. Phase 6：`M6-A -> M6-B -> M6-C`。
8. Phase 7：`M7-A -> M7-B`。

对应分支主线：

1. `sync_v0.8.3_base`
2. `sync_v0.8.3_stage1_platform`
3. `sync_v0.8.3_stage3_scope`
4. `sync_v0.8.3_stage2_auth`
5. `sync_v0.8.3_stage4_kb`
6. `sync_v0.8.3_stage5_agent`
7. `sync_v0.8.3_stage6_export_parse`
8. `sync_v0.8.3` -> `sync_v0.8.3_rc1` -> `sync_v0.8.3_trial`

---

## 15. 各阶段主参考文档映射表

| 执行阶段 | 主参考文档 | 补充参考文档 | 用法 |
|---|---|---|---|
| M0 | [03](my_docs/sync_0.8.3/20261016-03-sync-M0-M2-checklist.md) | [01](my_docs/sync_0.8.3/20261016-01-sync-plan-0.7.2-to-0.8.3.md), [02](my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md) | 按命令建基线，遇到路线或配置取舍时回看 01/02 |
| M1 | [03](my_docs/sync_0.8.3/20261016-03-sync-M0-M2-checklist.md) | [02](my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md) | 按 A/B/C 子分支执行，避免误碰业务语义 |
| M3 | [04](my_docs/sync_0.8.3/20261016-04-sync-M3-M4-checklist.md) | [01](my_docs/sync_0.8.3/20261016-01-sync-plan-0.7.2-to-0.8.3.md), [02](my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md), [09](my_docs/sync_0.8.3/20261016-09-phase2-execution-report.md), [10](my_docs/sync_0.8.3/20261016-10-phase3-substrate-execution-report.md) | 第二轮中优先于 M2 执行，权限主线完全以后端收口为主，严格按 04 顺序推进 |
| M2 | [03](my_docs/sync_0.8.3/20261016-03-sync-M0-M2-checklist.md) | [01](my_docs/sync_0.8.3/20261016-01-sync-plan-0.7.2-to-0.8.3.md), [02](my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md), [09](my_docs/sync_0.8.3/20261016-09-phase2-execution-report.md), [10](my_docs/sync_0.8.3/20261016-10-phase3-substrate-execution-report.md) | 在 M3 substrate 与权限主线完成后执行，入口层与初始化编排统一回填 |
| M4 | [04](my_docs/sync_0.8.3/20261016-04-sync-M3-M4-checklist.md) | [01](my_docs/sync_0.8.3/20261016-01-sync-plan-0.7.2-to-0.8.3.md), [02](my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md) | 目录/下载/来源定位严格依赖 M3 授权模型，并在 M2 完成后执行 |
| M5 | [05](my_docs/sync_0.8.3/20261016-05-sync-M5-M7-checklist.md) | [02](my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md), [04](my_docs/sync_0.8.3/20261016-04-sync-M3-M4-checklist.md) | Agent/MCP/Sandbox 以上游骨架为准，本地只补门禁和范围 |
| M6 | [05](my_docs/sync_0.8.3/20261016-05-sync-M5-M7-checklist.md) | [02](my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md), [03](my_docs/sync_0.8.3/20261016-03-sync-M0-M2-checklist.md) | 导出与解析分开走，避免互相污染 |
| M7 | [05](my_docs/sync_0.8.3/20261016-05-sync-M5-M7-checklist.md) | [01](my_docs/sync_0.8.3/20261016-01-sync-plan-0.7.2-to-0.8.3.md), [02](my_docs/sync_0.8.3/20261016-02-sync-modules-0.8.3.md) | 回归、试运行、备份和回滚演练全部以 05 为执行底稿 |

---

## 16. 建议的执行节奏

建议按以下节奏推进，而不是试图并行太多批次：

1. 每次只推进一个阶段主分支。
2. 同一个阶段内，子分支按 checklist 顺序做完一个再并回一个。
3. 每个子分支都留下验证记录。
4. 每完成一个阶段，就更新一次“阶段状态表”和“未决问题清单”。

建议维护一张执行状态表，字段至少包括：

1. 阶段名。
2. 当前分支。
3. 开始时间。
4. 当前负责人。
5. 已完成验证。
6. 未解决问题。
7. 是否允许进入下一阶段。

---

## 17. 建议的产出物清单

整个同步过程建议至少沉淀以下产物：

1. `M0 环境变量差异表`。
2. `M0 迁移执行记录`。
3. `M1 脚本与资源恢复记录`。
4. `M2 登录与初始化回归记录`。
5. `M3 权限矩阵回归记录`。
6. `M4 知识库体验回归记录`。
7. `M5 Agent/MCP/Sandbox 回归记录`。
8. `M6 导出与解析链路回归记录`。
9. `M7 rc1 回归报告`。
10. `M7 trial 试运行与回滚报告`。

---

## 18. 执行中的回退原则

如果某阶段失败，不要直接跳到后面阶段“顺手修”。建议按以下规则回退：

1. M0/M1 问题：回退到环境与脚本层解决。
2. M2 问题：回退到登录、初始化与门禁层解决。
3. M3 问题：回退到作用域解析或组织/IM 归属层解决。
4. M4 问题：先判断是否其实是 M3 权限模型不稳导致。
5. M5 问题：先判断是否其实是 M3/M4 的知识范围与授权没收好。
6. M6 问题：先拆分是导出问题还是解析问题，再回到对应子链。
7. M7 问题：只做“定位与归类”，不要在 M7 直接堆积功能性修复。

---

## 19. 结论

`sync_0.8.3` 目录中的五份文档已经分别覆盖了：

1. 升级背景与总路线。
2. 模块拆分与依赖。
3. M0-M2 执行清单。
4. M3-M4 执行清单。
5. M5-M7 执行清单。

而这份文档的作用，是把它们收束成一份**统一的总执行计划**，让后续实施时可以做到：

1. 先看总路线。
2. 再打开对应阶段的 checklist 执行。
3. 每个阶段都清楚自己到底参考哪份文档。
4. 出问题时知道该回退到哪一份文档、哪一个阶段定位。

到这里，`my_docs/sync_0.8.3/` 下已经形成了一套从“判断为什么不能硬 merge”到“如何逐阶段上线”的完整作战文档体系。