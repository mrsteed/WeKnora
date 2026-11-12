#!/usr/bin/env bash
set -euo pipefail

#
# 租户「模型管理 + 网络搜索」数据同步脚本（直库写入）。
#
# 功能：把【源租户】（默认 张毅强 tenant 10004）下的
#   1) models              —— 模型管理（LLM/Embedding/Rerank 等）
#   2) web_search_providers —— 网络搜索供应商
# 复制进【目标租户】。两张表都是纯 tenant_id 作用域配置（组织 = 租户顶层
# organization，数据挂在租户下），行内 parameters(jsonb) 只含远程端点配置
# （base_url/api_key 等），不含任何租户/组织 id，可整体照搬。
#
# 规则：
#   - 每条生成新 UUID（PK 全局唯一，不能复用源 id）
#   - 幂等去重：models 按 name+type、web_search_providers 按 name+provider，
#     目标租户已有的跳过，可重复执行
#   - 单事务，ON_ERROR_STOP，任何报错自动回滚
#   - 只增不删：不改动、不删除目标租户已有数据
#
# 用法:
#   ./sync_tenant_models.sh <目标租户ID> [源租户ID]
#
#   目标租户ID   必填，tenants.id（如 10006 = 成都中法环境）
#   源租户ID     可选，默认 10004（张毅强）
#
# 示例:
#   sg docker -c "./my_docs/prod_scripts/sync_tenant_models.sh 10006"
#   sg docker -c "./my_docs/prod_scripts/sync_tenant_models.sh 10005 10004"
#
# 依赖: docker 组权限（或 sg docker -c 包裹）、WeKnora-postgres 容器运行中

TAG="[sync-tenant-models]"
log() { printf '%s %s\n' "$TAG" "$*"; }
warn() { printf '%s WARN: %s\n' "$TAG" "$*" >&2; }
die() { printf '%s ERROR: %s\n' "$TAG" "$*" >&2; exit 1; }

usage() {
  sed -n '3,30p' "$0" | sed 's/^# \{0,1\}//'
  exit "${1:-0}"
}

for a in "$@"; do [[ "$a" == -h || "$a" == --help ]] && usage 0; done
(( $# >= 1 && $# <= 2 )) || { usage 1; }

TARGET_TID="$1"
SRC_TID="${2:-10004}"
[[ "$TARGET_TID" =~ ^[0-9]+$ ]] || die "目标租户ID必须是纯数字（当前: $TARGET_TID）"
[[ "$SRC_TID" =~ ^[0-9]+$ ]] || die "源租户ID必须是纯数字（当前: $SRC_TID）"
[[ "$TARGET_TID" != "$SRC_TID" ]] || die "目标租户与源租户相同（$TARGET_TID），无需同步"

# ---------- 前置检查 ----------
command -v docker >/dev/null 2>&1 || die "未安装 docker"
docker exec WeKnora-postgres psql -U postgres -d weknora_prod -tAc "SELECT 1" >/dev/null 2>&1 \
  || die "无法访问 WeKnora-postgres 容器（权限或容器未运行）。不在 docker 组时请: sg docker -c \"\$0\""

# 注意用数组而非字符串拼接命令：本机 bash 5.3.9 在 $( "串" ) 内引用变量命令串
# 会整串当单命令名（不拆分），静默空输出。数组展开不受影响。
PSQL=(docker exec WeKnora-postgres psql -U postgres -d weknora_prod)
psqlq() { "${PSQL[@]}" -tAc "$1"; }

# ---------- 租户校验 ----------
TENANT_IDS="$(psqlq "SELECT id::text FROM tenants WHERE id IN ($SRC_TID,$TARGET_TID) ORDER BY id" 2>/dev/null || true)"
grep -qx "$TARGET_TID" <<<"$TENANT_IDS" || die "目标租户 $TARGET_TID 不存在（或被删除）"
grep -qx "$SRC_TID" <<<"$TENANT_IDS" || die "源租户 $SRC_TID 不存在（或被删除）"
TENANT_NAMES="$(psqlq "SELECT '[' || string_agg(t.name || '(' || t.id || ')', ', ') || ']' FROM tenants t WHERE t.id IN ($SRC_TID,$TARGET_TID)" 2>/dev/null || true)"
log "租户确认: ${TENANT_NAMES:-?}"

SRC_CNT="$(psqlq "SELECT (SELECT count(*) FROM models WHERE tenant_id=$SRC_TID AND deleted_at IS NULL) * 1000 + (SELECT count(*) FROM web_search_providers WHERE tenant_id=$SRC_TID AND deleted_at IS NULL)" 2>/dev/null || true)"
SRC_TOTAL=$(( SRC_CNT / 1000 )); SRC_PROVIDERS=$(( SRC_CNT % 1000 ))
(( SRC_TOTAL > 0 )) || die "源租户 $SRC_TID 没有可同步的模型数据"
log "源租户 ${SRC_TID}: models=${SRC_TOTAL} web_search_providers=${SRC_PROVIDERS}"

# ---------- 快照（同步前） ----------
snap() { # $1=租户id
  psqlq "SELECT (SELECT count(*) FROM models WHERE tenant_id=$1 AND deleted_at IS NULL) || '|' || (SELECT count(*) FROM web_search_providers WHERE tenant_id=$1 AND deleted_at IS NULL)"
}
BEFORE="$(snap "$TARGET_TID")"
log "目标租户 $TARGET_TID 同步前: models=${BEFORE%%|*} providers=${BEFORE##*|}"

# ---------- 同步（单事务；两条 INSERT 均为脚本受控常量 + 源表数据，无外部拼接） ----------
# 关键: docker exec 必须带 -i，否则 heredoc 的 SQL 不会传给 psql（stdin 断开 → 空跑"成功"）
docker exec -i WeKnora-postgres psql -U postgres -d weknora_prod -v ON_ERROR_STOP=1 <<SQL
BEGIN;

-- 1) 模型管理（models）：新 UUID，tenant_id 改目标，其余列（含 parameters 端点/api_key）照搬
INSERT INTO models (id, tenant_id, name, display_name, type, source, description,
                    parameters, is_default, status, is_builtin, managed_by,
                    created_at, updated_at, deleted_at)
SELECT gen_random_uuid(), $TARGET_TID,
       m.name, m.display_name, m.type, m.source, m.description, m.parameters,
       m.is_default, m.status, m.is_builtin, m.managed_by,
       m.created_at, m.updated_at, NULL
FROM models m
WHERE m.tenant_id = $SRC_TID AND m.deleted_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM models t
                  WHERE t.tenant_id = $TARGET_TID AND t.name = m.name AND t.type = m.type
                    AND t.deleted_at IS NULL);

-- 2) 网络搜索（web_search_providers）：同上，按 name+provider 去重
INSERT INTO web_search_providers (id, tenant_id, name, provider, description, parameters,
                                  is_default, created_at, updated_at, deleted_at)
SELECT gen_random_uuid(), $TARGET_TID,
       w.name, w.provider, w.description, w.parameters,
       w.is_default, w.created_at, w.updated_at, NULL
FROM web_search_providers w
WHERE w.tenant_id = $SRC_TID AND w.deleted_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM web_search_providers t
                  WHERE t.tenant_id = $TARGET_TID AND t.name = w.name AND t.provider = w.provider
                    AND t.deleted_at IS NULL);

COMMIT;
SQL

# ---------- 结果核对（同步后） ----------
AFTER="$(snap "$TARGET_TID")"
log "目标租户 $TARGET_TID 同步后: models=${AFTER%%|*} providers=${AFTER##*|}"
log "同步完成 ✓  落库明细:"
docker exec -i WeKnora-postgres psql -U postgres -d weknora_prod <<SQL
SELECT 'models' AS tbl, name, type AS detail FROM models
WHERE tenant_id=$TARGET_TID AND deleted_at IS NULL
UNION ALL
SELECT 'web_search_providers', name, provider FROM web_search_providers
WHERE tenant_id=$TARGET_TID AND deleted_at IS NULL
ORDER BY 1, 2;
SQL
