#!/usr/bin/env bash
set -Eeuo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
output_dir=${1:-$root/dist/manual}
public_gateway_url=${NEXT_PUBLIC_GATEWAY_URL:-https://ygate-api.yokogawasolution.com}

if [[ $(uname -s) != Linux ]]; then
    echo "build-release.sh must run on Linux or WSL2 so the Next.js artifact matches Linux production" >&2
    exit 1
fi
if [[ -z $public_gateway_url ]]; then
    echo "NEXT_PUBLIC_GATEWAY_URL is required" >&2
    exit 1
fi

release_sha=$(git -C "$root" rev-parse HEAD)
if [[ ! $release_sha =~ ^[0-9a-f]{40}$ ]] || [[ -n $(git -C "$root" status --porcelain) ]]; then
    echo "build from a clean Git commit only" >&2
    exit 1
fi

work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT
release_dir=$work_dir/release
mkdir -p "$release_dir/bin" "$release_dir/web" "$output_dir"

(
    cd "$root/services/platform-api"
    go test ./...
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=$release_sha" -o "$release_dir/bin/platform-api" ./cmd/platform-api
)
(
    cd "$root/services/api-gateway"
    go test ./...
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o "$release_dir/bin/api-gateway" ./cmd/api-gateway
)
(
    cd "$root/apps/web"
    npm ci
    npm run typecheck
    NEXT_PUBLIC_GATEWAY_URL="$public_gateway_url" npm run build
)

cp -R "$root/apps/web/.next/standalone/." "$release_dir/web/"
cp "$root/packages/api-contracts/platform-api.yaml" "$release_dir/platform-api.yaml"
printf '%s\n' "$release_sha" > "$release_dir/VERSION"

artifact=$output_dir/ygate-$release_sha.tar.gz
tar -C "$release_dir" -czf "$artifact" .
(
    cd "$output_dir"
    sha256sum "$(basename "$artifact")" > "$(basename "$artifact").sha256"
)

echo "release: $release_sha"
echo "artifact: $artifact"
echo "checksum: $artifact.sha256"
