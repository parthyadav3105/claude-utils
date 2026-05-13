#!/usr/bin/env bash
set -euo pipefail

# Installer for the /commit-message slash command.
#
# Usage:
#   ./install.sh                   # install to ~/.claude/commands/  (user-level)
#   ./install.sh --project [DIR]   # install to <DIR>/.claude/commands/ (default DIR=.)
#   ./install.sh --uninstall       # remove from user-level location
#   ./install.sh --uninstall --project [DIR]
#   ./install.sh -f | --force      # overwrite without prompting
#   ./install.sh -h | --help

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_FILE="$SCRIPT_DIR/commit-message.md"
COMMAND_NAME="commit-message.md"

scope="user"
project_dir=""
force=0
uninstall=0

usage() {
  sed -n '3,12p' "$0" | sed 's/^# \{0,1\}//'
  exit "${1:-0}"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --project)
      scope="project"
      if [[ $# -ge 2 && "$2" != -* ]]; then
        project_dir="$2"; shift 2
      else
        project_dir="."; shift
      fi
      ;;
    --uninstall) uninstall=1; shift ;;
    -f|--force)  force=1; shift ;;
    -h|--help)   usage 0 ;;
    *) echo "Unknown argument: $1" >&2; usage 1 ;;
  esac
done

if [[ "$scope" == "user" ]]; then
  dest_dir="${HOME}/.claude/commands"
else
  dest_dir="$(cd "$project_dir" && pwd)/.claude/commands"
fi
dest="$dest_dir/$COMMAND_NAME"

if [[ "$uninstall" -eq 1 ]]; then
  if [[ -f "$dest" ]]; then
    rm "$dest"
    echo "Removed: $dest"
  else
    echo "Nothing to remove at: $dest"
  fi
  exit 0
fi

if [[ ! -f "$SOURCE_FILE" ]]; then
  echo "Source file not found: $SOURCE_FILE" >&2
  exit 1
fi

mkdir -p "$dest_dir"

if [[ -f "$dest" && "$force" -ne 1 ]]; then
  if cmp -s "$SOURCE_FILE" "$dest"; then
    echo "Already up to date: $dest"
    exit 0
  fi
  echo "Destination exists and differs: $dest"
  echo "Diff (existing → new):"
  diff -u "$dest" "$SOURCE_FILE" || true
  read -r -p "Overwrite? [y/N] " reply
  case "$reply" in
    y|Y|yes|YES) ;;
    *) echo "Aborted."; exit 1 ;;
  esac
fi

install -m 0644 "$SOURCE_FILE" "$dest"
echo "Installed: $dest"
echo "Invoke with: /commit-message"
