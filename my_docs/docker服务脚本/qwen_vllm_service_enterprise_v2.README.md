# qwen_vllm_service_enterprise_v2.sh — 多实例版使用说明

企业级多实例生产脚本，在双路 Xeon Gold 6530 + 4×RTX 4090 + 128GB DDR5 + Ubuntu
26.04 + vLLM 0.23.x 上同时部署 **两份** Qwen3.6-27B-FP8 推理服务：

| 实例 | GPU | TP | 宿主机端口 | 模型名（vLLM served name）|
|---|---|---|---|---|
| 实例 A | GPU0 + GPU1 | 2 | 8000 | `qwen3.6-27b-fp8` |
| 实例 B | GPU2 + GPU3 | 2 | 8001 | `qwen3.6-27b-fp8` |

> 两个实例默认同名（客户端可以走端口分流，或在应用层做 round-robin）。
> 如需独立 model name，设 `QWEN_VLLM_INSTANCE_<idx>_MODEL_NAME`。

> v2 相对 v1 的改造：单实例→多实例、加入 `--language-model-only`、降低
> `--max-cudagraph-capture-size` 至 256、按 worker 精细 NUMA 分配。

---

## 关于 262144 上下文 + 2×4090 的可行性

**结论：大概率可行。**

关键事实：
- Qwen3.6-27B 是 **64 层混合架构**：16 个 block × (3 层 Gated DeltaNet + 1 层 Gated Attention)
- 仅 16/64 层有真正 KV cache，其余 48 层是 O(1) 循环状态
- TP=2 切分后每卡约 **13.5 GB 权重**；4090 24 GB × 0.90 利用率 ≈ 21.6 GB/卡可用
- 余量约 **8 GB/卡** 给 KV cache + 激活值
- `--kv-cache-dtype fp8` 再压缩一半
- `--language-model-only` 省掉 vision encoder（v2 默认开启）

启动后用 `docker logs` 看这几行直接告诉你实际可支撑多少并发：

```bash
docker logs weknora-qwen-vllm-a 2>&1 | grep -iE "GPU blocks|KV cache"
```

---

## 关于"内存共享"

| 方案 | 可行性 | 备注 |
|---|---|---|
| GPU P2P 显存共享 | ❌ 不可用 | 本机 `nvidia-smi topo -p2p r` 显示所有 GPU 对为 **CNS**（芯片组不支持） |
| CPU offload (`--cpu-offload-gb`) | ⚠️ 可用但不推荐 | 走 PCIe，与 TP 通信抢带宽；目前每卡 8 GB 余量够用 |
| 共享 HF cache (`/data/models/hf-cache`) | ✅ 已用 | 只读快照，两个容器可同时挂载 |

本脚本默认**不**启用 CPU offload。先跑满 2×4090 + 262K 上下文，不够再加。

---

## 推荐部署顺序（按连通性最优）

| 优先级 | 实例 | GPU 对 | 连接类型 |
|---|---|---|---|
| 1 | A | GPU0 + GPU1 | **NODE**（最佳） |
| 2 | B | GPU2 + GPU3 | **SYS**（跨 NUMA PCIe） |

本机拓扑（实测）：
```
GPU0 → NUMA0   GPU1 → NUMA0   GPU2 → NUMA2   GPU3 → NUMA3
NUMA0: 0-15, 64-79   NUMA2: 32-47, 96-111
NUMA1: 16-31, 80-95  NUMA3: 48-63, 112-127
```

---

## v2 自动 NUMA 调度结果（per-worker 精细分配）

```
[实例 A] worker 0 (GPU0 @ NUMA0) → CPUs 0-15
         worker 1 (GPU1 @ NUMA0) → CPUs 64-79
         cpuset-cpus = 0-15,64-79  mems=0

[实例 B] worker 0 (GPU2 @ NUMA2) → CPUs 32-47
         worker 1 (GPU3 @ NUMA3) → CPUs 112-127
         cpuset-cpus = 32-47,112-127  mems=2,3
```

每个 worker 严格绑定其 GPU 的本地 NUMA（worker 1 in B 跑在 NUMA3 而不是
全从 NUMA2 取，避免 PCIe 跨 NUMA 内存访问）。

合计 vLLM 占用 64 线程 = 4 worker × 16 线程，剩余 64 线程给系统/docker/其他。

---

## 快速开始

```bash
chmod +x qwen_vllm_service_enterprise_v2.sh

# 预检
./qwen_vllm_service_enterprise_v2.sh preflight

# 预览两个实例的 docker run 命令（不实际启动）
./qwen_vllm_service_enterprise_v2.sh start --dry-run

# 实际启动两个实例（**不再阻塞等就绪**，启动后立即返回）
./qwen_vllm_service_enterprise_v2.sh start

# 仅启动实例 A
./qwen_vllm_service_enterprise_v2.sh start 0

# 阻塞等 vLLM /v1/models 就绪（手动）
./qwen_vllm_service_enterprise_v2.sh wait-healthy         # 所有实例
./qwen_vllm_service_enterprise_v2.sh wait-healthy 0       # 仅实例 A

# 状态 + NUMA 拓扑
./qwen_vllm_service_enterprise_v2.sh status
./qwen_vllm_service_enterprise_v2.sh list

# 健康检查（并行检查两个实例）
./qwen_vllm_service_enterprise_v2.sh health
./qwen_vllm_service_enterprise_v2.sh health 0       # 仅实例 A
./qwen_vllm_service_enterprise_v2.sh watch

# 日志（并行输出，Ctrl+C 全部退出）
./qwen_vllm_service_enterprise_v2.sh logs
./qwen_vllm_service_enterprise_v2.sh logs 1         # 仅实例 B

# 基准测试（两个实例各自跑一次）
./qwen_vllm_service_enterprise_v2.sh bench --concurrent 8 --max-tokens 512

# 生命周期
./qwen_vllm_service_enterprise_v2.sh stop
./qwen_vllm_service_enterprise_v2.sh restart
./qwen_vllm_service_enterprise_v2.sh reset          # 重拉镜像 + 清理 + 重启
```

## start 的新行为（v2 与 v1 的关键差异）

| 行为 | v1 | v2 |
|---|---|---|
| `start` 是否阻塞等 vLLM 就绪 | 是（最长 `HEALTH_START_PERIOD=300s`） | **否**（启动后立即返回） |
| Docker `--restart` 策略 | `unless-stopped`（无限重启） | **`on-failure:3`**（最多自愈 3 次） |
| 容器启动失败后是否自动重试 | 否 | **否**（保持） |
| 阻塞等待的命令 | 内置在 start 里 | **`wait-healthy`**（独立子命令） |

恢复 v1 行为：
```bash
QWEN_VLLM_WAIT_HEALTHY=1 ./qwen_vllm_service_enterprise_v2.sh start
```

恢复无限重启（不推荐）：
```bash
QWEN_VLLM_RESTART_POLICY=unless-stopped ./qwen_vllm_service_enterprise_v2.sh start
```

---

## 客户端使用

```bash
# 实例 A（端口 8000）
curl http://localhost:8000/v1/chat/completions \
  -H "Authorization: Bearer sk-hlsa-local-vllm" \
  -d '{"model":"qwen3.6-27b-fp8","messages":[{"role":"user","content":"hi"}]}'

# 实例 B（端口 8001）
curl http://localhost:8001/v1/chat/completions \
  -H "Authorization: Bearer sk-hlsa-local-vllm" \
  -d '{"model":"qwen3.6-27b-fp8","messages":[{"role":"user","content":"hi"}]}'
```

两实例默认同名，靠端口区分；客户端负载均衡按 `:8000` / `:8001` 做 round-robin
即可。OpenAI SDK 配置示例：

```python
from openai import OpenAI
clients = [
    OpenAI(base_url="http://localhost:8000/v1", api_key="sk-hlsa-local-vllm"),
    OpenAI(base_url="http://localhost:8001/v1", api_key="sk-hlsa-local-vllm"),
]
# 按 idx % len(clients) 选 instance
```

---

## v2 新增 / 修改的关键参数

| 参数 | v2 默认值 | 说明 |
|---|---|---|
| `--language-model-only` | 启用 | 省 vision encoder 显存 |
| `--max-cudagraph-capture-size` | 256 | 绕开 Qwen3.6 混合架构 CUDA graph 报错 |
| `MALLOC_ARENA_MAX` | 2 | NUMA 友好（原 v1 = 4） |
| `UV_THREADPOOL_SIZE` | 32 | 节省内存（原 v1 = 64） |
| `CUDA_DEVICE_MAX_CONNECTIONS` | 32 | 保留 TP 通信叠加（原 v1 已为 32） |
| `NCCL_BUFFSIZE` | 8 MiB | 推理场景够用（原 v1 = 32 MiB 过激） |
| `--max-num-seqs` | 512 | 单实例并发（原 v1 = 768，因 TP 减半） |

---

## 环境变量

```bash
# 实例数与拓扑（默认即可工作）
QWEN_VLLM_INSTANCES=2
QWEN_VLLM_INSTANCE_0_NAME=weknora-qwen-vllm-a
QWEN_VLLM_INSTANCE_0_GPUS="0,1"
QWEN_VLLM_INSTANCE_0_TP=2
QWEN_VLLM_INSTANCE_0_PORT=8000
QWEN_VLLM_INSTANCE_0_MODEL_NAME=qwen3.6-27b-fp8-a

QWEN_VLLM_INSTANCE_1_NAME=weknora-qwen-vllm-b
QWEN_VLLM_INSTANCE_1_GPUS="2,3"
QWEN_VLLM_INSTANCE_1_TP=2
QWEN_VLLM_INSTANCE_1_PORT=8001
QWEN_VLLM_INSTANCE_1_MODEL_NAME=qwen3.6-27b-fp8-b

# 全局（共享）
QWEN_VLLM_MODEL=Qwen/Qwen3.6-27B-FP8
QWEN_VLLM_MAX_MODEL_LEN=262144
QWEN_VLLM_CPU_THREADS_PER_WORKER=16
QWEN_VLLM_LANGUAGE_MODEL_ONLY=1
QWEN_VLLM_MAX_CUDAGRAPH_CAPTURE_SIZE=256

# 系统调优
QWEN_VLLM_APPLY_SYSCTLS=1             # 非 root 时设为 0
QWEN_VLLM_LOG_DIR=/var/log/qwen-vllm
QWEN_VLLM_DEBUG=0                     # 设为 1 看 debug 日志
```

---

## 已知问题与缓解

1. **混合架构 CUDA graph 报错**（"CUDA graph / Mamba cache size error"）
   - 已默认 `--max-cudagraph-capture-size=256`；若仍报错可继续调小到 128
2. **长上下文 + 高并发** 可能 OOM
   - 启动时观察 `grep "GPU blocks" docker logs` 输出，若剩余 KV block 数太少
   - 降低 `--max-num-seqs` 或 `--max-model-len`
3. **两个实例同时启动时** 共享 HF cache 读取安全（只读），但若两实例并发加载
   模型权重到 GPU 显存，瞬时 PCIe 带宽紧张属正常

---

## 文件清单

| 文件 | 行数 | 用途 |
|---|---|---|
| `qwen_vllm_service_enterprise_v2.sh` | ~1500 | **多实例生产脚本** |
| `qwen_vllm_service_enterprise_v2.README.md` | — | 本文件 |
| `qwen_vllm_service_enterprise.sh` | 1183 | 单实例 v1（保留） |
| `qwen_vllm_service_enterprise.README.md` | 157 | v1 说明 |