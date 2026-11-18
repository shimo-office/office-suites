#!/usr/bin/env bash
# shimo-net 一键配置：下载 CLI、初始化、登录并验证。
# 用法：bash bootstrap.sh workbuddy|dsh|codex|doubao
set -euo pipefail

AGENT="${1:-}"
case "$AGENT" in
  workbuddy|dsh|codex|doubao) ;;
  *) echo "用法：bash bootstrap.sh workbuddy|dsh|codex|doubao" >&2; exit 2 ;;
esac

OS="$(uname -s)"
ARCH="$(uname -m)"
case "$OS" in
  Darwin) PLATFORM="darwin" ;;
  Linux)  PLATFORM="linux" ;;
  *) echo "暂不支持 $OS；Windows 请用 install.ps1" >&2; exit 2 ;;
esac
case "$ARCH" in
  arm64|aarch64) CPU="arm64" ;;
  x86_64|amd64)  CPU="amd64" ;;
  *) echo "暂不支持架构 $ARCH" >&2; exit 2 ;;
esac

VERSION="${SHIMO_NET_VERSION:-latest}"
DEST="${SHIMO_NET_BIN:-$HOME/.local/bin/shimo-net}"
ASSET="shimo-net_${PLATFORM}_${CPU}"
if [ -n "${SHIMO_NET_BIN_URL:-}" ]; then
  BIN_URL="$SHIMO_NET_BIN_URL"
elif [ "$VERSION" = "latest" ]; then
  BIN_URL="https://github.com/shimo-office/office-suites/releases/latest/download/${ASSET}"
else
  BIN_URL="https://github.com/shimo-office/office-suites/releases/download/${VERSION}/${ASSET}"
fi

NEED_DOWNLOAD=0
if [ ! -x "$DEST" ] || [ "$VERSION" = "latest" ]; then
  NEED_DOWNLOAD=1
else
  CURRENT="$("$DEST" version 2>/dev/null | awk '{print $2}')"
  if [ "$CURRENT" != "${VERSION#v}" ]; then
    echo "== 发现已安装 ${CURRENT:-未知版本}，升级到 ${VERSION} =="
    NEED_DOWNLOAD=1
  fi
fi

if [ "$NEED_DOWNLOAD" = "1" ]; then
  echo "== 下载 shimo-net ${VERSION} (${PLATFORM}/${CPU}) =="
  mkdir -p "$(dirname "$DEST")"
  TMP="${DEST}.download"
  if ! curl -fL --retry 2 --connect-timeout 20 "$BIN_URL" -o "$TMP"; then
    echo "下载失败：$BIN_URL" >&2
    echo "请检查网络，或手动执行仓库里的 ./install.command" >&2
    rm -f "$TMP"
    exit 1
  fi
  chmod +x "$TMP"
  mv "$TMP" "$DEST"
fi

SKILL_DIR=""
case "$AGENT" in
  # WorkBuddy has an official market skill. Do not install the repository
  # custom skill into its private skills directory.
  workbuddy) SKILL_DIR="" ;;
  dsh)       SKILL_DIR="$HOME/.dsh/skills/shimo-net-cli" ;;
  codex)     SKILL_DIR="$HOME/.codex/skills/shimo-net-cli" ;;
  doubao)    SKILL_DIR="" ;;
esac

if [ -n "$SKILL_DIR" ]; then
  echo "== 安装技能到 ${SKILL_DIR} =="
  mkdir -p "$SKILL_DIR"
  if [ -n "${SHIMO_NET_SKILL_URL:-}" ]; then
    SKILL_URL="$SHIMO_NET_SKILL_URL"
  elif [ "$VERSION" = "latest" ]; then
    SKILL_URL="https://github.com/shimo-office/office-suites/releases/latest/download/SKILL.md"
  else
    SKILL_URL="https://github.com/shimo-office/office-suites/releases/download/${VERSION}/SKILL.md"
  fi
  if ! curl -fsSL --retry 2 --connect-timeout 20 "$SKILL_URL" -o "$SKILL_DIR/SKILL.md"; then
    echo "技能下载失败：$SKILL_URL" >&2
    exit 1
  fi
fi

# 先初始化网址，避免 login 在无 stdin 时等待“产品网址”输入。
if [ -n "${SHIMO_NET_ORIGIN:-}" ]; then
  "$DEST" init --origin "$SHIMO_NET_ORIGIN"
elif [ -f "$HOME/.shimo-net-cli/config.json" ]; then
  "$DEST" init
else
  "$DEST" init --origin https://shimo.net
fi

if ! "$DEST" whoami --json >/dev/null 2>&1; then
  echo "== 需要登录 shimo.net：请在打开的 Chrome 窗口完成登录 =="
  "$DEST" login
fi

if ! "$DEST" whoami --json >/dev/null 2>&1; then
  echo "未登录或登录失败，请运行：$DEST login" >&2
  exit 1
fi

echo "已配置完成，账号可用"
case "$AGENT" in
  workbuddy) echo "请在 SkillHub 安装 shimo-net-cli 技能（不在 WorkBuddy 内置技能市场）；本脚本不会安装仓库自定义技能。" ;;
  dsh)       echo "新建一个 DSH 会话即可使用技能。" ;;
  codex)     echo "新开一个 Codex 任务即可使用技能。" ;;
  doubao)    echo "在豆包「工作」里选择「本地电脑」，直接发任务即可。" ;;
esac
