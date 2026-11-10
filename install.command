#!/bin/bash
# shimo-net CLI 安装：有 Go 就本地构建，没有就下载最新 release 二进制
set -e
DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$HOME/.local/bin"
BIN="$BIN_DIR/shimo-net"
mkdir -p "$BIN_DIR"

if command -v go >/dev/null 2>&1; then
  echo "使用 Go 构建：$(go version)"
  (cd "$DIR" && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$BIN" ./cli)
else
  OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$(uname -m)" in
    arm64|aarch64) ARCH=arm64 ;;
    x86_64|amd64) ARCH=amd64 ;;
    *) echo "不支持的架构：$(uname -m)"; exit 1 ;;
  esac
  URL="https://github.com/shimo-office/office-suites/releases/latest/download/shimo-net_${OS}_${ARCH}"
  echo "未找到 Go，下载预编译二进制：$URL"
  TMP_BIN="$BIN.tmp.$$"
  if ! curl -fL --retry 2 --connect-timeout 15 "$URL" -o "$TMP_BIN"; then
    rm -f "$TMP_BIN"
    echo "下载失败。请安装 Go 1.22+ 后重试，或手动从 release 页面下载 $URL"
    exit 1
  fi
  chmod +x "$TMP_BIN"
  mv "$TMP_BIN" "$BIN"
fi

if [ -d "$HOME/.dsh" ]; then
  mkdir -p "$HOME/.dsh/skills/shimo-net-cli"
  cp "$DIR/cli/SKILL.md" "$HOME/.dsh/skills/shimo-net-cli/SKILL.md"
  echo "技能：~/.dsh/skills/shimo-net-cli"
fi

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "提示：把 ~/.local/bin 加进 PATH：export PATH=\"\$HOME/.local/bin:\$PATH\"" ;;
esac

echo ""
echo "== 完成 =="
"$BIN" doctor || true
echo "首次使用：shimo-net init && shimo-net login"
