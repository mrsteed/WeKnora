#!/usr/bin/env bash
set -euo pipefail

#
# 生产库直接创建管理员账号（DISABLE_REGISTRATION=true 时注册接口被 403 拦截，
# 这是绕过邀请流程、用 SQL 直接落库的兜底通道）。
#
# 与 app 内部 Register 流程（user.go）完全对齐，落 3 张表：
#   1) tenants        —— 无租户时建一个默认工作区（有则复用第一个）
#   2) users          —— bcrypt(password_hash)，is_active=true
#   3) tenant_members —— role='owner'（= types.TenantRoleOwner），保证登录即全权
#
# 用法:
#   ./create_admin.sh                          # admin / admin@hlsa.com / Admin@Hlsa2026
#   ./create_admin.sh <username> <email> <password>
#
# 依赖: 宿主机 python3 + bcrypt（pip install bcrypt）; docker 组权限
# 幂等: 同 email/username 已存在时报错退出，不会重复建。

TAG="[create-admin]"
log() { printf '%s %s\n' "$TAG" "$*"; }
warn() { printf '%s WARN: %s\n' "$TAG" "$*" >&2; }
die() { printf '%s ERROR: %s\n' "$TAG" "$*" >&2; exit 1; }

USERNAME="${1:-admin}"
EMAIL="${2:-admin@hlsa.com}"
PASSWORD="${3:-Admin@Hlsa2026}"

# ---------- 前置检查 ----------
command -v python3 >/dev/null 2>&1 || die "宿主机缺 python3（用于生成 bcrypt 哈希）"
python3 -c 'import bcrypt' 2>/dev/null || die "缺 python bcrypt 库: pip install bcrypt"
command -v docker >/dev/null 2>&1 || die "未安装 docker"
docker exec WeKnora-postgres psql -U postgres -d weknora_prod -tAc "SELECT 1" >/dev/null 2>&1 \
  || die "无法访问 WeKnora-postgres 容器（权限或容器未运行）。不在 docker 组时请: sg docker -c \"\$0\""

PSQL="docker exec WeKnora-postgres psql -U postgres -d weknora_prod -tAc"

# 密码强度与前端登录端一致: 8-32 字符且含字母和数字
n=${#PASSWORD}
if (( n < 8 || n > 32 )); then die "密码长度须 8-32（当前 ${n}）"; fi
printf '%s' "$PASSWORD" | grep -qE '[A-Za-z]' || die "密码须含字母"
printf '%s' "$PASSWORD" | grep -qE '[0-9]'   || die "密码须含数字"

# 幂等检查（-t 单行模式；docker exec 需 -i 才能读 stdin，这里走 -c 参数不受影响）
dup_email="$(docker exec -i WeKnora-postgres psql -U postgres -d weknora_prod -tAc "SELECT 1 FROM users WHERE email='${EMAIL}' AND deleted_at IS NULL" 2>/dev/null || true)"
dup_user="$(${PSQL} "SELECT 1 FROM users WHERE username='${USERNAME}' AND deleted_at IS NULL" 2>/dev/null || true)"
[[ -n "$dup_email" || -n "$dup_user" ]] && die "用户已存在: username='${USERNAME}' email='${EMAIL}'"

# ---------- 生成 bcrypt 哈希（与 user.go 的 bcrypt.DefaultCost 一致 = 10） ----------
HASH="$(python3 -c 'import bcrypt,sys;print(bcrypt.hashpw(sys.argv[1].encode(), bcrypt.gensalt(10)).decode())' "$PASSWORD")"
case "$HASH" in \$2*\$10\$*) : ;; *) die "bcrypt 哈希格式异常: ${HASH:0:7}..." ;; esac
log "bcrypt 哈希已生成 (cost=10, 与 app 端 DefaultCost 一致)"

# ---------- 落库（单事务；SQL 值均为脚本受控输入，密码只走 HASH 变量注入） ----------
# 关键: docker exec 必须带 -i，否则 heredoc 的 SQL 不会传给 psql（stdin 断开 → 空跑成功）
#       ON_ERROR_STOP=1 保证任何 SQL 报错立即中止并回滚（BEGIN 未 COMMIT 自动回滚）
log "创建 user='${USERNAME}' email='${EMAIL}'（所属租户: 无则新建，成员角色 owner）"
docker exec -i WeKnora-postgres psql -U postgres -d weknora_prod -v ON_ERROR_STOP=1 <<SQL
BEGIN;
-- 1) 默认工作区（已有租户则跳过，复用最小 id 的租户）
--    name = "<username>'s Workspace"（chr(39) 拼撇号，避免 SQL 引号转义问题）
INSERT INTO tenants (name, description, status, business, created_at, updated_at)
SELECT '${USERNAME}' || chr(39) || 's Workspace', 'Default workspace', 'active', '', now(), now()
WHERE NOT EXISTS (SELECT 1 FROM tenants WHERE deleted_at IS NULL);

-- 2) 用户
INSERT INTO users (id, username, email, password_hash, tenant_id, is_active, created_at, updated_at)
VALUES (gen_random_uuid(), '${USERNAME}', '${EMAIL}', '${HASH}',
        (SELECT id FROM tenants ORDER BY id LIMIT 1), true, now(), now());

-- 3) Owner 成员关系（EnsureOwner 的 SQL 等价物）
INSERT INTO tenant_members (user_id, tenant_id, role, status, joined_at, created_at)
VALUES ((SELECT id FROM users WHERE email='${EMAIL}' AND deleted_at IS NULL),
        (SELECT id FROM tenants ORDER BY id LIMIT 1), 'owner', 'active', now(), now());
COMMIT;
SQL

# ---------- 结果核对 ----------
log "落库结果:"
docker exec WeKnora-postgres psql -U postgres -d weknora_prod \
  -c "SELECT u.username, u.email, u.tenant_id, t.name AS tenant, m.role
      FROM users u
      LEFT JOIN tenants t ON t.id = u.tenant_id
      LEFT JOIN tenant_members m ON m.user_id = u.id
      WHERE u.email='${EMAIL}' AND u.deleted_at IS NULL"

log "DONE. 登录: http://<host>/  → 邮箱 ${EMAIL} / 密码 ${PASSWORD}"
warn "请牢记该密码（仅本次输出，不落盘）；如需重置可 DROP 对应 users 行后用本脚本重建"
