#!/usr/bin/env bash
# Pointbreak PreToolUse hook for Edit / Write / MultiEdit / NotebookEdit.
# Inside a Pointbreak workspace, every write to a file outside context/ must be approved by the
# user: the hook returns permissionDecision "ask", so Claude Code shows the change and waits,
# even when edits would otherwise be auto-accepted. context/ files (the agent's memory) pass.
# Outside a Pointbreak workspace it prints nothing and changes nothing.
set -uo pipefail

input="$(cat)"
path="$(printf '%s' "$input" | grep -oE '"(file_path|notebook_path)"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed -E 's/.*:[[:space:]]*"(.*)"$/\1/')"
[ -z "$path" ] && exit 0

start="${CLAUDE_PROJECT_DIR:-$PWD}"
case "$path" in /*) ;; *) path="$start/$path" ;; esac

ws=""
d="$start"
while :; do
  if [ -f "$d/context/README.md" ]; then ws="$d"; break; fi
  [ "$d" = "/" ] && break
  d="$(dirname "$d")"
done
[ -z "$ws" ] && exit 0

case "$path" in
  */../*|*/./*|*/..|*/.) ;;          # unnormalised path: never exempt, always ask
  "$ws"/context/*) exit 0 ;;
esac

rel="${path#"$ws"/}"
reason="Pointbreak: every file write needs your approval. Check the change to $rel, then approve or reject it."
printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"%s"}}\n' "${reason//\"/\\\"}"
exit 0
