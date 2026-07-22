# Prompt Loop Runbook（一页纸）

> 日常启动、排错、轮胎场景联调速查。当前公司环境仅使用 **Podman**；Docker Desktop 不可用。详细状态见 [HANDOFF.md](./HANDOFF.md) 和 [architecture.md](./architecture.md)。

---

## 访问地址

| 服务 | URL | 用途 |
|------|-----|------|
| Prompt Loop Web | http://localhost:8082 | 登录、Prompt、评测、观测 |
| Prompt Loop OpenAPI | http://localhost:8888 | Trace / SDK |
| 前端 dev（热更新） | http://localhost:8090 | UI 开发 |
| tire 诊断 API | http://127.0.0.1:8000 | 轮胎检测（独立仓库） |

默认 smoke 账号：`codex-local-smoke@example.com` / `Codex123456`（仅本地）


当前高优待办见 [HANDOFF.md §5](./HANDOFF.md#5-当前高优待办按优先级)。本地 `no_gain` 镜像拉起/回归：

```bash
podman machine start podman-loop-dev
podman machine ssh --username root podman-loop-dev \
  'bash /Users/wangdujuan10/Projects/Draft-Design/.local-build/redeploy-no-gain-smoke.sh'
# 可选：python3 .local-build/regress-no-gain.py（在 VM 内）
```


---

## 每天开始（30 秒）

```bash
cd /path/to/Draft-Design

export PODMAN_LOOP="$HOME/Documents/Codex/2026-07-14/du-q/podman-fuse-bin/podman"
/opt/podman/bin/podman machine start podman-loop-dev
DOCKER_BIN="$PODMAN_LOOP" scripts/local/docker-compose-local.sh start
scripts/local/smoke-test.sh                   # Web + 登录 + 空间 OK
bash scripts/local/model-health-check.sh      # 看哪个模型可用
```

前端改 UI 时另开终端：

```bash
cd frontend/apps/cozeloop && npm run dev      # → :8090
```

---

## 常用命令

| 目的 | 命令 |
|------|------|
| 看容器状态 | `DOCKER_BIN="$PODMAN_LOOP" scripts/local/docker-compose-local.sh status` |
| 看日志 | `DOCKER_BIN="$PODMAN_LOOP" scripts/local/docker-compose-local.sh logs app` |
| 改模型后生效 | `refresh-config` → `restart-app` |
| 导入轮胎 Prompt/评测集 | `bash scripts/local/seed-tire-prompt.sh` |
| 生成 Trace 用 PAT | `bash scripts/local/create-smoke-pat.sh` |
| DeepSeek 连通 | `bash scripts/local/deepseek-check.sh` |
| 公网 tunnel 检查 | `bash scripts/local/cloudflare-tunnel.sh check` |
| 停止栈 | `DOCKER_BIN="$PODMAN_LOOP" scripts/local/docker-compose-local.sh stop` |

---

## 模型配置（`.env.local` 在项目根）

```bash
# 推荐可用
DEEPSEEK_API_KEY=sk-...
DEEPSEEK_MODEL=deepseek-v4-pro

# 可选（常遇 quota/欠费）
OPENAI_API_KEY=sk-...
ARK_API_KEY=ark-...
JOYBUILD_API_KEY=pk-...    # 已在本地配置；禁止提交或打印
```

改完后：

```bash
DOCKER_BIN="$PODMAN_LOOP" scripts/local/docker-compose-local.sh refresh-config
DOCKER_BIN="$PODMAN_LOOP" scripts/local/docker-compose-local.sh restart-app
```

从 tire 项目同步 Ark/JoyBuild：

```bash
scripts/local/import-tire-ark-config.sh
scripts/local/import-tire-joybuild-config.sh
```

---

## 轮胎场景联调

**Prompt Loop 侧**

```bash
bash scripts/local/seed-tire-prompt.sh
# → Prompt Key: tire_wheel_diagnosis
# → 评测集: 轮胎检测评测集
```

**tire API 侧**（`~/Projects/tire-ai-diagnosis/services/api/.env`）

```bash
PROMPT_LOOP_TRACE_ENABLED=true
PROMPT_LOOP_OPENAPI_URL=http://localhost:8888
PROMPT_LOOP_WORKSPACE_ID=<create-smoke-pat 输出>
PROMPT_LOOP_API_TOKEN=pat_...
MOCK_VLM=false          # 真实 VLM
VLM_PROVIDER=ark        # 或 joybuild
```

发一次诊断后，在 Prompt Loop **观测** 模块查 Trace。

---

## 故障速查

| 现象 | 处理 |
|------|------|
| Podman 容器 `Created` 不启动 | 确认 `podman-loop-dev` 正在运行，再通过 wrapper 执行 `docker-compose-local.sh` |
| Playground 报 `601505009` | 运行 `model-health-check.sh`；换 DeepSeek 或修 Key/余额 |
| OpenAI 429 / Ark 欠费 | `refresh-config` 会标记 unavailable；用 DeepSeek |
| UI 改动 8082 看不到 | 用 **8090 dev** 或通过 Podman rebuild 镜像 |
| seed 评测集失败 | 确认 `field_schemas` 含 `default_display_format: 1` |
| Trace 无数据 | 查 PAT、workspace_id、8888 可达、`PROMPT_LOOP_TRACE_ENABLED=true` |
| 脚本 permission denied | 使用 `bash scripts/local/xxx.sh` |

---

## 公网暴露（可选）

```bash
bash scripts/local/cloudflare-tunnel.sh setup   # 看步骤
# .env.local: CF_TUNNEL_HOSTNAME=promptloop.yourdomain.com
bash scripts/local/cloudflare-tunnel.sh install && login && create
bash scripts/local/cloudflare-tunnel.sh route-dns
bash scripts/local/cloudflare-tunnel.sh write-config && run
```

---

## 关键路径

```text
Draft-Design/                    ← Prompt Loop
  .env.local                     ← 模型密钥（勿提交）
  scripts/local/                 ← 运维脚本
  examples/tire-ai-diagnosis/    ← 轮胎样例

~/Projects/tire-ai-diagnosis/    ← 轮胎 API + H5
  services/api/.env              ← VLM + Trace
```

---

## 升级 / 合并上游前

1. `git fetch upstream`（若已配 coze-loop remote）
2. 重点解决冲突：`loop-lng` locales、Navbar、Footer、`scripts/local/`
3. 合并后：使用 `DOCKER_BIN="$PODMAN_LOOP"` 启动 + `smoke-test.sh` + `seed-tire-prompt.sh`

> 安全红线：不要执行 `podman system reset`，也不要切回 Docker Desktop。需要迁移或清理存储时先备份命名卷并更新 HANDOFF。

---

**文档**：[architecture.md](./architecture.md) · [contributing.md](./contributing.md) · [tire README](../../examples/tire-ai-diagnosis/README.md)
