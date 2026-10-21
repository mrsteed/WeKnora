# sync v0.8.3 trial 试运行与回滚报告（2026-10-17）

## 1. 结论

本次会话为 Phase 7 `trial` 阶段做了两件事：

1. 生成可复用的备份/回滚脚本。
2. 在当前本地环境上做了一次只读试运行探测与 snapshot 生成。

本次没有执行破坏性切换操作（未执行 `docker compose pull`、`docker compose up -d` 覆盖部署、`make migrate-version`、数据库恢复），因为当前会话没有正式变更窗口，也没有要求对现运行环境做实际切换。

因此，本报告的性质是：

1. 已形成可执行 trial 工具链。
2. 已验证本地运行态与回滚脚本 dry-run。
3. 真正的切换/回滚演练仍需在明确的试运行窗口执行。

## 2. 已落地脚本

新增脚本：

1. `scripts/phase7_backup_snapshot.sh`
   - 生成 snapshot 目录
   - 备份 `.env`
   - 记录 `git` 版本与工作区状态
   - 记录 `docker compose ps` / `docker compose config`
   - 打包 `config/export`
   - 若存在 `DB_URL`/`DATABASE_URL` 且未禁用，则尝试 `pg_dump`

2. `scripts/phase7_restore_snapshot.sh`
   - 默认 dry-run
   - `--apply` 时恢复 `.env` 并执行 `docker compose up -d`
   - `--with-db` 时通过 `psql` 恢复 SQL dump

## 3. 本地 snapshot 结果

本次实际执行的 snapshot：

1. 目录：`backups/20261017-104210`

snapshot 脚本校验结果：

1. shell 语法检查：通过
2. 可执行位：已设置
3. 实际运行：通过

snapshot 中包含：

1. `.env.pre-upgrade`（若 `.env` 存在）
2. `git.head.txt`
3. `git.status.txt`
4. `docker-compose.ps.txt`
5. `docker-compose.config.yaml`
6. `config-export.tgz`
7. `weknora.sql` 或 `db-backup-skipped.txt`（取决于 `DB_URL` 是否存在）

## 4. 本地运行态探测

只读探测结果如下：

1. `docker compose ps`：通过
   - 容器在位：`app`、`docreader`、`frontend`、`postgres`、`redis`
   - 已确认健康：`app`、`docreader`、`postgres`

2. `.env` 存在：通过

3. `curl -sf http://localhost:8080/health`：通过
   - 响应：`{"status":"ok"}`

4. `curl -sf http://localhost:8080/api/v1/system/capabilities`：未通过
   - 当前判定：环境阻塞
   - 原因：该接口在当前会话条件下不能匿名通过；需要实际认证态再核验

## 5. rollback dry-run 结果

执行：

1. `scripts/phase7_restore_snapshot.sh backups/20261017-104210`

dry-run 输出验证通过，确认脚本会执行以下动作：

1. 若 snapshot 中存在 `.env.pre-upgrade`，则恢复到仓库根 `.env`
2. 执行 `docker compose up -d`
3. 若显式传 `--with-db`，则会尝试通过 `psql` 导入 `weknora.sql`

本次未执行 `--apply`，原因：

1. `--apply` 会修改当前运行环境
2. `--with-db` 会触发数据库恢复
3. 这些都应只在试运行窗口内执行

## 6. 进入真实 trial 的建议顺序

建议在实际试运行窗口执行：

1. `scripts/phase7_backup_snapshot.sh`
2. 记录 snapshot 目录
3. `docker compose pull`
4. `docker compose up -d`
5. `make migrate-version`
6. 带认证做：
   - `/health`
   - `/api/v1/system/capabilities`
   - 导出 / 解析 / MCP / sandbox / 知识引用 smoke
7. 如需回滚：
   - `scripts/phase7_restore_snapshot.sh <snapshot-dir> --apply`
   - 如数据库需要恢复，再追加 `--with-db`

## 7. 当前 trial 判定

当前可确认：

1. 备份脚本可执行。
2. rollback 脚本可 dry-run。
3. 本地 compose 运行态可读。
4. 最小健康检查通过。

当前尚未执行：

1. 真实切换
2. 真实数据库回滚
3. 带认证的 capabilities 检查
4. 真实浏览器/多角色 smoke

因此本次 trial 结论为：

1. 已具备进入试运行窗口的脚本与本地基线。
2. 真实 trial 仍需在明确的变更窗口内完成一次 `--apply` 级别演练。
