#!/usr/bin/env bash
set -e

REPO="parthyadav3105/claude-utils"
BINARY="claudeline"
INSTALL_DIR="${HOME}/.claude"
SETTINGS="${INSTALL_DIR}/settings.json"

# detect OS and arch
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac
case "$OS" in
  linux|darwin) ;;
  *) echo "Unsupported OS: $OS. For Windows use install.ps1"; exit 1 ;;
esac

ASSET="${BINARY}-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/download/statusline-latest/${ASSET}"

echo "Downloading ${ASSET}..."
mkdir -p "$INSTALL_DIR"
if command -v curl &>/dev/null; then
  curl -fsSL "$URL" -o "${INSTALL_DIR}/${BINARY}"
elif command -v wget &>/dev/null; then
  wget -qO "${INSTALL_DIR}/${BINARY}" "$URL"
else
  echo "curl or wget required"; exit 1
fi
chmod +x "${INSTALL_DIR}/${BINARY}"

# patch settings.json — always sets both type and command (idempotent)
COMMAND="${INSTALL_DIR}/${BINARY}"
patch_settings_jq() {
  local tmp
  tmp=$(mktemp)
  if jq --arg cmd "$COMMAND" '.statusLine = {"type": "command", "command": $cmd}' "$SETTINGS" > "$tmp" 2>/dev/null; then
    mv "$tmp" "$SETTINGS"
  else
    rm -f "$tmp"
    return 1
  fi
}

patch_settings_python() {
  python3 - "$SETTINGS" "$COMMAND" <<'EOF'
import json, sys
path, cmd = sys.argv[1], sys.argv[2]
try:
    with open(path) as f:
        data = json.load(f)
except (FileNotFoundError, json.JSONDecodeError):
    data = {}
data['statusLine'] = {'type': 'command', 'command': cmd}
with open(path, 'w') as f:
    json.dump(data, f, indent=2)
    f.write('\n')
EOF
}

if [ ! -f "$SETTINGS" ]; then
  printf '{"statusLine":{"type":"command","command":"%s"}}\n' "$COMMAND" > "$SETTINGS"
elif command -v jq &>/dev/null && patch_settings_jq; then
  :
elif command -v python3 &>/dev/null && patch_settings_python; then
  :
else
  echo "Warning: could not patch ${SETTINGS} automatically."
  echo "Add this manually:"
  echo '  "statusLine": { "type": "command", "command": "'"${COMMAND}"'" }'
fi

echo "Installed to ${INSTALL_DIR}/${BINARY}"
echo "Restart Claude Code to apply."
