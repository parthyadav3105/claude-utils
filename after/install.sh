#!/usr/bin/env bash
set -e

# Installer for the after hook.
#
# Usage:
#   ./install.sh              # install to ~/.claude (or $CLAUDE_DIR)
#   ./install.sh --uninstall  # remove the binary and both hook entries
#   ./install.sh -h | --help

REPO="parthyadav3105/claude-utils"
BINARY="claudeafter"
INSTALL_DIR="${CLAUDE_DIR:-${HOME}/.claude}"
# a quoted CLAUDE_DIR ("~/.claude-max") reaches us with a literal ~, so expand it
case "$INSTALL_DIR" in
  "~")   INSTALL_DIR="$HOME" ;;
  "~/"*) INSTALL_DIR="${HOME}/${INSTALL_DIR#\~/}" ;;
esac
SETTINGS="${INSTALL_DIR}/settings.json"

uninstall=0
case "${1:-}" in
  -h|--help)   sed -n '4,9p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
  --uninstall) uninstall=1 ;;
  "")          ;;
  *)           echo "Unknown argument: $1" >&2; exit 1 ;;
esac

COMMAND="${INSTALL_DIR}/${BINARY}"

# Both hook entries are rewritten from scratch every time: any entry already
# pointing at our binary is dropped before ours is added back, so re-running
# never stacks duplicates. On uninstall we stop after the dropping.
patch_settings_jq() {
  local tmp
  tmp=$(mktemp)
  if jq --arg cmd "$COMMAND" --argjson add "$1" '
        def strip: map(select(((.hooks // []) | map(.command // "") | index($cmd)) == null));
        def put($k): .hooks[$k] = (((.hooks[$k] // []) | strip) + (if $add == 1 then [{hooks:[{type:"command",command:$cmd}]}] else [] end))
                   | (if (.hooks[$k] | length) == 0 then del(.hooks[$k]) else . end);
        (.hooks //= {}) | put("UserPromptSubmit") | put("Stop")
        | (if (.hooks | length) == 0 then del(.hooks) else . end)
      ' "$SETTINGS" > "$tmp" 2>/dev/null; then
    mv "$tmp" "$SETTINGS"
  else
    rm -f "$tmp"
    return 1
  fi
}

patch_settings_python() {
  python3 - "$SETTINGS" "$COMMAND" "$1" <<'EOF'
import json, sys
path, cmd, add = sys.argv[1], sys.argv[2], sys.argv[3] == "1"
try:
    with open(path) as f:
        data = json.load(f)
except (FileNotFoundError, json.JSONDecodeError):
    data = {}
hooks = data.get('hooks') or {}
for event in ('UserPromptSubmit', 'Stop'):
    kept = [e for e in (hooks.get(event) or [])
            if cmd not in [h.get('command') for h in (e.get('hooks') or [])]]
    if add:
        kept.append({'hooks': [{'type': 'command', 'command': cmd}]})
    if kept:
        hooks[event] = kept
    else:
        hooks.pop(event, None)
if hooks:
    data['hooks'] = hooks
else:
    data.pop('hooks', None)
with open(path, 'w') as f:
    json.dump(data, f, indent=2)
    f.write('\n')
EOF
}

patch_settings() {
  local add="$1"
  if [ ! -f "$SETTINGS" ]; then
    [ "$add" = "0" ] && return 0
    mkdir -p "$INSTALL_DIR"
    printf '{}\n' > "$SETTINGS"
  fi
  if command -v jq &>/dev/null && patch_settings_jq "$add"; then
    return 0
  elif command -v python3 &>/dev/null && patch_settings_python "$add"; then
    return 0
  fi
  return 1
}

if [ "$uninstall" -eq 1 ]; then
  if ! patch_settings 0; then
    echo "Warning: could not edit ${SETTINGS} automatically."
    echo "Remove the UserPromptSubmit and Stop entries naming ${COMMAND} by hand."
  fi
  rm -f "${INSTALL_DIR}/${BINARY}"
  echo "Removed ${INSTALL_DIR}/${BINARY} and its hook entries."
  echo "Queued messages were left in ${INSTALL_DIR}/after-queue.json"
  exit 0
fi

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
URL="https://github.com/${REPO}/releases/download/after-latest/${ASSET}"

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

if ! patch_settings 1; then
  echo "Warning: could not patch ${SETTINGS} automatically."
  echo "Add this manually:"
  echo '  "hooks": {'
  echo '    "UserPromptSubmit": [ { "hooks": [ { "type": "command", "command": "'"${COMMAND}"'" } ] } ],'
  echo '    "Stop":             [ { "hooks": [ { "type": "command", "command": "'"${COMMAND}"'" } ] } ]'
  echo '  }'
fi

echo "Installed to ${INSTALL_DIR}/${BINARY}"
echo "Restart Claude Code, then type: after 5m <message>"
