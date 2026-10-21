# 20261016 WeKnora M0-M2 具体执行清单

## 1. 文档目标

本文档把 M0、M1、M2 三个模块继续细化为可以直接执行的清单，重点覆盖：

1. 推荐分支名。
2. 建议执行顺序。
3. 建议命令。
4. 验证命令。
5. 每一批完成后如何回并到阶段主分支。

本文分两类根目录：

```bash
cd "$HOME/workspace/WeKnora"
```

1. 主仓规划目录：`$HOME/workspace/WeKnora`
用途：同步文档维护、remote 管理、worktree 管理。

2. 执行 worktree：`$HOME/workspace/WeKnora-sync_v0.8.3_base`
用途：M0 起所有实际代码同步、编译、测试、运行命令。

除非命令本身是在创建/删除 worktree 或管理 remote，本文中涉及实际代码修改、编译、测试、启动的命令，都应在执行 worktree 根目录运行。

---

## 2. 执行约定

## 2.1 分支命名约定

本清单建议使用以下主分支：

1. `sync_v0.8.3_base`：M0 基线分支。
2. `sync_v0.8.3_stage1_platform`：M1 平台/品牌/运维分支。
3. `sync_v0.8.3_stage2_auth`：M2 登录/初始化/门禁分支。

M1、M2 再拆子分支：

1. `sync_v0.8.3_stage1_platform_a_brand`
2. `sync_v0.8.3_stage1_platform_b_ops`
3. `sync_v0.8.3_stage1_platform_c_export`
4. `sync_v0.8.3_stage2_auth_a_identifier`
5. `sync_v0.8.3_stage2_auth_b_bootstrap`
6. `sync_v0.8.3_stage2_auth_c_gate`

## 2.2 远端约定

当前仓库只有 `origin`，没有长期配置的 `upstream`。建议先把腾讯上游加成只读 remote：

```bash
cd "$HOME/workspace/WeKnora"
git status --short --branch
git remote get-url upstream >/dev/null 2>&1 || git remote add upstream https://github.com/Tencent/WeKnora.git
git fetch upstream --tags
git fetch upstream main
git tag -l 'v0.8.3'
git branch -r | grep 'upstream/main'
```

预期：

1. 能看到 `v0.8.3` 标签。
2. 能看到 `upstream/main`。

## 2.3 工作方式约定

建议使用以下节奏：

1. 每个子模块只做一类改动。
2. 每个子模块完成后，先跑最小验证，再回并到其阶段主分支。
3. 阶段主分支只接受已经验证通过的子分支。

回并建议统一用：

```bash
git switch <阶段主分支>
git merge --ff-only <子分支>
```

## 2.4 再次同步硬约束

1. 新一轮同步必须从 upstream `v0.8.3` **直接创建全新 worktree**，禁止复用旧 `WeKnora-sync_v0.8.3_base` 目录或其 Git 元数据。
2. `sync_v0.8.3` / `0.7.2` 私有分支只能作为差异参考，不得对上游已有文件整文件 checkout。
3. 对 `internal/container/container.go`、`internal/router/router.go`、`internal/router/routes_*.go` 等装配文件，只允许最小补丁，不允许目录级复制、大段覆盖或整文件替换。
4. 如果上游 `v0.8.3` 已经具备某能力，必须在上游文件上补语义，不能新增平行兼容文件替代上游文件组织。
5. 历史执行报告只用于复盘依赖和坑点，不得充当文件级回迁来源。

## 2.5 执行 worktree 创建约束

1. 主仓 `WeKnora` 保留为规划/管理目录，不在其上直接落代码同步改动。
2. M0 开始前先在主仓创建执行 worktree，后续 M0-M2 的实际代码命令默认在该 worktree 运行。
3. 如果后文命令仍写 `cd "$HOME/workspace/WeKnora"`，执行时应等价替换为当前执行 worktree 根目录，除非该命令本身是在管理 remote / branch / worktree。

---

## 3. M0 执行清单：上游 0.8.3 基线、迁移、配置与启动

## 3.1 M0 分支

```bash
cd "$HOME/workspace/WeKnora"
git worktree add "$HOME/workspace/WeKnora-sync_v0.8.3_base" -b sync_v0.8.3_base v0.8.3
cd "$HOME/workspace/WeKnora-sync_v0.8.3_base"
git status --short --branch
```

预期：

1. 独立 worktree 路径为 `$HOME/workspace/WeKnora-sync_v0.8.3_base`。
2. 当前分支为 `sync_v0.8.3_base`。
3. 基线直接来自上游 `v0.8.3` 标签。

## 3.2 M0 建议执行顺序

1. 先起纯上游环境。
2. 再补本地必须保留的 secrets/config。
3. 再跑迁移与 smoke。
4. 只记录差异，不在 M0 引入任何业务补丁。

## 3.3 M0 建议命令

### Step M0-1：准备 `.env`

```bash
cd "$HOME/workspace/WeKnora-sync_v0.8.3_base"
cp -n .env.example .env
make check-env
```

说明：

1. 这里只是生成 0.8.3 默认 `.env`。
2. 现网 `SYSTEM_AES_KEY` 必须手工带入，不要用新值覆盖。
3. `SYSTEM_SIGNING_KEY`、`JWT_SECRET` 需要显式补齐。

### Step M0-2：记录配置差异

建议先人工维护一份对照表，同时可以用以下命令快速抽出关心的键：

```bash
cd "$HOME/workspace/WeKnora-sync_v0.8.3_base"
rg '^(JWT_SECRET|SYSTEM_AES_KEY|SYSTEM_SIGNING_KEY|DB_DRIVER|DB_URL|STORAGE_TYPE|OLLAMA_BASE_URL|MINERU|MCP|OIDC|DINGTALK|BROWSERSKILL|WEKNORA_SANDBOX)' .env.example .env
```

### Step M0-3：拉镜像并启动纯上游服务

M0 优先按上游 README 路径走，不先走本地自定义运维脚本：

```bash
cd "$HOME/workspace/WeKnora-sync_v0.8.3_base"
docker compose pull
docker compose up -d
docker compose ps
```

如果只想先检查基础镜像/compose 语法，可以先跑：

```bash
cd "$HOME/workspace/WeKnora-sync_v0.8.3_base"
docker compose config >/dev/null
make list-containers
```

执行注记（2026-10-16）：

1. 当前环境中，frontend 首次按纯上游镜像启动时，出现过 `host not found in upstream "app"` 的 Nginx 启动期解析失败；M0 执行 worktree 已通过最小前端代理启动补丁 + 重建 frontend 镜像消除此问题。
2. 当前宿主机端口 `80` 已被占用，因此执行 worktree 需在 `.env` 中把 `FRONTEND_PORT` 改到空闲端口；本次执行使用 `8081`。

### Step M0-4：检查迁移版本

```bash
cd "$HOME/workspace/WeKnora-sync_v0.8.3_base"
docker compose exec -T app ./scripts/migrate.sh version || true
docker compose exec -T app ./scripts/migrate.sh up
docker compose exec -T app ./scripts/migrate.sh version
```

说明：

1. `make migrate-version` 首次可能返回“尚未有版本”，这是可接受的。
2. 重点记录 `000080` 到 `000118` 是否全部执行完成。
3. 纯上游 compose 默认不对宿主机暴露 PostgreSQL，且 `.env` 中 `DB_HOST=postgres` 只对容器网络可解析；因此在宿主机直接执行 `make migrate-*` 容易报 `lookup postgres`。M0 以 `docker compose exec -T app ./scripts/migrate.sh ...` 作为默认记录命令。

### Step M0-5：最小健康检查

```bash
cd "$HOME/workspace/WeKnora-sync_v0.8.3_base"
curl -sf http://localhost:8080/health
AUTH_TOKEN=<viewer_jwt>
curl -sf -H "Authorization: Bearer $AUTH_TOKEN" http://localhost:8080/api/v1/system/capabilities
curl -sf -H "Authorization: Bearer $AUTH_TOKEN" http://localhost:8080/api/v1/system/info
```

说明：

1. 上游 `v0.8.3` 的 `system/capabilities` 与 `system/info` 都挂在 `Viewer` 权限后面；未登录时返回 `401` 属预期行为，不应误判为服务未启动。
2. M0 验证时可注册一个临时 viewer 账号并登录取 JWT，再完成这两条 smoke。

### Step M0-6：查看日志

```bash
cd "$HOME/workspace/WeKnora-sync_v0.8.3_base"
docker compose logs --tail=80 app
docker compose logs --tail=80 frontend
docker compose logs --tail=80 docreader
```

## 3.4 M0 验证命令

建议至少依次执行：

```bash
cd "$HOME/workspace/WeKnora-sync_v0.8.3_base"
git status --short --branch
docker compose ps
docker compose exec -T app ./scripts/migrate.sh version
curl -sf http://localhost:8080/health
AUTH_TOKEN=<viewer_jwt>
curl -sf -H "Authorization: Bearer $AUTH_TOKEN" http://localhost:8080/api/v1/system/capabilities >/dev/null
curl -sf -H "Authorization: Bearer $AUTH_TOKEN" http://localhost:8080/api/v1/system/info >/dev/null
```

如果要补一条本地开发链 smoke，可选跑：

```bash
cd "$HOME/workspace/WeKnora-sync_v0.8.3_base"
make dev-start
make dev-status
```

## 3.5 M0 完成标准

满足以下条件才进入 M1：

1. `sync_v0.8.3_base` 能启动。
2. 迁移跑通。
3. `health`、`system/capabilities`、`system/info` 可访问。
4. 已整理出配置差异表。

## 3.6 M0 输出物

建议至少落两份文字产物：

1. `0.8.3 环境变量差异表`。
2. `0.8.3 迁移执行记录`。

---

## 4. M1 执行清单：品牌、部署、运维、导出资源

## 4.1 M1 主分支

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage1_platform sync_v0.8.3_base
git status --short --branch
```

## 4.2 M1 子分支 A：品牌层

### 分支名

`sync_v0.8.3_stage1_platform_a_brand`

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage1_platform_a_brand sync_v0.8.3_stage1_platform
git show --stat 60dbb9a90
git show --stat 5aa91ccd9
```

如果这两批改动冲突很轻，可试探性取参考：

```bash
cd "$HOME/workspace/WeKnora"
git cherry-pick -n 60dbb9a90 5aa91ccd9 || git cherry-pick --abort
```

更推荐的做法是：

1. 只迁移 logo、标题、登录页视觉资源和品牌文案。
2. 不覆盖 0.8.3 的新页面结构、toolbox、model catalog、platform user management 入口。

### 验证命令

```bash
cd "$HOME/workspace/WeKnora/frontend"
npm run type-check
npm run build
```

再做一轮静态校验：

```bash
cd "$HOME/workspace/WeKnora"
rg -n 'IntraMind|WeKnora' frontend/src frontend/public README.md README_CN.md
```

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage1_platform
git merge --ff-only sync_v0.8.3_stage1_platform_a_brand
```

## 4.3 M1 子分支 B：部署/运维脚本层

### 分支名

`sync_v0.8.3_stage1_platform_b_ops`

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage1_platform_b_ops sync_v0.8.3_stage1_platform
git show --stat a0c4561ba
git show --stat d23178000
git show --stat 35f600bc5
git show --stat 7ef9f2239
git show --stat e1646c048
```

建议处理方式：

1. 把本地脚本当“覆盖层”，不要整份替换上游 `docker-compose.yml`。
2. 对端口、MinIO、前端 dev 代理这类环境适配项优先落地。
3. 构建脚本保留上游主流程，只补本地编译/拉镜像便利性。

### 验证命令

```bash
cd "$HOME/workspace/WeKnora"
make check-env
make dev-start
make dev-status
make dev-stop
docker compose config >/dev/null
```

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage1_platform
git merge --ff-only sync_v0.8.3_stage1_platform_b_ops
```

## 4.4 M1 子分支 C：导出资源层

### 分支名

`sync_v0.8.3_stage1_platform_c_export`

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage1_platform_c_export sync_v0.8.3_stage1_platform
git show --stat 49b61b9e5
git show --stat cfd6a5b87
```

建议处理方式：

1. 只恢复 `config/export` 资源与挂载。
2. 暂不在 M1 恢复完整导出逻辑；逻辑回迁放到 M6。

### 验证命令

```bash
cd "$HOME/workspace/WeKnora"
test -d config/export
find config/export -maxdepth 2 -type f | head
docker compose config >/dev/null
```

如果导出资源也被本地运行使用，可补一轮：

```bash
cd "$HOME/workspace/WeKnora"
make dev-start
make dev-app
```

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage1_platform
git merge --ff-only sync_v0.8.3_stage1_platform_c_export
```

## 4.5 M1 阶段验证命令

M1 三个子分支都回并后，统一跑：

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage1_platform
git status --short --branch
make check-env
docker compose config >/dev/null
make dev-start
make dev-status
make dev-stop
```

可选补充：

```bash
cd "$HOME/workspace/WeKnora/frontend"
npm run build
```

## 4.6 M1 完成标准

1. 品牌元素恢复。
2. 本地 deploy/dev 脚本可用。
3. `config/export` 资源已挂回。
4. 不引入权限和业务语义改动。

---

## 5. M2 执行清单：登录标识、空间初始化、创建门禁

## 5.1 M2 主分支

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage2_auth sync_v0.8.3_stage1_platform
git status --short --branch
```

## 5.2 M2 子分支 A：登录 identifier 化

### 分支名

`sync_v0.8.3_stage2_auth_a_identifier`

### 参考来源

重点参考提交：`3440abf08`

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage2_auth_a_identifier sync_v0.8.3_stage2_auth
git show --stat 3440abf08
git show 3440abf08 -- client/auth.go internal/application/service/user.go internal/handler/auth.go frontend/src
```

建议手工移植，不建议直接把旧登录页整块 cherry-pick 进来。

优先落的文件面：

1. `internal/types` / `internal/handler/auth.go`
2. `internal/application/service/user.go`
3. `client/auth.go`
4. `frontend/src` 登录页与 i18n

### 验证命令

后端单测：

```bash
cd "$HOME/workspace/WeKnora"
go test ./internal/handler -run 'TestLoginFailureAppError'
```

前端静态验证：

```bash
cd "$HOME/workspace/WeKnora/frontend"
npm run type-check
```

建议补一轮手工 smoke：

```bash
cd "$HOME/workspace/WeKnora"
make dev-start
make dev-app
make dev-frontend
```

浏览器手工验证：

1. 用邮箱登录一次。
2. 用用户名登录一次。
3. 输入错误 identifier/密码，确认仍只返回统一错误。

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage2_auth
git merge --ff-only sync_v0.8.3_stage2_auth_a_identifier
```

## 5.3 M2 子分支 B：空间初始化与 bootstrap

### 分支名

`sync_v0.8.3_stage2_auth_b_bootstrap`

### 参考来源

重点参考提交：

1. `fb629a50e`
2. `6e6304af8`
3. `e57a27a4f`

另参考现有审查结论：

1. `WorkspaceSetupWizard` 不应承担真实状态记忆。
2. 用户冲突必须返回结构化 409。
3. 根组织应支持复用，避免重复创建。

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage2_auth_b_bootstrap sync_v0.8.3_stage2_auth
git show --stat fb629a50e
git show --stat 6e6304af8
git show --stat e57a27a4f
```

优先处理顺序：

1. 后端 bootstrap 编排/错误分类。
2. 用户冲突类型化（用户名/邮箱/手机号）。
3. 前端根组织复用与向导恢复逻辑。

### 验证命令

后端单测：

```bash
cd "$HOME/workspace/WeKnora"
go test ./internal/application/service -run 'TestCreateUserByAdmin'
go test ./internal/handler -run 'TestRegistrationFailureAppError'
```

前端单测：

```bash
cd "$HOME/workspace/WeKnora/frontend"
npm test -- src/utils/workspaceSetupRootOrg.test.ts
npm run type-check
```

建议补一轮手工 smoke：

```bash
cd "$HOME/workspace/WeKnora"
make dev-start
make dev-app
make dev-frontend
```

浏览器手工验证：

1. 新建空间后打开初始化向导。
2. 故意使用已占用手机号，确认提示具体冲突字段而不是通用 500。
3. 刷新页面后再次进入向导，确认不会重复创建根组织。

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage2_auth
git merge --ff-only sync_v0.8.3_stage2_auth_b_bootstrap
```

## 5.4 M2 子分支 C：创建门禁与会话缓存卫生

### 分支名

`sync_v0.8.3_stage2_auth_c_gate`

### 参考来源

重点参考提交或结论：

1. `da33523e4`：协作者创建资源时隐藏全局可见选项。
2. `8479a969d`：orgTree 跨账号缓存残留修复。
3. Agent create readiness 结论：校验必须收口到 shared opener，而不是仅绑在单个按钮上。

### 建议命令

```bash
cd "$HOME/workspace/WeKnora"
git switch -c sync_v0.8.3_stage2_auth_c_gate sync_v0.8.3_stage2_auth
git show --stat da33523e4
git show --stat 8479a969d
rg -n 'openCreateModal|handleOpenAgentEditor' frontend/src/views/agent/AgentList.vue
rg -n 'clearSessionResourceCaches|useOrgTreeStore\(\)\.clearState\(' frontend/src/stores/auth.ts frontend/src/views/admin frontend/src/stores/orgTree.ts
```

建议处理顺序：

1. 先修共享入口 `openCreateModal`/等价 opener。
2. 再收口协作者可见性选项。
3. 最后修跨账号 session cache 清理与页面重拉逻辑。

### 验证命令

前端静态验证：

```bash
cd "$HOME/workspace/WeKnora/frontend"
npm run type-check
npm run build
```

建议补一轮开发态 smoke：

```bash
cd "$HOME/workspace/WeKnora"
make dev-start
make dev-app
make dev-frontend
```

浏览器手工验证：

1. 在无可用聊天模型时，从所有创建入口打开智能体创建，确认都被统一拦截。
2. 用超管登录进入组织管理页，再登出并登录子组织管理员，确认组织树不会残留全量节点。
3. 用协作者身份检查 KB/Agent 创建页面，确认看不到不该开放的全局可见选项。

### 回并命令

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage2_auth
git merge --ff-only sync_v0.8.3_stage2_auth_c_gate
```

## 5.5 M2 阶段验证命令

三个子分支都回并后，统一跑：

```bash
cd "$HOME/workspace/WeKnora"
git switch sync_v0.8.3_stage2_auth
go test ./internal/application/service -run 'TestCreateUserByAdmin'
go test ./internal/handler -run 'Test(LoginFailureAppError|RegistrationFailureAppError)'
cd frontend && npm test -- src/utils/workspaceSetupRootOrg.test.ts
cd "$HOME/workspace/WeKnora/frontend" && npm run type-check
```

然后补一轮端到端 smoke：

```bash
cd "$HOME/workspace/WeKnora"
make dev-start
make dev-app
make dev-frontend
```

手工重点看四件事：

1. 邮箱/用户名双路登录。
2. 初始化向导冲突提示与重试恢复。
3. 组织树跨账号切换不残留。
4. 智能体创建入口统一门禁。

## 5.6 M2 完成标准

1. identifier 登录可用。
2. 初始化向导支持冲突恢复与根组织复用。
3. Agent 创建门禁和协作者可见性选项收口完成。
4. 跨账号 session cache 卫生问题修复。

---

## 6. M0-M2 串行回并顺序

建议按下面顺序合并：

```text
sync_v0.8.3_base
  -> sync_v0.8.3_stage1_platform
     -> sync_v0.8.3_stage1_platform_a_brand
     -> sync_v0.8.3_stage1_platform_b_ops
     -> sync_v0.8.3_stage1_platform_c_export
  -> sync_v0.8.3_stage2_auth
     -> sync_v0.8.3_stage2_auth_a_identifier
     -> sync_v0.8.3_stage2_auth_b_bootstrap
     -> sync_v0.8.3_stage2_auth_c_gate
```

阶段主分支的推进规则：

1. M0 验证通过后再开 M1。
2. M1 三个子分支都验证通过后再开 M2。
3. M2 的三个子分支尽量按 A -> B -> C 顺序推进，避免初始化与登录入口反复变基线。

---

## 7. 一页式命令索引

如果只看命令，最短路径如下：

```bash
cd "$HOME/workspace/WeKnora"
git remote get-url upstream >/dev/null 2>&1 || git remote add upstream https://github.com/Tencent/WeKnora.git
git fetch upstream --tags && git fetch upstream main

git switch -c sync_v0.8.3_base v0.8.3
cp -n .env.example .env
make check-env
docker compose pull && docker compose up -d
make migrate-up
curl -sf http://localhost:8080/health

git switch -c sync_v0.8.3_stage1_platform sync_v0.8.3_base
git switch -c sync_v0.8.3_stage1_platform_a_brand
git switch sync_v0.8.3_stage1_platform && git merge --ff-only sync_v0.8.3_stage1_platform_a_brand
git switch -c sync_v0.8.3_stage1_platform_b_ops
git switch sync_v0.8.3_stage1_platform && git merge --ff-only sync_v0.8.3_stage1_platform_b_ops
git switch -c sync_v0.8.3_stage1_platform_c_export
git switch sync_v0.8.3_stage1_platform && git merge --ff-only sync_v0.8.3_stage1_platform_c_export

git switch -c sync_v0.8.3_stage2_auth sync_v0.8.3_stage1_platform
git switch -c sync_v0.8.3_stage2_auth_a_identifier
git switch sync_v0.8.3_stage2_auth && git merge --ff-only sync_v0.8.3_stage2_auth_a_identifier
git switch -c sync_v0.8.3_stage2_auth_b_bootstrap
git switch sync_v0.8.3_stage2_auth && git merge --ff-only sync_v0.8.3_stage2_auth_b_bootstrap
git switch -c sync_v0.8.3_stage2_auth_c_gate
git switch sync_v0.8.3_stage2_auth && git merge --ff-only sync_v0.8.3_stage2_auth_c_gate
```

---

## 8. 结论

M0-M2 的执行关键点不是“尽快改完”，而是：

1. M0 先拿到一个干净、可启动、可迁移的 0.8.3 基线。
2. M1 只做低冲突覆盖层，不提早引入权限语义。
3. M2 把登录、初始化、创建门禁拆成三个单独可验证的子批次。

只要按这个节奏推进，后面的 M3 权限主线和 M4 知识库模块就会有稳定的落脚点，而不会在入口层和环境层反复返工。