#!/usr/bin/env sh
set -e

cd $(dirname "$0")

SCRIPTPATH=$(pwd)

cd ${SCRIPTPATH}/..

VERSION=${1:-$(sed -r -n "s/^const Version = \"(.*)\"/\1/p" version.go)}

export GOOS=${2:-linux}
export GOARCH=${3:-amd64}
DOCKER=${4:-false}

IMAGE=flachnetz/flatnet-agent:$VERSION

echo Building version ${VERSION} ${GOOS} ${GOARCH}
CGO_ENABLED=0 go build -a -installsuffix cgo -v -ldflags "-X=main.GitHash=$(git rev-parse HEAD)" -o flatnet-agent-${VERSION}-${GOOS}-${GOARCH}

