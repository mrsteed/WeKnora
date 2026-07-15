#!/usr/bin/env bash
# 在调用 docker compose 前先验证 docker 可执行文件是否存在。
# 如果系统中已经装了 docker 但 PATH 没读到，可以在这里设置 DOCKER_BIN。

set -u

: "${DOCKER_BIN:=docker}"

if ! command -v "$DOCKER_BIN" >/dev/null 2>&1; then
  echo "[缺] 找不到 docker 命令 (DOCKER_BIN=$DOCKER_BIN)" >&2
  cat <<'EOF'
解决办法：
1. 安装 docker：
   sudo apt-get update && sudo apt-get install -y docker.io
2. 安装 docker compose v2：
   sudo apt-get install -y docker-compose-plugin
3. 或者显式指定可执行文件位置后再调用脚本：
   DOCKER_BIN=/usr/bin/docker ./start_model_services.sh
4. 也可以直接运行 scripts/check_prereqs.sh 看完整缺失项。
EOF
  exit 1
fi

docker_info_err="$("$DOCKER_BIN" info 2>&1 >/dev/null || true)"
if [[ -n "$docker_info_err" ]]; then
  if [[ "$docker_info_err" == *"permission denied"* || "$docker_info_err" == *"Got permission denied"* || "$docker_info_err" == *"docker.sock"* ]]; then
    echo "[缺] 当前用户无权访问 docker daemon (docker.sock 权限不足)" >&2
    echo "      请重新登录使 docker 组生效，或使用 sudo docker 执行。" >&2
    exit 1
  fi

  if echo "$docker_info_err" | grep -qi 'Cannot connect to the Docker daemon\|Is the docker daemon running'; then
    echo "[缺] docker daemon 未运行，请先执行 sudo systemctl start docker" >&2
    exit 1
  fi

  echo "[缺] docker 不可用: $docker_info_err" >&2
  exit 1
fi

exit 0