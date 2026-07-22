# macOS 本地 Docker 启动指南

本文档记录当前本机验证过的 Coze Loop Docker Compose 启动方式。

## 访问地址

- Web 应用: `http://localhost:8082`
- App OpenAPI: `http://localhost:8888`

## 一键脚本

在项目根目录执行:

```bash
scripts/local/docker-compose-local.sh start
```

常用命令:

```bash
scripts/local/docker-compose-local.sh status
scripts/local/docker-compose-local.sh logs
scripts/local/docker-compose-local.sh logs app
scripts/local/docker-compose-local.sh refresh-config
scripts/local/docker-compose-local.sh render-config
scripts/local/docker-compose-local.sh restart-app
scripts/local/docker-compose-local.sh stop
scripts/local/docker-compose-local.sh doctor
```

启动后可以跑一次冒烟测试，确认网页、OpenAPI、注册、登录、个人空间接口都可用:

```bash
scripts/local/smoke-test.sh
```

## 为什么不用普通 `make compose-up`

本机 Docker Desktop 曾出现容器挂载 volume 后长期停在 `Created` 状态的问题。

已验证的规避方式:

1. Docker Desktop 使用 `overlay2` storage driver。
2. 不直接 bind mount 项目目录里的 `bootstrap/` 和 `conf/`。
3. 启动脚本会把 Docker Compose 配置复制到 `/private/tmp/coze-loop-docker-compose`，再把 `bootstrap/` 和 `conf/` 写入 Docker 命名卷。

这样做不会改变项目源码，也不会删除业务数据卷。

## 模型配置

模型配置文件在:

```text
release/deployment/docker-compose/conf/model_config.yaml
```

当前默认值是占位符:

```yaml
api_key: "***"
model: "***"
```

源码里的默认配置继续保持占位符，避免把本地密钥写进 Git 文件。若项目根目录的 `.env.local` 里存在 `OPENAI_API_KEY`，本地启动脚本会在 `/private/tmp/coze-loop-docker-compose/conf/model_config.yaml` 生成运行时 OpenAI 配置，并同步到 Docker 命名卷。

默认模型是:

```text
gpt-5.6-luna
```

可通过环境变量覆盖:

```bash
COZE_LOOP_OPENAI_MODEL=gpt-5.6-terra scripts/local/docker-compose-local.sh refresh-config
```

也可以写在本地 `.env.local` 中:

```text
OPENAI_API_KEY=...
COZE_LOOP_OPENAI_BASE_URL=https://api.openai.com/v1
COZE_LOOP_OPENAI_MODEL=gpt-5.6-luna
```

### Ark 模型

可直接复用 `/Users/wangdujuan/Projects/tire-ai-diagnosis/services/api/.env` 里的 Ark 配置。执行:

```bash
scripts/local/import-tire-ark-config.sh
scripts/local/docker-compose-local.sh refresh-config
scripts/local/docker-compose-local.sh restart-app
```

导入脚本会把这些变量写入当前项目 `.env.local`，不会打印密钥:

```text
ARK_API_KEY=...
ARK_BASE_URL=https://ark.cn-beijing.volces.com
ARK_MODEL=doubao-seed-2-0-pro-260215
```

配置 `ARK_API_KEY` 后，`refresh-config` 会把 Ark 作为新增模型写入运行时 `model_config.yaml`，协议为 `ark`。

如果 `ARK_BASE_URL` 来自 `tire-ai-diagnosis`，通常是:

```text
https://ark.cn-beijing.volces.com
```

本地启动脚本会在运行时自动补成 ChatModel 需要的:

```text
https://ark.cn-beijing.volces.com/api/v3
```

配置 Ark 后可以先检查真实模型连通性:

```bash
scripts/local/ark-check.sh
```

### JoyBuild 模型

已复用 `/Users/wangdujuan/Projects/tire-ai-diagnosis` 里的 JoyBuild 配置命名:

```text
JOYBUILD_BASE_URL=http://ai-api.jdcloud.com
JOYBUILD_API_KEY=...
JOYBUILD_MODEL=Gemini-3.1-Flash-Lite
```

配置 `JOYBUILD_API_KEY` 后，先启动 OpenAI 兼容代理，再刷新 Docker 配置:

```bash
scripts/local/joybuild-proxy.sh
scripts/local/docker-compose-local.sh refresh-config
scripts/local/docker-compose-local.sh restart-app
```

官方 Docker 镜像里没有内置 `joybuild` adapter。本地 Docker 会把 JoyBuild 配成 `protocol: openai`，指向本机 `host.docker.internal:18081`（由 `joybuild-openai-proxy.mjs` 把 chat/completions 转到 `JOYBUILD_BASE_URL/v1/responses`）。

热更新前端（`:8090`）会代理 `/cozeloop-minio` 到 `:8082`。若此前上传过图片，需要删掉旧图片后重新上传；否则 LLM 预取对象时会报 `HTTP请求失败: 404`（界面常显示为模型调用错误）。

如果当前网络无法直连 OpenAI，可以把 `COZE_LOOP_OPENAI_BASE_URL` 换成你能访问的 OpenAI 兼容网关地址，然后执行:

```bash
scripts/local/openai-check.sh
scripts/local/docker-compose-local.sh refresh-config
scripts/local/docker-compose-local.sh restart-app
```

如果要让 Prompt 调试、评测等模型调用功能真正工作，需要提供可用的模型服务配置。项目提供的示例在:

```text
release/deployment/docker-compose/conf/model_config_example/
```

修改配置后执行:

```bash
scripts/local/docker-compose-local.sh refresh-config
scripts/local/docker-compose-local.sh restart-app
```

## Docker Desktop 排查

检查当前 Docker 状态:

```bash
scripts/local/docker-compose-local.sh doctor
```

如果输出里看到:

```text
Storage driver: overlayfs
```

并且容器长期停在 `Created`，可以在 Docker Desktop 设置里关闭 `UseContainerdSnapshotter`，然后重启 Docker Desktop。当前本机已改为:

```text
Storage driver: overlay2
```

## 登录

开源版默认允许注册。进入 `http://localhost:8082/auth/login` 后，可以先点注册，使用任意测试邮箱和密码创建本地账号。

如果前端开发服务 `http://localhost:8090` 仍在运行，它使用的是本地开发兜底逻辑；完整 Docker 版入口是 `http://localhost:8082`。
