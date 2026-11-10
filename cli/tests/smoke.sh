#!/bin/bash
# 无网络冒烟测试：不登录、不访问 shimo.net
set -euo pipefail

CLI_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$CLI_DIR"

echo "[1/6] py_compile"
python3 -m py_compile bin/shimo.py

echo "[2/6] version"
[ "$(./shimo version)" = "shimo-net 0.6.0" ]
./shimo --version | grep -q "0.6.0"

echo "[3/6] help"
./shimo help | grep -q "15 个场景"

echo "[4/6] schema"
./shimo schema | python3 -c '
import json, sys
d = json.load(sys.stdin)
assert d["version"] == "0.6.0", d["version"]
assert len(d["commands"]) == 20, len(d["commands"])
assert len(d["scenarios"]) == 15, len(d["scenarios"])
assert len(d["shareModes"]) == 7, len(d["shareModes"])
'

echo "[5/6] --json"
./shimo version --json | python3 -c '
import json, sys
assert json.load(sys.stdin)["version"] == "0.6.0"
'

echo "[6/6] unknown command exits 2"
set +e
./shimo definitely-not-a-command >/dev/null 2>&1
code=$?
set -e
[ "$code" -eq 2 ]

# 根目录入口也能用
"$CLI_DIR/../shimo" version >/dev/null

echo "smoke ok"
