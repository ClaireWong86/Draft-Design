# Prompt Loop — AI 交接文档（HANDOFF）

> 给下一个 AI 编程工具 / 工程师：先读本文件，再按需深入 PRD/TRD/代码。  
> 更新时间：2026-07-17（UTC+8）

---

## 1. 一句话现状

Prompt Loop（Coze Loop fork）本地栈可跑；**智能优化**已交付 **PRD + TRD + Prompt 开发页 Mock UI**（入口位置对齐商业版），**尚未接真实实验/评测集 API 与后端 Worker**。与扣子罗盘商业版仍有明显产品差距（见 §6）。

---

## 2. 仓库与分支

| 项 | 值 |
|----|-----|
| 本地路径 | `/Users/wangdujuan10/Projects/Draft-Design` |
| 远程 | `https://github.com/ClaireWong86/Draft-Design.git` |
| 工作分支 | `feat/prompt-loop-multimodal-joybuild` |
| PR | https://github.com/ClaireWong86/Draft-Design/pull/1 |
| 最新相关提交 | `f1ec7069` — `feat(prompt): add Smart Optimize PRD/TRD and Mock UI entry` |
| 勿提交 | `.local-migration/`（Docker volume 备份） |

读仓入口：根目录 [AGENTS.md](../../AGENTS.md)、[ARCHITECTURE.md](../../ARCHITECTURE.md)。

---

## 3. 本地环境（Podman）

本机用 **Podman Desktop**，不用 Docker Desktop。VM 内存建议 ≥5GB。

| 服务 | URL | 备注 |
|------|-----|------|
| Web（nginx 静态） | http://localhost:8082 | 旧镜像，**不含**最新智能优化前端 |
| OpenAPI | http://localhost:8888/ping → `pong` | |
| 前端热更新 | http://localhost:8090 | **含**智能优化代码；API 代理到 `:8888` |

账号（本地）：`promptloop@local.dev` / `PromptLoop@2026`  
（runbook 另有 smoke：`codex-local-smoke@example.com` / `Codex123456`）

### 每天启动

```bash
export PATH="/opt/podman/bin:$PATH"
podman machine start   # 若未 running
# 防 idle 熄火
nohup podman machine ssh --username root 'sleep 28800' >/tmp/podman-keepalive.log 2>&1 &

# 栈（若容器已停）
scripts/local/docker-compose-local.sh start
# 或逐个 podman start coze-loop-*

# 前端热更新（智能优化调试用这个）
cd frontend/apps/cozeloop && npm run dev   # → :8090，可能只绑 [::1]
```

速查：[runbook.md](./runbook.md)、[local-macos-docker.md](../guidance/local-macos-docker.md)。

### 常见坑

- Podman VM OOM：加内存；保持 keepalive SSH。
- App panic 缺 RMQ topic：`trace_ingestion_event` 需存在。
- `:8090` 可能只 listen IPv6（`[::1]`），浏览器 `localhost` 一般可用。
- 见最新 UI：用 **8090**；或 rebuild `@cozeloop/community-base` → `coze-loop:local-fix` → `restart-app` 同步 nginx volume。

---

## 4. 智能优化：已交付物

### 文档

| 文件 | 内容 |
|------|------|
| [prd-smart-prompt-optimize.md](./prd-smart-prompt-optimize.md) | 目标、入口、流程、限制、指标 |
| [trd-smart-prompt-optimize.md](./trd-smart-prompt-optimize.md) | 架构、**Eval-Driven Iterative Prompt Rewrite** 算法、API 草案、超参 |

算法结论（务必读 TRD）：商业版**未开源**内部算法；本地定义为「诊断 → Meta-LLM 改写 → 同样本再评估 → 选优」循环；效果/性价比映射 N/T/K。勿声称二进制复刻商业黑盒。

### 前端代码（MVP Mock）

目录：

```text
frontend/packages/loop-pages/prompt-pages/src/components/smart-optimize/
  header-dropdown.tsx      # Header 下拉两路径
  wizard.tsx               # 多步向导（手填 ID / Mock）
  task-panel.tsx           # Tab：任务列表 + 报告 + 采纳
  mock-client.ts           # localStorage Mock OptimizeTask
  types.ts
  render-header-buttons.tsx
  index.ts
```

挂载点：[prompt-pages/.../develop/index.tsx](../../frontend/packages/loop-pages/prompt-pages/src/pages/develop/index.tsx)

- `renderHeaderButtons`：插在 **版本记录** 与 **提交新版** 之间  
- `extraTabs`：`smart_optimize` Tab  
- 采纳：`usePromptStore.setMessageList` 写草稿 + Toast，**未**自动打开提交弹窗  

接口占位：`@cozeloop/adapter-interfaces` → [prompt/index.ts](../../frontend/packages/loop-components/adapter-interfaces/src/prompt/index.ts)（`PromptAdapters` 类型已扩；实现暂未抽独立 `prompt-adapter` 包，因 rush update 策略受限）

Guard：`pe.prompt.smart_optimize` 已定义，入口**未包** `<Guard>`（OSS 默认放行）。

商业参考文档：https://loop.coze.cn/open/docs/cozeloop/optimize-prompts-with-ai  

---

## 5. 与商业版差距（下一工具优先做这些）

对照用户提供的扣子罗盘截图（实验页入口、「新建智能优化」选实验弹窗、带图标下拉）。

### P0 — 产品流程

1. **实验详情页也有「智能优化」入口**（评测模块目前为零）  
2. **首屏弹窗对齐**：「新建智能优化」+ 紫底说明 + **下拉选择真实评测实验**（`ListExperiments`），不要手填 ID  
3. **样本选择表**：接 `BatchGetExperimentResult`，10–500 条，校验成功实验 / 非满分 / reference 可映射  
4. **字段映射**：接评测集 schema，复用实验创建 mapping UI  
5. **后端 OptimizeTask + Worker**：按 TRD 算法；去掉仅 Mock 的闭环  

### P1 — UI / 体验

- 按钮星星图标；下拉项仪表盘 / 数据层图标  
- 报告：分数分布、评估器列、回跳样本  
- 采纳后打开「提交新版」流  
- 包 `Guard`；任务创建前做 Jinja2 / 多模态变量 / String 变量校验  

### 建议下一迭代顺序

1. 实验选择器 + 「新建智能优化」弹窗对齐截图  
2. 实验详情页入口（当前实验预填）  
3. 样本表 + 映射接真实 API  
4. 后端 Worker；Mock 仅作 `USE_MOCK_OPTIMIZE=1` 开关  

---

## 6. 相关已完成能力（非智能优化，但同分支）

- 品牌 Prompt Loop、多模态评测上传/预览、JoyBuild LLM adapter  
- 轮胎评测集导入、标签管理恢复、Podman/SELinux/RMQ 本地脚本加固  
- 详见 PR #1 与提交 `6e9f3241`

待办备忘（用户规则）：用户提供 `JOYBUILD_API_KEY` 后跑  
`import-tire-joybuild-config.sh` → `joybuild-check.sh` → `refresh-config` → `restart-app`。

---

## 7. 给下一个 AI 的开场指令（可复制）

```text
你在 Prompt Loop fork（Draft-Design）上工作。
先读 docs/prompt-loop/HANDOFF.md，再读 prd/trd-smart-prompt-optimize.md。
当前任务：缩小与扣子罗盘「智能优化」差距——优先做真实实验选择器 +「新建智能优化」弹窗，以及实验详情页入口。
前端热更新：http://localhost:8090 （后端 :8888 / Web :8082）。
分支：feat/prompt-loop-multimodal-joybuild。不要提交 .local-migration/。
```

---

## 8. 验证清单（切换前）

- [x] PRD / TRD 在仓  
- [x] Prompt 开发页 Header 下拉 + Tab（Mock）已推送  
- [x] 栈可访问 8082=200、8888=pong（交接时）  
- [x] 前端 dev 8090 可访问（交接时）  
- [ ] 真实 ListExperiments / 样本表  
- [ ] 实验页入口  
- [ ] 后端 Worker  
