#!/usr/bin/env bash
# 检查 WeKnora 本机部署所需的基础依赖：docker、docker compose、NVIDIA Container Toolkit、curl、openssl。
# 缺什么会给出具体安装命令，不做静默失败。

set -u

have() { command -v "$1" >/dev/null 2>&1; }

need_docker=0
need_compose=0
need_curl=0
need_openssl=0
need_nvidia=0
need_docker_access=0

if ! have docker; then
  echo "[缺] docker 命令未找到" >&2
  need_docker=1
fi

if have docker; then
  docker_info_err="$(docker info 2>&1 >/dev/null || true)"
  if [[ -n "$docker_info_err" ]]; then
    if [[ "$docker_info_err" == *"permission denied"* || "$docker_info_err" == *"Got permission denied"* || "$docker_info_err" == *"docker.sock"* ]]; then
      echo "[缺] 当前用户无权访问 docker daemon (docker.sock 权限不足)" >&2
      need_docker_access=1
    elif echo "$docker_info_err" | grep -qi 'Cannot connect to the Docker daemon\|Is the docker daemon running'; then
      echo "[缺] docker daemon 未运行" >&2
      need_docker_access=1
    else
      echo "[缺] docker info 执行失败: $docker_info_err" >&2
      need_docker_access=1
    fi
  fi
fi

# docker compose 子命令是 Docker v2 plugin 的常见形态
if have docker; then
  if ! docker compose version >/dev/null 2>&1; then
    echo "[缺] docker compose (v2 plugin) 不可用" >&2
    need_compose=1
  fi
else
  if ! have docker-compose; then
    echo "[缺] docker-compose 也未安装，建议安装 docker compose v2 plugin" >&2
    need_compose=1
  fi
fi

if ! have curl; then
  echo "[缺] curl 未安装" >&2
  need_curl=1
fi

if ! have openssl; then
  echo "[缺] openssl 未安装" >&2
  need_openssl=1
fi

# nvidia-container-cli 是 docker 调用 GPU 的关键
if ! have nvidia-container-cli; then
  echo "[缺] nvidia-container-cli 未安装，GPU 容器将无法启动" >&2
  need_nvidia=1
fi

if [[ $need_docker -eq 0 && $need_compose -eq 0 && $need_curl -eq 0 && $need_openssl -eq 0 && $need_nvidia -eq 0 && $need_docker_access -eq 0 ]]; then
  echo "[OK] 基础依赖齐全"
  exit 0
fi

cat <<'EOF'

需要补齐的前置环境：

1. Docker 与 Compose v2 (Ubuntu 26.04):
   sudo apt-get update
   sudo apt-get install -y ca-certificates curl gnupg
   sudo install -m 0755 -d /etc/apt/keyrings
   curl -fsSL https://download.docker.com/linux/ubuntu/gpg | \
     sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
   sudo tee /etc/apt/sources.list.d/docker.list > /dev/null <<'SRC'
deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo $VERSION_CODENAME) stable
SRC
   sudo apt-get update
   sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
   sudo usermod -aG docker $USER
   newgrp docker
   docker version

  如果 docker 已安装但当前账号仍然提示 permission denied / docker.sock，
  说明当前 shell 还没有 docker 组权限。请重新登录，或者临时使用：
  sudo docker info

   注意：Ubuntu 26.04 的 /etc/os-release 没有 VERSION_CODENAME 这一行，
   直接套用上面脚本里 "VERSION_CODENAME" 会展开成空字符串，apt 会报
   “文件 list 第 1 行的记录格式有误”。如果遇到该报错，请手动把
   /etc/apt/sources.list.d/docker.list 的 $(. /etc/os-release && echo $VERSION_CODENAME)
   替换成实际代号 resolute，或者重新执行：

   printf '%s\n' "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu resolute stable" | \
     sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
   sudo apt-get update

2. NVIDIA Container Toolkit:
   curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | \
     sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg
   curl -fsSL https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | \
     sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
     sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list > /dev/null
   sudo apt-get update
   sudo apt-get install -y nvidia-container-toolkit
   sudo nvidia-ctk runtime configure --runtime=docker
   sudo systemctl restart docker

3. 验证 NVIDIA 容器是否能拿到 GPU:
   docker run --rm --gpus all nvidia/cuda:12.6.3-base-ubuntu24.04 nvidia-smi

EOF
exit 1