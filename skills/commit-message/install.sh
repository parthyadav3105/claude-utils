#!/usr/bin/env bash
set -euo pipefail

# Installer for the commit-message skill.
#
# Usage:
#   ./install.sh                   # install to ~/.claude/skills/        (user-level)
#   ./install.sh --project [DIR]   # install to <DIR>/.claude/skills/    (default DIR=.)
#   ./install.sh --uninstall       # remove from user-level location
#   ./install.sh --uninstall --project [DIR]
#   ./install.sh -f | --force      # overwrite without prompting
#   ./install.sh -h | --help

SKILL_NAME="commit-message"
RAW_BASE="https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/skills/${SKILL_NAME}"

# Locate SKILL.md. When run from a checkout it sits next to this script;
# when piped via `curl ... | bash` there is no local copy, so download it.
if [[ -n "${BASH_SOURCE[0]:-}" && -f "$(dirname "${BASH_SOURCE[0]}")/SKILL.md" ]]; then
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  SOURCE_FILE="$SCRIPT_DIR/SKILL.md"
else
  SOURCE_FILE="$(mktemp)"
  trap 'rm -f "$SOURCE_FILE"' EXIT
  if command -v curl &>/dev/null; then
    curl -fsSL "$RAW_BASE/SKILL.md" -o "$SOURCE_FILE"
  elif command -v wget &>/dev/null; then
    wget -qO "$SOURCE_FILE" "$RAW_BASE/SKILL.md"
  else
    echo "curl or wget required to download SKILL.md" >&2
    exit 1
  fi
fi

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
  dest_dir="${HOME}/.claude/skills/${SKILL_NAME}"
else
  dest_dir="$(cd "$project_dir" && pwd)/.claude/skills/${SKILL_NAME}"
fi
dest="$dest_dir/SKILL.md"

if [[ "$uninstall" -eq 1 ]]; then
  if [[ -f "$dest" ]]; then
    rm "$dest"
    rmdir "$dest_dir" 2>/dev/null || true
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
