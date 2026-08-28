#!/usr/bin/env bash
# ==============================================================================
# Claude Code Profile Manager — one profile per directory
#
# Maps a directory (and everything under it) to its own Claude profile. Claude
# keeps all state, credentials included, in $CLAUDE_CONFIG_DIR; that must be set
# before the process starts, so this installs a `claude` shell function which
# picks the config dir from $PWD. Run it and use the menu — there are no flags.
#
# Set CLAUDE_PROFILES_DIR to keep profile state outside ~/.claude-profiles.
# ==============================================================================

# Needs bash: arrays and $'..' quoting. Keep this check POSIX-clean and above
# any bash-only syntax so `sh install.sh` fails with a readable message.
if [ -z "${BASH_VERSION:-}" ]; then
    echo "This installer needs bash. Try:  curl -fsSL <url> | bash" >&2
    exit 1
fi

set -euo pipefail

RAW_BASE="https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/profiles"
PROFILES_DIR="${CLAUDE_PROFILES_DIR:-${HOME}/.claude-profiles}"
SWITCH="${PROFILES_DIR}/switch.sh"
MAP="${PROFILES_DIR}/profiles"
MARKER='### Directory-scoped Claude Code accounts (claude-utils/profiles)'

# Keep the rc line portable ($HOME form) for the default location; use a literal
# path when CLAUDE_PROFILES_DIR points somewhere else.
if [ "$PROFILES_DIR" = "$HOME/.claude-profiles" ]; then
    SOURCE_LINE='[ -f "$HOME/.claude-profiles/switch.sh" ] && . "$HOME/.claude-profiles/switch.sh"'
else
    SOURCE_LINE="[ -f \"$SWITCH\" ] && . \"$SWITCH\""
fi

# Every rc line this tool has ever written, so old installs are replaced rather
# than stacked. Bracket expressions, not backslash escapes: this string is also
# passed to awk via -v, which would warn about unknown escape sequences.
RC_GREP='Directory-scoped Claude Code accounts|claude-profiles/switch[.]sh|switch[.]sh" []] && [.]'
MARKER_GREP='Directory-scoped Claude Code accounts'
RC_FILES=("$HOME/.bashrc" "$HOME/.zshrc" "$HOME/.bash_profile" "$HOME/.profile")
RESERVED="profiles switch.sh"
NEXT_CD=""           # directory to suggest after a successful add
REPORT=""            # last action's output, reprinted above the menu
RUNOUT=""            # scratch file the report is collected in
ROW_LABELS=()        # the list-as-menu: what is drawn ...
ROW_IDS=()           # ... and what each row means
TARGET=""            # the directory row being acted on
GUIDE_ROWS=2         # lines the guide printed, and lines the menu will draw:
MENU_ROWS=3          # together they say how much room is left for the report
DID=""               # added | deleted | unwired -- picks the follow-up hint
WAS_WIRED=0          # was the wrapper already sourced from a shell rc?

# ==============================================================================
# FORMATTING HELPERS — terminal colors, output helpers, interactive menu
# No need to read or edit this section during normal use.
# ==============================================================================

# Real escape characters, not the '\033' printf spelling: these are also passed
# as %s arguments (menu items), where printf would not expand them.
if [ -t 2 ]; then
    _BOLD=$'\033[1m'; _DIM=$'\033[2m'; _GREY=$'\033[90m'
    _CYAN=$'\033[36m'; _GREEN=$'\033[32m'; _RED=$'\033[31m'
    _RST=$'\033[0m'
else
    _BOLD=''; _DIM=''; _GREY=''; _CYAN=''; _GREEN=''; _RED=''; _RST=''
fi

_AP_MARK='▶'
die()  { printf "${_RED}${_BOLD}ERROR:${_RST} %s\n" "$*" >&2; exit 1; }
info() { printf "${_GREEN}${_BOLD}==>${_RST} %s\n" "$*"; } 2>/dev/null
note() { printf "    ${_DIM}%s${_RST}\n" "$*"; } 2>/dev/null
warn() {
    printf "  ${_RED}${_BOLD}!${_RST}  %s\n" "$*" >&2
    # Also keep it for the redraw: warn is often called from inside a $(...)
    # subshell, so appending to a file is the only way the message survives.
    [ -z "$RUNOUT" ] || printf "  ${_RED}${_BOLD}!${_RST}  %s\n" "$*" >> "$RUNOUT"
}
rule() { printf "\n  ${_DIM}──────────────────────────────────────${_RST}\n"; } 2>/dev/null

# menu TITLE ITEM... — arrow-key picker; echoes the chosen item on stdout while
# the interface itself goes to stderr, so callers just use $(menu ...). TITLE is
# printed verbatim so a caller can hand over a styled table header. Two items are
# layout, not choices: "-" draws a blank spacer, and "=" turns everything after
# it into one horizontal row of buttons. The cursor skips both.
# The cursor starts on the first item, so pass the default first. jk/←→ and the
# number keys work too, unadvertised, to keep the hint short. `read -rsn1`
# handles the raw-mode work itself, so there is no stty juggling, and the script
# already requires a tty (see main), so there is no fallback path.
menu() {
    local title="$1"; shift
    local -a items=("$@")
    local n=${#items[@]} cur=0 key i t rows btn line
    # Where the buttons begin, and therefore how many lines get drawn: one per
    # item up to the marker, plus the single button row.
    rows=$n
    for i in "${!items[@]}"; do
        if [ "${items[i]}" = "=" ]; then rows=$((i + 1)); break; fi
    done
    _menu_layout() { case "${items[$1]}" in '-'|'=') return 0 ;; esac; return 1; }
    # Move by $1, hopping over layout items. A blocked move has to be a no-op
    # rather than a failure: `set -e` reads a bare `[ ... ] && cur=...` that
    # tests false as a fatal command.
    _menu_step() {
        t=$((cur + $1))
        while [ "$t" -ge 0 ] && [ "$t" -lt "$n" ] && _menu_layout "$t"; do
            t=$((t + $1))
        done
        if [ "$t" -ge 0 ] && [ "$t" -lt "$n" ]; then cur=$t; fi
    }
    # Number keys count only real choices, so a spacer never eats a digit.
    _menu_pick() {
        local i c=0
        for i in "${!items[@]}"; do
            if ! _menu_layout "$i"; then
                c=$((c + 1))
                if [ "$c" = "$1" ]; then cur=$i; return 0; fi
            fi
        done
        return 1
    }
    printf "\n  %s  ${_DIM}↑↓ Enter${_RST}\n" "$title" >&2
    printf '\033[?25l' >&2                                   # hide cursor
    # bash restarts a `read` interrupted by a trapped signal, so the handler has
    # to leave by itself. This runs in the $(...) subshell; the non-zero status
    # travels up through the assignment and `set -e` ends the run.
    trap 'printf "\033[?25h" >&2' RETURN
    trap 'printf "\033[?25h\n" >&2; exit 130' INT TERM
    while :; do
        line=""
        for i in "${!items[@]}"; do
            if [ "$i" -ge "$rows" ]; then
                # A button: text only, no fill. A background colour is a bet on
                # the user's theme -- a grey box behind background-coloured text
                # is dark-on-dark for anyone on a dark terminal -- so the picked
                # one is cyan and bold and the rest are grey. The padding stays,
                # so the row never shifts as the cursor moves.
                if [ "$i" = "$cur" ]; then
                    btn="${_CYAN}${_BOLD} ${items[i]} ${_RST}"
                else
                    btn="${_GREY} ${items[i]} ${_RST}"
                fi
                if [ -z "$line" ]; then line="$btn"; else line="$line  $btn"; fi
            elif [ "${items[i]}" = "-" ] || [ "${items[i]}" = "=" ]; then
                printf '\033[K\n' >&2
            elif [ "$i" = "$cur" ]; then
                printf "  ${_CYAN}${_BOLD}▶  %s${_RST}\033[K\n" "${items[i]}" >&2
            else
                printf "     %s${_RST}\033[K\n" "${items[i]}" >&2
            fi
        done
        # The marker's own line is where the buttons land.
        if [ -n "$line" ]; then
            printf '\033[1A\r    %s\033[K\n' "$line" >&2
        fi
        # A failed read means EOF, not Enter -- treating them alike would spin
        # this loop forever when stdin dies.
        IFS= read -rsn1 key || { printf '\033[?25h\n' >&2; exit 130; }
        [ -n "$key" ] || break                                # Enter confirms
        case "$key" in
            $'\033')
                IFS= read -rsn2 -t 0.2 key || key=""
                case "$key" in
                    '[A'|'[D') _menu_step -1 ;;
                    '[B'|'[C') _menu_step 1 ;;
                esac ;;
            k|h)  _menu_step -1 ;;
            j|l)  _menu_step 1 ;;
            [1-9]) if _menu_pick "$key"; then break; fi ;;
        esac
        printf '\033[%dA' "$rows" >&2                         # rewind over the list
    done
    printf "\033[%dA\033[J  ${_CYAN}${_BOLD}▶  %s${_RST}\n" "$rows" "${items[cur]}" >&2
    unset -f _menu_step _menu_pick _menu_layout
    printf '%s' "${items[cur]}"                           # result on stdout
}

# confirm HEADER — Yes/No, defaulting to No.
confirm() { [ "$(menu "${_BOLD}$1${_RST}" "No" "Yes")" = "Yes" ]; }

# ask PROMPT [DEFAULT] [HINT] — free-text input (readline); echoes the answer on
# stdout. HINT is shown in the same dim style the menu uses for its key hints.
ask() {
    local reply
    # Tab cycles through matches (menu-complete) instead of needing a
    # double-Tab listing; Shift-Tab steps back. Harmless if bind is unavailable.
    bind 'set menu-complete-display-prefix off' 2>/dev/null || true
    bind 'set completion-ignore-case on'        2>/dev/null || true
    bind 'set colored-stats on'                 2>/dev/null || true
    bind 'TAB:menu-complete'                    2>/dev/null || true
    bind '"\e[Z":menu-complete-backward'        2>/dev/null || true
    printf "\n  ${_BOLD}%s${_RST}${3:+  ${_DIM}($3)${_RST}}${2:+  ${_DIM}[$2]${_RST}}\n" "$1" >&2
    read -e -r -p "  ▶  " reply
    printf '%s' "${reply:-${2:-}}"
}

# ask_path PROMPT [DEFAULT] — path input with fish-style inline suggestions: the
# best matching directory trails the cursor in grey as you type, Tab cycles
# through the matches, Right accepts the suggestion. Hand-rolled because
# readline (`read -e`) has no ghost-text hook; the trade-off is that only the
# editing keys handled below work — no history, no Ctrl-A/E. Esc cancels;
# Ctrl-C is left to the tty driver, which sends SIGINT and ends the run.
ask_path() {
    local prompt="$1" default="${2:-}"
    local buf="" key seq ghost show disp m ex
    local -a hits=()
    local hidx=-1

    # Matching directories for what is typed so far. Recomputed on every edit,
    # so cycling always reflects the current buffer.
    _ap_match() {
        hits=(); hidx=-1
        [ -n "$buf" ] || return 0
        ex="${buf/#\~/$HOME}"
        for m in "$ex"*; do
            [ -d "$m" ] || continue                  # dirs only; literal glob fails this
            hits+=("$m")
        done
    }
    # Show paths back in the shape the user is typing them (~ stays ~).
    _ap_disp() { case "$buf" in '~'*) tilde "$1" ;; *) printf '%s' "$1" ;; esac; }
    _ap_draw() {
        ghost=""
        if [ "${#hits[@]}" -gt 0 ]; then
            disp="$(_ap_disp "${hits[0]}")"
            case "$disp" in "$buf"*) ghost="${disp#"$buf"}" ;; esac
        fi
        printf '\r  %s  %s' "$_AP_MARK" "$buf" >&2
        [ -z "$ghost" ] || printf "${_DIM}%s${_RST}" "$ghost" >&2
        printf '\033[K' >&2
        [ -z "$ghost" ] || printf '\033[%dD' "${#ghost}" >&2
    }

    printf "\n  ${_BOLD}%s${_RST}  ${_DIM}(Tab cycles, → accepts)${_RST}${default:+  ${_DIM}[$default]${_RST}}\n" "$prompt" >&2
    _ap_draw
    while :; do
        IFS= read -rsn1 key || { printf '\n' >&2; return 1; }
        case "$key" in
            '')  break ;;                                    # Enter
            $'\177'|$'\b') buf="${buf%?}"; _ap_match ;;       # Backspace
            $'\025') buf=""; _ap_match ;;                     # Ctrl-U
            $'\027') buf="${buf%/}"; buf="${buf%/*}"; _ap_match ;;   # Ctrl-W
            $'\t')                                           # Tab: cycle matches
                [ "${#hits[@]}" -gt 0 ] || _ap_match
                if [ "${#hits[@]}" -gt 0 ]; then
                    hidx=$(( (hidx + 1) % ${#hits[@]} ))
                    buf="$(_ap_disp "${hits[$hidx]}")"
                    # keep the cycled entry as the shown suggestion
                    hits=("${hits[@]:$hidx}" "${hits[@]:0:$hidx}"); hidx=0
                fi ;;
            $'\033')                                         # Esc / arrows
                IFS= read -rsn2 -t 0.2 seq || seq=""
                case "$seq" in
                    '[C') [ "${#hits[@]}" -eq 0 ] || { buf="$(_ap_disp "${hits[0]}")"; _ap_match; } ;;
                    '')   printf '\n' >&2; return 1 ;;            # bare Esc cancels
                esac ;;
            *) buf="$buf$key"; _ap_match ;;                   # printable
        esac
        _ap_draw
    done
    printf '\n' >&2
    unset -f _ap_match _ap_disp _ap_draw
    printf '%s' "${buf:-$default}"
}

# Renders $HOME as ~ so paths stay short. ${p/#$HOME/~} does not work here:
# once $HOME expands its slashes are read as the pattern delimiter.
tilde() {
    case "$1" in
        "$HOME")   printf '~' ;;
        "$HOME"/*) printf '~%s' "${1#"$HOME"}" ;;
        *)         printf '%s' "$1" ;;
    esac
}

# ==============================================================================
# END OF FORMATTING HELPERS
# ==============================================================================

# ── wiring ────────────────────────────────────────────────────────────────────

# Locate switch.sh: next to this script in a checkout, else download it.
locate_switch_source() {
    local here tmp
    if [ -n "${BASH_SOURCE[0]:-}" ]; then
        here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
        if [ -f "$here/switch.sh" ]; then printf '%s' "$here/switch.sh"; return 0; fi
    fi
    tmp="$(mktemp)"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$RAW_BASE/switch.sh" -o "$tmp" || die "Failed to download switch.sh"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "$tmp" "$RAW_BASE/switch.sh" || die "Failed to download switch.sh"
    else
        die "curl or wget required to download switch.sh"
    fi
    printf '%s' "$tmp"
}

rc_file() {
    case "${SHELL:-/bin/bash}" in
        *zsh) printf '%s' "$HOME/.zshrc" ;;
        *)    printf '%s' "$HOME/.bashrc" ;;
    esac
}

# Strip every rc block this tool has added, from all plausible rc files. This is
# what makes re-runs idempotent instead of additive.
unwire_rc() {
    local f found=1
    for f in "${RC_FILES[@]}"; do
        [ -f "$f" ] || continue
        grep -qE "$RC_GREP" "$f" || continue
        # Drop our lines, plus the single blank line we inserted before the
        # marker. The blank is popped only for the MARKER line -- the block's
        # first line -- since matching the source line too would eat a second,
        # user-owned blank.
        awk -v pat="$RC_GREP" -v mark="$MARKER_GREP" '
          $0 ~ pat {
            if ($0 ~ mark && n > 0 && buf[n] ~ /^[[:space:]]*$/) n--
            next
          }
          { buf[++n] = $0 }
          END { for (i = 1; i <= n; i++) print buf[i] }
        ' "$f" > "$f.tmp.$$" || die "Failed to rewrite $f"
        mv "$f.tmp.$$" "$f"
        note "unwired  $(tilde "$f")"
        found=0
    done
    return $found
}

wire_rc() {
    local rc; rc="$(rc_file)"
    # Already exactly right? Say nothing -- every mapping change passes through
    # here, and "wired ~/.bashrc" on each one is noise.
    if grep -qxF "$MARKER" "$rc" 2>/dev/null && grep -qxF "$SOURCE_LINE" "$rc" 2>/dev/null; then
        return 0
    fi
    unwire_rc >/dev/null || true
    printf '\n%s\n%s\n' "$MARKER" "$SOURCE_LINE" >> "$rc"
    note "wired    $(tilde "$rc")"
}

install_switch() {
    local src; src="$(locate_switch_source)"
    mkdir -p "$PROFILES_DIR"; chmod 700 "$PROFILES_DIR"
    if ! cmp -s "$src" "$SWITCH"; then
        install -m 0644 "$src" "$SWITCH" || die "Failed to install $SWITCH"
        note "wrapper  $(tilde "$SWITCH")"
    fi
    [ -f "$(dirname "${BASH_SOURCE[0]:-.}")/switch.sh" ] || rm -f "$src"
    [ -f "$MAP" ] || { : > "$MAP"; chmod 600 "$MAP"; }
}

# Anything that would fight with our claude() function.
check_conflicts() {
    local f hits found=0
    for f in "${RC_FILES[@]}"; do
        [ -f "$f" ] || continue
        hits="$(grep -nE 'alias[[:space:]]+claude=|^[[:space:]]*(function[[:space:]]+)?claude[[:space:]]*\(\)' "$f" 2>/dev/null || true)"
        if [ -n "$hits" ]; then
            found=1
            warn "another 'claude' alias/function in $(tilde "$f") — the last one defined wins:"
            printf '%s\n' "$hits" | sed 's/^/       /' >&2
        fi
    done
    for f in "${RC_FILES[@]}" "$HOME/.zshenv"; do
        [ -f "$f" ] || continue
        hits="$(grep -nE '^[[:space:]]*export[[:space:]]+CLAUDE_CONFIG_DIR' "$f" 2>/dev/null || true)"
        if [ -n "$hits" ]; then
            found=1
            warn "CLAUDE_CONFIG_DIR exported in $(tilde "$f") — pins every directory to one account:"
            printf '%s\n' "$hits" | sed 's/^/       /' >&2
        fi
    done
    [ "$found" -eq 0 ] || confirm "Continue anyway?" || die "Aborted."
}

# ── mappings ──────────────────────────────────────────────────────────────────

# Echoes a normalised absolute path, or fails with a message.
norm_dir() {
    local d="${1/#\~/$HOME}"
    [ "${d:0:1}" = "/" ] || { warn "not an absolute path: $1"; return 1; }
    d="${d%/}"
    case "$d" in
        "") warn "to change the global profile, pick its row and Mark as global"; return 1 ;;
        "$HOME") warn "refusing $1 — that mapping would capture everything"; return 1 ;;
    esac
    printf '%s' "$d"
}

valid_profile() {
    local n="$1" r
    case "$n" in
        ''|*/*|*'|'*|*' '*|.|..) warn "profile name must be a plain word: $n"; return 1 ;;
        # `+` opens the picker's sentinels (+new, +remove, +cancel).
        '+'*) warn "profile name cannot start with '+': $n"; return 1 ;;
    esac
    for r in $RESERVED; do
        [ "$n" != "$r" ] || { warn "'$n' is reserved"; return 1; }
    done
    printf '%s' "$n"
}

# The mapping of `/` names the global profile -- the one used wherever no mapped
# directory matches -- so it is drawn as a `*` on that profile's row rather than
# as a directory. `default` is a real profile name meaning ~/.claude, which is
# what lets a directory be pinned back to Claude's own config.
map_lines()      { [ -f "$MAP" ] && grep -vE '^[[:space:]]*(#|$)' "$MAP" || true; }
dir_lines()      { map_lines | grep -v '^/|' || true; }
profile_of_dir() { map_lines | awk -F'|' -v d="$1" '$1==d {print $2; exit}'; }
dir_of_profile() { map_lines | awk -F'|' -v n="$1" '$2==n {print $1; exit}'; }

add_mapping() {
    local dir="$1" name="$2" tmp
    tmp="$(mktemp)"
    # keyed by directory: an existing entry for this dir is replaced, not appended
    map_lines | awk -F'|' -v d="$dir" '$1!=d' > "$tmp" || true
    printf '%s|%s\n' "$dir" "$name" >> "$tmp"
    sort -o "$tmp" "$tmp"
    install -m 0600 "$tmp" "$MAP"; rm -f "$tmp"
}

remove_mapping() {
    local dir="$1" tmp
    tmp="$(mktemp)"
    map_lines | awk -F'|' -v d="$dir" '$1!=d' > "$tmp" || true
    install -m 0600 "$tmp" "$MAP"; rm -f "$tmp"
}

# Not displayed; the menu uses this to decide whether to offer Uninstall.
wiring_state() {
    local f wired=""
    [ -f "$SWITCH" ] || { printf 'not installed'; return; }
    for f in "${RC_FILES[@]}"; do
        [ -f "$f" ] && grep -qE "$RC_GREP" "$f" && wired="$wired $(tilde "$f")"
    done
    [ -n "$wired" ] && printf '%s' "${wired# }" || printf 'switch.sh present, not sourced'
}

# Rows carry ${_DIM} but never ${_RST}: the menu closes every line, so the same
# label reads correctly selected (cyan) or not.
row_label() {                     # row_label MARK PROFILE LOCATION [FLAG]
    if [ -z "${4:-}" ]; then
        printf '%s%-10s %s' "$1" "$2" "$3"
    else
        printf '%s%-10s %-22s %s(%s)' "$1" "$2" "$3" "$_DIM" "$4"
    fi
}

# Only annotate what is not yet true; both notes share one parenthetical rather
# than stacking two. The `default` profile keeps its state in ~/.claude, so there
# is nothing of ours to be missing.
row_flag() {                      # row_flag PROFILE [DIRECTORY]
    local f=""
    if [ "$1" != default ] && [ ! -f "$PROFILES_DIR/$1/.credentials.json" ]; then
        f="no login yet"
    fi
    if [ -n "${2:-}" ] && [ ! -d "$2" ]; then
        f="${f:+$f, }dir missing"
    fi
    printf '%s' "$f"
}

# The list IS the menu: every row is selectable, so ROW_IDS holds what each one
# means -- "+default" for Claude's own profile, a path for a mapping, "/" for a
# profile that is global without owning a directory, "+verb" for a command.
#
# One row per profile, never two for the same one. `*` marks whichever profile is
# global; `default` -- Claude's own ~/.claude -- always has a row, because it is a
# profile like any other and making something else global must not hide it.
build_rows() {
    local dir name g i seen=0 mark
    ROW_LABELS=(); ROW_IDS=()
    g="$(profile_of_dir /)"; g="${g:-default}"

    if [ "$g" = default ]; then mark='* '; seen=1; else mark='  '; fi
    ROW_LABELS+=("$(row_label "$mark" default "~/.claude" "$(row_flag default)")")
    ROW_IDS+=("+default")

    while IFS='|' read -r dir name || [ -n "${dir:-}" ]; do
        [ -n "${dir:-}" ] || continue
        if [ "$name" = "$g" ] && [ "$seen" -eq 0 ]; then mark='* '; seen=1; else mark='  '; fi
        ROW_LABELS+=("$(row_label "$mark" "$name" "$(tilde "$dir")" "$(row_flag "$name" "$dir")")")
        ROW_IDS+=("$dir")
    done < <(dir_lines)

    # A profile can be global without owning a directory, so it still needs a row.
    if [ "$seen" -eq 0 ]; then
        ROW_LABELS+=("$(row_label '* ' "$g" "(global only)" "$(row_flag "$g")")")
        ROW_IDS+=("/")
    fi
    ROW_LABELS+=("-");                            ROW_IDS+=("-")
    ROW_LABELS+=("=");                            ROW_IDS+=("=")
    ROW_LABELS+=("Add new");                      ROW_IDS+=("+add")
    if [ "$(wiring_state)" != "not installed" ]; then
        ROW_LABELS+=("Uninstall");                ROW_IDS+=("+unwire")
    fi
    ROW_LABELS+=("Quit (ctrl+c)");                ROW_IDS+=("+quit")
    # How many lines the menu draws: everything up to and including the "="
    # marker, since the whole button row lands on the marker's own line.
    MENU_ROWS=${#ROW_LABELS[@]}
    for i in "${!ROW_LABELS[@]}"; do
        if [ "${ROW_LABELS[$i]}" = "=" ]; then MENU_ROWS=$((i + 1)); break; fi
    done
}

row_id() {                        # row_id LABEL — what the chosen row means
    local i
    for i in "${!ROW_LABELS[@]}"; do
        if [ "${ROW_LABELS[$i]}" = "$1" ]; then printf '%s' "${ROW_IDS[$i]}"; return 0; fi
    done
    return 1
}

# ── actions ───────────────────────────────────────────────────────────────────

# Everything a mapping needs before it can take effect.
ensure_wired() {
    check_conflicts
    install_switch
    wire_rc
}

login_note() {                    # login_note PROFILE
    if [ "$1" = default ]; then
        note "The 'default' profile is whatever ~/.claude is already logged into."
    elif [ -f "$PROFILES_DIR/$1/.credentials.json" ]; then
        note "'$1' is already logged in."
    else
        note "Claude creates the profile and prompts for login on its first run."
    fi
}

action_add() {
    local raw dir name def prev here=""
    # Offering $PWD is handy when curl-piped from inside the target repo, but not
    # when the user is standing in the claude-utils checkout they just cloned.
    # -f, not -n: piped in, BASH_SOURCE[0] is "main", whose dirname resolves to
    # $PWD and would suppress the very default we want to offer there.
    if [ -f "${BASH_SOURCE[0]:-}" ]; then
        here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    fi
    if [ -n "$here" ] && { [ "$PWD" = "$here" ] || [ "$PWD" = "$(dirname "$here")" ]; }; then
        raw="$(ask_path "Directory that should use its own profile:")" || raw=""
    else
        raw="$(ask_path "Directory that should use its own profile:" "$(tilde "$PWD")")" || raw=""
    fi
    # Backing out is silent: the list coming back unchanged already says it.
    [ -n "$raw" ] || return 0
    dir="$(norm_dir "$raw")" || return 0
    # An already-mapped directory keeps its profile as the pre-filled answer, so
    # Add new doubles as "re-point this one" -- add_mapping replaces by directory.
    prev="$(profile_of_dir "$dir")"
    def="${prev:-$(basename "$dir")}"
    name="$(ask "Profile name:" "$def")"
    name="$(valid_profile "$name")" || return 0

    ensure_wired
    add_mapping "$dir" "$name"
    DID=added
    if [ -n "$prev" ] && [ "$prev" != "$name" ]; then
        info "$(tilde "$dir") moved from '$prev' to '$name'."
    else
        info "$(tilde "$dir") and everything under it now uses '$name'."
    fi
    [ -d "$dir" ] || note "$(tilde "$dir") does not exist yet — the mapping activates once it does."
    login_note "$name"
    NEXT_CD="$(tilde "$dir")"
}

# Make a profile the global one -- used wherever no mapped directory matches.
# `default` is Claude's own ~/.claude, so choosing it drops the root mapping
# instead of writing one. "Global", never "default", is the word for this: the
# `default` profile is a profile, and the two meanings collided.
make_global() {                   # make_global PROFILE
    local name="$1" cur
    cur="$(profile_of_dir /)"; cur="${cur:-default}"
    if [ "$name" = "$cur" ]; then
        info "No change — '$cur' is already the global profile."
        return 0
    fi
    if [ "$name" = default ]; then
        remove_mapping /
        DID=changed
        info "'default' is global again — unlisted directories use your ~/.claude login."
        return 0
    fi
    ensure_wired
    add_mapping / "$name"
    DID=changed
    info "'$name' is now global — used in every directory not listed."
    note "Listed directories keep their own profiles."
    login_note "$name"
}

# pick_row_action ROWID PROFILE — echoes +global or +delete, or fails when
# cancelled. Labels carry a dim explanation of the consequence, so no verb has to
# be guessed at, and the plain values travel alongside them.
#
# Every row gets this same menu, the (global) row included -- one shape to learn,
# and no row that behaves unlike its neighbours. Both entries are always present
# even where they would be no-ops: a menu that changes shape from one row to the
# next reads as a missing feature, so the hint carries the state instead.
#
# There is deliberately no "change this directory's profile": Add new on an
# already-mapped directory replaces its entry, so a second way to do the same
# thing would only be another verb to read past.
pick_row_action() {
    local dir="$1" name="$2" g chosen i mark del
    local -a labels=() values=()
    _pr_add() { values+=("$1"); labels+=("$2"); }
    g="$(profile_of_dir /)"; g="${g:-default}"

    if [ "$name" = "$g" ]; then
        mark="already global"
    else
        mark="use '$name' in every directory not listed"
    fi
    # Delete removes this row's mapping. Claude's own profile has none to remove;
    # a global-only row's mapping is the global one, so dropping it hands the
    # global back to `default`.
    case "$dir" in
        +default) del="Claude's own profile — nothing to remove" ;;
        /)        del="stop '$name' being global" ;;
        *)        del="unmap $(tilde "$dir")" ;;
    esac

    _pr_add +global "$(printf '%-16s %s%s' "Mark as global" "$_DIM" "$mark")"
    _pr_add +delete "$(printf '%-16s %s%s' "Delete"         "$_DIM" "$del")"
    _pr_add - -
    _pr_add +cancel "Cancel"

    chosen="$(menu "${_BOLD}$name — $(row_where "$dir")${_RST}" "${labels[@]}")"
    for i in "${!labels[@]}"; do
        if [ "${labels[$i]}" = "$chosen" ]; then chosen="${values[$i]}"; break; fi
    done
    unset -f _pr_add
    [ "$chosen" != +cancel ] || return 1
    printf '%s' "$chosen"
}

# How a row names its place: only a directory row has a path.
row_where() {
    case "$1" in
        +default) printf '~/.claude' ;;
        /)        printf '(global only)' ;;
        *)        tilde "$1" ;;
    esac
}

# Any row: what can be done with this profile. Making it global lives here
# because this is where people look for it -- pointing the global at a profile
# already on screen beats a separate picker. Re-pointing a directory is Add new,
# which replaces an existing entry.
action_row() {
    local dir="$TARGET" name chosen
    case "$dir" in
        +default) name=default ;;
        /)        name="$(profile_of_dir /)"; name="${name:-default}" ;;
        *)        name="$(profile_of_dir "$dir")" ;;
    esac
    chosen="$(pick_row_action "$dir" "$name")" || return 0
    case "$chosen" in
        +global) make_global "$name" ;;
        +delete)
            case "$dir" in
                +default) info "'default' is Claude's own ~/.claude — it cannot be deleted." ;;
                /)        make_global default ;;
                *)        remove_row "$dir" "$name" ;;
            esac ;;
    esac
}

# Unmap a directory. The profile's login is kept unless asked otherwise, because
# re-mapping later should not mean logging in again.
remove_row() {
    local dir="$1" name="$2" cfg
    remove_mapping "$dir"
    DID=changed
    info "$(tilde "$dir") is unmapped — it uses the global profile now."
    cfg="$PROFILES_DIR/$name"
    [ -d "$cfg" ] || return 0
    if [ -n "$(dir_of_profile "$name")" ]; then
        note "'$name' is still mapped elsewhere, so its login is kept."
        return 0
    fi
    if confirm "Delete the stored login for '$name' too?"; then
        rm -rf "$cfg"
        info "Deleted $(tilde "$cfg")"
    else
        note "Login kept (reused if you map it again) — rm -rf $(tilde "$cfg") to drop it."
    fi
}

action_unwire() {
    confirm "Remove the claude() function? Mappings and logins are kept." || return 0
    unwire_rc || note "no shell-rc lines to remove"
    rm -f "$SWITCH"
    DID=unwired
    note "removed  $(tilde "$SWITCH")"
    info "Every directory is back on Claude's own ~/.claude profile."
    note "Re-run this installer to wire it back."
}

# ── menu ──────────────────────────────────────────────────────────────────────

# Redraws the whole interface from the top of the screen, so the menu stays in
# one place instead of scrolling a fresh copy into view after every action. The
# previous action's report is reprinted above the menu as a one-off flash.
# Rows in the window. `tput` needs a terminal, and $LINES is only set in
# interactive shells, so fall back to the classic 24.
term_rows() {
    local r
    r="$(tput lines 2>/dev/null)" || r=""
    case "$r" in ''|*[!0-9]*) r="${LINES:-24}" ;; esac
    printf '%s' "$r"
}

# Redraws the whole interface from the top of the window, so the menu stays in
# one place. The previous action's report is a one-off flash above the list --
# and it is trimmed to fit, because the menu redraws itself with relative cursor
# moves that cannot climb back over a scroll. Push the list past the bottom edge
# and every later redraw lands in the wrong place.
screen() {
    printf '\033[H\033[J'
    printf "\n  ${_BOLD}%s${_RST}\n" "Claude Code Profile Manager"
    rule
    guide
    [ -z "$REPORT" ] || flash
}

# Chrome above the report: a blank line, the title, rule (blank + line), then
# guide (blank + GUIDE_ROWS). Below it: the report's own blank, then the menu's
# blank + title + MENU_ROWS, and one line left spare at the bottom.
flash() {
    local keep
    keep=$(( $(term_rows) - GUIDE_ROWS - MENU_ROWS - 9 ))
    [ "$keep" -gt 0 ] || return 0
    printf '\n%s\n' "$(printf '%s\n' "$REPORT" | head -n "$keep")"
}

# The whole manual, for someone who has never seen this screen: what a keypress
# does, and what the `*` row means. Two dim lines earn their space -- without
# them the list reads as a status table nobody thinks to press Enter on.
guide() {
    local l
    local -a g=("Enter opens what you can do with a row."
                "* is the global profile, used everywhere not listed.")
    # Only for a genuinely untouched install -- a global profile is a mapping, so
    # saying "nothing mapped" alongside one would be a lie.
    if [ -z "$(dir_lines)" ] && [ -z "$(profile_of_dir /)" ]; then
        g+=('Nothing mapped yet — try "Add new".')
    fi
    GUIDE_ROWS=${#g[@]}
    printf '\n'
    for l in "${g[@]}"; do printf "  ${_DIM}%s${_RST}\n" "$l"; done
}

# run ACTION — the report is collected instead of printed, because the redraw
# owns the screen. A file, not $(...), so the action still runs in this shell and
# its DID/NEXT_CD assignments stick. Interactive prompts write to stderr, so they
# still show up live while this is running.
run() {
    : > "$RUNOUT"
    "$1" >> "$RUNOUT"
    [ -z "$DID" ] || next_step >> "$RUNOUT"
    REPORT="$(cat "$RUNOUT")"
}

# Printed after each action, then the menu comes back — several changes can be
# made in one sitting.
#
# What actually needs a new shell is narrow: switch.sh re-reads the mappings file
# on every `claude` call, so adding or deleting a mapping is live immediately in
# any shell that already sources it. Only the rc line changing (first install, or
# removing the wiring) requires reloading the shell.
next_step() {
    local sh; sh="$(basename "${SHELL:-bash}")"
    case "$DID" in
        added)
            if [ "$WAS_WIRED" -eq 1 ]; then
                info "Ready: cd $NEXT_CD && claude"
            else
                info "Next: exec $sh (or open a new terminal), then: cd $NEXT_CD && claude"
            fi ;;
        changed)
            # Already wired? Then a mapping change needs nothing from the user --
            # switch.sh re-reads the file on every call -- and saying so costs two
            # lines that push the list toward the bottom of a short window.
            [ "$WAS_WIRED" -eq 1 ] || info "Next: exec $sh to load the wrapper." ;;
        unwired)
            info "Open a new shell, or: unset -f claude claude-profile _claude_profile_for" ;;
    esac
    DID=""
}

# main is defined last and called on the final line: bash must read this whole
# function before it can run, so by then the script has been fully consumed from
# stdin. That is what lets us safely take stdin over for prompts below when the
# script arrived through a pipe (curl ... | bash).
main() {
    local choice f

    case "${1:-}" in
        "") ;;
        -h|--help) sed -n '3,9p' "$0" 2>/dev/null | sed 's/^# \{0,1\}//'; exit 0 ;;
        *) die "This installer takes no arguments — just run it." ;;
    esac

    # Piped in? The script came down stdin, so borrow the real terminal for
    # prompts. Safe here only because the whole script is already parsed. The
    # probe runs in a subshell: a failed `exec <` redirection would kill us, and
    # /dev/tty can be readable-looking yet unopenable (no controlling terminal).
    if [ ! -t 0 ] && (true < /dev/tty) 2>/dev/null; then
        exec < /dev/tty
    fi

    [ "$(id -u)" -ne 0 ] || die "Run this as your normal user, not root — it edits your shell rc."
    [ -t 0 ] || die "This installer is interactive and could not open a terminal.
  Try:  bash -c \"\$(curl -fsSL ${RAW_BASE}/install.sh)\""

    # Snapshot before any action: was the wrapper already loadable from an rc?
    if [ -f "$SWITCH" ]; then
        for f in "${RC_FILES[@]}"; do
            [ -f "$f" ] && grep -qE "$RC_GREP" "$f" && WAS_WIRED=1 && break
        done
    fi

    RUNOUT="$(mktemp)"
    trap 'rm -f "$RUNOUT"' EXIT

    while true; do
        build_rows
        screen
        REPORT=""
        # Five leading spaces line the header up with the profile names, which sit
        # past the menu's cursor column and the row's own `*` marker.
        choice="$(menu "     ${_DIM}$(printf '%-10s %s' PROFILE WHERE)${_RST}" "${ROW_LABELS[@]}")"
        case "$(row_id "$choice")" in
            "+add")    run action_add ;;
            "+unwire") run action_unwire ;;
            "+quit")  echo; exit 0 ;;
            *)        TARGET="$(row_id "$choice")"; run action_row ;;
        esac
    done
}

main "$@"
