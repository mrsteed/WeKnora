# WeKnora Docker Compose 生产上线步骤

## 1. 文档说明

本文从《20260528-02-WeKnora生产上线自动化手册.md》中单独拆出 Docker Compose 生产上线流程，适用于单机、小规模内网生产、演示生产或尚未接入 Kubernetes 的部署环境。

配套脚本：

```text
my_docs/运维部署/20260528-03-weknora-compose-prod.sh
```

这个脚本只处理 Docker Compose 生产上线，不包含 Helm/Kubernetes，也不包含源码构建流水线。它会完成：

1. 生成 Compose 生产配置模板。
2. 复制运行所需文件到生产目录。
3. 渲染生产 `.env`。
4. 渲染 `docker-compose.prod.override.yml`，把 Compose 默认镜像替换为企业仓库版本镜像。
5. 拉取镜像并启动服务。
6. 检查容器状态、App `/health` 和前端入口。
7. 执行 PostgreSQL 备份。
8. 执行数据库迁移辅助命令。
9. 支持回滚到旧镜像版本。

## 2. 适用范围和前置条件

### 2.1 适用范围

适合以下部署：

1. 单台 Linux 服务器。
2. Docker Compose 管理 WeKnora 全套核心服务。
3. App、frontend、docreader、sandbox 镜像已经构建并推送到企业镜像仓库。
4. PostgreSQL、Redis 使用当前 `docker-compose.yml` 内置服务。
5. 文件存储可使用 Compose 内置 MinIO profile，或使用外部 MinIO/S3 兼容存储。

### 2.2 不适用范围

以下场景优先使用 Helm 文档：

1. 多节点高可用。
2. 需要 Kubernetes Ingress、HPA、PDB、网络策略。
3. 数据库和 Redis 使用云托管并需要独立 Chart/Secret 管理。
4. 需要 GitOps 或外部 Secret Operator。

### 2.3 服务器工具要求

在生产服务器上确认：

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

生产运行目录默认使用：

```text
/opt/weknora
```

脚本配置文件默认使用：

```text
my_docs/运维部署/20260528-03-weknora-compose-prod.env
```

如果生产服务器上的仓库路径不同，可用环境变量指定：

```bash
export WEKNORA_REPO_ROOT=/path/to/WeKnora
```

如果配置文件路径不同，可用：

```bash
export WEKNORA_COMPOSE_ENV=/path/to/weknora-compose-prod.env
```

## 4. 初始化 Compose 生产配置

### 4.1 生成配置模板

```bash
cd /home/xmkp/workspace/WeKnora

bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh init-config
```

生成文件：

```text
my_docs/运维部署/20260528-03-weknora-compose-prod.env
```

权限会自动设置为 `600`。

如果文件已存在且确实要覆盖：

```bash
FORCE=true bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh init-config
```

### 4.2 编辑配置

```bash
vi my_docs/运维部署/20260528-03-weknora-compose-prod.env
```

必须替换所有 `change-me-*` 值。

关键配置示例：

```dotenv
REGISTRY=registry.example.com/weknora
WEKNORA_VERSION=0.6.0-20260528.1
COMPOSE_PROJECT_DIR=/opt/weknora
COMPOSE_PULL=true

DOMAIN=weknora.example.com
APP_EXTERNAL_URL=https://weknora.example.com
DISABLE_REGISTRATION=true

DB_USER=weknora
DB_PASSWORD=change-me-strong-db-password
DB_NAME=weknora

REDIS_PASSWORD=change-me-strong-redis-password

STORAGE_TYPE=minio
MINIO_ENDPOINT=minio:9000
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
7. Redis 密码
8. 对象存储密钥

## 5. 检查环境

```bash
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh check
```

预期输出包含：

1. 仓库路径。
2. 配置文件路径。
3. Compose 生产目录。
4. Docker Compose 命令类型。
5. 本次版本号。
6. App、docreader、frontend、sandbox 四个镜像名。

如果提示配置仍是 `change-me-*`，先回到第 4 章修改配置。

## 6. 准备生产运行目录

创建目录：

```bash
sudo mkdir -p /opt/weknora
sudo chown -R $USER:$USER /opt/weknora
```

复制运行文件：

```bash
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh prepare-dir
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

## 7. 渲染生产配置文件

### 7.1 渲染 `.env`

```bash
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh render-env
```

输出：

```text
/opt/weknora/.env
```

该文件会写入当前生产环境变量，包括：

1. `WEKNORA_VERSION`
2. 数据库配置
3. Redis 配置
4. MinIO/对象存储配置
5. JWT 和 AES 密钥
6. docreader 地址
7. sandbox 镜像
8. 文件大小限制
9. 外部访问地址

### 7.2 渲染 Compose override

```bash
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh render-override
```

输出：

```text
/opt/weknora/docker-compose.prod.override.yml
```

该文件会覆盖默认镜像：

1. `frontend.image=${REGISTRY}/weknora-ui:${WEKNORA_VERSION}`
2. `app.image=${REGISTRY}/weknora-app:${WEKNORA_VERSION}`
3. `docreader.image=${REGISTRY}/weknora-docreader:${WEKNORA_VERSION}`
4. `sandbox.image=${REGISTRY}/weknora-sandbox:${WEKNORA_VERSION}`

之后脚本会始终以以下方式执行 Compose：

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.override.yml ...
```

## 8. 拉取镜像

如果配置中：

```dotenv
COMPOSE_PULL=true
```

部署时会自动拉取镜像。也可以手动执行：

```bash
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh pull
```

如果使用私有仓库，先登录：

```bash
docker login registry.example.com
```

离线环境可以提前导入镜像：

```bash
docker load -i weknora-images_0.6.0-20260528.1.tar
```

## 9. 首次上线

### 9.1 一键上线

```bash
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh online
```

`online` 会依次执行：

1. `check`
2. `prepare-dir`
3. `deploy`
4. `verify`

### 9.2 分步上线

如果希望每一步人工确认，按以下顺序执行：

```bash
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh check
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh prepare-dir
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh render-env
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh render-override
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh pull
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh deploy
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh verify
```

说明：当前 App 默认会在启动时执行 `migrations/versioned` 数据库迁移，迁移失败会阻断 App 启动。因此首次上线需要重点观察 App 日志。

## 10. 验证上线结果

执行：

```bash
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh verify
```

脚本会检查：

1. `docker compose ps`
2. `docker compose logs app --tail=120`
3. `docker compose logs docreader --tail=80`
4. `curl http://127.0.0.1:${APP_PORT}/health`
5. `curl -I http://127.0.0.1:${FRONTEND_PORT}/`

也可以手工检查：

```bash
cd /opt/weknora

docker compose -f docker-compose.yml -f docker-compose.prod.override.yml ps
docker compose -f docker-compose.yml -f docker-compose.prod.override.yml logs app --tail=200
curl -f http://127.0.0.1:8080/health
curl -I http://127.0.0.1:80/
```

业务验收：

1. 浏览器访问 `APP_EXTERNAL_URL`。
2. 登录系统。
3. 配置模型、Embedding、Rerank。
4. 创建知识库。
5. 上传 PDF/DOCX/URL 文档。
6. 确认 docreader 完成解析。
7. 发起问答并查看引用片段。

## 11. 查看状态和日志

查看状态：

```bash
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh status
```

跟随日志：

```bash
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh logs
```

重启服务：

```bash
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh restart
```

停止服务：

```bash
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh stop
```

## 12. 数据库备份

升级前必须备份。

```bash
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh backup-db
```

默认输出：

```text
/opt/weknora/backups/weknora_${DB_NAME}_YYYYmmddHHMMSS.dump
```

脚本内部使用：

```bash
pg_dump -U ${DB_USER} -d ${DB_NAME} -Fc
```

恢复时可使用：

```bash
pg_restore -U ${DB_USER} -d ${DB_NAME} --clean --if-exists backup.dump
```

对象存储需要单独备份，例如：

```bash
mc mirror minio/weknora-prod ./backup-object/weknora-prod-$(date +%Y%m%d%H%M%S)
```

## 13. 数据库迁移辅助命令

当前生产推荐让 App 启动时自动迁移。若需要手工触发：

```bash
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh migrate
```

脚本会在 App 容器内执行：

```bash
/app/scripts/migrate.sh version
/app/scripts/migrate.sh up
```

如果迁移失败，优先检查：

1. PostgreSQL 是否 healthy。
2. 数据库账号密码是否正确。
3. 数据库账号是否有建表、建索引、创建扩展权限。
4. `migrations/versioned` 是否存在。
5. 是否出现 dirty migration 状态。

## 14. 升级流程

### 14.1 更新配置版本

编辑：

```bash
vi my_docs/运维部署/20260528-03-weknora-compose-prod.env
```

修改：

```dotenv
WEKNORA_VERSION=0.6.0-20260528.2
```

### 14.2 升级前备份

```bash
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh backup-db
```

### 14.3 执行升级

```bash
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh deploy
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh verify
```

`deploy` 会重新渲染 `.env` 和 override 文件，并按新版本启动服务。

## 15. 回滚流程

回滚前必须确认数据库结构兼容旧镜像。如果新版本已经执行不可逆迁移，只回滚镜像可能无法恢复服务，应优先恢复数据库备份。

执行：

```bash
ROLLBACK_VERSION=0.6.0-20260528.1 \
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh rollback
```

脚本会：

1. 修改 `/opt/weknora/.env` 的 `WEKNORA_VERSION`。
2. 重新渲染 `docker-compose.prod.override.yml` 为旧版本镜像。
3. 拉取旧镜像。
4. 执行 `docker compose up -d --remove-orphans`。
5. 自动验证。

## 16. dry-run 预演

如果只想查看将执行的命令：

```bash
DRY_RUN=true bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh deploy
```

也可以在配置文件里设置：

```dotenv
DRY_RUN=true
```

注意：`dry-run` 不会写生产目录、不拉镜像、不启动容器，但仍会做基础配置校验。

## 17. Compose profile 使用

默认不启用 profile，只启动核心服务。

如需完整依赖：

```dotenv
COMPOSE_PROFILES=full
```

如需只启用 SearXNG：

```dotenv
COMPOSE_PROFILES=searxng
```

多个 profile：

```dotenv
COMPOSE_PROFILES=searxng,jaeger
```

脚本会自动追加：

```bash
--profile searxng --profile jaeger
```

## 18. 最终上线判定标准

满足以下条件后，可认为 Docker Compose 生产上线完成：

1. `/opt/weknora/.env` 已生成并离线备份。
2. `/opt/weknora/docker-compose.prod.override.yml` 已生成。
3. `app`、`frontend`、`docreader`、`postgres`、`redis` 容器启动正常。
4. `http://127.0.0.1:${APP_PORT}/health` 返回 200。
5. 前端入口返回 200 或 304。
6. App 日志没有持续出现 `database migration failed`。
7. 可以登录系统。
8. 可以上传文档并完成解析。
9. 可以发起问答并返回引用片段。
10. 数据库和对象存储备份策略已确认。
11. 旧版本镜像仍可拉取，回滚命令已记录。

## 19. 常见问题

### 19.1 私有镜像仓库拉取失败

处理：

```bash
docker login registry.example.com
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh pull
```

确认 `REGISTRY` 和 `WEKNORA_VERSION` 正确。

### 19.2 App 迁移失败

查看日志：

```bash
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh logs
```

重点检查 PostgreSQL 扩展、权限、磁盘空间和 dirty migration 状态。

### 19.3 前端能打开但 API 失败

确认：

1. `APP_HOST=app`
2. `APP_BACKEND_PORT=8080`
3. App `/health` 正常
4. `frontend` 容器使用了正确 Nginx 配置

### 19.4 上传大文件 413

修改：

```dotenv
MAX_FILE_SIZE_MB=200
```

然后：

```bash
bash my_docs/运维部署/20260528-03-weknora-compose-prod.sh deploy
```

`MAX_FILE_SIZE_MB` 需要 App、frontend、docreader 同步重启后生效。

至此，Docker Compose 生产上线步骤和配套脚本已经拆分完成，可独立用于 Compose 生产部署。
