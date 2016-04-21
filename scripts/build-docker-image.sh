#!/usr/bin/env sh
set -e

cd $(dirname "$0")

SCRIPTPATH=$(pwd)

cd ${SCRIPTPATH}/..

VERSION=${1:-$(sed -r -n "s/^const Version = \"(.*)\"/\1/p" version.go)}

export GOOS=${2:-linux}
export GOARCH=${3:-amd64}

IMAGE=iwg/winnings-watchdog:$VERSION

echo Building docker image based on ${VERSION} ${GOOS} ${GOARCH}

# always provide it as pure binary file without version etc
cp winnings-watchdog-${VERSION}-${GOOS}-${GOARCH} winnings-watchdog

docker build --no-cache --pull=true --rm -t $IMAGE .



if [[ "$VERSION" != *"SNAPSHOT"* ]]; then
  docker tag  $IMAGE devservices01.office.tipp24.de:5000/$IMAGE
  docker push devservices01.office.tipp24.de:5000/$IMAGE

  docker tag  $IMAGE docker-registry.games.live.tipp24.net:5000/$IMAGE
  docker push docker-registry.games.live.tipp24.net:5000/$IMAGE
else
  docker tag -f $IMAGE devservices01.office.tipp24.de:5000/iwg/winnings-watchdog:latest
  docker push devservices01.office.tipp24.de:5000/iwg/winnings-watchdog:latest
fi

