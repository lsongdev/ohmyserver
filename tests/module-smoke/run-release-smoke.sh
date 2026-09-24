#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
module_id=${1:?usage: run-release-smoke.sh <module-id>}
module_path="github.com/lsongdev/ohmyserver/modules/$module_id"
version=v0.0.0
host_dir="$root/tests/module-smoke/$module_id-host"

if grep -Eq '^[[:space:]]*replace[[:space:]]' "$host_dir/go.mod"; then
	echo "release smoke host must not use local replace directives" >&2
	exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
mkdir -p "$tmp/proxy" "$tmp/host"
(cd "$root" && GOWORK=off go run ./tests/module-smoke/proxy "$root/modules/$module_id" "$module_path" "$version" "$tmp/proxy")
if [ "$module_id" = terminal ]; then
	(cd "$root" && GOWORK=off go run ./tests/module-smoke/proxy "$root/pkg/terminal" github.com/lsongdev/ohmyserver/pkg/terminal "$version" "$tmp/proxy")
fi
cp -R "$host_dir/." "$tmp/host/"
(cd "$tmp/host" && GOWORK=off GOPROXY="file://$tmp/proxy,https://proxy.golang.org" GOSUMDB=off go test -mod=mod ./...)
