#!/usr/bin/env python3
"""Create a smoke-test PAT and print Prompt Loop trace env vars."""
from __future__ import annotations

import json
import os
import sys
import urllib.error
import urllib.request

BASE_URL = os.environ.get("COZE_LOOP_BASE_URL", "http://localhost:8082")
EMAIL = os.environ.get("COZE_LOOP_SMOKE_EMAIL", "codex-local-smoke@example.com")
PASSWORD = os.environ.get("COZE_LOOP_SMOKE_PASSWORD", "Codex123456")
OPENAPI_URL = os.environ.get("COZE_LOOP_OPENAPI_URL", "http://localhost:8888")
PAT_NAME = os.environ.get("COZE_LOOP_SMOKE_PAT_NAME", "tire-trace-smoke")


def request_json(method: str, url: str, payload: dict | None = None, token: str | None = None) -> dict:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Cookie"] = f"session_key={token}"
    data = None
    if payload is not None:
        data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    with urllib.request.urlopen(req, timeout=30) as resp:
        body = resp.read().decode("utf-8")
        return json.loads(body) if body else {}


def main() -> int:
    register = request_json(
        "POST",
        f"{BASE_URL}/api/foundation/v1/users/register",
        {"email": EMAIL, "password": PASSWORD},
    )
    if register.get("code") not in (0, 602002002):
        print(f"register failed: {register}", file=sys.stderr)
        return 1

    login = request_json(
        "POST",
        f"{BASE_URL}/api/foundation/v1/users/login_by_password",
        {"email": EMAIL, "password": PASSWORD},
    )
    if login.get("code") != 0 or not login.get("token"):
        print(f"login failed: {login}", file=sys.stderr)
        return 1
    token = login["token"]

    spaces = request_json(
        "POST",
        f"{BASE_URL}/api/foundation/v1/spaces/list",
        {"page_number": 1, "page_size": 100},
        token=token,
    )
    if spaces.get("code") != 0 or not spaces.get("spaces"):
        print(f"spaces/list failed: {spaces}", file=sys.stderr)
        return 1
    workspace_id = str(spaces["spaces"][0]["id"])

    created = request_json(
        "POST",
        f"{BASE_URL}/api/auth/v1/personal_access_tokens",
        {"name": PAT_NAME, "duration_day": "365"},
        token=token,
    )
    if created.get("code") != 0 or not created.get("token"):
        print(f"create PAT failed: {created}", file=sys.stderr)
        return 1

    pat_token = created["token"]
    print("Add to tire-ai-diagnosis/services/api/.env:")
    print(f"PROMPT_LOOP_TRACE_ENABLED=true")
    print(f"PROMPT_LOOP_OPENAPI_URL={OPENAPI_URL}")
    print(f"PROMPT_LOOP_WORKSPACE_ID={workspace_id}")
    print(f"PROMPT_LOOP_API_TOKEN={pat_token}")
    print(f"PROMPT_LOOP_PROMPT_KEY=tire_wheel_diagnosis")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
