#!/usr/bin/env python3
"""Mark unhealthy models as unavailable in generated model_config.yaml."""
from __future__ import annotations

import pathlib
import re
import subprocess
import sys


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: apply-model-health.py <model_config.yaml>", file=sys.stderr)
        return 1

    config_path = pathlib.Path(sys.argv[1])
    if not config_path.exists():
        return 0

    root = pathlib.Path(__file__).resolve().parents[2]
    script = root / "scripts/local/model-health-check.sh"
    proc = subprocess.run(
        ["bash", str(script), "--json"],
        cwd=root,
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        print(proc.stderr or proc.stdout, file=sys.stderr)
        return proc.returncode

    failed: set[str] = set()
    for line in (proc.stdout or "").splitlines():
        line = line.strip()
        if not line:
            continue
        parts = line.split("|", 3)
        if len(parts) < 3:
            continue
        _provider, model, status = parts[0], parts[1], parts[2]
        if status != "ok":
            failed.add(model)

    if not failed:
        return 0

    text = config_path.read_text(encoding="utf-8")
    for model_name in failed:
        pattern = re.compile(
            rf'(name: "{re.escape(model_name)}"[\s\S]*?scenario_configs:[\s\S]*?prompt_debug:[\s\S]*?unavailable: )false',
            re.MULTILINE,
        )
        text, count = pattern.subn(r"\1true", text, count=1)
        if count:
            print(f"Marked unavailable: {model_name}")

    config_path.write_text(text, encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
