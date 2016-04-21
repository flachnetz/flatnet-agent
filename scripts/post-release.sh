#!/usr/bin/env sh

set -e


cd $(dirname "$0")

SCRIPTPATH=$(pwd)

. ${SCRIPTPATH}/git.properties
cd ${SCRIPTPATH}/..

VERSION=$(sed -r -n "s/^const Version = \"[0-9]+\.[0-9]+\.(.*)\"/\1/p" version.go)
echo Current patch version is $VERSION
VERSION=$(echo $((${VERSION} + 1)))
echo New patch version is $VERSION

sed -i -r  "s/const Version = \"([0-9]+\.[0-9]+\.).*\"/const Version = \"\1${VERSION}-SNAPSHOT\"/g" version.go; \
git commit -am "setup new dev version ${VERSION}"


if [ "$REMAP_UID" != "" ]; then
    chown -R $REMAP_UID:$REMAP_UID .
fi