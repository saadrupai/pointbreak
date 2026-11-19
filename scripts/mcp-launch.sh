#!/usr/bin/env bash
# Starts the Pointbreak MCP server. Uses pointbreak-mcp from PATH if installed;
# otherwise builds it once from this plugin's source (needs Go 1.23+).
# The server finds the workspace itself: nearest parent of the current directory with context/README.md.
set -euo pipefail
root="${CLAUDE_PLUGIN_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"

if command -v pointbreak-mcp >/dev/null 2>&1; then
  exec pointbreak-mcp "$@"
fi

bin="$root/bin/pointbreak-mcp"
if [ ! -x "$bin" ] || [ -n "$(find "$root" -name '*.go' -newer "$bin" -print -quit 2>/dev/null)" ]; then
  if ! command -v go >/dev/null 2>&1; then
    echo "pointbreak: Go 1.23+ is needed to build the MCP server, or install pointbreak-mcp on your PATH" >&2
    exit 1
  fi
  mkdir -p "$root/bin"
  (cd "$root" && go build -o "$bin" ./cmd/pointbreak-mcp) >&2
fi
exec "$bin" "$@"
