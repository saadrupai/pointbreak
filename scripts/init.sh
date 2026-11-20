#!/usr/bin/env bash
# Create a Pointbreak context/ folder in a microservice workspace.
# Usage: scripts/init.sh <workspace-dir> [service-dir ...]
# With no service names, every subdirectory that is a git repo becomes a service.
# Existing files are never overwritten.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tpl="$here/templates/context"
ws="${1:?usage: init.sh <workspace-dir> [service-dir ...]}"
shift || true
ctx="$ws/context"
month="$(date +%Y-%m)"

copy_if_missing() { [ -e "$2" ] || { mkdir -p "$(dirname "$2")"; cp "$1" "$2"; echo "  + ${2#"$ws"/}"; }; }

echo "Pointbreak: setting up $ctx"
copy_if_missing "$tpl/README.md" "$ctx/README.md"
copy_if_missing "$tpl/STANDARDS.md" "$ctx/STANDARDS.md"
copy_if_missing "$tpl/global-architecture.md" "$ctx/global-architecture.md"

if [ "$#" -eq 0 ]; then
  for d in "$ws"/*/; do
    name="$(basename "$d")"
    [ "$name" = "context" ] && continue
    [ -d "$d/.git" ] && set -- "$@" "$name"
  done
fi

for svc in "$@"; do
  [ -d "$ws/$svc" ] || { echo "  ! skipping $svc: no such directory in $ws" >&2; continue; }
  for f in temp.md architecture.md features.md database.md; do
    dest="$ctx/$svc/$f"
    if [ ! -e "$dest" ]; then
      mkdir -p "$ctx/$svc"
      sed "s/<service>/$svc/g" "$tpl/_service/$f" > "$dest"
      echo "  + context/$svc/$f"
    fi
  done
  log="$ctx/$svc/logs/$month.md"
  if [ ! -e "$log" ]; then
    mkdir -p "$ctx/$svc/logs"
    sed -e "s/<service>/$svc/g" -e "s/<YYYY-MM>/$month/g" "$tpl/_service/logs/YYYY-MM.md" > "$log"
    echo "  + context/$svc/logs/$month.md"
  fi
done

echo "Done. Next: fill the <placeholders> in context/README.md, then ask your agent to fill each service's files from its code."
