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
  → OptimizeTask API（同步：创建/查询/终止/采纳）
  → OptimizeWorker（异步：诊断 → 候选改写；评估/选择闭环为下一阶段）
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
| 评测集（Goodcase） | 有评估器则复用；否则 LLM-as-Judge（输出 vs `reference_output`）或结构化字段匹配分 |

### 3.3 诊断 / 改写 / 选择

1. **诊断**：抽取得分最低（或与参考差距最大）的 K 条，生成 `failure_modes[]`、`suggested_instruction_changes[]`
2. **改写**：Meta-LLM 输入 `P_current` + 诊断 + exemplars + 约束（变量名、JSON schema、语言）；输出完整消息列表
3. **选择**：`argmax J`；无提升则 `no_improve++`

### 3.4 模式滑块 → 超参

| 模式 | 候选数 N | 最大轮次 T | 每轮评估样本 | 诊断条数 K |
|------|----------|------------|--------------|------------|
| 性价比优先 | 1 | 2–3 | 抽样 30%–50%（下限 10） | 5–8 |
| 效果优先 | 3–5 | 5–8 | 全量选中样本 | 10–20 |

`mode_score ∈ [0,1]`：0=性价比优先，1=效果优先；中间线性插值。

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

`AdoptOptimizeTask`（或直接读 `result.after_prompt`）→ 将完整 `messages[]` 写入 `usePromptStore.setMessageList` → Toast 引导用户点「提交新版」。不绕过版本提交校验，不只替换 system 文本。

### 6.3 文本与非文本能力边界

智能优化的**优化对象是 Prompt 消息结构**，不是只支持纯文本输入：

- 文本：Meta-LLM 可改写 `message.content`、角色指令、输出格式和 Few-shot 文本。
- 图片/视频：样本选择、诊断输入和基线/候选 Prompt 均保留 `messages[].parts[]`，媒体 URI 只读透传；Worker 调用目标 VLM 时必须按原顺序重建图文消息。
- 当前不做：裁剪/放大、Set-of-Mark、OCR 增强、图像生成或替换。这些属于后续 `VisualTransformPlan`，需要可复现的变换参数和派生媒体存储，不能混入文本改写字段。

因此，MVP 是“**支持非文本证据的 Prompt 自动优化**”，还不是“自动联合优化图像和文本输入”。

## 7. 存储与异步（已实现）

- MySQL `optimize_task` 保存不可变基线、映射、样本 ID、模型、状态与结果 JSON；SQL 已同步 Docker init/patch 与 Helm init。
- Create 先持久化 `queued` 再入内部有界队列；Worker 原子 claim 后更新 `running/progress/succeeded/failed/cancelled`。
- 每 30 秒扫描持久化队列；服务重启会恢复 queued 任务，并将超过 10 分钟未更新的 running 任务重新排队。
- Worker 在调用优化模型前后检查取消标记；模型空响应、非 JSON、缺少 `optimized_prompt` 均进入 `failed` 并保留错误。
- 当前采用进程内 Worker，适合单实例 OSS MVP；多实例/高吞吐部署应将同一持久任务协议迁移到 RocketMQ，并增加租约/重试次数。

### 7.1 当前 Worker 的准确能力边界

本轮已完成“可运行的第一阶段后端”，不是完整的迭代优化算法：

1. 输入不可变 Prompt 快照、字段映射、来源与选中 case ID；
2. 使用用户选择的 `optimizer_model_id` 调用 LLM Runtime；
3. 生成结构化诊断与候选 Prompt，保留变量定义和多模态 parts；
4. 持久化结果，允许查询、取消、恢复和前端采纳。

尚未完成的是从实验/评测集加载每条真实 input/output/score、执行候选 Prompt、调用原评估器重评，以及优化集/验证集选优。因此当前成功任务若无分数，UI 明确显示“候选已生成（待重评）”，不会伪造提升分。

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
5. [ ] Worker 加载完整 case 证据，执行候选并复用评估器重评
6. [ ] 优化集/验证集拆分、多候选多轮选优、成本预算与 RocketMQ 多实例化
