#!/usr/bin/env sh
set -e

cd $(dirname "$0")

SCRIPTPATH=$(pwd)

. ${SCRIPTPATH}/git.properties
cd ${SCRIPTPATH}/..


VERSION=$1

if [ "$VERSION" == "" ]; then
    echo "No version provided"
    exit 1
fi

APP=winnings-watchdog
export GITHUB_TOKEN=$GIT_TOKEN
export GITHUB_API=$GITHUB_API

# MAC
github-release release --user ${GITHUB_USER} --repo ${GITHUB_REPO} --tag v${VERSION}

tar cfvz ${APP}-${VERSION}-darwin-amd64.tar.gz ${APP}-${VERSION}-darwin-amd64
github-release upload \
  --user ${GITHUB_USER} --repo ${GITHUB_REPO} --tag v${VERSION} --name "${APP}-${VERSION}-darwin-amd64.tar.gz" \
  --file ${APP}-${VERSION}-darwin-amd64.tar.gz

rm ${APP}-${VERSION}-darwin-amd64.tar.gz

# LINUX
tar cfvz ${APP}-${VERSION}-linux-amd64.tar.gz ${APP}-${VERSION}-linux-amd64
github-release upload \
  --user ${GITHUB_USER} --repo ${GITHUB_REPO} --tag v${VERSION} --name "${APP}-${VERSION}-linux-amd64.tar.gz" \
  --file ${APP}-${VERSION}-linux-amd64.tar.gz

rm ${APP}-${VERSION}-linux-amd64.tar.gz

