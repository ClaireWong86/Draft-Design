# TRD：智能优化 Prompt

> 技术设计。产品需求见 [PRD](./prd-smart-prompt-optimize.md)。

## 1. 范围与原则

- 产品形态对齐扣子罗盘「智能优化」；**不声称复刻商业闭源算法**
- OptimizeTask 已采用 IDL-first 落地；前端默认调用真实 API，`mock-client.ts` 仅保留为本地演示夹具
- MVP 前端落在 `@cozeloop/prompt-pages`（`src/components/smart-optimize`），由 Prompt 页与实验详情页复用；后续可抽到中立业务组件包
- Prompt 快照始终保存完整 `messages[]` / `parts[]` / `variable_defs[]`，不得压平成字符串

## 2. 总体架构

```text
FE (Header 下拉 + 四工作区 Tab + Confirm Modal + Create Page)
  → OptimizeTask API（同步：创建/查询/终止）
  → OptimizeWorker（异步：诊断 → 多候选改写 → 原评估器重评 → 验证集选优）
       ├─ Evaluation APIs（实验明细 / 评测集条目）
       ├─ Prompt Version API（加载 / 采纳后由用户提交）
       └─ Rewrite & Judge LLM
```

### 2.1 复用

| 能力 | 现有接口 / 模块 |
|------|-----------------|
| 实验列表 | `ListExperiments` |
| 实验带分明细 | `BatchGetExperimentResult`（优先 `correction.score`） |
| 评测集版本/条目 | `ListEvaluationSetVersions` / `ListEvaluationSetItems` |
| 字段映射语义 | 实验创建 `field_name` / `from_field_name` |
| Prompt 提交 | 现有「提交新版」流；采纳仅写草稿 |

## 3. 算法：Eval-Driven Iterative Prompt Rewrite

商业版未公开内部算法。本仓库采用可解释的 APO/DSPy 类闭环。

### 3.1 流程

```text
加载 Prompt + 样本 + 映射
  → Baseline：J(P0)
  → 循环：
       诊断（Badcase / Goodcase 差距）
       → Meta-LLM 改写生成 N 个候选 P'
       → 在优化集上评估 J(P')
       → 接受更优；否则保留当前
  → 在独立验证集复测，阻止过拟合候选
  → 停止（轮次 / 增益阈值 / 用户终止）
  → 报告
```

### 3.2 优化目标 `J(P)`

| 路径 | `J(P)` |
|------|--------|
| 实验（Badcase） | `mean(evaluator_scores)`，多评估器可加权；优先人工校准分 |
| 评测集（Goodcase） | 原 Prompt 与候选统一由用户选择的优化模型执行 LLM-as-Judge（输出 vs `reference_output`）；裁判前做 JSON/拒答硬校验与可疑全满分复核；验证集增益 ≤ 0.001 时任务终态为 `no_gain`（未提升），保留报告、不可采纳 |

### 3.3 诊断 / 改写 / 选择

1. **诊断**：抽取得分最低（或与参考差距最大）的 K 条，生成 `failure_modes[]`、`suggested_instruction_changes[]`
2. **改写**：Meta-LLM 输入 `P_current` + 诊断 + exemplars + 约束（变量名、JSON schema、语言）；输出完整消息列表
3. **选择**：`argmax J`；无提升则 `no_improve++`

### 3.4 模式滑块 → 超参

| 模式 | 候选数 N | 最大轮次 T | 每轮评估样本 | 诊断条数 K |
|------|----------|------------|--------------|------------|
| 性价比优先（`mode_score ≤ 0.33`） | 2 | 1 | 固定优化集/验证集拆分 | 按证据样本生成 |
| 均衡（`0.33 < mode_score < 0.67`） | 2 | 2 | 固定优化集/验证集拆分 | 按证据样本生成 |
| 效果优先（`mode_score ≥ 0.67`） | 3 | 3 | 固定优化集/验证集拆分 | 按证据样本生成 |

`mode_score ∈ [0,1]`：0=性价比优先，1=效果优先；当前按上述三档映射，不做线性插值。

### 3.5 停止条件

- 达到 `T`
- 相对提升 `< ε`（默认 0.01）
- 用户 `CancelOptimizeTask`

## 4. 领域模型：`OptimizeTask`

| 字段 | 说明 |
|------|------|
| `id` | 任务 ID |
| `name` | 任务名称；未传时由服务端按时间生成 |
| `workspace_id` | 空间 |
| `prompt_id` / `prompt_version` | 优化基线 |
| `source` | 可辨识联合：`{type: experiment, experiment_id}` 或 `{type: eval_set, eval_set_id, eval_set_version_id}` |
| `baseline_prompt` | 不可变 Prompt 快照：版本、完整消息列表、变量定义 |
| `case_item_ids[]` | 选中样本 |
| `mapping` | variables / actual_output / reference_output |
| `mode_score` | 0–1 |
| `optimizer_model_id` | 用户选择的可用优化模型；同时作为当前诊断/改写模型 |
| `status` | `queued` \| `running` \| `succeeded` \| `failed` \| `cancelled` |
| `progress` | 0–100 |
| `error_msg` | 失败信息 |
| `result` | 见下 |
| `created_by` / timestamps | 审计 |

`result`：

```json
{
  "before_prompt": {
    "prompt_id": "123",
    "prompt_version": "0.0.1",
    "messages": [{ "role": "system", "content": "...", "parts": [] }],
    "variable_defs": []
  },
  "after_prompt": {
    "prompt_id": "123",
    "prompt_version": "0.0.1",
    "messages": [{ "role": "system", "content": "...", "parts": [] }],
    "variable_defs": []
  },
  "before_score": 0.62,
  "after_score": 0.81,
  "before_score_distribution": [],
  "after_score_distribution": [],
  "case_details": [],
  "diagnosis": { "failure_modes": [], "suggested_instruction_changes": [] }
}
```

## 5. API 契约（已落地）

IDL 位于 `idl/thrift/coze/loop/evaluation/coze.loop.evaluation.optimize.thrift`，由根 Evaluation/API service 扩展；Go Kitex、Hertz 路由和 TypeScript API 均由该 IDL 生成。

| RPC | 说明 |
|-----|------|
| `CreateOptimizeTask` | 创建并入队 |
| `ListOptimizeTasks` | 按 prompt / status / 源过滤 |
| `GetOptimizeTask` | 详情 + 进度 |
| `CancelOptimizeTask` | 终止（已消耗不回滚） |

报告由 `GetOptimizeTask.result` 返回；采纳是前端读取 `result.after_prompt` 写入草稿，不额外增加无状态 RPC。

### 5.1 Create 校验（创建前）

- Prompt 模板类型、变量类型、样本量
- 实验状态成功且存在非满分
- 映射完整（至少变量 + reference_output；实验路径建议含 score）
- 多模态媒体只读透传；签名 URL/URI 在任务执行期必须可解析

### 5.2 预估消耗

```text
estimate ≈ cases × T × N × (target_tokens + judge_tokens)
```

前端展示预估值；账单以后端为准。

## 6. 前端落点

| 层 | 包 | 职责 |
|----|----|------|
| Header 顺序 | `prompt-components-v2` PromptHeader | 通过 `renderHeaderButtons` 插入：版本记录 → 智能优化 → 提交新版 |
| 接口 | `@cozeloop/adapter-interfaces/prompt` | `PromptAdapters` 类型（后续抽包用） |
| 实现（MVP） | `@cozeloop/prompt-pages` → `components/smart-optimize` | 下拉、确认弹窗、创建向导、任务列表、报告、真实 OptimizeTask API |
| 挂载 | `prompt-pages/.../develop/index.tsx` | 注入 buttons + `extraTabs`，形成编排/观测/评测/智能优化四工作区 |
| 创建路由 | `pe/prompts/:promptID/optimize/create` | 独立全页两步流程；URL 携带 source / experiment_id / prompt_version |
| 实验入口 | `evaluate-pages` → `ExperimentDetailPage.renderExtraButtons` | 当前实验和版本确认后导航到同一创建路由 |
| 权限 | `pe.prompt.smart_optimize` | OSS 默认放行；后续可包 `Guard` |

### 6.1 页面状态与导航

```text
Prompt Header / Experiment Detail
  → Confirm Modal（实验路径）
  → /pe/prompts/:promptID/optimize/create
       Step 1：实验数据明细（真实 BatchGetExperimentResult）
       Step 2：字段映射 + 优化模型 + 优化模式
  → CreateOptimizeTask
  → /pe/prompts/:promptID?tab=smart_optimize
```

- 直接从「优质评测集」进入时跳过实验确认弹窗，在创建页选择数据源。
- `tab=smart_optimize` 用于刷新或返回时恢复任务工作区。
- 观测/评测 Tab 当前提供带 Prompt Key 上下文的 Trace 入口，以及评测实验/评测集入口；完整列表内嵌属于后续体验迭代。
- 任务台已使用真实 API；后端支持 `keyword/status/page_number/page_size`，前端目前取前 100 条后进行轻量筛选。

### 6.2 采纳

前端直接读取 `result.after_prompt` → 将完整 `messages[]` 写入 `usePromptStore.setMessageList` → Toast 引导用户点「提交新版」。当前无独立 Adopt RPC；采纳不绕过版本提交校验，也不只替换 system 文本。

### 6.3 文本与非文本能力边界

智能优化的**优化对象是 Prompt 消息结构**，不是只支持纯文本输入：

- 文本：Meta-LLM 可改写 `message.content`、角色指令、输出格式和 Few-shot 文本。
- 图片/视频：样本选择、诊断输入和基线/候选 Prompt 均保留 `messages[].parts[]`，媒体 URI 只读透传；Worker 调用目标 VLM 时必须按原顺序重建图文消息。
- 当前不做：裁剪/放大、Set-of-Mark、OCR 增强、图像生成或替换。这些属于后续 `VisualTransformPlan`，需要可复现的变换参数和派生媒体存储，不能混入文本改写字段。

因此，MVP 是“**支持非文本证据的 Prompt 自动优化**”，还不是“自动联合优化图像和文本输入”。

### 6.4 候选 Prompt 临时执行适配器（方案 1）

候选 Prompt 不创建持久化 Prompt 版本，Worker 通过临时执行适配器直接调用模型：

```text
CandidateExecutionRequest
  workspace_id
  task_id
  candidate_messages[]       # 完整 system/user/assistant/tool + parts
  variable_values{}          # 当前 case 的变量值
  model_config               # 任务选择的执行模型与参数

CandidateExecutionResult
  output_message             # 文本或多模态 parts
  usage
  latency
  error
```

适配器保持候选消息的角色、顺序、Few-shot 和多模态 parts。Worker 按 `OptimizeFieldMapping` 将每个 case 的 `from_field_name` 转换为变量名并精确渲染到模板；候选输出写入 `case_details[].after_actual`，随后交给原 evaluator version 重评。禁止使用模型自评结果替代评估器分数。

## 7. 存储与异步（已实现）

- MySQL `optimize_task` 保存不可变基线、映射、样本 ID、模型、状态与结果 JSON；SQL 已同步 Docker init/patch 与 Helm init。
- Create 先持久化 `queued` 再入内部有界队列；Worker 使用 lease token 原子 claim 后更新 `running/progress/succeeded/failed/cancelled`。
- 每 30 秒扫描持久化队列；服务重启会恢复 queued 任务，并将 lease 过期的 running 任务重新排队，最多认领 3 次。
- Worker 在调用优化模型前后检查取消标记；模型空响应、非 JSON、缺少 `optimized_prompt` 均进入 `failed` 并保留错误。
- 当前 Worker 可多实例部署：MySQL 是任务事实源，lease 保证并发正确性。RocketMQ 后续只需承担低延迟唤醒，不改变持久任务协议。

### 7.1 当前 Worker 的准确能力边界

实验来源与 Goodcase 评测集来源均已完成可运行的迭代优化算法：

1. 输入不可变 Prompt 快照、字段映射、来源与选中 case ID；
2. 使用用户选择的 `optimizer_model_id` 调用 LLM Runtime；
3. 生成多轮多候选 Prompt，保留变量定义和多模态 parts；
4. 实验来源复用原 evaluator version 重评；Goodcase 来源通过 `evaluation_set_id + version_id + item_ids` 调用 `BatchGetEvaluationSetItems`，再由所选优化模型按显式参考答案裁判；
5. 在独立验证集按得分选优，持久化报告，允许查询、取消、恢复和前端采纳。

Goodcase 不伪造实验分数：Worker 先用原 Prompt 对每个选中样本执行一次 baseline，随后所有候选复用该 baseline 输出；报告中的评估器显示为 `Goodcase Judge`，before/after 分数来自同一次批量裁判对 baseline、候选与版本化参考答案的比较。任务持久化完整 `OptimizeSource` 快照，确保 Worker 重启后不会混淆评测集 ID 与版本 ID。

当评测集字段为 MultiPart 时，Worker 会把映射变量中的图片/视频恢复为真正的 runtime user message parts；基线 Prompt 文本中的变量位置只保留“多模态输入见后续用户消息”标记，禁止把媒体对象或 URL JSON 直接当作视觉输入文本发送。该转换是 VLM Goodcase 真实执行的必要条件。

跨层契约必须 fail closed：EvaluationSet Content 先按声明的 `content_type` 判型（不能让图片节点上的空 `text` 指针抢先命中），Evaluation → LLM RPC 必须保留 multipart，LLM Runtime → JoyBuild 再把内部 URL 转为带 MIME 的 inline data。若上游存在媒体而 RPC DTO 已无媒体，立即终止任务；适配器仅记录图片数量与编码长度，不记录 URL、base64 内容或密钥。

优化模型生成的 Prompt 只能引用基线 Prompt 已声明或已实际使用的变量。模型偶发生成未知 `{{variable}}` 时，将其降级为普通文本并写入诊断，不能让候选渲染阶段把整项异步任务击穿。

## 8. 权限

- Guard：`pe.prompt.smart_optimize`
- OSS 默认 `ACTION`（不隐藏）

## 9. 轮胎场景映射（示例）

- Badcase：缺陷类型/位置错 → 强化 inspection process 与 JSON schema
- Goodcase：优质标注 JSON → 优化字段完备性与 severity 一致性

## 10. 实现阶段

1. [x] FE 任务闭环
2. [x] 实验详情入口、真实 `ListExperiments` / `BatchGetExperimentResult`、多模态选择与 Schema 映射
3. [x] OptimizeTask IDL、MySQL、API、异步 Worker 与恢复
4. [x] 可用优化模型选择、FE 真实任务客户端
5. [x] Worker 加载完整 case 证据，执行候选并复用评估器重评
6. [x] 优化集/验证集拆分、多候选多轮选优、成本预算与租约式多实例安全
7. [x] Goodcase 评测集选择、版本选择、Items API、字段映射与参考答案裁判
8. [x] Goodcase 图片从 EvaluationSet 到 JoyBuild 的跨服务 multipart 保真与真实单样本验收
9. [x] Goodcase baseline 原 Prompt 实跑、同裁判 before/after 对照及无增益拒绝推荐
10. [ ] Goodcase Judge 增加输出 Schema、拒答、可疑全满分复核等确定性保护
# 2026-07-21 实现落地说明

实验驱动路径已经按本文算法落地为可恢复的 OptimizeTask：

1. 字段映射支持显式 JSON path；`{{ variable }}` 在执行前渲染到消息正文和多模态 text part，缺失变量直接终止任务，不再把 variables/evidence JSON 追加成伪用户消息。
2. 每个候选输出复用基线实验记录中的 evaluator version 与 evaluator input，替换 actual output 后同步重评；报告保存综合分、分布、逐 case 及逐评估器 before/after。
3. cost/balanced/quality 分别映射到 1/2/3 轮、每轮 2/2/3 候选及 24/48/96 次模型调用基础预算；Goodcase 预算会按样本执行与裁判批次数扩展。case 固定拆分优化集和验证集，验证集选优，增益不超过 0.001 时提前停止。
4. Worker 使用 MySQL 原子 claim 和 10 分钟 lease，执行中续租；lease token 约束进度、失败和完成写入，过期任务最多重试认领 3 次。因此可多实例运行且旧 Worker 不会覆盖新结果。
5. 创建任务可配置 evaluator version、score_min、score_max、only_failed；reference output 必须走显式映射。采纳后写入完整 messages，并触发产品标准「提交新版」流。

6. Goodcase 路径调用 `BatchGetEvaluationSetItems` 获取版本化条目，把首轮字段按 key/name 展平用于变量映射，并保留多模态 Content；候选输出与 `reference_output_field` 由所选优化模型评分。裁判按最多 12 条一批返回逐 case before/after 分数，调用数由 N 次降为 `ceil(N/12)` 次；调用预算以 24/48/96 为基础下限，并按候选执行数与裁判批次数扩展。

7. Goodcase baseline 在候选生成前执行且每个 case 只调用一次，所有候选共享其输出。候选排名使用验证集平均增益 `mean(after_score - before_score)`，不再使用 after 绝对分；最佳增益必须严格大于 `0.001`，否则任务以“无候选优于 baseline”结束，不产生可采纳推荐。Goodcase 调用预算因此额外增加 `case_count` 次 baseline 调用。

### 7.2 Goodcase 裁判批次为什么暂定为 12

`12` 是当前实现的保守工程默认值，不是模型、EvaluationSet Items API 或业务协议的硬限制。选择该值主要考虑：

1. 每条裁判数据同时包含 Prompt 变量、参考答案、可选基线回答和候选回答；VLM 场景还可能携带较大的多模态描述，批次过大会快速消耗上下文。
2. 裁判必须稳定返回每个 `case_id` 的 before/after 分数；批次越大，越容易出现 JSON 截断、漏项、重复 ID 或 case 对错位。
3. 当前单次裁判输出上限为 4096 tokens；实现按每条约 160 个输出 token 加 512 个基础 token 估算，12 条约需 2432 tokens，仍保留较充足的格式与推理余量。

仅统计裁判请求时，固定 12 条分批的调用数如下：

| Goodcase 数量 | 逐条裁判 | 12 条一批 |
|---:|---:|---:|
| 10 | 10 | 1 |
| 50 | 50 | 5 |
| 100 | 100 | 9 |
| 500 | 500 | 42 |

后续应升级为**基于上下文预算的动态分批**：调用前估算文本和多模态证据大小，在模型上下文及输出预算内自动确定批次。短样本可提高到约 20–40 条，长文本或复杂 VLM 样本自动降低到约 4–8 条；遇到上下文超限、JSON 截断或结果缺失时，对当前批次二分后重试。动态策略落地前，固定 12 条优先保证评分结果的完整性和可校验性。

> **2026-07-22 已落地**：`estimateGoodcaseJudgeBatchSize` 按输出 token（上限 4096，约 160/条）与输入字符预算（约 120k）在 **4–40** 间选型；解析失败 / 漏 case / 上下文类错误时 `callGoodcaseJudgeBatchWithSplit` 二分重试。调用预算按最小批次 4 预留。回归：`scripts/local/regress-protocol-judge.sh`。

边界：Goodcase 当前使用模型裁判，不等同于实验路径的业务评估器；baseline 对照已经阻止“绝对高分但不优于原 Prompt”的候选被推荐，但 LLM Judge 仍可能对输出协议变形或拒答产生假阳性。因此在完成第 10 项前，Goodcase 分数不能作为自动发布依据。视觉裁剪、SoM 等图像侧自动变换仍属于后续能力。RocketMQ 可作为未来的低延迟唤醒通道，但任务事实源和并发正确性由 MySQL lease 保证。
