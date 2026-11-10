#!/bin/bash
# 仓库根目录安装入口：转调 cli/install.command
set -e
DIR="$(cd "$(dirname "$0")" && pwd)"
exec "$DIR/cli/install.command" "$@"
