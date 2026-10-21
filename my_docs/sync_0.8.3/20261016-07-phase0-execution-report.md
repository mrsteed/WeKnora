# 20261016 WeKnora Phase 0 执行报告

## 1. 执行摘要

本轮 Phase 0 / M0 已按第二轮同步硬约束完成，执行方式如下：

1. 从 upstream `v0.8.3` 标签直接创建了全新独立 worktree `WeKnora-sync_v0.8.3_base`。
2. 执行分支为 `sync_v0.8.3_base`，基线 commit 为 `3e22822c1d2397be730c458c7539d0eb8c113a24`。
3. 未复用旧 worktree、旧阶段分支或旧 Git 元数据。
4. 未把 `0.7.2` / 私有分支代码整文件覆盖到上游装配文件上。

本阶段最终状态：

1. `app`、`postgres`、`docreader` 均为 healthy。
2. `redis` 正常运行。
3. `frontend` 经最小基础设施修正后已稳定启动，对外端口为 `8081`。
4. `/health` 可访问；`/api/v1/system/capabilities` 与 `/api/v1/system/info` 在 viewer 鉴权下返回 `200`。
5. 迁移版本已确认在 `118`，`migrate up` 结果为 `no change`。

结论：M0 可以视为完成，可进入 M1。

---

## 2. 环境变量差异表

本阶段只记录“必须保留 / 必须补齐 / 本地环境适配”的差异，不记录 secret 明文。

| 项目 | upstream `v0.8.3` 默认 | 本次 M0 处理 | 说明 |
|---|---|---|---|
| `SYSTEM_AES_KEY` | `.env.example` 不携带现网值 | 从主仓现有 `.env` 带入 | 保证现有加密数据可继续解密 |
| `JWT_SECRET` | `.env.example` 需显式填写 | 从主仓现有 `.env` 带入 | 维持现有 JWT 行为 |
| `SYSTEM_SIGNING_KEY` | `.env.example` 需显式填写 | 新生成强随机值 | 主仓现有 `.env` 缺失该项 |
| `FRONTEND_PORT` | `80` | 改为 `8081` | 当前宿主机 `80` 端口已被占用 |
| `DB_HOST` | `postgres` | 保持不变 | 供容器网络内的 `app` 使用 |

补充说明：

1. `.env` 初始化方式为 `cp -n .env.example .env`。
2. `make check-env` 与 `docker compose config >/dev/null` 均已通过。
3. 本报告不落任何 secret 实值。

---

## 3. 启动与迁移记录

### 3.1 启动记录

已执行：

```bash
cd "$HOME/workspace/WeKnora-sync_v0.8.3_base"
docker compose pull
docker compose up -d
```

第一次启动阶段观察到两个基础设施问题：

1. `frontend` 初次启动报错：`host not found in upstream "app" in /etc/nginx/conf.d/default.conf:78`。
2. `frontend` 修正后再次启动，又命中本机 `80` 端口占用。

本次 M0 的实际处理：

1. 对 `frontend/nginx.conf` 做了最小前端代理启动修正，使其通过 Docker DNS 在请求时解析 `APP_HOST`，避免在 Nginx 启动瞬间因 `app` 名称解析失败而崩溃。
2. 在执行 worktree `.env` 中把 `FRONTEND_PORT` 从 `80` 调整为 `8081`。

最终 compose 状态：

1. `WeKnora-app`：healthy，宿主机端口 `8080`。
2. `WeKnora-postgres`：healthy。
3. `WeKnora-docreader`：healthy。
4. `WeKnora-redis`：running。
5. `WeKnora-frontend`：running，宿主机端口 `8081`。

### 3.2 迁移记录

按 checklist 原始写法，宿主机直接执行：

```bash
make migrate-version
make migrate-up
```

会失败并报：

```text
error: failed to open database: dial tcp: lookup postgres on 127.0.0.53:53: server misbehaving
```

根因：

1. 纯上游 compose 默认不把 PostgreSQL 映射到宿主机端口。
2. `.env` 中 `DB_HOST=postgres` 只对容器网络有效，不对宿主机 shell 有效。

因此本次 M0 的迁移记录改为在 `app` 容器内执行：

```bash
docker compose exec -T app ./scripts/migrate.sh version
docker compose exec -T app ./scripts/migrate.sh up
docker compose exec -T app ./scripts/migrate.sh version
```

结果：

1. 首次 `version` 返回 `118`。
2. `up` 返回 `no change`。
3. 末次 `version` 仍为 `118`。

结论：数据库 schema 已在 `000118`，本次 M0 不需要额外迁移变更。

---

## 4. 健康检查与 smoke 结果

### 4.1 无鉴权检查

```bash
curl -sf http://localhost:8080/health
```

结果：

1. 返回 `200`。
2. 响应体为 `{"status":"ok"}`。

### 4.2 `system/capabilities` / `system/info` 的实际鉴权语义

本次执行确认：

1. 上游 `v0.8.3` 中，这两个接口都注册在 `Viewer` 权限之后。
2. 未登录访问返回 `401`，这是预期行为，不是服务未启动。

对应路由语义位于：

1. `internal/router/routes_auth_tenant.go` 中 `RegisterSystemRoutes()`。

### 4.3 viewer smoke

本次使用一个临时 viewer 账号完成注册、登录与 smoke：

1. `POST /api/v1/auth/register` 返回 `201`。
2. `POST /api/v1/auth/login` 返回 `200`，拿到 JWT。
3. 带 `Authorization: Bearer <token>` 后：
   - `GET /api/v1/system/capabilities` 返回 `200`
   - `GET /api/v1/system/info` 返回 `200`

`system/info` 关键返回值：

1. `version = 0.8.3`
2. `edition = standard`
3. `commit_id = 3e22822`
4. `go_version = go version go1.26.0 linux/amd64`

---

## 5. 对后续阶段的约束结论

从本次 M0 可确认以下事实，供后续 M1+ 继续沿用：

1. 当前执行 worktree 的唯一正确起点是 upstream `v0.8.3`。
2. 迁移验证应优先使用 `docker compose exec -T app ./scripts/migrate.sh ...`，不要在宿主机直接依赖 `DB_HOST=postgres`。
3. `system/capabilities` 与 `system/info` 的 smoke 需要 viewer JWT，未登录 `401` 不应当被误判为 M0 失败。
4. 当前宿主机上，frontend 不能继续使用默认对外端口 `80`；本执行线统一改用 `8081`。
5. 本轮为启动 frontend，已在执行 worktree 中引入最小前端代理启动修正；该修正不涉及业务语义，但属于当前环境下使 `v0.8.3` 基线完整起盘所需的基础设施差异。

因此，Phase 0 / M0 已满足“可迁移、可启动、可健康检查”的进入条件，可继续进入 M1。