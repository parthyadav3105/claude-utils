# Picks Claude's profile (CLAUDE_CONFIG_DIR) from the current directory.
#
# Installed by claude-utils/profiles/install.sh and sourced from your shell rc.
# Sourcing this only DEFINES the functions below; nothing runs until you type
# `claude`. Mappings live in $CLAUDE_PROFILES_DIR/profiles, one per line:
#
#     /abs/path/to/repo|profilename
#
# The global profile -- used wherever no mapped directory matches -- is the
# mapping of `/`; with no such line the `default` profile (~/.claude) is global.
# `default` is a real profile name meaning exactly that, so a directory can be
# pinned back to Claude's own config.
#
# Claude keeps all of its state -- credentials included -- in $CLAUDE_CONFIG_DIR
# (default ~/.claude), and that variable must be set before the process starts.
# That is why this is a shell function and not a Claude setting.

CLAUDE_PROFILES_DIR="${CLAUDE_PROFILES_DIR:-$HOME/.claude-profiles}"

# Echoes the profile name for $PWD; returns 1 when the `default` profile applies.
# The longest matching directory wins, so a nested mapping overrides its parent.
# `/` is stripped to the empty string here, so it matches every absolute path at
# length 0 -- which is why best_len starts below zero.
_claude_profile_for() {
  local map="$CLAUDE_PROFILES_DIR/profiles" dir name best='' best_len=-1 len
  [ -f "$map" ] || return 1
  while IFS='|' read -r dir name || [ -n "$dir" ]; do
    case "$dir" in ''|\#*) continue ;; esac
    [ -n "$name" ] || continue
    dir="${dir%/}"
    case "$PWD" in
      "$dir"|"$dir"/*)
        len=${#dir}
        if [ "$len" -gt "$best_len" ]; then best="$name"; best_len="$len"; fi
        ;;
    esac
  done < "$map"
  [ -n "$best" ] || return 1
  printf '%s' "$best"
}

claude() {
  local name
  name="$(_claude_profile_for)" || name=default
  if [ "$name" = default ]; then
    # Drop any inherited value, so a nested session cannot leak its profile
    # into a directory that should use the default account.
    env -u CLAUDE_CONFIG_DIR claude "$@"
  else
    CLAUDE_CONFIG_DIR="$CLAUDE_PROFILES_DIR/$name" command claude "$@"
  fi
}

# Which profile applies here? `claude-profile --list` shows every profile.
#
# One line per profile, never two for the same one. `*` marks the global profile,
# the one used wherever no mapped directory matches; `default` -- Claude's own
# ~/.claude -- is always listed, because it is a profile like any other.
claude-profile() {
  local map="$CLAUDE_PROFILES_DIR/profiles" name dir n g seen=0
  if [ "${1:-}" = "--list" ] || [ "${1:-}" = "-l" ]; then
    g="$(awk -F'|' '$1=="/" {print $2; exit}' "$map" 2>/dev/null)"
    [ -n "$g" ] || g=default
    if [ "$g" = default ]; then
      printf '* %-8s %s\n' default "$HOME/.claude"; seen=1
    else
      printf '  %-8s %s\n' default "$HOME/.claude"
    fi
    if [ -f "$map" ]; then
      while IFS='|' read -r dir n || [ -n "$dir" ]; do
        case "$dir" in ''|\#*|/) continue ;; esac
        if [ "$n" = "$g" ] && [ "$seen" -eq 0 ]; then
          printf '* '; seen=1
        else
          printf '  '
        fi
        printf '%-8s %s\n' "$n" "$dir"
      done < "$map"
    fi
    # A profile can be global without owning a directory.
    [ "$seen" -eq 1 ] || printf '* %-8s %s\n' "$g" "(global only)"
    return 0
  fi
  name="$(_claude_profile_for)" || name=default
  if [ "$name" = default ]; then
    printf 'default -> %s\n' "$HOME/.claude"
  else
    printf '%s -> %s\n' "$name" "$CLAUDE_PROFILES_DIR/$name"
  fi
}
