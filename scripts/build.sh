#!/usr/bin/env bash
# scripts/build.sh — 带版本信息注入的构建脚本
# 用法: ./scripts/build.sh [输出路径]
#
# 注入 git describe --tags --always --dirty 作为版本号，
# commit hash 与构建时间一并写入。未在 git 仓库时回退到 unknown。
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "unknown")
COMMIT=$(git rev-parse --short=8 HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS="-X github.com/LaoQi/vistty/internal/version.buildVersion=${VERSION}"
LDFLAGS="${LDFLAGS} -X github.com/LaoQi/vistty/internal/version.buildCommit=${COMMIT}"
LDFLAGS="${LDFLAGS} -X github.com/LaoQi/vistty/internal/version.buildTime=${BUILD_TIME}"

OUT="${1:-vistty}"

CGO_ENABLED=0 go build -ldflags "${LDFLAGS}" -o "${OUT}" ./cmd/vistty

echo "Built ${OUT} ${VERSION} (commit ${COMMIT}, ${BUILD_TIME})"
