# WeKnora Docker Compose 外置 PostgreSQL / 外置 Redis 上线步骤

## 1. 文档说明

本文基于现有 Compose 版上线脚本，单独生成一份适用于“应用服务器跑 Docker Compose，PostgreSQL 与 Redis 部署在局域网其他主机”的生产上线方案。

配套脚本：

```text
my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh
```

这个版本不替代现有的 03 版文档。03 版继续用于 PostgreSQL、Redis 也跟随 Compose 一起启动的场景；04 版专门处理以下拓扑：

1. App、frontend、docreader、sandbox 运行在当前 Linux 服务器。
2. PostgreSQL 运行在局域网另一台服务器，或数据库高可用地址。
3. Redis 运行在局域网另一台服务器，或 Redis 哨兵/代理前的固定接入地址。
4. MinIO 既可以继续使用当前 Compose 内置 profile，也可以使用外部 MinIO / S3 兼容存储。

脚本会完成：

1. 生成外置 PostgreSQL / Redis 配置模板。
2. 复制 Compose 运行目录。
3. 渲染生产 `.env`。
4. 渲染外置依赖专用 `docker-compose.prod.override.yml`。
5. 渲染 App 启动前的远端依赖等待脚本。
6. 拉取镜像并启动服务。
7. 校验远端 PostgreSQL、远端 Redis、App `/health` 和前端入口。
8. 通过占位 `postgres` 服务执行远端 PostgreSQL 备份。
9. 支持迁移、升级和镜像回滚。

## 2. 适用范围和前置条件

### 2.1 适用范围

适合以下部署：

1. 单台 Linux 应用服务器运行 Docker Compose。
2. PostgreSQL、Redis 已经由 DBA、基础设施团队或其他宿主机提供。
3. WeKnora 镜像已经构建并推送到企业镜像仓库。
4. 需要保留当前 Compose 部署方式，但不希望在应用机本地再起 PostgreSQL、Redis 容器。

### 2.2 不适用范围

以下场景优先使用 Helm / Kubernetes：

1. 多节点高可用。
2. 需要 Ingress、HPA、PDB、NetworkPolicy。
3. 需要 Secret Operator、GitOps、独立数据库 Chart。
4. 需要 Redis Sentinel、Redis Cluster、云数据库自动切换逻辑由平台层托管。

### 2.3 远端基础设施前置条件

在正式部署前，必须先确认远端数据库和 Redis 满足以下要求。

#### PostgreSQL

1. 当前仓库默认 Compose 镜像使用的是 ParadeDB / PostgreSQL 17 基线，因此生产推荐继续使用兼容的 PostgreSQL 17 / ParadeDB 17 环境。
2. 首次迁移前，需要确认数据库可创建或已预创建以下扩展：

```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_search;
```

3. 如果远端数据库不是 ParadeDB，而是普通 PostgreSQL，请先核实 `pg_search` 是否可用。当前仓库的迁移中存在对 `pg_search` 的更新与使用，不满足时会导致自动迁移失败。
4. 数据库账号在首次上线时至少要具备建表、建索引、执行迁移所需权限；若无创建扩展权限，需要 DBA 预先创建扩展。
5. 需要从应用服务器访问到 `DB_HOST:DB_PORT`，并在 `pg_hba.conf`、防火墙、安全组中放通应用服务器 IP。

#### Redis

1. 远端 Redis 需要允许应用服务器访问对应监听地址。
2. 若 Redis 配置了 `requirepass`，请填写 `REDIS_PASSWORD`。
3. 若 Redis 使用 ACL 用户，请同时填写 `REDIS_USERNAME` 和 `REDIS_PASSWORD`。
4. 本脚本要求 `REDIS_ADDR` 使用 `host:port` 格式。若你使用 IPv6，请优先配置一个可解析的 DNS 主机名，而不是直接写裸 IPv6 地址。

### 2.4 服务器工具要求

在应用服务器上确认：

```bash
docker version
docker compose version || docker-compose version
curl --version
openssl version
```

Docker 服务必须运行：

```bash
docker info
```

## 3. 文件和目录约定

本文假设仓库位于：

```text
/home/xmkp/workspace/WeKnora
```

默认运行目录：

```text
/opt/weknora-external
```

默认配置文件：

```text
my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.env
```

如果生产服务器上的仓库路径不同，可用：

```bash
export WEKNORA_REPO_ROOT=/path/to/WeKnora
```

如果配置文件路径不同，可用：

```bash
export WEKNORA_COMPOSE_ENV=/path/to/20260528-04-weknora-compose-external-db-redis.env
```

## 4. 初始化外置依赖配置

### 4.1 生成配置模板

```bash
cd /home/xmkp/workspace/WeKnora

bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh init-config
```

生成文件：

```text
my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.env
```

权限会自动设置为 `600`。

如果文件已存在且确认要覆盖：

```bash
FORCE=true bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh init-config
```

### 4.2 编辑配置

```bash
vi my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.env
```

必须替换所有 `change-me-*` 值，并将数据库、Redis 地址改成局域网可达地址。

关键配置示例：

```dotenv
REGISTRY=registry.example.com/weknora
WEKNORA_VERSION=0.6.0-20260528.1
COMPOSE_PROJECT_DIR=/opt/weknora-external
COMPOSE_PULL=true

DOMAIN=weknora.example.com
APP_EXTERNAL_URL=https://weknora.example.com
DISABLE_REGISTRATION=true

DB_HOST=192.168.10.21
DB_PORT=5432
DB_USER=weknora
DB_PASSWORD=change-me-strong-db-password
DB_NAME=weknora
EXTERNAL_DB_WAIT_TIMEOUT=180

REDIS_ADDR=192.168.10.22:6379
REDIS_USERNAME=
REDIS_PASSWORD=change-me-strong-redis-password
REDIS_DB=0
EXTERNAL_REDIS_WAIT_TIMEOUT=120

STORAGE_TYPE=minio
MINIO_ENDPOINT=192.168.10.23:9000
MINIO_ACCESS_KEY_ID=change-me-minio-access-key
MINIO_SECRET_ACCESS_KEY=change-me-minio-secret-key
MINIO_BUCKET_NAME=weknora-prod

JWT_SECRET=change-me-openssl-rand-base64-48
TENANT_AES_KEY=change-me-keep-stable-value
SYSTEM_AES_KEY=change-me-exactly-32-byte-value
CRYPTO_MASTER_KEY=change-me-openssl-rand-hex-32
CRYPTO_SALT=change-me-openssl-rand-base64-32
```

密钥生成参考：

```bash
openssl rand -base64 48
openssl rand -hex 32
openssl rand -base64 32
```

必须离线保存以下字段：

1. `JWT_SECRET`
2. `TENANT_AES_KEY`
3. `SYSTEM_AES_KEY`
4. `CRYPTO_MASTER_KEY`
5. `CRYPTO_SALT`
6. 数据库密码
7. Redis 密码或 ACL 凭据
8. 对象存储密钥

## 5. 执行部署前检查

执行：

```bash
bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh check
```

预期输出包含：

1. 仓库路径。
2. 配置文件路径。
3. Compose 运行目录。
4. 外置 PostgreSQL 地址。
5. 外置 Redis 地址。
6. App、docreader、frontend、sandbox 四个镜像名。

若出现以下错误，先修正配置：

1. `DB_HOST must point to a LAN reachable external host`
2. `REDIS_ADDR must use host:port`
3. `Required config ... is empty or still a placeholder`

## 6. 准备运行目录

首次上线前创建目录：

```bash
sudo mkdir -p /opt/weknora-external
sudo chown -R $USER:$USER /opt/weknora-external
```

复制运行文件：

```bash
bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh prepare-dir
```

脚本会复制：

1. `docker-compose.yml`
2. `.env.example`
3. `config/`
4. `scripts/`
5. `migrations/`
6. `skills/`
7. `docker/searxng/`

如果 `COMPOSE_PROJECT_DIR` 就是仓库根目录，脚本会跳过复制。

## 7. 渲染生产文件

### 7.1 渲染 `.env`

```bash
bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh render-env
```

输出：

```text
/opt/weknora-external/.env
```

这个 `.env` 会写入：

1. 外置 PostgreSQL 地址、账号、库名。
2. 外置 Redis 地址、认证信息。
3. 外置依赖等待超时参数。
4. 对象存储配置。
5. App、frontend、docreader 运行参数。
6. JWT / AES / Crypto 密钥。

### 7.2 渲染 override 和等待脚本

```bash
bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh render-override
```

输出：

```text
/opt/weknora-external/docker-compose.prod.override.yml
/opt/weknora-external/.weknora-compose/wait-external-deps.sh
```

这里的关键点是：

1. App 容器会覆盖掉默认的 `DB_HOST=postgres` 和 `REDIS_ADDR=redis:6379`。
2. App 启动前会先等待远端 PostgreSQL 和远端 Redis 可达。
3. `postgres` 与 `redis` 服务会变成“占位依赖 + 连通性检查容器”，不再承载本地数据库和本地 Redis 数据。
4. 镜像会覆盖成企业仓库版本镜像。

之后脚本统一使用：

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.override.yml ...
```

## 8. 首次上线

### 8.1 一键上线

```bash
bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh online
```

`online` 会依次执行：

1. `check`
2. `prepare-dir`
3. `deploy`
4. `verify`

### 8.2 分步上线

如果需要每一步人工确认，按以下顺序执行：

```bash
bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh check
bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh prepare-dir
bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh render-env
bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh render-override
bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh pull
bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh deploy
bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh verify
```

说明：当前 App 默认会在启动时执行 `migrations/versioned` 数据库迁移，迁移失败会直接阻断服务启动。因此首次上线除了看 App 日志，还要重点看远端 PostgreSQL 扩展和权限是否满足要求。

## 9. 验证上线结果

执行：

```bash
bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh verify
```

脚本会检查：

1. `docker compose ps`
2. `docker compose logs app --tail=120`
3. `docker compose logs docreader --tail=80`
4. 占位 `postgres` 服务到远端 PostgreSQL 的 `pg_isready`
5. 占位 `redis` 服务到远端 Redis 的 `PING`
6. `curl http://127.0.0.1:${APP_PORT}/health`
7. `curl -I http://127.0.0.1:${FRONTEND_PORT}/`

手工检查示例：

```bash
cd /opt/weknora-external

docker compose -f docker-compose.yml -f docker-compose.prod.override.yml ps
docker compose -f docker-compose.yml -f docker-compose.prod.override.yml logs app --tail=200
docker compose -f docker-compose.yml -f docker-compose.prod.override.yml exec -T postgres \
  pg_isready -h 192.168.10.21 -p 5432 -U weknora -d weknora
docker compose -f docker-compose.yml -f docker-compose.prod.override.yml exec -T redis \
  sh -lc 'redis-cli -a "$REDIS_PASSWORD" -h 192.168.10.22 -p 6379 ping'
curl -f http://127.0.0.1:8080/health
curl -I http://127.0.0.1:80/
```

业务验收：

1. 浏览器访问 `APP_EXTERNAL_URL`。
2. 登录系统。
3. 配置模型、Embedding、Rerank。
4. 创建知识库。
5. 上传 PDF、DOCX、URL 文档。
6. 确认 docreader 完成解析。
7. 发起问答并查看引用片段。

## 10. 备份与迁移

### 10.1 远端 PostgreSQL 备份

升级前必须备份。

```bash
bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh backup-db
```

默认输出：

```text
/opt/weknora-external/backups/weknora_${DB_NAME}_YYYYmmddHHMMSS.dump
```

这个命令不是备份本地容器数据库，而是通过占位 `postgres` 容器内置的 `pg_dump` 直接连接远端 PostgreSQL：

```bash
pg_dump -h ${DB_HOST} -p ${DB_PORT} -U ${DB_USER} -d ${DB_NAME} -Fc
```

恢复示例：

```bash
pg_restore -h 192.168.10.21 -p 5432 -U weknora -d weknora --clean --if-exists backup.dump
```

对象存储仍需单独备份，例如：

```bash
mc mirror minio/weknora-prod ./backup-object/weknora-prod-$(date +%Y%m%d%H%M%S)
```

### 10.2 数据库迁移辅助命令

如果需要手工触发迁移：

```bash
bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh migrate
```

脚本会在 App 容器内执行：

```bash
/app/scripts/migrate.sh version
/app/scripts/migrate.sh up
```

如果迁移失败，优先检查：

1. 远端 PostgreSQL 是否可达。
2. 数据库账号密码是否正确。
3. `uuid-ossp`、`pg_trgm`、`vector`、`pg_search` 是否满足当前迁移要求。
4. 数据库账号是否有建表、建索引、创建扩展权限。
5. 是否存在 dirty migration 状态。

## 11. 升级与回滚

### 11.1 升级

先修改：

```bash
vi my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.env
```

更新版本号：

```dotenv
WEKNORA_VERSION=0.6.0-20260528.2
```

然后执行：

```bash
bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh backup-db
bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh deploy
bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh verify
```

### 11.2 回滚

回滚前必须确认数据库结构仍兼容旧版本。若新版本已经执行不可逆迁移，应先恢复数据库备份。

执行：

```bash
ROLLBACK_VERSION=0.6.0-20260528.1 \
bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh rollback
```

脚本会：

1. 修改 `/opt/weknora-external/.env` 的 `WEKNORA_VERSION`。
2. 重新渲染 `docker-compose.prod.override.yml`。
3. 拉取旧镜像。
4. 执行 `docker compose up -d --remove-orphans`。
5. 自动验证远端依赖和 App 健康状态。

## 12. 常见问题

### 12.1 App 一直卡在等待 PostgreSQL

查看：

```bash
bash my_docs/运维部署/20260528-04-weknora-compose-external-db-redis.sh logs
```

重点检查：

1. `DB_HOST`、`DB_PORT` 是否写成了容器内地址或本机 `localhost`。
2. 数据库防火墙、安全组、`pg_hba.conf` 是否放行当前应用服务器。
3. 数据库账号是否可以从应用服务器所在网段登录。

### 12.2 App 一直卡在等待 Redis

重点检查：

1. `REDIS_ADDR` 是否为 `host:port`。
2. Redis 是否启用了密码或 ACL，但配置文件里未同步填写。
3. Redis 是否只绑定了 `127.0.0.1`。
4. Redis 是否开启了受保护模式，导致远端访问被拒绝。

### 12.3 App 健康检查失败，但 PostgreSQL / Redis 都正常

继续看 App 日志，通常是以下原因：

1. 自动迁移失败。
2. PostgreSQL 缺少扩展。
3. `config/config.yaml` 中模型、存储、检索配置与当前环境不一致。
4. `DOCREADER_ADDR` 或对象存储配置错误。

### 12.4 为什么 Compose 里还能看到 `postgres` 和 `redis` 服务

这是当前方案的设计结果，不是误配。

原因：仓库根 `docker-compose.yml` 中 App 对 `postgres` 和 `redis` 存在固定 `depends_on`，而 Compose override 对 `depends_on` 的行为是“追加合并”，不能直接删除原依赖。因此脚本将这两个服务改造成“占位依赖 + 连通性检查容器”，用来：

1. 满足现有 Compose 依赖关系。
2. 复用 `pg_isready`、`pg_dump`、`redis-cli` 做远端连通性和备份检查。
3. 避免在应用机真正启动一套本地 PostgreSQL / Redis 数据服务。

## 13. 最终上线判定标准

满足以下条件后，可认为“局域网外置 PostgreSQL / 外置 Redis”的 Compose 生产上线完成：

1. `/opt/weknora-external/.env` 已生成并离线备份。
2. `/opt/weknora-external/docker-compose.prod.override.yml` 已生成。
3. `/opt/weknora-external/.weknora-compose/wait-external-deps.sh` 已生成。
4. `app`、`frontend`、`docreader` 容器启动正常。
5. 占位 `postgres` 服务可以连通远端 PostgreSQL。
6. 占位 `redis` 服务可以 `PING` 远端 Redis。
7. `http://127.0.0.1:${APP_PORT}/health` 返回 200。
8. 前端入口返回 200 或 304。
9. 系统可登录、可上传文档、可完成解析与问答。
10. 远端 PostgreSQL 备份策略和对象存储备份策略已确认。
11. 旧版本镜像仍可拉取，回滚命令已记录。

至此，这一版 Docker Compose 外置 PostgreSQL / 外置 Redis 的生产上线文档和脚本已经补齐，可独立用于局域网分离式部署。