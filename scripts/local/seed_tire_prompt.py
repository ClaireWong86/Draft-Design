#!/usr/bin/env python3
"""Seed tire diagnosis prompt and evaluation dataset into local Prompt Loop."""
from __future__ import annotations

import json
import os
import sys
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
EXAMPLES = ROOT / "examples/tire-ai-diagnosis"
PROMPT_KEY = "tire_wheel_diagnosis"
PROMPT_NAME = "轮胎单轮检测"
EVAL_SET_NAME = "轮胎检测评测集"

BASE_URL = os.environ.get("COZE_LOOP_BASE_URL", "http://localhost:8082")
EMAIL = os.environ.get("COZE_LOOP_SMOKE_EMAIL", "codex-local-smoke@example.com")
PASSWORD = os.environ.get("COZE_LOOP_SMOKE_PASSWORD", "Codex123456")

POS_LABEL = {"LF": "左前轮", "RF": "右前轮", "LR": "左后轮", "RR": "右后轮"}

SYSTEM_PROMPT = """你是一位轮胎检测技师，正在为门店出具轮位检测报告。当前轮位：{{position_label}}（代码 {{position}}）。
本次共 {{image_count}} 张照片，拍摄顺序与角度不固定，请综合所有照片中实际可见信息判断。

输出要求：
- 仅输出一段合法 JSON，不要 markdown 代码块，不要前后说明文字。
- position 必须为 "{{position}}"；anomalies 的 id 从 "{{position_lower}}_1" 起递增。

14 类病名与 type：bulge / nail / wear / tread / age / unknown。
证据策略：仅当区域清晰、特征可辨认时写入 anomalies；conf < 0.50 不输出该 anomaly。
sev 标准：crit > sev > mod > min，不确定时取保守档。
image_quality：ok / blurry / no_tire；blurry 或 no_tire 时必填 image_quality_index（0-based）。

JSON 需包含：position, score, level, spec, dotInfo, pattern, anomalies, passed, image_quality。"""


def request_json(
    method: str,
    url: str,
    payload: dict | None = None,
    token: str | None = None,
) -> dict:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Cookie"] = f"session_key={token}"
    data = None
    if payload is not None:
        data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            body = resp.read().decode("utf-8")
            return json.loads(body) if body else {}
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"{method} {url} failed: HTTP {exc.code}: {detail}") from exc


def ensure_login() -> str:
    register = request_json(
        "POST",
        f"{BASE_URL}/api/foundation/v1/users/register",
        {"email": EMAIL, "password": PASSWORD},
    )
    if register.get("code") not in (0, 602002002):
        raise RuntimeError(f"register failed: {register}")

    login = request_json(
        "POST",
        f"{BASE_URL}/api/foundation/v1/users/login_by_password",
        {"email": EMAIL, "password": PASSWORD},
    )
    if login.get("code") != 0 or not login.get("token"):
        raise RuntimeError(f"login failed: {login}")
    return login["token"]


def get_workspace_id(token: str) -> str:
    spaces = request_json(
        "POST",
        f"{BASE_URL}/api/foundation/v1/spaces/list",
        {"page_number": 1, "page_size": 100},
        token=token,
    )
    if spaces.get("code") != 0 or not spaces.get("spaces"):
        raise RuntimeError(f"spaces/list failed: {spaces}")
    return str(spaces["spaces"][0]["id"])


def find_prompt_id(token: str, workspace_id: str) -> str | None:
    listed = request_json(
        "POST",
        f"{BASE_URL}/api/prompt/v1/prompts/list",
        {
            "workspace_id": workspace_id,
            "key_word": PROMPT_KEY,
            "page_num": 1,
            "page_size": 20,
        },
        token=token,
    )
    if listed.get("code") != 0:
        raise RuntimeError(f"prompts/list failed: {listed}")
    for prompt in listed.get("prompts") or []:
        if prompt.get("prompt_key") == PROMPT_KEY:
            return str(prompt["id"])
    return None


def create_prompt(token: str, workspace_id: str) -> str:
    created = request_json(
        "POST",
        f"{BASE_URL}/api/prompt/v1/prompts",
        {
            "workspace_id": workspace_id,
            "prompt_name": PROMPT_NAME,
            "prompt_key": PROMPT_KEY,
            "prompt_description": "来自 tire-ai-diagnosis 的轮胎单轮 VLM 检测 Prompt",
            "draft_detail": {
                "prompt_template": {
                    "template_type": "normal",
                    "messages": [
                        {"role": "system", "content": SYSTEM_PROMPT},
                        {
                            "role": "user",
                            "content": "请根据上传的 {{image_count}} 张轮胎照片，完成轮位 {{position}}（{{position_label}}）的检测并输出 JSON。",
                        },
                    ],
                    "variable_defs": [
                        {"key": "position", "type": "string", "desc": "轮位代码 LF/RF/LR/RR"},
                        {"key": "position_label", "type": "string", "desc": "轮位中文名"},
                        {"key": "position_lower", "type": "string", "desc": "轮位小写，用于 anomaly id"},
                        {"key": "image_count", "type": "string", "desc": "图片数量 1-3"},
                    ],
                },
                "model_config": {"temperature": 0.2, "max_tokens": 4096},
            },
        },
        token=token,
    )
    if created.get("code") != 0 or not created.get("prompt_id"):
        raise RuntimeError(f"create prompt failed: {created}")
    prompt_id = str(created["prompt_id"])

    committed = request_json(
        "POST",
        f"{BASE_URL}/api/prompt/v1/prompts/{prompt_id}/drafts/commit",
        {
            "commit_version": "0.0.1",
            "commit_description": "Seed from tire-ai-diagnosis examples",
        },
        token=token,
    )
    if committed.get("code") != 0:
        raise RuntimeError(f"commit prompt failed: {committed}")
    return prompt_id


def find_eval_set_id(token: str, workspace_id: str) -> str | None:
    listed = request_json(
        "POST",
        f"{BASE_URL}/api/evaluation/v1/evaluation_sets/list",
        {
            "workspace_id": workspace_id,
            "name": EVAL_SET_NAME,
            "page_size": 20,
            "page_num": 1,
        },
        token=token,
    )
    if listed.get("code") != 0:
        raise RuntimeError(f"evaluation_sets/list failed: {listed}")
    for item in listed.get("evaluation_sets") or []:
        if item.get("name") == EVAL_SET_NAME:
            return str(item["id"])
    return None


def create_eval_set(token: str, workspace_id: str) -> str:
    created = request_json(
        "POST",
        f"{BASE_URL}/api/evaluation/v1/evaluation_sets",
        {
            "workspace_id": workspace_id,
            "name": EVAL_SET_NAME,
            "description": "轮胎单轮检测评测样例，来自 tire-ai-diagnosis",
            "evaluation_set_schema": {
                "field_schemas": [
                    {"key": "position", "name": "轮位", "content_type": "Text", "default_display_format": 1},
                    {"key": "image_count", "name": "图片数量", "content_type": "Text", "default_display_format": 1},
                    {"key": "scenario", "name": "场景描述", "content_type": "Text", "default_display_format": 1},
                    {"key": "reference_output", "name": "参考输出", "content_type": "Text", "default_display_format": 1},
                ]
            },
        },
        token=token,
    )
    if created.get("code") != 0 or not created.get("evaluation_set_id"):
        raise RuntimeError(f"create evaluation set failed: {created}")
    return str(created["evaluation_set_id"])


def seed_eval_items(token: str, workspace_id: str, eval_set_id: str) -> int:
    dataset_path = EXAMPLES / "evaluation-dataset.json"
    rows = json.loads(dataset_path.read_text(encoding="utf-8"))
    items = []
    for row in rows:
        position = row["position"]
        items.append(
            {
                "item_key": row["item_key"],
                "turns": [
                    {
                        "field_data_list": [
                            {
                                "key": "position",
                                "content": {"content_type": "Text", "text": position},
                            },
                            {
                                "key": "image_count",
                                "content": {"content_type": "Text", "text": row["image_count"]},
                            },
                            {
                                "key": "scenario",
                                "content": {"content_type": "Text", "text": row["scenario"]},
                            },
                            {
                                "key": "reference_output",
                                "content": {"content_type": "Text", "text": row["reference_output"]},
                            },
                        ]
                    }
                ],
            }
        )

    created = request_json(
        "POST",
        f"{BASE_URL}/api/evaluation/v1/evaluation_sets/{eval_set_id}/items/batch_create",
        {
            "workspace_id": workspace_id,
            "items": items,
            "skip_invalid_items": True,
        },
        token=token,
    )
    if created.get("code") != 0:
        raise RuntimeError(f"batch_create items failed: {created}")
    outputs = created.get("item_outputs") or []
    return len(outputs) or len(items)


def main() -> int:
    print(f"Prompt Loop base URL: {BASE_URL}")
    token = ensure_login()
    workspace_id = get_workspace_id(token)
    print(f"Workspace: {workspace_id}")

    prompt_id = find_prompt_id(token, workspace_id)
    if prompt_id:
        print(f"Prompt already exists: {PROMPT_KEY} (id={prompt_id})")
    else:
        prompt_id = create_prompt(token, workspace_id)
        print(f"Created prompt: {PROMPT_KEY} (id={prompt_id}, version=0.0.1)")

    eval_set_id = find_eval_set_id(token, workspace_id)
    if eval_set_id:
        print(f"Evaluation set already exists: {EVAL_SET_NAME} (id={eval_set_id})")
    else:
        eval_set_id = create_eval_set(token, workspace_id)
        print(f"Created evaluation set: {EVAL_SET_NAME} (id={eval_set_id})")

    count = seed_eval_items(token, workspace_id, eval_set_id)
    print(f"Seeded {count} evaluation item(s)")
    print("Done.")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:  # noqa: BLE001
        print(f"ERROR: {exc}", file=sys.stderr)
        raise SystemExit(1) from exc
