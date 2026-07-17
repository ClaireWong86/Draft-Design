# TRD：智能优化 Prompt

> 技术设计。产品需求见 [PRD](./prd-smart-prompt-optimize.md)。

## 1. 范围与原则

- 产品形态对齐扣子罗盘「智能优化」；**不声称复刻商业闭源算法**
- 开源仓无 OptimizeTask IDL；MVP 前端先接 Mock，API 形状以本文为准
- MVP 前端落在 `@cozeloop/prompt-pages`（`src/components/smart-optimize`）；后续可抽到独立 `prompt-adapter` 包

## 2. 总体架构

```text
FE (Header 下拉 + Tab + Wizard)
  → OptimizeTask API（同步：创建/查询/终止/采纳）
  → OptimizeWorker（异步：诊断 → 改写 → 评估 → 选择）
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
       → 在同样本（或抽样）上评估 J(P')
       → 接受更优；否则保留当前
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
| `workspace_id` | 空间 |
| `prompt_id` / `prompt_version` | 优化基线 |
| `source_type` | `eval_set` \| `experiment` |
| `source_id` / `source_version_id` / `experiment_id` | 数据源 |
| `case_item_ids[]` | 选中样本 |
| `mapping` | variables / actual_output / reference_output |
| `mode_score` | 0–1 |
| `status` | `queued` \| `running` \| `succeeded` \| `failed` \| `cancelled` |
| `progress` | 0–100 |
| `error_msg` | 失败信息 |
| `result` | 见下 |
| `created_by` / timestamps | 审计 |

`result`：

```json
{
  "before_prompt": "...",
  "after_prompt": "...",
  "before_score": 0.62,
  "after_score": 0.81,
  "score_distribution": { "before": [], "after": [] },
  "case_details": [],
  "diagnosis": { "failure_modes": [], "suggested_instruction_changes": [] }
}
```

## 5. API 契约（IDL 草案）

后续落在 `idl/thrift/coze/loop/prompt/`（或 evaluation 旁）。前端 MVP 使用同形状 Mock。

| RPC | 说明 |
|-----|------|
| `CreateOptimizeTask` | 创建并入队 |
| `ListOptimizeTasks` | 按 prompt / status / 源过滤 |
| `GetOptimizeTask` | 详情 + 进度 |
| `GetOptimizeTaskReport` | 报告（成功后） |
| `CancelOptimizeTask` | 终止（已消耗不回滚） |
| `AdoptOptimizeTask` | 返回优化后消息列表，供前端写入草稿 |

### 5.1 Create 校验（创建前）

- Prompt 模板类型、变量类型、样本量
- 实验状态成功且存在非满分
- 映射完整（至少变量 + reference_output；实验路径建议含 score）

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
| 实现（MVP） | `@cozeloop/prompt-pages` → `components/smart-optimize` | 下拉、向导、任务列表、报告、Mock API |
| 挂载 | `prompt-pages/.../develop/index.tsx` | 注入 buttons + `extraTabs` |
| 权限 | `pe.prompt.smart_optimize` | OSS 默认放行；后续可包 `Guard` |

### 6.1 采纳

`AdoptOptimizeTask`（或直接读 `result.after_prompt`）→ `usePromptStore.setMessageList` 写入草稿 → Toast 引导用户点「提交新版」。不绕过版本提交校验。

## 7. 存储与异步（后端后续）

- MySQL 任务表 + 结果 JSON（大报告可对象存储）
- MQ / 内部异步任务
- 可对齐 `ResourceTypeCozeloopOptimizeTask` 做缓存失效

## 8. 权限

- Guard：`pe.prompt.smart_optimize`
- OSS 默认 `ACTION`（不隐藏）

## 9. 轮胎场景映射（示例）

- Badcase：缺陷类型/位置错 → 强化 inspection process 与 JSON schema
- Goodcase：优质标注 JSON → 优化字段完备性与 severity 一致性

## 10. 实现阶段

1. FE Mock 闭环（本迭代）
2. IDL + API + Worker
3. FE 切真实客户端；去掉 Mock
