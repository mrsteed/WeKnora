#!/usr/bin/env bash
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  if command -v sudo >/dev/null 2>&1; then
    exec sudo bash "$0" "$@"
  fi
  echo "请使用 root 运行，或先安装 sudo。" >&2
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive

apt-get update
apt-get install -y --no-install-recommends \
  chromium \
  pandoc \
  fonts-noto-cjk \
  fonts-noto-cjk-extra \
  fonts-liberation \
  texlive-xetex \
  texlive-lang-chinese \
  texlive-fonts-recommended

echo
echo "已安装的关键命令版本："
for bin in pandoc chromium xelatex; do
  if command -v "$bin" >/dev/null 2>&1; then
    printf '%s: ' "$bin"
    "$bin" --version | head -n 1
  else
    printf '%s: 未找到\n' "$bin"
  fi
done

echo
echo "安装完成。若服务运行在容器内，请同步补齐 docker/Dockerfile.app 中的 TeX 相关依赖。"