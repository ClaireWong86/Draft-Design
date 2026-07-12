# tire-ai-diagnosis × Prompt Loop 示例

本目录包含轮胎检测场景的 Prompt 模板与评测集样例，配合 `scripts/local/seed-tire-prompt.sh` 一键导入本地 Prompt Loop。

## 文件

| 文件 | 说明 |
|------|------|
| `prompt-template.md` | 轮位检测 Prompt 说明与模板变量 |
| `evaluation-dataset.json` | 5 条评测样例（轮位 + 场景 + 参考 JSON） |

## 导入

```bash
# 确保 Prompt Loop 本地栈已启动（8082）
scripts/local/docker-compose-local.sh start

# 写入 Prompt + 评测集（使用 smoke-test 同款账号）
scripts/local/seed-tire-prompt.sh
```

导入后可在 Prompt Loop 中查看：

- Prompt Key：`tire_wheel_diagnosis`
- 评测集名称：`轮胎检测评测集`

## Trace 打通

1. 生成 PAT 与环境变量（smoke 账号）：

```bash
bash scripts/local/create-smoke-pat.sh
```

2. 将输出写入 `tire-ai-diagnosis/services/api/.env`：

```bash
PROMPT_LOOP_TRACE_ENABLED=true
PROMPT_LOOP_OPENAPI_URL=http://localhost:8888
PROMPT_LOOP_WORKSPACE_ID=<你的空间 ID>
PROMPT_LOOP_API_TOKEN=<PAT>
PROMPT_LOOP_PROMPT_KEY=tire_wheel_diagnosis
```

诊断请求会自动上报各轮位 VLM 调用 Span 到 Prompt Loop 观测模块。
