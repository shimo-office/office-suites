#!/bin/bash
# shimo-net CLI 安装
set -e
DIR="$(cd "$(dirname "$0")" && pwd)"
DEST="$HOME/.shimo-net-py"
APP="$DEST/app"
mkdir -p "$APP"
if command -v rsync >/dev/null 2>&1; then
  rsync -a "$DIR/" "$APP/" --exclude 'install.command' --exclude '.DS_Store'
else
  cp -R "$DIR"/bin "$DIR"/examples "$DIR"/shimo "$DIR"/install.command "$DIR"/README.md "$DIR"/SKILL.md "$DIR"/scenarios.md "$DIR"/安装使用说明.md "$APP/" 2>/dev/null || true
fi
chmod +x "$APP/bin/shimo.py" "$APP/shimo" 2>/dev/null || true

PY="${SHIMO_PYTHON:-}"
[ -z "$PY" ] && PY="$(command -v python3 2>/dev/null || true)"
[ -z "$PY" ] && PY=/usr/bin/python3
if [ ! -x "$PY" ]; then
  echo "需要 Python 3.9+：请先执行 xcode-select --install，或设置 SHIMO_PYTHON 指向 python3"
  exit 1
fi
if ! "$PY" -c 'import sys; raise SystemExit(0 if sys.version_info >= (3,9) else 1)' 2>/dev/null; then
  echo "Python 版本过低（需要 3.9+）：$PY"
  exit 1
fi
echo "python: $PY ($("$PY" -V 2>&1))"

mkdir -p "$HOME/.local/bin"
cat > "$HOME/.local/bin/shimo-net" <<WRAP
#!/bin/sh
exec "$PY" "$APP/bin/shimo.py" "\$@"
WRAP
chmod +x "$HOME/.local/bin/shimo-net"

if [ -d "$HOME/.dsh" ]; then
  mkdir -p "$HOME/.dsh/skills/shimo-net-cli"
  cp "$APP/SKILL.md" "$HOME/.dsh/skills/shimo-net-cli/SKILL.md"
  echo "技能：~/.dsh/skills/shimo-net-cli"
fi

case ":$PATH:" in
  *":$HOME/.local/bin:"*) ;;
  *) echo "提示：把 ~/.local/bin 加进 PATH：export PATH=\"\$HOME/.local/bin:\$PATH\"" ;;
esac

echo ""
echo "== 完成 =="
"$PY" "$APP/bin/shimo.py" doctor || true
echo "首次登录：shimo-net login"
