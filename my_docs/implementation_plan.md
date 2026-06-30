# 上游同步实施计划 (2026-06-08)

## 背景

将 `Tencent/WeKnora` 最新上游代码同步到本地 `mrsteed/WeKnora` 的 `dev` 分支，同时保留所有本地业务功能。

### 当前状态分析

| 指标 | 数值 |
|------|------|
| 本地独有提交 (dev) | 58 个 |
| 上游新增提交 (upstream/main) | 137 个 |
| 差异文件数 | 879 个 |
| 差异行数 | +68,592 / -137,582 |
| 工作区未提交文件 | 2 个 (`zh-CN.ts`, `StorageEngineSettings.vue`) |

### 本地关键业务功能（必须保留）

1. **长文档模式** — 分阶段输出、完整翻译等
2. **数据库查询知识库** — DuckDB/私有数据库集成
3. **对话导出功能** — 导出为文件
4. **组织权限管理** — 人员、组织、权限体系
5. **智能体分享功能** — 分享页面及细节
6. **界面定制** — 登录样式、知识库列表、TEI RERANK 等
7. **管理员功能** — 密码重置、超级管理员路由

### 上游重要更新（需要吸收）

1. **知识库文档解析追踪时间线** — span tree、timeline UI
2. **OpenSearch 向量存储驱动** — 新的向量存储后端
3. **MCP Server 多传输支持** — stdio/SSE/HTTP
4. **RBAC 分享链接邀请** — 多次使用的邀请链接
5. **HNSW 索引** — bge-m3 1024 维嵌入
6. **DuckDB 启动修复** — 避免启动时安装扩展
7. **安全修复** — SSRF 策略、租户验证、文件访问
8. **前端改进** — 新用户引导、模型编辑器、思维模式配置

---

## 同步策略选择

> [!IMPORTANT]
> 鉴于差异规模巨大（879 文件、137 上游提交），直接 rebase 极可能产生大量逐提交冲突。建议采用 **merge 策略**（`git merge upstream/main`），一次性解决所有冲突，效率更高且历史更清晰。

**推荐方案：merge 策略**
- 从 `dev` 创建临时同步分支
- `git merge upstream/main` 一次性合并
- 冲突时以本地代码功能为准
- 验证后推送到 `origin/dev`

**备选方案：rebase 策略**（手册推荐，但本次差异过大，逐提交冲突成本极高）

> [!WARNING]
> 用户明确要求"上游代码和本地功能有冲突，以本地代码功能为准"。这与手册中"先接受上游新接口，再把本地业务语义迁移"的建议略有不同。本计划将严格遵循用户要求：**冲突以本地为准**。

---

## 执行步骤

### 阶段一：同步前准备

1. **提交工作区未保存文件**
   ```bash
   git add frontend/src/i18n/locales/zh-CN.ts frontend/src/views/settings/StorageEngineSettings.vue
   git commit -m "chore: save local changes before upstream sync 20260608"
   ```

2. **创建备份分支和 tag**
   ```bash
   git checkout dev
   git branch backup/dev-20260608-before-sync
   git tag sync-dev-20260608-pre
   ```

3. **拉取最新远端代码**
   ```bash
   git fetch origin --prune
   git fetch upstream --prune
   ```

---

### 阶段二：创建同步分支并合并

4. **创建临时同步分支**
   ```bash
   git checkout -b sync/dev-20260608-main dev
   ```

5. **合并上游主线**
   ```bash
   git merge upstream/main --no-edit
   ```

---

### 阶段三：冲突解决

6. **冲突解决原则**（以本地代码功能为准）

   | 冲突类型 | 处理方式 |
   |---------|---------|
   | 本地业务功能代码 | **保留本地** |
   | 本地界面定制和样式 | **保留本地** |
   | 上游纯新增文件/功能 | **接受上游**（无冲突） |
   | 上游安全修复与本地无冲突 | **接受上游** |
   | 上游安全修复与本地有冲突 | **以本地为基础，手动补入安全逻辑** |
   | 配置文件 / go.mod / go.sum | **合并双方依赖** |
   | 前端 i18n 文件 | **合并双方翻译条目** |
   | CHANGELOG / VERSION | **采用上游版本号，保留本地变更记录** |

7. **逐文件解决冲突并提交**
   ```bash
   # 查看冲突文件列表
   git diff --name-only --diff-filter=U
   
   # 对每个冲突文件：编辑解决冲突
   git add <resolved-file>
   
   # 所有冲突解决后
   git commit
   ```

---

### 阶段四：验证

8. **后端编译验证**
   ```bash
   go build ./...
   ```

9. **后端测试（聚焦）**
   ```bash
   go test ./internal/agent/...
   go test ./internal/handler/...
   go test ./internal/container
   ```

10. **前端构建验证**
    ```bash
    cd frontend
    pnpm install
    pnpm build
    ```

---

### 阶段五：推送到 dev 分支

11. **先推送临时同步分支**
    ```bash
    git push -u origin sync/dev-20260608-main
    ```

12. **确认无误后更新 dev 分支**
    ```bash
    git checkout dev
    git merge sync/dev-20260608-main --ff-only
    git push origin dev
    ```

---

## 回滚方案

如果同步失败，随时可以回退：

```bash
# 回到同步前状态
git checkout dev
git reset --hard backup/dev-20260608-before-sync
git push --force-with-lease origin dev
```

---

## 验证通过标准

- [ ] 同步分支包含最新 `upstream/main` 的所有提交
- [ ] 本地所有业务功能代码完整保留
- [ ] `go build ./...` 编译通过
- [ ] 聚焦后端测试通过
- [ ] 前端 `pnpm build` 构建通过
- [ ] 结果已推送到 `origin/dev`
