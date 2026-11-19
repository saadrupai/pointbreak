#!/usr/bin/env bash
# Pointbreak SessionStart hook.
# Prints one short note for the agent: which workspace and service it is in, what is active today,
# and what is unfinished. Prints nothing outside a microservice workspace, so it costs no tokens there.
# Reads only context/ files and directory names; never reads repo code or secrets.
set -uo pipefail

start="${CLAUDE_PROJECT_DIR:-$PWD}"
today="$(date +%Y-%m-%d)"

# Nearest parent holding context/README.md.
ws=""
d="$start"
while :; do
  if [ -f "$d/context/README.md" ]; then ws="$d"; break; fi
  [ "$d" = "/" ] && break
  d="$(dirname "$d")"
done

json_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  s="${s//$'\t'/ }"
  s="${s//$'\r'/}"
  s="${s//$'\n'/\\n}"
  printf '%s' "$s"
}

emit() {
  printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"%s"}}\n' "$(json_escape "$1")"
  exit 0
}

if [ -z "$ws" ]; then
  # Not set up yet: suggest setup only if this looks like a multi-repo workspace.
  repos=0
  for r in "$start"/*/; do [ -d "$r/.git" ] && repos=$((repos + 1)); done
  if [ "$repos" -ge 2 ]; then
    emit "Pointbreak: this folder has $repos git repos but no context/ folder. If it is a microservice workspace, suggest running /pointbreak-setup once to create per-service context."
  fi
  exit 0
fi

services=()
for f in "$ws"/context/*/architecture.md; do
  [ -f "$f" ] || continue
  name="$(basename "$(dirname "$f")")"
  case "$name" in _*) continue ;; esac
  services+=("$name")
done

# Service the session was opened in: first path part under the workspace root.
here=""
if [ "$start" != "$ws" ]; then
  rel="${start#"$ws"/}"
  first="${rel%%/*}"
  [ -f "$ws/context/$first/architecture.md" ] && here="$first"
fi

active=()
stale=()
for svc in "${services[@]}"; do
  t="$ws/context/$svc/temp.md"
  [ -f "$t" ] || continue
  day=""
  while IFS= read -r line; do
    case "$line" in
      "## "[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]*) day="${line:3:10}" ;;
      "### "*)
        task="${line#\#\#\# }"
        if [ "$day" = "$today" ]; then
          active+=("$svc ($task)")
        elif [ -n "$day" ] && [[ "$day" < "$today" ]]; then
          stale+=("$svc from $day ($task)")
        fi
        ;;
    esac
  done < "$t"
done

msg="Pointbreak workspace: $ws. Services: ${services[*]:-none registered}."
[ -n "$here" ] && msg+=" Session opened in service: $here."
if [ "${#active[@]}" -gt 0 ]; then
  msg+=" Active today: $(IFS=';'; echo "${active[*]}")."
fi
if [ "${#stale[@]}" -gt 0 ]; then
  msg+=" Unfinished from earlier days: $(IFS=';'; echo "${stale[*]}") — offer /pointbreak end or continue it."
fi
if ! grep -qs "pointbreak:start" "$ws/CLAUDE.md"; then
  msg+=" Team rules are in context/README.md and are not imported in CLAUDE.md: read them before changing code."
fi
msg+=" Before working on a service, load its memory with the pointbreak load_context tool (or read context/<service>/ files); start tasks with /pointbreak."
emit "$msg"
