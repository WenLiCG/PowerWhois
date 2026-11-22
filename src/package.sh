#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
arch=${1:-amd64}
case "$arch" in amd64|arm64) ;; *) echo "Expected amd64 or arm64" >&2; exit 2 ;; esac
rev=$(git -C "$root" rev-parse --short=12 HEAD)
if [[ -n "$(git -C "$root" status --porcelain)" ]]; then
    echo "Package a committed revision. Run the frontend build in that checkout." >&2
    exit 2
fi
output=${POWERWHOIS_OUTPUT_DIR:-"$root/dist"}
stage=$(mktemp -d)
trap 'rm -rf -- "$stage"' EXIT
mkdir -p "$output"

# Use only committed deployment templates. Never package operator configs, logs, or Redis data.
git -C "$root" archive HEAD deploy | tar -xf - -C "$stage"
cp -r "$root/src/frontend/dist/spa/." "$stage/deploy/frontend/"
(
    cd "$root/src/backend"
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -buildvcs=true -o "$stage/deploy/backend/typonamer" .
)
printf '%s\n' "$(git -C "$root" rev-parse HEAD)" > "$stage/deploy/REVISION"
name="PowerWhois-linux-$arch-$rev.tar.gz"
tar -C "$stage" -czf "$output/$name" deploy
(cd "$output" && sha256sum "$name" > "$name.sha256")
printf '%s\n' "$output/$name"
