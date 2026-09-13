#!/usr/bin/env bash
set -e

# Installer for task.
#
# Usage:
#   ./install.sh              # install task to ~/.local/bin (or $INSTALL_DIR)
#   ./install.sh --uninstall  # remove the Claude Code hook and the binary; tasks are kept
#   ./install.sh -h | --help

REPO="parthyadav3105/claude-utils"
BINARY="task"
INSTALL_DIR="${INSTALL_DIR:-${HOME}/.local/bin}"
case "$INSTALL_DIR" in
  "~")   INSTALL_DIR="$HOME" ;;
  "~/"*) INSTALL_DIR="${HOME}/${INSTALL_DIR#\~/}" ;;
esac
DEST="${INSTALL_DIR}/${BINARY}"

uninstall=0
case "${1:-}" in
  -h|--help)   sed -n '4,9p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
  --uninstall) uninstall=1 ;;
  "")          ;;
  *)           echo "Unknown argument: $1" >&2; exit 1 ;;
esac

ours() { "$DEST" --help 2>/dev/null | grep -q "Task Management Tool for AI Coding Agent"; }

if [ -e "$DEST" ] && ! ours; then
  echo "${DEST} is a different program called task; leaving it alone." >&2
  echo "Set INSTALL_DIR to use another folder." >&2
  exit 1
fi

if [ "$uninstall" -eq 1 ]; then
  if [ -e "$DEST" ] && ! "$DEST" uninstall claude-code; then
    echo "Could not remove the Claude Code setup, so ${DEST} was kept." >&2
    exit 1
  fi
  rm -f "$DEST"
  echo "Removed ${DEST}. Your tasks were kept."
  exit 0
fi

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
URL="https://github.com/${REPO}/releases/download/tasks-latest/${ASSET}"

echo "Downloading ${ASSET}..."
mkdir -p "$INSTALL_DIR"
TMP="${DEST}.download.$$"
trap 'rm -f "$TMP"' EXIT
if command -v curl &>/dev/null; then
  curl -fsSL "$URL" -o "$TMP"
elif command -v wget &>/dev/null; then
  wget -qO "$TMP" "$URL"
else
  echo "curl or wget required"; exit 1
fi
chmod +x "$TMP"
mv -f "$TMP" "$DEST"

echo "Installed to ${DEST}"
case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *) echo "Add ${INSTALL_DIR} to your PATH: the Claude Code hook runs task by name." ;;
esac
echo "To use it with Claude Code, run: task setup claude-code"
