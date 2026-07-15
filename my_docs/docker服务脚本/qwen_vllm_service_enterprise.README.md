# qwen_vllm_service_enterprise.sh — 使用说明

企业级生产脚本，用于在 **双路 Xeon Gold 6530 (64C/128T) + 4×RTX 4090 + 128GB
DDR5 + Ubuntu 26.04 + vLLM 0.23.x** 平台上部署 Qwen3.6-27B-FP8 张量并行推理服务。

> 重写自 `qwen_vllm_service_gpt.sh`（备份：`qwen_vllm_service_gpt.sh.bak.2026-07-01`）。
> 约 1180 行，覆盖硬件检测 / NUMA 调度 / 系统调优 / 容器生命周期 / 健康检查 /
> 基准测试。

---

## 快速开始

```bash
chmod +x qwen_vllm_service_enterprise.sh

# 1) 预检：探测硬件、NUMA、调度布局（不启动容器）
./qwen_vllm_service_enterprise.sh preflight

# 2) 预览 docker run 命令（不实际启动）
./qwen_vllm_service_enterprise.sh start --dry-run

# 3) 真正启动
./qwen_vllm_service_enterprise.sh start

# 4) 健康检查 / 周期 watch
./qwen_vllm_service_enterprise.sh health
./qwen_vllm_service_enterprise.sh watch

# 5) 性能基准（4 并发 × 8 轮）
./qwen_vllm_service_enterprise.sh bench
./qwen_vllm_service_enterprise.sh bench --concurrent 8 --max-tokens 512 --prompt "..."

# 6) 生命周期
./qwen_vllm_service_enterprise.sh status
./qwen_vllm_service_enterprise.sh logs
./qwen_vllm_service_enterprise.sh restart
./qwen_vllm_service_enterprise.sh stop
./qwen_vllm_service_enterprise.sh reset     # 重拉镜像 + 清理 + 重启
```

---

## 关键调度布局（本机自动检测结果）

| GPU    | NUMA | CPU 列表 (前 16 线程)         | vLLM TP rank |
|--------|------|--------------------------------|--------------|
| GPU0   | 0    | 0–15                           | rank 0       |
| GPU1   | 0    | 0–15（共享 NUMA0）             | rank 1       |
| GPU2   | 2    | 32–47                          | rank 2       |
| GPU3   | 3    | 48–63                          | rank 3       |

- 容器 cpuset-cpus: `0-15,32-63`（64 线程，正好覆盖 4×16）
- 容器 cpuset-mems: `0,2,3`（避开无 GPU 的 NUMA1）
- 每个 TP worker 独占 16 线程，绑定本地 NUMA

> 注：上面是本机实际拓扑。脚本会自动探测，无需硬编码。

---

## 用户硬性要求（已实现）

| 要求 | 实现位置 |
|---|---|
| CPU 64 核全部利用（CPU_THREADS_PER_WORKER=16 固定） | `CPU_THREADS_PER_WORKER=16` |
| `--cpu-shares=4096` | `DOCKER_CPU_SHARES=4096` |
| `--memory-swappiness=0` | `DOCKER_MEM_SWAPPINESS=0` |
| `MALLOC_ARENA_MAX` | `=4`（NUMA 友好，默认 8×cores 太激进） |
| `UV_THREADPOOL_SIZE` | `=32`（libuv 异步 IO 线程池） |
| `CUDA_DEVICE_MAX_CONNECTIONS` | `=32`（允许 kernel pipeline 并发） |
| `CUDA_MODULE_LOADING` | `=LAZY`（按需加载，节省启动内存） |
| `NCCL_IGNORE_CPU_AFFINITY` | `=1`（让 vLLM/用户管理 CPU 亲和） |
| `NCCL_BUFFSIZE` | `=33554432`（32 MiB，提升集合通信吞吐） |
| 双路 Xeon 6530 NUMA 调度 | `compute_optimal_layout()` |

附加调优：
- `NCCL_P2P_LEVEL=SYS`、`NCCL_IB_DISABLE=1`、`NCCL_SOCKET_IFNAME=^lo,docker`
- `NCCL_DEBUG=WARN`、`NCCL_ASYNC_ERROR_HANDLING=1`
- `PYTORCH_CUDA_ALLOC_CONF=expandable_segments:True`
- `OMP_NUM_THREADS / MKL_NUM_THREADS / OPENBLAS_NUM_THREADS = 16`
- `TOKENIZERS_PARALLELISM=true`
- `VLLM_USE_V1=1`
- 系统层 sysctl：`vm.swappiness=0`, `vm.overcommit_memory=1`, `kernel.numa_balancing=0`,
  `net.ipv4.tcp_congestion_control=bbr` 等
- 透明大页 = `madvise`
- CPU 调度器尝试设为 `performance`

---

## 文件位置

| 文件 | 行数 | 说明 |
|---|---|---|
| `qwen_vllm_service_enterprise.sh` | ~1180 | **新脚本（推荐使用）** |
| `qwen_vllm_service_enterprise.README.md` | — | 本文件 |
| `qwen_vllm_service_gpt.sh` | 463 | 原脚本（保留） |
| `qwen_vllm_service_gpt.sh.bak.2026-07-01` | 463 | 原脚本备份 |

---

## 日志

- 默认：`/var/log/qwen-vllm/<hostname>-<YYYYMMDD>.log`
- 可覆盖：`QWEN_VLLM_LOG_DIR=...`
- 调试：`QWEN_VLLM_DEBUG=1`

---

## 子命令一览

| 命令 | 说明 |
|---|---|
| `start` | 预检 + 启动容器 |
| `start --dry-run` | 仅打印 docker run 命令 |
| `start --dry-run --image foo:bar` | 指定镜像并预览 |
| `stop` | 优雅停止并移除容器 |
| `restart` | stop + start |
| `reset` | 删除容器/镜像 + 清理缓存 + 重拉镜像 + 重启 |
| `status` | 容器状态 + NUMA 拓扑概览 |
| `logs` | 跟踪容器日志（Ctrl+C 退出） |
| `health` | 单次健康检查（容器 + /health + /v1/models + GPU 快照） |
| `watch` | 周期健康检查（`QWEN_VLLM_WATCH_INTERVAL` 秒） |
| `bench` | 简单性能基准（TTFT 采样） |
| `preflight` | 仅运行预检，不启动容器 |
| `detect` | 仅打印硬件检测结果 |
| `version` / `-V` | 脚本版本 |
| `help` / `-h` | 帮助 |

---

## 常见调优开关

```bash
# 关闭 sysctl 调优（无 root 时）
QWEN_VLLM_APPLY_SYSCTLS=0 ./qwen_vllm_service_enterprise.sh start

# 强制启用 eager 模式（节省显存但降速）
QWEN_VLLM_ENFORCE_EAGER=1 ./qwen_vllm_service_enterprise.sh start

# 禁用 MTP 推测解码
QWEN_VLLM_ENABLE_MTP=0 ./qwen_vllm_service_enterprise.sh start

# 调整 NCCL 缓冲
QWEN_VLLM_NCCL_BUFFSIZE=67108864 ./qwen_vllm_service_enterprise.sh start
```

---

## 风险与注意

1. **首次启动会下载/加载 27B-FP8 模型**，时间较长（10–30 分钟，取决于磁盘）。
   健康检查 5 分钟内不会首次通过属正常。
2. **MTPSpeculative decoding** 与某些 vLLM 版本不兼容，启动失败时可设
   `QWEN_VLLM_ENABLE_MTP=0` 回退。
3. **NUMA 拓扑** 在 CPU 直连 GPU 的板卡上可能与本机不同，脚本会按
   `nvidia-smi topo -m` 自动重映射，无需手动指定。
4. **sysctl 调优** 需要 root 或 NET_ADMIN 权限；非 root 用户加
   `QWEN_VLLM_APPLY_SYSCTLS=0` 跳过。