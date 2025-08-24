#!/bin/bash

MODULE=$(grep module go.mod | cut -d\  -f2)
BINBASE=${MODULE##*/}
VERSION=${VERSION:-$GITHUB_REF_NAME}
VERSION=${VERSION:-v0.0.0}
COMMIT_HASH="$(git rev-parse --short HEAD 2>/dev/null)"
COMMIT_HASH=${COMMIT_HASH:-00000000}
DIRTY=$(git diff --quiet 2>/dev/null || echo '-dirty')
BUILD_TIMESTAMP=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
BUILDER=$(go version)
PROJECT_URL="$PWD by $USER@$HOSTNAME"

LDFLAGS=(
    "-X 'main.Version=${VERSION}'"
    "-X 'main.CommitHash=${COMMIT_HASH}${DIRTY}'"
    "-X 'main.BuildTimestamp=${BUILD_TIMESTAMP}'"
    "-X 'main.Builder=${BUILDER}'"
    "-X 'main.ProjectURL=${PROJECT_URL}'"
)

echo "[*] Build info"
echo "   Version=${VERSION}"
echo "   CommitHash=${COMMIT_HASH}${DIRTY}"
echo "   BuildTimestamp=${BUILD_TIMESTAMP}"
echo "   Builder=${BUILDER}"
echo "   ProjectURL=${PROJECT_URL}"

[ "$1" != "docker" ] && echo "Building local binary" && CGO_ENABLED=0 go build -ldflags="${LDFLAGS[*]}" -o cosanet .

[ "$1" != "local" ] && echo "Building docker image" && docker build \
    --progress=plain \
    --build-arg=VERSION="${VERSION}" \
    --build-arg=COMMIT_HASH="${COMMIT_HASH}" \
    --build-arg=BUILD_TIMESTAMP="${BUILD_TIMESTAMP}" \
    --build-arg=PROJECT_URL="${PROJECT_URL}" \
    -t cosanet:latest .