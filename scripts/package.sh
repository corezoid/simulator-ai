#!/usr/bin/env bash
set -euo pipefail
export GOMAXPROCS=2
cd "$(dirname "$0")/.."
if [[ -n "$(git status --porcelain)" ]]; then
  echo 'Package only a clean, committed checkout' >&2
  exit 1
fi
if (( $# == 0 )); then set -- linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; fi
mkdir -p dist
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
mkdir -p "$stage/licenses/go" "$stage/licenses/pdf"
cp LICENSE README.md CHANGELOG.md SECURITY.md AGENTS.md CLAUDE.md THIRD_PARTY_NOTICES.md "$stage/"
cp -R plugins .agents .claude-plugin docs "$stage/"
cp "$(go env GOROOT)/LICENSE" "$stage/licenses/go/LICENSE"
cp internal/third_party/pdf/LICENSE internal/third_party/pdf/README.md "$stage/licenses/pdf/"
# Include every module linked into the binary, including transitive dependencies.
go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}} {{.Dir}}{{end}}{{end}}' ./cmd/simulator-ai | sort -u > "$stage/modules.txt"
while read -r module directory; do
  [[ -n "$module" ]] || continue
  [[ -n "$directory" ]] || { echo "Missing downloaded module: $module" >&2; exit 1; }
  destination="$stage/licenses/$module"
  mkdir -p "$destination"
  licensed=false
  for notice in "$directory"/LICENSE "$directory"/LICENSE.* "$directory"/COPYING "$directory"/COPYING.* "$directory"/NOTICE "$directory"/NOTICE.*; do
    [[ -f "$notice" ]] || continue
    cp "$notice" "$destination/"
    case "$(basename "$notice")" in LICENSE*|COPYING*) licensed=true ;; esac
  done
  "$licensed" || { echo "Missing license for $module" >&2; exit 1; }
done < "$stage/modules.txt"
rm "$stage/modules.txt"
mkdir -p "$stage/bin"
: > dist/build-info.txt
: > dist/checksums.txt
for platform in "$@"; do
  case "$platform" in linux/amd64|linux/arm64|darwin/amd64|darwin/arm64) ;; *) echo "Unsupported platform: $platform" >&2; exit 1 ;; esac
  os=${platform%/*}; arch=${platform#*/}
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" GOMAXPROCS=2 go build -p=1 -trimpath -o "$stage/bin/simulator-ai" ./cmd/simulator-ai
  go version -m "$stage/bin/simulator-ai" | sed "s|$stage/bin/||" > "$stage/BUILD_INFO.txt"
  grep -q 'vcs.modified=false' "$stage/BUILD_INFO.txt"
  grep -q "vcs.revision=$(git rev-parse HEAD)" "$stage/BUILD_INFO.txt"
  cat "$stage/BUILD_INFO.txt" >> dist/build-info.txt
  archive="simulator-ai-$os-$arch.tar.gz"
  tar -czf "dist/$archive" -C "$stage" .
  (cd dist && shasum -a 256 "$archive") >> dist/checksums.txt
  echo "Packaged $platform"
done
