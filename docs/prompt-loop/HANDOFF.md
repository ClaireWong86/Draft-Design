# Prompt Loop — AI 交接文档（HANDOFF）

> 给下一个 AI 编程工具 / 工程师：先读本文件，再按需深入 PRD/TRD/代码。  
> 更新时间：2026-07-22（UTC+8，裁判一致性、`no_gain` 终态、本地部署与回归验收）

---

## 1. 一句话现状

Prompt Loop（Coze Loop fork）本地栈可跑；Prompt 详情已形成**编排、观测、评测、智能优化**四工作区。智能优化已接通 OptimizeTask IDL、MySQL、真实 API、可选优化模型和带租约的异步 Worker。Goodcase 含真图 baseline、裁判硬保护、`no_gain`（未提升），以及 P1 UI（实验过滤、报告回跳、Guard/变量校验）。**下一优先是提交/推送未入库改动与 P2 扩展项。**

---

## 2. 仓库与分支

| 项 | 值 |
|----|-----|
| 本地路径 | `/Users/wangdujuan10/Projects/Draft-Design` |
| 远程 | `https://github.com/ClaireWong86/Draft-Design.git` |
| 工作分支 | `feat/prompt-loop-multimodal-joybuild` |
| PR | https://github.com/ClaireWong86/Draft-Design/pull/1 |
| 最新相关提交 | `fb9652bc` — `feat(evaluation): add no_gain status, judge guards, and P1 UI`；已推送远端 |
| 勿提交 | `.local-migration/`、`.local-build/`（本地镜像/脚本产物） |

读仓入口：根目录 [AGENTS.md](../../AGENTS.md)、[ARCHITECTURE.md](../../ARCHITECTURE.md)。

---

## 3. 本地环境（Podman）

本机用 **Podman Desktop**，不用 Docker Desktop。VM 内存建议 ≥5GB。

| 服务 | URL | 备注 |
|------|-----|------|
| Web（nginx 静态） | http://localhost:8082 | 旧镜像，**不含**最新智能优化前端 |
| OpenAPI | http://localhost:8888/ping → `pong` | |
| 前端热更新 | http://localhost:8090 | **含**智能优化代码；API 代理到 `:8888` |

账号（本地恢复数据）：`codex-local-smoke@example.com` / `Codex123456`
`promptloop@local.dev` 不存在于恢复的 MySQL 备份中，不可用于登录。

### 每天启动

2026-07-20 起使用干净 machine `podman-loop-dev` 的 **rootful + 持久化 VFS** 隔离存储。原因是该 machine 的原生 overlay 曾反复报 `overlay/l: invalid argument`；旧 `podman-machine-default` 仍保留，禁止执行 `podman system reset`。Docker Desktop 因公司限制不可启动，不作为任何流程前提。

稳定运行所需的宿主机辅助文件（不在仓库内）：

```text
/Users/wangdujuan10/Documents/Codex/2026-07-14/du-q/
  podman-fuse-bin/podman          # 实际为 VFS wrapper，显式指定 root/runroot
  podman-loop-storage.conf        # persistent VFS 配置
  podman-loop-runtime/            # render-config 生成的 compose/.env/bootstrap
  podman-loop-runtime/podman-override.yml # FaaS 的 Podman 兼容覆盖
```

```bash
export PATH="/opt/podman/bin:$PATH"
podman machine start podman-loop-dev   # 若未 running

WRAPPER=/Users/wangdujuan10/Documents/Codex/2026-07-14/du-q/podman-fuse-bin/podman
$WRAPPER start coze-loop-redis coze-loop-mysql coze-loop-clickhouse \
  coze-loop-minio coze-loop-rmq-namesrv coze-loop-python-faas \
  coze-loop-js-faas coze-loop-rmq-broker coze-loop-app coze-loop-nginx

# Podman 远程模式若长期停在 health=starting，可手动触发已有 healthcheck：
$WRAPPER healthcheck run coze-loop-app
$WRAPPER healthcheck run coze-loop-nginx

curl http://localhost:8888/ping   # {"message":"pong"}
curl -I http://localhost:8082     # HTTP 200

# 前端热更新（智能优化调试用这个）
cd frontend/apps/cozeloop && npm run dev   # → :8090，可能只绑 [::1]
```

速查：[runbook.md](./runbook.md)、[local-macos-docker.md](../guidance/local-macos-docker.md)。文档中的 Docker Desktop / `docker` 命令均不适用于当前环境，统一替换为 Podman。

### 常见坑

- 当前 wrapper 必须持续显式使用 `--storage-driver=vfs`、`--root=/var/lib/containers/storage-vfs-persistent`、`--runroot=/var/lib/containers/runroot-vfs-persistent`；不要改回 `/run/...`，虚拟机重启会清空 `/run` 并使容器运行时文件失效。
- machine `/etc/hosts` 需保留 `192.168.127.1 gateway.containers.internal host.containers.internal # codex-podman-gateway`，否则带端口映射的 app/nginx 会因 forwarder 无法解析而启动失败。
- Docker Hub 在公司网络下需要 machine 内的 `# codex-dockerhub` 静态解析项；不要删除，除非已确认公司 DNS 恢复正常。
- `.local-migration/` 中的旧 MySQL、MinIO 数据已于 2026-07-21 导入新 Podman 正式卷：恢复 1 个用户、5 个评测集、16 条数据、3 个 Prompt 及多模态附件。ClickHouse 备份中的 138 条 Span 尚未导入；Nginx 已由当前镜像重建，MinIO config 备份仅含空证书目录。
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
  evaluation-set-case-selector.tsx # ListEvaluationSetItems、版本化分页/多选/多模态预览
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
- 采纳：完整 `messages[]` 写草稿，保留角色、Few-shot 与多模态 parts，并自动打开标准「提交新版」弹窗

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
- Worker：持久化后异步 claim，调用用户选择的 `optimizer_model_id`，先按实验 ID 快照真实评测证据（input/actual/reference/evaluator result），生成候选并按选中 case 执行临时候选消息；变量会精确渲染到普通消息和多模态文本 part，随后复用实验里的 evaluator version 重评。
- Goodcase MultiPart 字段会恢复为真正的 runtime user 图片/视频 part；纯文本变量位置仅保留提示标记，不再把媒体对象 JSON 当文本交给 VLM。
- EvaluationSet 媒体归一化按 `content_type` 优先判型；图片节点即使同时携带空 `text` 指针也不会被降级为空字符串。Evaluation → LLM RPC 显式保留 multipart，并在跨层转换后做 fail-closed 完整性检查。
- 优化模型若生成未定义的 `{{variable}}`，Worker 会保留原 Prompt 已定义变量，并把未知标记降级为普通文本、写入 diagnosis，避免单个候选把整项任务击穿。
- 根据模式使用 1–3 轮、每轮 2–3 个候选；固定拆分优化集/验证集。Goodcase 先对每个样本执行一次 baseline，所有候选复用该输出，再按验证集 `after-before` 增益选优；最佳增益不超过 0.001 时终态为 **`no_gain`（未提升）**：保留完整报告、前端隐藏采纳；`failed` 仅表示技术/映射/执行错误。调用预算以 24/48/96 为基础，Goodcase 额外计入一次 baseline 样本执行和每候选的 12 条一批裁判，并为可疑全满分批次预留一次独立复核；LLM 临时错误最多重试 3 次。
- Goodcase 裁判前增加确定性保护：空输出和明确前缀式中英文拒答直接记 0 分；参考答案为 JSON 对象/数组时，候选必须是无代码块/附加说明的独立 JSON，并保持必填字段、嵌套结构和基础类型；全满分批次用更严格指令独立复核，并保守取两次较低分。
- Worker claim 使用 `lease_token + lease_expires_at + attempt_count`；多实例原子认领、续租、过期重排，最多 3 次认领，旧 Worker 无权覆盖新 Worker 结果。
- 字段映射支持显式 JSON path、显式 `reference_output_field`，以及 evaluator/分数上下界/仅非满分的服务端过滤。
- 报告返回综合得分、分布、逐 case 与逐评估器 before/after；实验来源会批量解析 evaluator version 的展示名，查询失败时回退 `Evaluator <version_id>`；采纳完整消息后自动打开标准「提交新版」弹窗。
- 前端 API Schema 已生成 `evaluationOptimize` 并合入 `StoneEvaluationApi`。
- `OptimizeTaskResult` 的评分字段为 optional；当前没有执行重评时 UI 显示“候选已生成（待重评）”。

商业参考文档：https://loop.coze.cn/open/docs/cozeloop/optimize-prompts-with-ai  

---

## 5. 当前高优待办（按优先级）

对照用户提供的扣子罗盘截图（实验页入口、「新建智能优化」选实验弹窗、带图标下拉）。

### 立刻要做（工程收口）

1. [x] 提交并推送本轮改动（`fb9652bc`）
2. （可选）协议变形样本再跑一次真图回归：期望硬校验记 0，而不是 Judge 给满分。

### P1 — UI / 体验

1. [x] 确认弹窗补实验状态 / Prompt 目标兼容性过滤（`buildSmartOptimizeExperimentFilters`）
2. [x] 报告回跳原始样本（`查看样本` → 实验/评测集 `item_id`；实验详情自动 ItemID 筛选）
3. [x] 入口包 `<Guard pe.prompt.smart_optimize>`；创建前校验 Jinja2 / 多模态变量 / 仅 String 变量

### 下一优先（原 P1 完成后）

- （可选）协议变形样本真图硬校验回归  
- 批量裁判按 token 预算动态选 4–40 条（TRD 7.2）  
- RocketMQ 唤醒；VLM 诊断探针等（见 research P1–P4）  

### 已完成的 P0（勿再当高优）

1. [x] Goodcase 裁判一致性保护（JSON 硬校验、拒答检测、全满分复核）  
2. [x] 实验 evaluator 展示名批量补齐  
3. [x] Goodcase 无增益终态 `no_gain`（未提升）；可看报告、不可采纳；`failed` 仅技术错误  
4. [x] 本地部署 `localhost/coze-loop:local-no-gain` + 回归：旧任务 `7664954798201896961`（`failed`）→ 新任务 `7665228875797889025`（`no_gain`，before 0.9 / after 0.85，有 result）  

运维备忘：只用 **`podman-loop-dev`**；一键 `.local-build/redeploy-no-gain-smoke.sh` / `.local-build/regress-no-gain.py`；宿主机 16GB 易 OOM；回滚镜像 `localhost/coze-loop:local-goodcase-baseline`。

候选执行方案仍为方案 1（临时执行适配器，不建持久 Prompt 版本）；契约见 TRD 6.4。

### P2 — 扩展（不阻塞当前体验）

- 批量裁判按 token 预算动态选 4–40 条，超限/截断时二分重试（TRD 7.2）  
- RocketMQ 唤醒（当前 MySQL lease 队列已正确；任务量上来后再做）  
- VLM 诊断探针 / 文本策略路由 / 视觉变换（见 [vlm-prompt-optimization-research.md](./vlm-prompt-optimization-research.md) P1–P4）  

### 2026-07-21 / 07-22 已按顺序完成

1. [x] field mapping 变量精确渲染，复用原评估器重评
2. [x] 优化集/验证集拆分，多候选、多轮、验证集选优与增益停止
3. [x] 任务级调用预算、LLM 重试、数据库租约与多实例安全认领
4. [x] 服务端样本过滤与显式 `reference_output`/JSON path 映射
5. [x] 报告分布/评估器列，采纳后打开「提交新版」
6. [x] Goodcase 评测集/版本选择、Items API、来源快照、字段映射与参考答案裁判
7. [x] Goodcase 12 条一批裁判，保留逐 case before/after 并降低模型调用数
8. [x] Goodcase 真图链路：修复空 `text` 覆盖 Image、RPC multipart 丢失、case ID 精度、Worker 用户上下文与超长错误写回
9. [x] Goodcase baseline 原 Prompt 实跑、同裁判 before/after、按验证集增益选优及无增益拒绝推荐
10. [x] Goodcase 裁判协议/拒答硬保护与全满分二次复核
11. [x] 实验 evaluator version 展示名批量补齐
12. [x] `no_gain` 状态 + 前端「未提升」+ 本地部署与无增益回归
13. [x] P1：实验兼容性过滤、报告回跳样本、Guard + 创建前变量校验

批次 `12` 是保守默认值，不是 API 或模型硬限制：它为 4096 输出 token、VLM 大证据和结构化逐 case 返回预留安全余量。详细依据见 TRD 7.2。

---

## 6. 相关已完成能力（非智能优化，但同分支）

- 品牌 Prompt Loop、多模态评测上传/预览、JoyBuild LLM adapter  
- 轮胎评测集导入、标签管理恢复、Podman/SELinux/RMQ 本地脚本加固  
- 详见 PR #1 与提交 `6e9f3241`

JoyBuild 已完成本地配置和 VLM 真图 E2E。密钥只保存在本地环境，禁止写入仓库或日志；密钥轮换时再按 `import-tire-joybuild-config.sh` → `joybuild-check.sh` → `refresh-config` → `restart-app` 更新。

---

## 7. 给下一个 AI 的开场指令（可复制）

```text
你在 Prompt Loop fork（Draft-Design）上工作。
先读 docs/prompt-loop/HANDOFF.md，再读 prd/trd-smart-prompt-optimize.md。
P0 与 P1 UI（实验过滤、报告回跳、Guard/变量校验、no_gain）已完成。
下一优先：提交/推送未入库改动；可选协议变形样本真图回归；再做 P2（动态分批 / RMQ / VLM 探针）。
不要把单次 Goodcase 高分直接视为自动发布依据；无增益看「未提升」报告，勿当失败。
前端热更新：http://localhost:8090 （后端 :8888 / Web :8082）。
分支：feat/prompt-loop-multimodal-joybuild。不要提交 .local-migration/ 或 .local-build/。
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
- [x] 候选 Prompt 临时执行（按选中 case）
- [x] 按 field mapping 构造每个 case 的 variables/evidence 载荷，并保存候选 actual_output
- [x] 将 variables 精确渲染进模板 / 原评估器重评 / 多轮验证选优
- [x] Goodcase 评测集/版本选择 / EvaluationSet Items / 参考答案裁判
- [x] Goodcase 裁判硬保护 + `no_gain`（未提升）+ 本地镜像回归 `7665228875797889025`

### 历史实现记录（2026-07-20，已被 2026-07-21 实现取代）

- Worker 不再只把原始 case JSON 作为黑盒输入；会解析 `OptimizeFieldMapping`，构造变量映射和 evidence 载荷。
- 每个候选 case 的模型输出写入 `OptimizeTaskResult.case_details[].after_actual`，同时尽力保留 baseline actual/reference，供下一阶段评估器重评使用。
- 当日版本尚未把变量替换进模板，也未调用原评估器计算 after score；上述缺口已于 2026-07-21 完成。

### 本轮验证结果

```bash
# 后端定向编译/测试：通过（Go 1.26.5 darwin/arm64）
go test ./modules/evaluation/application \
  ./modules/evaluation/infra/repo/optimize \
  ./api/handler/coze/loop/apis \
  ./api/router/coze/loop/apis

# 前端：通过（0 error）
cd frontend/packages/loop-base/api-schema && rushx lint
cd frontend/packages/loop-pages/prompt-pages && rushx lint
```

本轮额外修复了实验服务接口包引用与 Prompt role（string）到评测运行时 role（enum）的显式转换。

Podman 环境阻塞已于 2026-07-20 解除：新 machine `podman-loop-dev` 使用 rootful、持久化 VFS 隔离存储；Docker Hub 镜像、15 个 bootstrap/config 卷与完整 compose 服务均已就绪。实测 `:8888/ping` 返回 `pong`、`:8082` 返回 HTTP 200；Redis、MySQL、ClickHouse、MinIO、RocketMQ、Python/JS FaaS、app、nginx 均达到 healthy。旧 machine 和 `.local-migration/` 原始备份均保留。

2026-07-21 修复登录错误语义：`GetUserByEmail` 将 `gorm.ErrRecordNotFound` 映射为 `UserNotExistCode`，未知邮箱在中文 Cookie 下返回“用户不存在”，真正的数据库故障才返回“MySQL错误”。本地运行镜像为 `docker.io/library/coze-loop:local-fix`；旧 `coze-loop-app-before-account-fix` 容器保留用于回滚。

2026-07-21 智能优化 1–5、Goodcase 双来源、批量裁判、真实多模态 part 注入及 baseline 对照选优均已编译部署；当时镜像为 `localhost/coze-loop:local-goodcase-baseline`。MySQL 已应用 `lease_token / lease_expires_at / attempt_count` 和完整来源快照 `source` 列。

2026-07-22 当前运行镜像为 `localhost/coze-loop:local-no-gain`（含裁判硬保护与 `no_gain`）。回滚仍可用 `local-goodcase-baseline`。

真实验收盘点：轮胎 Goodcase 版本 `0.1.1` 有 11 条 MultiPart 样本。任务 `7664935468542197761` 成功证明真图进入模型（`inline_images=1`）。baseline 任务 `7664954798201896961` 曾以 `failed` + `no candidate improved…`（增益 `-0.1000`）结束；同配置重跑任务 `7665228875797889025` 终态为 **`no_gain`**（before 0.9 / after 0.85，完整报告保留），证明无增益不再误标为技术失败。

IDL 已用项目锁定版本生成：Kitex `v0.13.1`、Hertz `v0.9.7`、ThriftGo `v0.4.1`，Wire `v0.6.0`。社区前端全量 `tsc -b` 仍被仓内既有的 `prompt-components-v2` SVG/CSS 与 `@/` alias 解析问题阻塞；筛选本轮目录后，仅剩同一既有 alias 问题，`smart-optimize` 组件本身无新增类型错误。
