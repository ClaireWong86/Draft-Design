# Contributing 补充 — Prompt Loop Fork

本文是 [CONTRIBUTING.md](../../CONTRIBUTING.md) 的 **fork 专用补充**。通用流程（分支、Commit 规范、Go 测试、CI）仍以上游 CONTRIBUTING 为准。

---

## 1. 本仓库与上游的关系

| 项 | 说明 |
|----|------|
| 上游 | [coze-dev/coze-loop](https://github.com/coze-dev/coze-loop) |
| 本 fork | [ClaireWong86/Draft-Design](https://github.com/ClaireWong86/Draft-Design) |
| 产品品牌 | UI 与面向用户文案使用 **Prompt Loop** |
| 代码标识 | 模块路径、`@cozeloop/*` 包名、部分 i18n key 仍保留 `coze` 前缀（刻意不改为减少 merge 冲突） |

合并上游时优先保留：**业务逻辑与 IDL 生成物**；冲突时 **品牌/i18n/脚本** 以本 fork 意图为准。

---

## 2. 什么应该改、什么不要改

### 欢迎贡献

- **用户可见品牌**：`loop-lng`  locales、登录页、Navbar、Footer、浏览器 title
- **本地运维脚本**：`scripts/local/`（Podman 生命周期、模型健康、seed、tunnel）
- **场景示例**：`examples/tire-ai-diagnosis/`
- **文档**：`docs/prompt-loop/`、`docs/guidance/local-macos-docker.md`
- **关联集成**：与 `tire-ai-diagnosis` 的配置导入、Trace 契约说明

### 请避免（除非有充分理由）

| 类型 | 原因 |
|------|------|
| 重命名 `@cozeloop/*`、`github.com/coze-dev/coze-loop` 模块路径 | 与上游 diff 巨大，升级困难 |
| 手改 `kitex_gen/`、`loop_gen/` | 应通过 IDL /codegen 重新生成 |
| 提交 `.env.local`、API Key、PAT | 密钥只放本地或密钥管理器 |
| 无 issue 的大规模架构重构 | 先与 maintainer 对齐 |

### 品牌文案原则

1. **界面与文档**写 Prompt Loop，不写 Coze Loop（评测里「智能体」已替代 Coze Bot 选项）。
2. **SDK 示例代码**中的 import 路径可保留 `coze-dev/cozeloop-go` 等真实包名；安装说明可加注释 `# Prompt Loop SDK`。
3. **环境变量**对外文档优先写 `PROMPT_LOOP_*`；SDK 仍兼容 `COZELOOP_*`。

---

## 3. 开发环境（fork 推荐路径）

### Prompt Loop 本仓库

```bash
# 1. 当前公司环境只使用 Podman；Docker Desktop 不可用
export PODMAN_LOOP="$HOME/Documents/Codex/2026-07-14/du-q/podman-fuse-bin/podman"
/opt/podman/bin/podman machine start podman-loop-dev
DOCKER_BIN="$PODMAN_LOOP" scripts/local/docker-compose-local.sh start

# 2. 配置模型（项目根 .env.local，勿提交）
#    OPENAI_API_KEY / ARK_API_KEY / DEEPSEEK_API_KEY / JOYBUILD_API_KEY

DOCKER_BIN="$PODMAN_LOOP" scripts/local/docker-compose-local.sh refresh-config
DOCKER_BIN="$PODMAN_LOOP" scripts/local/docker-compose-local.sh restart-app

# 3. 冒烟
scripts/local/smoke-test.sh

# 4. 前端 UI 改动（热更新）
cd frontend/apps/cozeloop && npm run dev   # http://localhost:8090
```

### 关联项目 tire-ai-diagnosis

路径通常为 `~/Projects/tire-ai-diagnosis`，通过 env 与 Prompt Loop 联调：

```bash
# 从 tire 导入 Ark / JoyBuild 到本仓库 .env.local
scripts/local/import-tire-ark-config.sh
scripts/local/import-tire-joybuild-config.sh
```

Trace 集成代码在 **tire 仓库**的 `services/api/clients/prompt_loop_trace.py`；改 Trace 字段时需同步更新 `docs/prompt-loop/architecture.md` 与 `examples/tire-ai-diagnosis/README.md`。

---

## 4. 本地脚本约定

新增或修改 `scripts/local/*.sh` 时请：

1. 使用 `#!/usr/bin/env bash` 与 `set -euo pipefail`
2. 提供 `usage()` 与子命令说明
3. **不要**在 stdout 打印完整 API Key / PAT（可打印 workspace id、模型名）
4. 敏感配置从 **项目根 `.env.local`** 读取，与 `docker-compose-local.sh` 一致
5. 若脚本需 Python，优先放在同目录 `*.py` 并由 `.sh` 调用（见 `seed_tire_prompt.py`）

常用脚本索引：

| 脚本 | 用途 |
|------|------|
| `docker-compose-local.sh` | compose-compatible 生命周期；当前由 Podman wrapper 调用 |
| `model-health-check.sh` | 模型连通性 |
| `seed-tire-prompt.sh` | 导入轮胎 Prompt + 评测集 |
| `create-smoke-pat.sh` | 生成 Trace 用 PAT（输出写入 tire .env） |
| `cloudflare-tunnel.sh` | Named Tunnel 安装与运行 |

---

## 5. 前端改动注意

| 场景 | 做法 |
|------|------|
| 改 UI 文案 | 编辑 `frontend/packages/loop-base/loop-lng/src/locales/**` |
| 改 Logo / 布局 | `frontend/apps/cozeloop/src/components/` |
| 验证 | 8090 dev；8082 需 rebuild 镜像才更新 |
| i18n key 名 | 含 `cozeloop` / `coze` 的 key **可保留**，只改 display 文案 |

运行前端 lint/test 遵循 `frontend/README.md` 与 Rush 工作流。

---

## 6. 后端改动注意

- 业务逻辑：`backend/modules/<domain>/`
- 新增 HTTP 路由：改 IDL → codegen → handler，**不要**只改生成物
- 本地模型列表：运行时由 `model_config.yaml` 注入容器，源码模板仍在 `release/deployment/docker-compose/conf/`

后端测试：

```bash
cd backend && go test -gcflags="all=-N -l" ./...
```

---

## 7. 文档要求（fork）

以下变更请 **同时更新文档**：

| 变更类型 | 更新位置 |
|----------|----------|
| 新 env 变量 | `docs/guidance/local-macos-docker.md` 或 `docs/prompt-loop/runbook.md` |
| 新 local 脚本 | 脚本 `usage()` + runbook 命令表 |
| 智能优化算法、任务状态或预算 | PRD + TRD + HANDOFF |
| Trace / OpenAPI 契约 | `architecture.md` + `examples/tire-ai-diagnosis/README.md` |
| 品牌/UI 大范围修改 | 考虑 `rebrand-changelog.md`（若创建） |

文档使用 **中文为主**（与本 fork 协作者习惯一致），命令与路径保持可复制。

---

## 8. Pull Request 检查清单

提交 PR 前请自检：

- [ ] 未包含 `.env.local`、密钥、PAT、个人域名凭证
- [ ] UI 文案无遗漏的 Coze Loop **用户可见** 品牌（SDK 包名除外）
- [ ] 若改 `scripts/local/`，已在本地跑通至少一条 happy path
- [ ] 若改 seed / 评测 schema，已运行 `bash scripts/local/seed-tire-prompt.sh` 验证
- [ ] 若改模型相关逻辑，已运行 `bash scripts/local/model-health-check.sh`
- [ ] 文档或脚本 `usage` 已同步
- [ ] 本地容器操作仅使用 Podman，且未执行 `podman system reset`

**Commit 类型**仍遵循 Conventional Commits；fork 特有改动可用 scope，例如：

```text
docs(prompt-loop): add architecture and runbook
feat(local): add cloudflare tunnel check command
fix(seed): set default_display_format for eval set schema
```

---

## 9. 安全与合规

- Apache 2.0 许可证不变；fork 新增文件保留 SPDX 头或与同目录一致
- PAT 仅用于开发/测试；生产使用独立账号与最小权限
- 不要将客户轮胎图片或诊断结果提交到 Git

---

## 10. 联系与范围

- GitHub Issues：bug、功能请求
- 轮胎业务场景：优先在 `examples/tire-ai-diagnosis/` 与 tire 仓库 issue 对齐

---

## 相关文档

- [CONTRIBUTING.md](../../CONTRIBUTING.md) — 上游通用规范
- [architecture.md](./architecture.md) — 架构图
- [runbook.md](./runbook.md) — 日常操作一页纸
