# Prompt Loop — AI 交接文档（HANDOFF）

> 给下一个 AI 编程工具 / 工程师：先读本文件，再按需深入 PRD/TRD/代码。  
> 更新时间：2026-07-20（UTC+8，OptimizeTask 后端、Worker、优化模型选择与真实前端客户端）

---

## 1. 一句话现状

Prompt Loop（Coze Loop fork）本地栈可跑；Prompt 详情已形成**编排、观测、评测、智能优化**四工作区。智能优化已接通 OptimizeTask IDL、MySQL、真实 API、可选优化模型和可恢复异步 Worker；前端默认使用真实客户端。当前 Worker 已完成诊断、候选生成和一次临时候选执行；逐 case 执行、评估器重评、多轮选优仍待实现。

---

## 2. 仓库与分支

| 项 | 值 |
|----|-----|
| 本地路径 | `/Users/wangdujuan10/Projects/Draft-Design` |
| 远程 | `https://github.com/ClaireWong86/Draft-Design.git` |
| 工作分支 | `feat/prompt-loop-multimodal-joybuild` |
| PR | https://github.com/ClaireWong86/Draft-Design/pull/1 |
| 最新相关提交 | `10beb769` — `feat(evaluation): snapshot experiment evidence for optimizer` |
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

# 栈（若容器已停；脚本内部使用 Podman）
scripts/local/docker-compose-local.sh start
# 或逐个 podman start coze-loop-*

# 前端热更新（智能优化调试用这个）
cd frontend/apps/cozeloop && npm run dev   # → :8090，可能只绑 [::1]
```

速查：[runbook.md](./runbook.md)、[local-macos-docker.md](../guidance/local-macos-docker.md)。文档中的 Docker Desktop / `docker` 命令均不适用于当前环境，统一替换为 Podman。

### 常见坑

- Podman VM OOM：加内存；保持 keepalive SSH。
- App panic 缺 RMQ topic：`trace_ingestion_event` 需存在。
- `:8090` 可能只 listen IPv6（`[::1]`），浏览器 `localhost` 一般可用。
- 见最新 UI：用 **8090**；或 rebuild `@cozeloop/community-base` → `coze-loop:local-fix` → `restart-app` 同步 nginx volume。
- 新增后端路由或数据表后，仅重启旧容器不会生效：先构建 `coze-loop:local-fix`、执行对应 `patch-sql`，再强制重建 app 容器；2026-07-17 已完成 `optimize_task` 迁移并部署包含 OptimizeTask 路由的本地镜像。

---

## 4. 智能优化：已交付物

### 文档

| 文件 | 内容 |
|------|------|
| [prd-smart-prompt-optimize.md](./prd-smart-prompt-optimize.md) | 目标、入口、流程、限制、指标 |
| [trd-smart-prompt-optimize.md](./trd-smart-prompt-optimize.md) | 架构、**Eval-Driven Iterative Prompt Rewrite** 算法、API 草案、超参 |

算法结论（务必读 TRD）：商业版**未开源**内部算法；本地定义为「诊断 → Meta-LLM 改写 → 同样本再评估 → 选优」循环；效果/性价比映射 N/T/K。勿声称二进制复刻商业黑盒。

### 前端代码

目录：

```text
frontend/packages/loop-pages/prompt-pages/src/components/smart-optimize/
  header-dropdown.tsx      # Header 下拉两路径
  create-confirm-modal.tsx # Prompt Header 选择实验 + 确认版本
  wizard.tsx               # 独立页面向导（真实实验、Schema 映射、优化模型、真实任务）
  experiment-case-selector.tsx # BatchGetExperimentResult、分页/多选/多模态预览
  task-panel.tsx           # Tab：搜索/状态筛选/业务列 + 报告 + 采纳
  client.ts                # StoneEvaluationApi 真实 OptimizeTask 客户端
  mock-client.ts           # 仅保留的 localStorage 演示夹具，默认不使用
  types.ts
  render-header-buttons.tsx
  index.ts
```

挂载点：[prompt-pages/.../develop/index.tsx](../../frontend/packages/loop-pages/prompt-pages/src/pages/develop/index.tsx)

- `renderHeaderButtons`：插在 **版本记录** 与 **提交新版** 之间  
- `extraTabs`：`smart_optimize` Tab  
- Prompt 详情现有 `编排 / 观测 / 评测 / 智能优化` 四 Tab；观测/评测当前是带 Prompt 上下文的模块入口，尚未内嵌完整列表
- 创建路由：`pe/prompts/:promptID/optimize/create`；确认后进入独立两步全页，完成后回到 `?tab=smart_optimize`
- 采纳：完整 `messages[]` 写草稿 + Toast，保留角色、Few-shot 与多模态 parts；**未**自动打开提交弹窗

实验详情入口：`evaluate-pages/src/components/experiment-smart-optimize-entry.tsx`，通过 `ExperimentDetailPage.renderExtraButtons` 注入；成功的 Prompt 实验确认当前实验、Prompt 与版本后导航到同一创建路由，不再叠加 Wizard 弹窗。

多模态边界：当前支持把图片/视频作为优化证据输入，并完整保留消息 `parts[]`；优化器改写 Prompt 文本与结构，**不**自动裁剪、放大、画 SoM 标记或替换图片。详见 TRD 6.3。

接口占位：`@cozeloop/adapter-interfaces` → [prompt/index.ts](../../frontend/packages/loop-components/adapter-interfaces/src/prompt/index.ts)（`PromptAdapters` 类型已扩；实现暂未抽独立 `prompt-adapter` 包，因 rush update 策略受限）

Guard：`pe.prompt.smart_optimize` 已定义，入口**未包** `<Guard>`（OSS 默认放行）。

### 后端代码（本轮新增）

```text
idl/thrift/coze/loop/evaluation/coze.loop.evaluation.optimize.thrift
backend/modules/evaluation/application/optimize_app.go
backend/modules/evaluation/domain/entity/optimize_task.go
backend/modules/evaluation/domain/repo/optimize_task.go
backend/modules/evaluation/infra/repo/optimize/optimize_task.go
backend/api/handler/coze/loop/apis/optimize_service.go
release/deployment/docker-compose/bootstrap/mysql-init/{init-sql,patch-sql}/optimize_task.sql
release/deployment/helm-chart/charts/app/bootstrap/init/mysql/init-sql/optimize_task.sql
```

- API：Create/List/Get/Cancel；报告随 Get/List 的 `result` 返回。
- Worker：持久化后异步 claim，调用用户选择的 `optimizer_model_id`，先按实验 ID 快照真实评测证据（input/actual/reference/evaluator result），生成候选并执行一次临时候选消息，支持取消、失败记录、周期扫描、重启恢复和 stale-running 重排。
- 前端 API Schema 已生成 `evaluationOptimize` 并合入 `StoneEvaluationApi`。
- `OptimizeTaskResult` 的评分字段为 optional；当前没有执行重评时 UI 显示“候选已生成（待重评）”。

商业参考文档：https://loop.coze.cn/open/docs/cozeloop/optimize-prompts-with-ai  

---

## 5. 与商业版差距（下一工具优先做这些）

对照用户提供的扣子罗盘截图（实验页入口、「新建智能优化」选实验弹窗、带图标下拉）。

### P0 — 产品流程

1. **Worker 第二阶段**：已接入证据快照、case 裁剪和一次临时候选执行；仍需逐 case 执行并复用评估器重评
2. 多候选、多轮迭代、优化集/验证集拆分与增益停止条件
3. 样本选择服务端过滤：非满分、评估器分数区间、跨多轮数据
4. `reference_output` 根据 schema_key/显式映射识别，避免仅靠字段名启发式
5. 评测集（Goodcase）路径接 `ListEvaluationSetVersions` / `ListEvaluationSetItems`
6. 成本预算、重试次数、RocketMQ 多实例 Worker 与租约

候选执行方案已确定为方案 1：通过临时执行适配器直接传入完整候选 `messages[]`、变量值和多模态 parts，不创建持久化 Prompt 版本；契约见 TRD 6.4。

### P1 — UI / 体验

- 确认弹窗补实验状态/Prompt 目标兼容性过滤
- 报告：分数分布、评估器列、回跳样本  
- 采纳后打开「提交新版」流  
- 包 `Guard`；任务创建前做 Jinja2 / 多模态变量 / String 变量校验  

### 建议下一迭代顺序

1. Worker 加载完整实验/评测集 case 证据
2. 候选执行 → 原评估器重评 → 验证集复测 → 选优
3. 加任务级预算、重试/租约并迁移 RocketMQ 多实例消费
4. 报告补分布/评估器列，采纳后自动打开「提交新版」

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
当前任务：继续实现 Worker 的候选执行、评估器重评和多轮选优；OptimizeTask API、持久化、第一阶段 Worker 与前端真实客户端已完成。
前端热更新：http://localhost:8090 （后端 :8888 / Web :8082）。
分支：feat/prompt-loop-multimodal-joybuild。不要提交 .local-migration/。
```

---

## 8. 验证清单（切换前）

- [x] PRD / TRD 在仓  
- [x] Prompt 开发页 Header 下拉 + 四工作区 Tab
- [x] 栈可访问 8082=200、8888=pong（交接时）  
- [x] 前端 dev 8090 可访问（交接时）  
- [x] 真实 ListExperiments / BatchGetExperimentResult 样本表
- [x] 实验页入口 + 当前实验/Prompt 版本回填
- [x] 完整消息与多模态内容契约
- [x] 确认弹窗 + 独立两步创建路由
- [x] 智能优化任务台搜索、状态筛选和业务字段
- [x] Prompt 四工作区与 URL Tab 状态同步
- [x] OptimizeTask IDL / MySQL / Create-List-Get-Cancel API
- [x] 可选优化模型 + 第一阶段诊断/候选 Worker + 取消/恢复
- [x] 前端真实 OptimizeTask 客户端
- [x] 按可解析的 case ID 精确裁剪实验结果证据
- [x] 候选 Prompt 临时执行（单次校验）
- [ ] 逐 case 候选执行 / 原评估器重评 / 多轮验证选优

### 本轮验证结果

```bash
# 后端：通过
go test ./modules/evaluation/application \
  ./modules/evaluation/infra/repo/optimize \
  ./api/handler/coze/loop/apis \
  ./api/router/coze/loop/apis

# 前端：通过（0 error）
cd frontend/packages/loop-base/api-schema && rushx lint
cd frontend/packages/loop-pages/prompt-pages && rushx lint
```

IDL 已用项目锁定版本生成：Kitex `v0.13.1`、Hertz `v0.9.7`、ThriftGo `v0.4.1`，Wire `v0.6.0`。社区前端全量 `tsc -b` 仍被仓内既有的 `prompt-components-v2` SVG/CSS 与 `@/` alias 解析问题阻塞；筛选本轮目录后，仅剩同一既有 alias 问题，`smart-optimize` 组件本身无新增类型错误。
