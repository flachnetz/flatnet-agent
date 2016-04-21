#!/usr/bin/env sh

set -e

cd $(dirname "$0")

SCRIPTPATH=$(pwd)


. ${SCRIPTPATH}/git.properties
cd ${SCRIPTPATH}/..

VERSION=$(sed -r -n "s/^const Version = \"([0-9]+\.[0-9]+\.[0-9]+).*\"/\1/p" version.go)
echo "Creating new release for version $VERSION"

sed -i -r  "s/const Version = \".*\"/const Version = \"${VERSION}\"/g" version.go; \
git commit -am "Release commit for version ${VERSION}"

# write release version in temp file
echo ${VERSION} > __RELEASE_VERSION__