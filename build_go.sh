#!/usr/bin/env bash
# Playerr Go backend cross-compile build script
# Produces static binaries for all target platforms.
# No C toolchain required — uses pure-Go SQLite (modernc.org/sqlite).

set -euo pipefail

export CGO_ENABLED=0

GOROOT_PATH="${GOROOT:-/home/kieran/go}"
export PATH="$GOROOT_PATH/bin:${PATH}"

VERSION=$(grep '"version"' package.json 2>/dev/null | head -1 | sed 's/.*"version": *"\([^"]*\)".*/\1/' || git rev-parse --short HEAD 2>/dev/null || echo "dev")
LDFLAGS="-s -w -X main.version=${VERSION}"

echo "Building Playerr Go backend v${VERSION}..."

mkdir -p _output/{linux-x64,linux-arm64,win-x64,osx-x64,osx-arm64}

build() {
    local os="$1" arch="$2" outdir="$3" ext="${4:-}"
    echo "  Building ${os}/${arch}..."
    GOOS="$os" GOARCH="$arch" go build \
        -ldflags "$LDFLAGS" \
        -o "_output/${outdir}/cargodeck${ext}" \
        .
}

build_agent() {
    local os="$1" arch="$2" outdir="$3" ext="${4:-}"
    echo "  Building agent ${os}/${arch}..."
    GOOS="$os" GOARCH="$arch" go build \
        -ldflags "$LDFLAGS" \
        -o "_output/${outdir}/cargodeck-agent${ext}" \
        ./cmd/agent/
}

echo "Building Playerr server..."
build linux  amd64 linux-x64
build linux  arm64 linux-arm64
build windows amd64 win-x64 .exe
build darwin amd64 osx-x64
build darwin arm64 osx-arm64

echo ""
echo "Building Playerr Agent..."
build_agent linux  amd64 linux-x64
build_agent linux  arm64 linux-arm64
build_agent windows amd64 win-x64 .exe
build_agent darwin amd64 osx-x64
build_agent darwin arm64 osx-arm64

# Write version sidecar so the server can report the hosted agent version
echo "${VERSION}" > _output/version.txt

echo ""
echo "Build complete. Binary sizes:"
ls -lh _output/*/cargodeck* | awk '{print "  " $5 "  " $9}'
